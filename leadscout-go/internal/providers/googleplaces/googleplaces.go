// Package googleplaces enriches a lead's reputation through the Google Places
// Text Search API (rating, review count, website, phone). These fields are
// billable, so it only runs when a lead is explicitly enriched.
package googleplaces

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

const endpoint = "https://places.googleapis.com/v1/places:searchText"
const userAgent = "LeadScout/6.0 reputation-enricher"

// Configured reports whether a Google Places key is present.
func Configured() bool {
	return strings.TrimSpace(os.Getenv("GOOGLE_PLACES_API_KEY")) != ""
}

var stopTokens = map[string]bool{
	"the": true, "and": true, "for": true, "clinic": true, "restaurant": true,
	"hotel": true, "salon": true, "shop": true, "services": true,
}

var wordRe = regexp.MustCompile(`[a-z0-9]+`)

func norm(value string) map[string]bool {
	out := map[string]bool{}
	for _, t := range wordRe.FindAllString(strings.ToLower(value), -1) {
		if len(t) >= 3 && !stopTokens[t] {
			out[t] = true
		}
	}
	return out
}

func distanceKm(lat1, lon1, lat2, lon2 float64) float64 {
	const radius = 6371.0088
	p1 := lat1 * math.Pi / 180
	p2 := lat2 * math.Pi / 180
	dp := (lat2 - lat1) * math.Pi / 180
	dl := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(dp/2)*math.Sin(dp/2) + math.Cos(p1)*math.Cos(p2)*math.Sin(dl/2)*math.Sin(dl/2)
	return radius * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(math.Max(0, 1-a)))
}

type place struct {
	ID          string `json:"id"`
	DisplayName struct {
		Text string `json:"text"`
	} `json:"displayName"`
	FormattedAddress string `json:"formattedAddress"`
	Location         struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	} `json:"location"`
	Rating              *float64 `json:"rating"`
	UserRatingCount     *int     `json:"userRatingCount"`
	WebsiteURI          string   `json:"websiteUri"`
	NationalPhoneNumber string   `json:"nationalPhoneNumber"`
}

func matchScore(p place, name string, lat, lon *float64) float64 {
	a := norm(name)
	b := norm(p.DisplayName.Text)
	overlap := 0
	for t := range a {
		if b[t] {
			overlap++
		}
	}
	nameScore := 0.0
	if len(a) > 0 {
		nameScore = float64(overlap) / float64(len(a))
	}
	score := nameScore * 0.82
	if lat != nil && lon != nil {
		d := distanceKm(*lat, *lon, p.Location.Latitude, p.Location.Longitude)
		switch {
		case d <= 0.25:
			score += 0.18
		case d <= 1.0:
			score += 0.12
		case d <= 3.0:
			score += 0.06
		}
	}
	if score > 1 {
		score = 1
	}
	return score
}

// Result is the reputation enrichment outcome.
type Result struct {
	Configured       bool     `json:"configured"`
	Matched          bool     `json:"matched"`
	Reason           string   `json:"reason,omitempty"`
	Query            string   `json:"query,omitempty"`
	CandidateCount   int      `json:"candidate_count,omitempty"`
	MatchConfidence  float64  `json:"match_confidence,omitempty"`
	PlaceID          string   `json:"place_id,omitempty"`
	DisplayName      string   `json:"display_name,omitempty"`
	FormattedAddress string   `json:"formatted_address,omitempty"`
	Rating           *float64 `json:"rating,omitempty"`
	ReviewCount      *int     `json:"review_count,omitempty"`
	Website          string   `json:"website,omitempty"`
	Phone            string   `json:"phone,omitempty"`
	Source           string   `json:"source,omitempty"`
}

// EnrichReputation looks up a business and returns reputation fields when matched.
func EnrichReputation(name, city, country string, lat, lon *float64) (Result, error) {
	key := strings.TrimSpace(os.Getenv("GOOGLE_PLACES_API_KEY"))
	if key == "" {
		return Result{Configured: false, Matched: false, Reason: "GOOGLE_PLACES_API_KEY is not configured."}, nil
	}
	textQuery := strings.Join(nonEmpty(name, city, country), ", ")
	payload := map[string]any{"textQuery": textQuery, "maxResultCount": 5}
	if lat != nil && lon != nil {
		payload["locationBias"] = map[string]any{
			"circle": map[string]any{
				"center": map[string]any{"latitude": *lat, "longitude": *lon},
				"radius": 5000.0,
			},
		}
	}
	fieldMask := strings.Join([]string{
		"places.id", "places.displayName", "places.formattedAddress", "places.location",
		"places.rating", "places.userRatingCount", "places.websiteUri", "places.nationalPhoneNumber",
	}, ",")
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Goog-Api-Key", key)
	req.Header.Set("X-Goog-FieldMask", fieldMask)
	req.Header.Set("User-Agent", userAgent)
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return Result{}, errors.New("Google Places request failed: " + resp.Status)
	}
	var data struct {
		Places []place `json:"places"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return Result{}, err
	}

	type ranked struct {
		score float64
		p     place
	}
	var all []ranked
	for _, p := range data.Places {
		all = append(all, ranked{matchScore(p, name, lat, lon), p})
	}
	for i := 1; i < len(all); i++ {
		for j := i; j > 0 && all[j].score > all[j-1].score; j-- {
			all[j], all[j-1] = all[j-1], all[j]
		}
	}
	if len(all) == 0 || all[0].score < 0.55 {
		return Result{Configured: true, Matched: false, Query: textQuery, CandidateCount: len(all)}, nil
	}
	best := all[0]
	return Result{
		Configured: true, Matched: true,
		MatchConfidence:  round3(best.score),
		PlaceID:          best.p.ID,
		DisplayName:      best.p.DisplayName.Text,
		FormattedAddress: best.p.FormattedAddress,
		Rating:           best.p.Rating,
		ReviewCount:      best.p.UserRatingCount,
		Website:          best.p.WebsiteURI,
		Phone:            best.p.NationalPhoneNumber,
		Source:           "google_places",
	}, nil
}

func nonEmpty(values ...string) []string {
	var out []string
	for _, v := range values {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func round3(f float64) float64 { return math.Round(f*1000) / 1000 }
