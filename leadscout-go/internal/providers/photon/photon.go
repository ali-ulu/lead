// Package photon geocodes a city/area through the public Photon API.
//
// It is a keyless fallback for Nominatim: OSM-backed and free, so discovery
// keeps working when Nominatim is rate-limited or unavailable.
package photon

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/ali-ulu/lead/leadscout-go/internal/cache"
	"github.com/ali-ulu/lead/leadscout-go/internal/model"
	"github.com/ali-ulu/lead/leadscout-go/internal/ratelimit"
)

const photonURL = "https://photon.komoot.io/api/"
const userAgent = "LeadScout/7.0 (local business research tool)"

// endpoint is overridable in tests.
var endpoint = photonURL

// ErrNotFound signals a location that Photon does not know.
var ErrNotFound = errors.New("location not found")

type feature struct {
	Properties struct {
		Name        string    `json:"name"`
		City        string    `json:"city"`
		District    string    `json:"district"`
		County      string    `json:"county"`
		Country     string    `json:"country"`
		CountryCode string    `json:"countrycode"`
		Extent      []float64 `json:"extent"`
	} `json:"properties"`
	Geometry struct {
		Coordinates []float64 `json:"coordinates"`
	} `json:"geometry"`
}

type response struct {
	Features []feature `json:"features"`
}

// GeocodeArea resolves a city/area into a bounding box and coordinates.
func GeocodeArea(city, country string, timeout time.Duration) (*model.Area, error) {
	query := joinNonEmpty(city, country)
	if query == "" {
		return nil, errors.New("City or area is required")
	}
	var cached model.Area
	if cache.Get("photon", []any{query}, &cached) {
		return &cached, nil
	}
	params := url.Values{}
	params.Set("q", query)
	params.Set("limit", "1")
	params.Set("lang", "en")

	fetch := func() (response, error) {
		req, _ := http.NewRequest(http.MethodGet, endpoint+"?"+params.Encode(), nil)
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Accept", "application/json")
		client := &http.Client{Timeout: timeout}
		resp, err := client.Do(req)
		if err != nil {
			return response{}, err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return response{}, fmt.Errorf("photon returned HTTP %d", resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		var parsed response
		if err := json.Unmarshal(body, &parsed); err != nil {
			return response{}, err
		}
		return parsed, nil
	}

	parsed, err := ratelimit.Call("photon", fetch, ratelimit.Options{MinInterval: 1.0, Retries: 2})
	if err != nil {
		return nil, err
	}
	if len(parsed.Features) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, query)
	}
	area := featureToArea(parsed.Features[0], city, country)
	cache.Put("photon", []any{query}, area)
	return &area, nil
}

func featureToArea(f feature, city, country string) model.Area {
	props := f.Properties
	var lat, lon float64
	if len(f.Geometry.Coordinates) > 1 {
		lon = f.Geometry.Coordinates[0]
		lat = f.Geometry.Coordinates[1]
	}
	bbox := model.BBox{}
	if len(props.Extent) == 4 {
		// Photon extent is [west, north, east, south].
		bbox = model.BBox{West: props.Extent[0], North: props.Extent[1], East: props.Extent[2], South: props.Extent[3]}
	} else if lat != 0 || lon != 0 {
		// A point result carries no extent; derive a small box so the caller
		// still gets a usable search area instead of a hard failure.
		const pad = 0.05
		bbox = model.BBox{South: lat - pad, North: lat + pad, West: lon - pad, East: lon + pad}
	}
	name := firstNonEmpty(props.Name, city)
	display := name
	for _, part := range []string{props.City, props.Country} {
		if part != "" {
			display += ", " + part
		}
	}
	return model.Area{
		DisplayName: display,
		Lat:         lat,
		Lon:         lon,
		BBox:        bbox,
		Country:     firstNonEmpty(props.Country, country),
		CountryCode: upper(props.CountryCode),
		City:        firstNonEmpty(props.City, props.District, props.County, city),
	}
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
