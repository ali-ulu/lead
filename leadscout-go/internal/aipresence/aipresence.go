// Package aipresence measures whether a business surfaces in AI-style answers
// by running category/city questions through the web-search chain. It is
// opt-in because it spends search-provider quota.
package aipresence

import (
	"os"
	"regexp"
	"strings"

	"github.com/ali-ulu/lead/leadscout-go/internal/providers/websearch"
)

// Enabled reports whether measurement is opted in.
func Enabled() bool {
	return os.Getenv("LEADSCOUT_AI_PRESENCE") == "1"
}

// provider seams keep measurement testable without live network calls.
var (
	configuredProviders = websearch.ConfiguredProviders
	runProvider         = websearch.RunProvider
)

var tokenRe = regexp.MustCompile(`[a-z0-9]+`)

func tokens(value string) map[string]bool {
	out := map[string]bool{}
	for _, t := range tokenRe.FindAllString(strings.ToLower(value), -1) {
		if len(t) >= 4 {
			out[t] = true
		}
	}
	return out
}

func questions(category, city, country string) []string {
	place := strings.TrimSpace(strings.Join(nonEmpty(city, country), " "))
	sector := strings.ReplaceAll(category, "_", " ")
	if place == "" {
		return nil
	}
	return []string{
		"best " + sector + " in " + place,
		"recommended " + sector + " " + place,
	}
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

func hostOf(rawURL string) string {
	// minimal host extraction without importing net/url twice
	i := strings.Index(rawURL, "://")
	rest := rawURL
	if i >= 0 {
		rest = rawURL[i+3:]
	}
	if j := strings.IndexAny(rest, "/?#"); j >= 0 {
		rest = rest[:j]
	}
	if j := strings.Index(rest, ":"); j >= 0 {
		rest = rest[:j]
	}
	return strings.TrimPrefix(strings.ToLower(rest), "www.")
}

// Measure returns how often a business appears in answer-style search results.
func Measure(name, category, city, country, website string, count int) map[string]any {
	if !Enabled() {
		return map[string]any{"configured": false, "reason": "LEADSCOUT_AI_PRESENCE is not enabled."}
	}
	providers := configuredProviders()
	if len(providers) == 0 {
		return map[string]any{"configured": false, "reason": "No web-search provider configured."}
	}
	qs := questions(category, city, country)
	if len(qs) == 0 {
		return map[string]any{"configured": false, "reason": "City/category are required to measure AI presence."}
	}
	nameTokens := tokens(name)
	domain := ""
	if website != "" {
		domain = hostOf(website)
	}

	type hit struct {
		Question string         `json:"question"`
		Cited    bool           `json:"cited"`
		Citation map[string]any `json:"citation"`
	}
	var hits []hit
	queriesRun := 0
	for _, question := range qs {
		var results []websearch.Result
		for _, provider := range providers {
			got, err := runProvider(provider, question, count, "")
			if err != nil {
				continue
			}
			if len(got) > 0 {
				results = got
				break
			}
		}
		if len(results) == 0 {
			continue
		}
		queriesRun++
		var citation map[string]any
		for index, item := range results {
			hay := strings.ToLower(item.URL + " " + item.Title + " " + item.Description)
			host := hostOf(item.URL)
			matched := (domain != "" && strings.Contains(hay, domain)) ||
				(len(nameTokens) > 0 && isSubset(nameTokens, tokens(item.Title+" "+item.Description)))
			if matched {
				citation = map[string]any{"rank": index + 1, "url": item.URL, "host": host, "title": item.Title}
				break
			}
		}
		hits = append(hits, hit{Question: question, Cited: citation != nil, Citation: citation})
	}

	if queriesRun == 0 {
		return map[string]any{"configured": true, "measured": false, "reason": "Search providers returned no results."}
	}
	cited := 0
	bestRank := 0
	for _, h := range hits {
		if h.Cited {
			cited++
			if rank, ok := h.Citation["rank"].(int); ok && (bestRank == 0 || rank < bestRank) {
				bestRank = rank
			}
		}
	}
	out := map[string]any{
		"configured": true, "measured": true, "queries": queriesRun,
		"cited_count": cited, "mention_rate": round3(float64(cited) / float64(queriesRun)),
		"hits": hits,
	}
	if bestRank > 0 {
		out["best_rank"] = bestRank
	} else {
		out["best_rank"] = nil
	}
	return out
}

func isSubset(a, b map[string]bool) bool {
	if len(a) == 0 {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

func round3(f float64) float64 {
	return float64(int(f*1000+0.5)) / 1000
}
