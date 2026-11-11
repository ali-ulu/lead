package config

import (
	"os"
	"testing"
)

func TestDefaultsPresent(t *testing.T) {
	vis := DefaultVisibilityWeights()
	for _, section := range []string{"seo", "aeo", "geo", "ai_visibility", "opportunity_gap"} {
		if _, ok := vis[section]; !ok {
			t.Errorf("visibility section %q missing", section)
		}
	}
	scores := DefaultScoreWeights()
	if scores["no_website"] != 35 {
		t.Errorf("no_website = %v, want 35", scores["no_website"])
	}
}

func TestEnvOverrideMerges(t *testing.T) {
	os.Setenv("LEADSCOUT_VISIBILITY_WEIGHTS", `{"seo": {"indexable": 99}}`)
	defer os.Unsetenv("LEADSCOUT_VISIBILITY_WEIGHTS")
	got := VisibilityWeights()
	if got["seo"]["indexable"] != 99 {
		t.Errorf("indexable = %v, want 99", got["seo"]["indexable"])
	}
	if got["seo"]["title"] != DefaultVisibilityWeights()["seo"]["title"] {
		t.Errorf("non-overridden weight should keep default")
	}
}

func TestInvalidEnvIsIgnored(t *testing.T) {
	os.Setenv("LEADSCOUT_VISIBILITY_WEIGHTS", "{not json")
	defer os.Unsetenv("LEADSCOUT_VISIBILITY_WEIGHTS")
	if VisibilityWeights()["seo"]["indexable"] != DefaultVisibilityWeights()["seo"]["indexable"] {
		t.Errorf("invalid JSON should fall back to defaults")
	}
}

func TestScoreWeightsOverride(t *testing.T) {
	os.Setenv("LEADSCOUT_SCORE_WEIGHTS", `{"phone": 42}`)
	defer os.Unsetenv("LEADSCOUT_SCORE_WEIGHTS")
	if ScoreWeights()["phone"] != 42 {
		t.Errorf("phone = %v, want 42", ScoreWeights()["phone"])
	}
}
