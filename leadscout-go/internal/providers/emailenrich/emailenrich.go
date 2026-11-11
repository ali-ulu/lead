// Package emailenrich runs a waterfall of hosted providers (Hunter, PDL,
// Clearbit) to find a contact address for a domain that does not publish one.
package emailenrich

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/ali-ulu/lead/leadscout-go/internal/cache"
	"github.com/ali-ulu/lead/leadscout-go/internal/ratelimit"
)

const userAgent = "LeadScout/7.0 (local business research tool)"

// Attempt records what happened with one provider in the waterfall.
type Attempt struct {
	Provider string `json:"provider"`
	Status   string `json:"status"` // matched, no_match, error
	Error    string `json:"error,omitempty"`
}

// Result is the enrichment outcome for a domain.
type Result struct {
	Found      bool      `json:"found"`
	Domain     string    `json:"domain"`
	Email      string    `json:"email,omitempty"`
	Source     string    `json:"source,omitempty"`
	Confidence *int      `json:"confidence,omitempty"`
	FirstName  string    `json:"first_name,omitempty"`
	LastName   string    `json:"last_name,omitempty"`
	Position   string    `json:"position,omitempty"`
	Company    string    `json:"company,omitempty"`
	Attempts   []Attempt `json:"attempts"`
}

type provider struct {
	name string
	fn   func(domain string, timeout time.Duration) (map[string]any, error)
}

func getJSON(rawURL string, headers map[string]string, timeout time.Duration) (map[string]any, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1_000_000))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("provider returned HTTP %d", resp.StatusCode)
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func hunter(domain string, timeout time.Duration) (map[string]any, error) {
	key := strings.TrimSpace(os.Getenv("HUNTER_API_KEY"))
	if key == "" {
		return nil, errors.New("HUNTER_API_KEY is not configured")
	}
	params := url.Values{"domain": {domain}, "api_key": {key}}
	data, err := getJSON("https://api.hunter.io/v2/domain-search?"+params.Encode(), nil, timeout)
	if err != nil {
		return nil, err
	}
	section, _ := data["data"].(map[string]any)
	emails, _ := section["emails"].([]any)
	best := ""
	bestConfidence := -1
	var meta map[string]any
	for _, raw := range emails {
		entry, _ := raw.(map[string]any)
		if entry == nil {
			continue
		}
		value, _ := entry["value"].(string)
		if value == "" {
			continue
		}
		confidence := int(toFloat(entry["confidence"]))
		if confidence > bestConfidence {
			best, bestConfidence, meta = value, confidence, entry
		}
	}
	if best == "" {
		return nil, nil
	}
	match := map[string]any{"email": best, "source": "hunter"}
	if bestConfidence >= 0 {
		match["confidence"] = bestConfidence
	}
	for _, field := range []string{"type", "first_name", "last_name", "position"} {
		if v, ok := meta[field].(string); ok && v != "" {
			match[field] = v
		}
	}
	return match, nil
}

func pdl(domain string, timeout time.Duration) (map[string]any, error) {
	key := strings.TrimSpace(os.Getenv("PEOPLE_DATA_LABS_API_KEY"))
	if key == "" {
		return nil, errors.New("PEOPLE_DATA_LABS_API_KEY is not configured")
	}
	params := url.Values{"website": {domain}}
	data, err := getJSON("https://api.peopledatalabs.com/v5/company/enrich?"+params.Encode(),
		map[string]string{"X-Api-Key": key}, timeout)
	if err != nil {
		return nil, err
	}
	candidate, _ := data["email"].(string)
	if candidate == "" {
		if emails, ok := data["emails"].([]any); ok {
			for _, raw := range emails {
				if entry, ok := raw.(map[string]any); ok {
					if addr, ok := entry["address"].(string); ok && addr != "" {
						candidate = addr
						break
					}
				}
			}
		}
	}
	if candidate == "" {
		return nil, nil
	}
	match := map[string]any{"email": candidate, "source": "pdl"}
	if name, ok := data["name"].(string); ok && name != "" {
		match["company"] = name
	}
	return match, nil
}

func clearbit(domain string, timeout time.Duration) (map[string]any, error) {
	key := strings.TrimSpace(os.Getenv("CLEARBIT_API_KEY"))
	if key == "" {
		return nil, errors.New("CLEARBIT_API_KEY is not configured")
	}
	params := url.Values{"domain": {domain}}
	data, err := getJSON("https://company.clearbit.com/v2/companies/find?"+params.Encode(),
		map[string]string{"Authorization": "Bearer " + key}, timeout)
	if err != nil {
		return nil, err
	}
	candidate, _ := data["email"].(string)
	if candidate == "" {
		if company, ok := data["company"].(map[string]any); ok {
			candidate, _ = company["email"].(string)
		}
	}
	if candidate == "" {
		return nil, nil
	}
	match := map[string]any{"email": candidate, "source": "clearbit"}
	if name, ok := data["name"].(string); ok && name != "" {
		match["company"] = name
	}
	return match, nil
}

// Order matters: Hunter is the email specialist, the other two are fallbacks.
var providers = []provider{
	{"hunter", hunter},
	{"pdl", pdl},
	{"clearbit", clearbit},
}

// EnrichDomain runs the waterfall for domain and returns the first address
// found. Attempts lists one entry per provider so a caller can see which keys
// are configured and why a provider was skipped.
func EnrichDomain(domain string, timeout time.Duration) (Result, error) {
	domain = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(domain)), "www.")
	if domain == "" || !strings.Contains(domain, ".") {
		return Result{}, errors.New("a registrable domain is required")
	}
	var cached Result
	if cache.Get("email_enrich", []any{domain}, &cached) {
		return cached, nil
	}

	attempts := []Attempt{}
	for _, p := range providers {
		match, err := ratelimit.Call("email_enrich_"+p.name, func() (map[string]any, error) {
			return p.fn(domain, timeout)
		}, ratelimit.Options{DisableBreaker: true})
		if err != nil {
			attempts = append(attempts, Attempt{Provider: p.name, Status: "error", Error: err.Error()})
			continue
		}
		if match == nil {
			attempts = append(attempts, Attempt{Provider: p.name, Status: "no_match"})
			continue
		}
		attempts = append(attempts, Attempt{Provider: p.name, Status: "matched"})
		result := Result{Found: true, Domain: domain, Attempts: attempts}
		if v, ok := match["email"].(string); ok {
			result.Email = v
		}
		if v, ok := match["source"].(string); ok {
			result.Source = v
		}
		if v, ok := match["confidence"].(int); ok {
			result.Confidence = &v
		}
		if v, ok := match["first_name"].(string); ok {
			result.FirstName = v
		}
		if v, ok := match["last_name"].(string); ok {
			result.LastName = v
		}
		if v, ok := match["position"].(string); ok {
			result.Position = v
		}
		if v, ok := match["company"].(string); ok {
			result.Company = v
		}
		cache.Put("email_enrich", []any{domain}, result)
		return result, nil
	}

	result := Result{Found: false, Domain: domain, Attempts: attempts}
	cache.Put("email_enrich", []any{domain}, result)
	return result, nil
}

func toFloat(value any) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case json.Number:
		f, _ := v.Float64()
		return f
	default:
		return 0
	}
}
