package aipresence

import (
	"os"
	"testing"

	"github.com/ali-ulu/lead/leadscout-go/internal/providers/websearch"
)

func TestDisabledByDefault(t *testing.T) {
	os.Setenv("LEADSCOUT_AI_PRESENCE", "0")
	defer os.Unsetenv("LEADSCOUT_AI_PRESENCE")
	result := Measure("X", "dentist", "Karachi", "", "", 10)
	if configured, _ := result["configured"].(bool); configured {
		t.Errorf("expected not configured: %v", result)
	}
}

func TestMeasuresCitation(t *testing.T) {
	os.Setenv("LEADSCOUT_AI_PRESENCE", "1")
	defer os.Unsetenv("LEADSCOUT_AI_PRESENCE")

	origProviders, origRun := configuredProviders, runProvider
	configuredProviders = func() []string { return []string{"duckduckgo"} }
	runProvider = func(provider, query string, count int, countryCode string) ([]websearch.Result, error) {
		return []websearch.Result{{URL: "https://example.test/", Title: "Example Dental", Description: "best dentist"}}, nil
	}
	defer func() { configuredProviders, runProvider = origProviders, origRun }()

	result := Measure("Example Dental", "dentist", "Karachi", "", "https://example.test/", 10)
	if measured, _ := result["measured"].(bool); !measured {
		t.Fatalf("not measured: %v", result)
	}
	queries, _ := result["queries"].(int)
	cited, _ := result["cited_count"].(int)
	if queries == 0 || cited != queries {
		t.Errorf("cited %d of %d queries, want all cited", cited, queries)
	}
	if rank, _ := result["best_rank"].(int); rank != 1 {
		t.Errorf("best_rank = %v, want 1", result["best_rank"])
	}
}

func TestNoProvidersReported(t *testing.T) {
	os.Setenv("LEADSCOUT_AI_PRESENCE", "1")
	defer os.Unsetenv("LEADSCOUT_AI_PRESENCE")
	origProviders := configuredProviders
	configuredProviders = func() []string { return nil }
	defer func() { configuredProviders = origProviders }()

	result := Measure("X", "dentist", "Karachi", "", "", 10)
	if configured, _ := result["configured"].(bool); configured {
		t.Errorf("expected not configured when no providers: %v", result)
	}
}
