// Package config is the single source of truth for scoring weights and
// thresholds. Defaults preserve the shipped Python behaviour; the weights can
// be overridden at runtime with a JSON env var so a deployment can retune
// without a code change.
package config

import (
	"encoding/json"
	"os"
)

const (
	PerformanceWeak = 40 // lead_score: "very low website performance"
	PerformanceGood = 60 // visibility: "usable performance signal"
	SEOScoreWeak    = 60 // lead_score: "weak SEO audit score"
	SpeedFlagMS     = 2500
)

// VisibilityWeights mirrors DEFAULT_VISIBILITY_WEIGHTS.
func DefaultVisibilityWeights() map[string]map[string]float64 {
	return map[string]map[string]float64{
		"seo": {
			"indexable": 15, "https": 8, "mobile": 8, "title": 10, "meta": 8,
			"h1": 8, "headings": 6, "canonical": 6, "schema": 12, "performance": 9,
			"lighthouse_share": 0.55,
		},
		"aeo": {
			"indexable": 10, "topic": 10, "hierarchy": 14, "questions": 14,
			"depth": 14, "schema": 16, "contact": 10, "semantic": 6, "summary": 6,
			"accessibility": 8,
		},
		"geo": {
			"indexable": 10, "schema": 18, "fields": 14, "identity": 10,
			"external": 10, "facts": 12, "depth": 10, "author": 8, "fresh": 8,
			"best_practices": 6,
		},
		"ai_visibility":   {"seo": 0.35, "aeo": 0.30, "geo": 0.35, "noindex_penalty": 0.45},
		"opportunity_gap": {"visibility": 0.75, "commercial": 0.25},
	}
}

// DefaultScoreWeights mirrors DEFAULT_SCORE_WEIGHTS.
func DefaultScoreWeights() map[string]float64 {
	return map[string]float64{
		"no_website": 35, "low_performance": 15, "mobile_problem": 15,
		"no_cta": 10, "no_booking": 5, "weak_seo": 5, "no_https": 10,
		"phone": 8, "email": 8, "social": 5, "confidence_high": 4,
		"confidence_medium": 2,
	}
}

func envJSON(name string) map[string]any {
	raw := os.Getenv(name)
	if raw == "" {
		return map[string]any{}
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return map[string]any{}
	}
	return v
}

// VisibilityWeights returns the defaults merged with LEADSCOUT_VISIBILITY_WEIGHTS.
func VisibilityWeights() map[string]map[string]float64 {
	base := DefaultVisibilityWeights()
	override := envJSON("LEADSCOUT_VISIBILITY_WEIGHTS")
	for section, value := range override {
		sectionMap, ok := value.(map[string]any)
		if !ok {
			continue
		}
		if _, exists := base[section]; !exists {
			base[section] = map[string]float64{}
		}
		for k, raw := range sectionMap {
			if f, ok := toFloat(raw); ok {
				base[section][k] = f
			}
		}
	}
	return base
}

// ScoreWeights returns the defaults merged with LEADSCOUT_SCORE_WEIGHTS.
func ScoreWeights() map[string]float64 {
	base := DefaultScoreWeights()
	for k, raw := range envJSON("LEADSCOUT_SCORE_WEIGHTS") {
		if f, ok := toFloat(raw); ok {
			base[k] = f
		}
	}
	return base
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	}
	return 0, false
}
