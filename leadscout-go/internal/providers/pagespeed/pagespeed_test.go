package pagespeed

import (
	"net/url"
	"testing"
	"time"
)

const sample = `{
  "loadingExperience": {
    "overall_category": "AVERAGE",
    "metrics": {
      "LARGEST_CONTENTFUL_PAINT_MS": {"percentile": 2500, "category": "AVERAGE"},
      "CUMULATIVE_LAYOUT_SHIFT_SCORE": {"percentile": 5, "category": "FAST"}
    }
  },
  "lighthouseResult": {
    "lighthouseVersion": "12.0.0",
    "categories": {
      "performance": {"score": 0.72},
      "accessibility": {"score": 0.94},
      "best-practices": {"score": 1.0},
      "seo": {"score": 0.85}
    }
  }
}`

func TestParseResponseScalesScores(t *testing.T) {
	result, err := ParseResponse([]byte(sample))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Performance == nil || *result.Performance != 72 {
		t.Errorf("performance = %v, want 72", result.Performance)
	}
	if result.Accessibility == nil || *result.Accessibility != 94 {
		t.Errorf("accessibility = %v, want 94", result.Accessibility)
	}
	if result.BestPractices == nil || *result.BestPractices != 100 {
		t.Errorf("best-practices = %v, want 100", result.BestPractices)
	}
	if result.SEO == nil || *result.SEO != 85 {
		t.Errorf("seo = %v, want 85", result.SEO)
	}
	if result.LighthouseVer != "12.0.0" {
		t.Errorf("version = %q", result.LighthouseVer)
	}
	if result.Source != "pagespeed" {
		t.Errorf("source = %q", result.Source)
	}
}

func TestParseResponseExtractsFieldData(t *testing.T) {
	result, err := ParseResponse([]byte(sample))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.FieldData == nil {
		t.Fatal("field data missing")
	}
	if result.FieldData["overall_category"] != "AVERAGE" {
		t.Errorf("overall_category = %v", result.FieldData["overall_category"])
	}
}

func TestParseResponseEmptyYieldsNilScores(t *testing.T) {
	result, err := ParseResponse([]byte(`{}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Performance != nil || result.FieldData != nil {
		t.Errorf("expected nil scores and field data, got %+v", result)
	}
}

func TestParseResponseSurfacesAPIError(t *testing.T) {
	_, err := ParseResponse([]byte(`{"error": {"message": "Quota exceeded"}}`))
	if err == nil || err.Error() != "Quota exceeded" {
		t.Fatalf("err = %v, want quota error", err)
	}
}

func TestConfiguredIsOptIn(t *testing.T) {
	t.Setenv("GOOGLE_PSI_API_KEY", "")
	t.Setenv("LEADSCOUT_PAGESPEED", "")
	if Configured() {
		t.Error("expected disabled by default")
	}
	t.Setenv("GOOGLE_PSI_API_KEY", "k")
	if !Configured() {
		t.Error("expected enabled with a key")
	}
	t.Setenv("GOOGLE_PSI_API_KEY", "")
	t.Setenv("LEADSCOUT_PAGESPEED", "1")
	if !Configured() {
		t.Error("expected enabled with the flag")
	}
}

func TestAnalyzeDisabledReturnsError(t *testing.T) {
	t.Setenv("GOOGLE_PSI_API_KEY", "")
	t.Setenv("LEADSCOUT_PAGESPEED", "")
	if _, err := Analyze("https://example.com", "mobile", time.Second); err == nil {
		t.Fatal("expected an error when disabled")
	}
}

func TestBuildURLIncludesCategoriesAndKey(t *testing.T) {
	original := Endpoint
	Endpoint = "https://psi.example/runPagespeed"
	defer func() { Endpoint = original }()

	t.Setenv("GOOGLE_PSI_API_KEY", "secret")
	raw := BuildURL("https://shop.example", "desktop")
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	query := parsed.Query()
	if query.Get("url") != "https://shop.example" {
		t.Errorf("url = %q", query.Get("url"))
	}
	if query.Get("strategy") != "desktop" {
		t.Errorf("strategy = %q", query.Get("strategy"))
	}
	if query.Get("key") != "secret" {
		t.Errorf("key = %q", query.Get("key"))
	}
	if got := query["category"]; len(got) != 4 {
		t.Errorf("categories = %v, want 4", got)
	}
}

func TestBuildURLDefaultsToMobile(t *testing.T) {
	original := Endpoint
	Endpoint = "https://psi.example/runPagespeed"
	defer func() { Endpoint = original }()

	t.Setenv("GOOGLE_PSI_API_KEY", "")
	parsed, _ := url.Parse(BuildURL("https://shop.example", ""))
	if parsed.Query().Get("strategy") != "mobile" {
		t.Errorf("strategy = %q, want mobile", parsed.Query().Get("strategy"))
	}
	if parsed.Query().Has("key") {
		t.Error("key should be absent without GOOGLE_PSI_API_KEY")
	}
}
