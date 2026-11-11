// Package websearch verifies a business website through a chain of web-search
// providers. DuckDuckGo (keyless) runs first, then Google Programmable Search
// when configured; Brave and self-hosted SearXNG are optional upgrades.
package websearch

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/ali-ulu/lead/leadscout-go/internal/cache"
	"github.com/ali-ulu/lead/leadscout-go/internal/ratelimit"
)

const userAgent = "LeadScout/6.0 web-verifier"

// DirectoryHosts are never accepted as an official website.
var DirectoryHosts = map[string]bool{
	"google.com": true, "googleusercontent.com": true, "maps.apple.com": true, "bing.com": true,
	"yelp.com": true, "tripadvisor.com": true, "foursquare.com": true, "yellowpages.com": true,
	"justdial.com": true, "facebook.com": true, "instagram.com": true, "linkedin.com": true,
	"x.com": true, "twitter.com": true, "tiktok.com": true, "youtube.com": true,
	"wikipedia.org": true, "mapquest.com": true,
}

// Result is a single search result.
type Result struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

var stopTokens = map[string]bool{
	"the": true, "and": true, "for": true, "clinic": true, "restaurant": true,
	"hotel": true, "salon": true, "shop": true, "services": true, "service": true,
	"official": true, "website": true, "karachi": true, "istanbul": true, "berlin": true,
}

var wordRe = regexp.MustCompile(`[a-z0-9]+`)

func tokens(value string) map[string]bool {
	out := map[string]bool{}
	for _, t := range wordRe.FindAllString(strings.ToLower(value), -1) {
		if len(t) >= 3 && !stopTokens[t] {
			out[t] = true
		}
	}
	return out
}

func hostOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
}

func isDirectory(rawURL string) bool {
	h := hostOf(rawURL)
	for d := range DirectoryHosts {
		if h == d || strings.HasSuffix(h, "."+d) {
			return true
		}
	}
	return false
}

func scoreResult(r Result, name, city, country string) float64 {
	if !(strings.HasPrefix(r.URL, "http://") || strings.HasPrefix(r.URL, "https://")) || isDirectory(r.URL) {
		return 0
	}
	needle := tokens(name)
	hay := tokens(r.Title + " " + hostOf(r.URL) + " " + r.Description)
	if len(needle) == 0 {
		return 0
	}
	overlap := 0
	for t := range needle {
		if hay[t] {
			overlap++
		}
	}
	score := float64(overlap) / float64(len(needle)) * 0.72
	text := strings.ToLower(r.Title + " " + r.Description)
	if city != "" && strings.Contains(text, strings.ToLower(city)) {
		score += 0.12
	}
	if country != "" && strings.Contains(text, strings.ToLower(country)) {
		score += 0.06
	}
	for _, w := range []string{"official", "contact", "about us"} {
		if strings.Contains(text, w) {
			score += 0.05
			break
		}
	}
	h := hostOf(r.URL)
	for t := range needle {
		if strings.Contains(h, t) {
			score += 0.12
			break
		}
	}
	if score > 1 {
		score = 1
	}
	return score
}

func braveSearch(query string, count int, countryCode string) ([]Result, error) {
	key := strings.TrimSpace(os.Getenv("BRAVE_SEARCH_API_KEY"))
	if key == "" {
		return nil, errors.New("BRAVE_SEARCH_API_KEY is not configured.")
	}
	params := url.Values{}
	params.Set("q", query)
	params.Set("count", itoa(clamp(count, 1, 20)))
	if len(countryCode) == 2 {
		params.Set("country", strings.ToUpper(countryCode))
	}
	req, _ := http.NewRequest(http.MethodGet, "https://api.search.brave.com/res/v1/web/search?"+params.Encode(), nil)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", key)
	req.Header.Set("User-Agent", userAgent)
	body, err := doJSON(req)
	if err != nil {
		return nil, err
	}
	var data struct {
		Web struct {
			Results []Result `json:"results"`
		} `json:"web"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}
	return data.Web.Results, nil
}

func googleSearch(query string, count int) ([]Result, error) {
	key := strings.TrimSpace(os.Getenv("GOOGLE_CSE_API_KEY"))
	cx := strings.TrimSpace(os.Getenv("GOOGLE_CSE_ID"))
	if key == "" || cx == "" {
		return nil, errors.New("GOOGLE_CSE_API_KEY and GOOGLE_CSE_ID are not configured.")
	}
	params := url.Values{}
	params.Set("key", key)
	params.Set("cx", cx)
	params.Set("q", query)
	params.Set("num", itoa(clamp(count, 1, 10)))
	req, _ := http.NewRequest(http.MethodGet, "https://www.googleapis.com/customsearch/v1?"+params.Encode(), nil)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	body, err := doJSON(req)
	if err != nil {
		return nil, err
	}
	var data struct {
		Items []struct {
			Link    string `json:"link"`
			Title   string `json:"title"`
			Snippet string `json:"snippet"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}
	var out []Result
	for _, item := range data.Items {
		out = append(out, Result{URL: item.Link, Title: item.Title, Description: item.Snippet})
	}
	return out, nil
}

func searxngSearch(query string, count int) ([]Result, error) {
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("SEARXNG_URL")), "/")
	if base == "" {
		return nil, errors.New("SEARXNG_URL is not configured.")
	}
	params := url.Values{}
	params.Set("q", query)
	params.Set("format", "json")
	params.Set("safesearch", "1")
	req, _ := http.NewRequest(http.MethodGet, base+"/search?"+params.Encode(), nil)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	body, err := doJSON(req)
	if err != nil {
		return nil, err
	}
	var data struct {
		Results []Result `json:"results"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}
	if len(data.Results) > clamp(count, 1, 20) {
		data.Results = data.Results[:clamp(count, 1, 20)]
	}
	return data.Results, nil
}

var tagRe = regexp.MustCompile(`<[^>]+>`)
var wsRe = regexp.MustCompile(`\s+`)

func cleanHTML(value string) string {
	return strings.TrimSpace(wsRe.ReplaceAllString(html.UnescapeString(tagRe.ReplaceAllString(value, "")), " "))
}

var ddgResultRe = regexp.MustCompile(`(?s)<a[^>]*class="result__a"[^>]*href="([^"]+)"[^>]*>(.*?)</a>(?:.*?class="result__snippet"[^>]*>(.*?)</a>)?`)

func duckduckgoSearch(query string, count int) ([]Result, error) {
	form := url.Values{}
	form.Set("q", query)
	req, _ := http.NewRequest(http.MethodPost, ddgHTMLURL, strings.NewReader(form.Encode()))
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120 Safari/537.36")
	req.Header.Set("Accept", "text/html")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return parseDuckDuckGo(string(raw), count)
}

// ddgHTMLURL is the DuckDuckGo HTML endpoint; overridable in tests.
var ddgHTMLURL = "https://html.duckduckgo.com/html/"

func parseDuckDuckGo(page string, count int) ([]Result, error) {
	lowered := strings.ToLower(page)
	if strings.Contains(page, "/anomaly") || strings.Contains(lowered, "anomaly-modal") ||
		strings.Contains(lowered, "unusual traffic") || strings.Contains(lowered, "captcha") {
		// DuckDuckGo serves a bot-check page once a burst of serial queries trips
		// its rate limiter; raise so the chain falls through to the next provider.
		return nil, errors.New("DuckDuckGo returned a bot-check page; try another provider.")
	}

	var results []Result
	for _, m := range ddgResultRe.FindAllStringSubmatch(page, -1) {
		href := html.UnescapeString(m[1])
		if strings.HasPrefix(href, "//duckduckgo.com/l/") {
			if target := uddgParam("https:" + href); target != "" {
				href = target
			}
		} else if strings.HasPrefix(href, "/l/") {
			if target := uddgParam(href); target != "" {
				href = target
			}
		}
		if !(strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://")) {
			continue
		}
		snippet := ""
		if len(m) > 3 {
			snippet = m[3]
		}
		results = append(results, Result{URL: href, Title: cleanHTML(m[2]), Description: cleanHTML(snippet)})
		if len(results) >= clamp(count, 1, 20) {
			break
		}
	}
	return results, nil
}

func uddgParam(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if v := u.Query().Get("uddg"); v != "" {
		return v
	}
	return ""
}

func doJSON(req *http.Request) ([]byte, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func explicitChain() []string {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv("LEADSCOUT_WEB_SEARCH")))
	if raw == "" {
		return nil
	}
	var items []string
	for _, part := range strings.Split(raw, ",") {
		if p := strings.TrimSpace(part); p != "" {
			items = append(items, p)
		}
	}
	if len(items) == 0 {
		return nil
	}
	if items[0] == "off" || items[0] == "none" || items[0] == "disabled" {
		return []string{}
	}
	return items
}

func available(provider string) bool {
	switch provider {
	case "brave":
		return strings.TrimSpace(os.Getenv("BRAVE_SEARCH_API_KEY")) != ""
	case "searxng":
		return strings.TrimSpace(os.Getenv("SEARXNG_URL")) != ""
	case "google":
		return strings.TrimSpace(os.Getenv("GOOGLE_CSE_API_KEY")) != "" && strings.TrimSpace(os.Getenv("GOOGLE_CSE_ID")) != ""
	case "duckduckgo":
		return true
	}
	return false
}

// ConfiguredProviders returns the effective provider chain.
func ConfiguredProviders() []string {
	explicit := explicitChain()
	if explicit != nil {
		var out []string
		for _, p := range explicit {
			if available(p) {
				out = append(out, p)
			}
		}
		return out
	}
	var out []string
	for _, p := range []string{"duckduckgo", "google"} {
		if available(p) {
			out = append(out, p)
		}
	}
	return out
}

// RunProvider runs a single provider (used by ai_presence too).
func RunProvider(provider, query string, count int, countryCode string) ([]Result, error) {
	key := []any{provider, query, count, countryCode}
	var cached []Result
	if cache.Get("websearch", key, &cached) {
		return cached, nil
	}
	call := func() ([]Result, error) {
		switch provider {
		case "brave":
			return braveSearch(query, count, countryCode)
		case "searxng":
			return searxngSearch(query, count)
		case "google":
			return googleSearch(query, count)
		default:
			return duckduckgoSearch(query, count)
		}
	}
	results, err := ratelimit.Call("websearch:"+provider, call, ratelimit.Options{MinInterval: 0, Retries: 0})
	if err != nil {
		return nil, err
	}
	cache.Put("websearch", key, results)
	return results, nil
}

// Candidate is a scored search result.
type Candidate struct {
	URL         string  `json:"url"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Score       float64 `json:"score"`
	Host        string  `json:"host"`
}

// VerifyResult is the outcome of a business web verification.
type VerifyResult struct {
	Configured bool                `json:"configured"`
	Provider   string              `json:"provider"`
	Providers  []string            `json:"providers,omitempty"`
	Query      string              `json:"query,omitempty"`
	Verified   bool                `json:"verified"`
	Website    *string             `json:"website"`
	Confidence float64             `json:"confidence"`
	Best       *Candidate          `json:"best"`
	Candidates []Candidate         `json:"candidates"`
	Reason     string              `json:"reason,omitempty"`
	Errors     []map[string]string `json:"errors,omitempty"`
}

// VerifyBusinessWeb checks whether a business has a verifiable official site.
func VerifyBusinessWeb(name, city, country, countryCode string, count int) VerifyResult {
	providers := ConfiguredProviders()
	if len(providers) == 0 {
		return VerifyResult{
			Configured: false, Provider: "", Verified: false,
			Reason: "No web-search provider configured.", Candidates: []Candidate{},
		}
	}

	query := strings.TrimSpace(strings.Join(nonEmpty(`"`+name+`"`, city, country, "official website"), " "))
	provider := ""
	var raw []Result
	var errs []map[string]string
	for _, candidate := range providers {
		got, err := RunProvider(candidate, query, count, countryCode)
		if err != nil {
			errs = append(errs, map[string]string{"provider": candidate, "error": err.Error()})
			continue
		}
		provider = candidate
		if len(got) > 0 {
			raw = got
			break
		}
	}

	var candidates []Candidate
	for _, item := range raw {
		if strings.TrimSpace(item.URL) == "" {
			continue
		}
		score := scoreResult(item, name, city, country)
		candidates = append(candidates, Candidate{
			URL: item.URL, Title: item.Title, Description: item.Description,
			Score: round3(score), Host: hostOf(item.URL),
		})
	}
	sortCandidates(candidates)
	var best *Candidate
	if len(candidates) > 0 {
		best = &candidates[0]
	}
	verified := best != nil && best.Score >= 0.58
	result := VerifyResult{
		Configured: true, Provider: provider, Providers: providers, Query: query,
		Verified: verified, Confidence: 0, Best: best,
		Candidates: firstN(candidates, 5),
	}
	if best != nil {
		result.Confidence = best.Score
	}
	if verified {
		result.Website = &best.URL
	}
	if len(errs) > 0 {
		result.Errors = errs
	}
	return result
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

func sortCandidates(c []Candidate) {
	for i := 1; i < len(c); i++ {
		for j := i; j > 0 && c[j].Score > c[j-1].Score; j-- {
			c[j], c[j-1] = c[j-1], c[j]
		}
	}
}

func firstN(c []Candidate, n int) []Candidate {
	if len(c) > n {
		return c[:n]
	}
	return c
}

func round3(f float64) float64 { return float64(int(f*1000+0.5)) / 1000 }

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func itoa(i int) string { return fmt.Sprintf("%d", i) }
