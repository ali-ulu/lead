package services

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ali-ulu/lead/leadscout-go/internal/db"
	"github.com/ali-ulu/lead/leadscout-go/internal/enrichment"
	"github.com/ali-ulu/lead/leadscout-go/internal/model"
	"github.com/ali-ulu/lead/leadscout-go/internal/providers/emailenrich"
)

// TestEnrichLeadRunsWaterfallWhenSiteHasNoEmail exercises the integration: the
// site publishes no address, so EnrichLead falls through to the hosted
// waterfall. With no provider keys configured the waterfall reports not-found,
// which proves the wiring without any network dependency.
func TestEnrichLeadRunsWaterfallWhenSiteHasNoEmail(t *testing.T) {
	useTempDB(t)
	t.Setenv("LEADSCOUT_RESPECT_ROBOTS", "0")
	t.Setenv("LEADSCOUT_CACHE_TTL", "0")
	for _, key := range []string{"HUNTER_API_KEY", "PEOPLE_DATA_LABS_API_KEY", "CLEARBIT_API_KEY"} {
		t.Setenv(key, "")
	}

	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body><h1>Shop</h1><a href="/contact">Contact</a></body></html>`))
	}))
	defer site.Close()

	ids, err := db.UpsertLeads([]*model.Lead{{
		Source: "t", SourceID: "wf-1", Name: "No Mail Shop",
		Country: "TR", Website: model.Str(site.URL), SocialLinks: map[string]string{},
	}})
	if err != nil {
		t.Fatal(err)
	}

	payload, err := EnrichLead(ids[0])
	if err != nil {
		t.Fatalf("enrich: %v", err)
	}
	enrichment, ok := payload["enrichment"].(enrichment.Result)
	if !ok {
		t.Fatalf("missing enrichment payload: %+v", payload)
	}
	if enrichment.EmailEnrichment == nil {
		t.Errorf("waterfall did not run: %+v", enrichment)
	}
	waterfall, ok := enrichment.EmailEnrichment.(emailenrich.Result)
	if !ok {
		t.Fatalf("waterfall payload has unexpected type: %T", enrichment.EmailEnrichment)
	}
	if len(waterfall.Attempts) != 3 {
		t.Errorf("waterfall attempts = %d, want 3", len(waterfall.Attempts))
	}
}
