// Package nominatim geocodes a city/area through the public Nominatim API.
package nominatim

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/ali-ulu/lead/leadscout-go/internal/cache"
	"github.com/ali-ulu/lead/leadscout-go/internal/model"
	"github.com/ali-ulu/lead/leadscout-go/internal/ratelimit"
)

const nominatimURL = "https://nominatim.openstreetmap.org/search"
const userAgent = "LeadScout/4.0 (local business research tool)"

// ErrNotFound signals a location that Nominatim does not know.
var ErrNotFound = errors.New("location not found")

func minInterval() float64 {
	raw := os.Getenv("LEADSCOUT_NOMINATIM_MIN_INTERVAL")
	if raw == "" {
		return 1.0
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil || f <= 0 {
		return 1.0
	}
	return f
}

type row struct {
	DisplayName string            `json:"display_name"`
	Lat         string            `json:"lat"`
	Lon         string            `json:"lon"`
	BoundingBox []string          `json:"boundingbox"`
	Address     map[string]string `json:"address"`
}

// GeocodeArea resolves a city/area into a bounding box and coordinates.
func GeocodeArea(city, country string, timeout time.Duration) (*model.Area, error) {
	query := joinNonEmpty(city, country)
	if query == "" {
		return nil, errors.New("City or area is required")
	}
	var cached model.Area
	if cache.Get("nominatim", []any{query}, &cached) {
		return &cached, nil
	}
	params := url.Values{}
	params.Set("q", query)
	params.Set("format", "jsonv2")
	params.Set("limit", "1")
	params.Set("addressdetails", "1")

	fetch := func() ([]row, error) {
		req, _ := http.NewRequest(http.MethodGet, nominatimURL+"?"+params.Encode(), nil)
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Accept-Language", "en")
		client := &http.Client{Timeout: timeout}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("nominatim returned HTTP %d", resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		var rows []row
		if err := json.Unmarshal(body, &rows); err != nil {
			return nil, err
		}
		return rows, nil
	}

	rows, err := ratelimit.Call("nominatim", fetch, ratelimit.Options{MinInterval: minInterval(), Retries: 2})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, query)
	}
	r := rows[0]
	lat, _ := strconv.ParseFloat(r.Lat, 64)
	lon, _ := strconv.ParseFloat(r.Lon, 64)
	area := &model.Area{
		DisplayName: firstNonEmpty(r.DisplayName, query),
		Lat:         lat,
		Lon:         lon,
		Country:     firstNonEmpty(r.Address["country"], country),
		CountryCode: upper(r.Address["country_code"]),
		City:        firstNonEmpty(r.Address["city"], r.Address["town"], r.Address["municipality"], r.Address["county"], city),
	}
	if len(r.BoundingBox) == 4 {
		south, _ := strconv.ParseFloat(r.BoundingBox[0], 64)
		north, _ := strconv.ParseFloat(r.BoundingBox[1], 64)
		west, _ := strconv.ParseFloat(r.BoundingBox[2], 64)
		east, _ := strconv.ParseFloat(r.BoundingBox[3], 64)
		area.BBox = model.BBox{South: south, North: north, West: west, East: east}
	}
	cache.Put("nominatim", []any{query}, area)
	return area, nil
}

func joinNonEmpty(parts ...string) string {
	out := ""
	for _, p := range parts {
		if p == "" {
			continue
		}
		if out != "" {
			out += ", "
		}
		out += p
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func upper(s string) string {
	out := []rune(s)
	for i, r := range out {
		if r >= 'a' && r <= 'z' {
			out[i] = r - 32
		}
	}
	return string(out)
}
