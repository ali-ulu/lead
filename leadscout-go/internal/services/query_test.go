package services

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ali-ulu/lead/leadscout-go/internal/db"
	"github.com/ali-ulu/lead/leadscout-go/internal/model"
)

func useTempDB(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	os.Setenv("LEADSCOUT_DB", filepath.Join(dir, "services.db"))
	if os.Getenv("LEADSCOUT_SCHEMA") == "" {
		for _, candidate := range []string{filepath.Join("..", "..", "..", "sql", "schema.sql")} {
			if _, err := os.Stat(candidate); err == nil {
				abs, _ := filepath.Abs(candidate)
				os.Setenv("LEADSCOUT_SCHEMA", abs)
				break
			}
		}
	}
	if err := db.Initialize(); err != nil {
		t.Fatalf("initialize: %v", err)
	}
}

func seedMarket(t *testing.T) []int64 {
	t.Helper()
	ids, err := db.UpsertLeads([]*model.Lead{
		{Source: "t", SourceID: "rel-1", Name: "Strong", Country: "US", City: "NY", Category: "dentist", AIVisibilityScore: model.Int(90), WebsiteStatus: "missing", SocialLinks: map[string]string{}},
		{Source: "t", SourceID: "rel-2", Name: "Weak", Country: "US", City: "NY", Category: "dentist", AIVisibilityScore: model.Int(30), WebsiteStatus: "missing", SocialLinks: map[string]string{}},
		{Source: "t", SourceID: "rel-3", Name: "Unmeasured", Country: "US", City: "NY", Category: "dentist", WebsiteStatus: "missing", SocialLinks: map[string]string{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return ids
}

func TestListLeadsRelativeAnnotatesEachRow(t *testing.T) {
	useTempDB(t)
	ids := seedMarket(t)
	rows, err := ListLeadsRelative(map[string]string{"city": "NY"})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[int64]*model.Lead{}
	for _, r := range rows {
		byID[r.ID] = r
	}
	check := func(id int64, want *int) {
		got := byID[id].AIVisibilityScorePercentile
		if want == nil {
			if got != nil {
				t.Errorf("lead %d percentile = %d, want nil", id, *got)
			}
			return
		}
		if got == nil || *got != *want {
			t.Errorf("lead %d percentile = %v, want %d", id, got, *want)
		}
	}
	check(ids[0], model.Int(100))
	check(ids[1], model.Int(50))
	check(ids[2], nil)
}

func TestAnnotateLeadRelativeUsesOwnMarket(t *testing.T) {
	useTempDB(t)
	ids := seedMarket(t)
	lead, err := db.GetLead(ids[1])
	if err != nil {
		t.Fatal(err)
	}
	annotated := AnnotateLeadRelative(lead)
	if annotated.AIVisibilityScorePercentile == nil || *annotated.AIVisibilityScorePercentile != 50 {
		t.Errorf("percentile = %v, want 50", annotated.AIVisibilityScorePercentile)
	}
	if AnnotateLeadRelative(nil) != nil {
		t.Errorf("nil lead should annotate to nil")
	}
	unmeasured, _ := db.GetLead(ids[2])
	if got := AnnotateLeadRelative(unmeasured).AIVisibilityScorePercentile; got != nil {
		t.Errorf("unmeasured percentile = %d, want nil", *got)
	}
}
