package diff

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ali-ulu/lead/leadscout-go/internal/db"
	"github.com/ali-ulu/lead/leadscout-go/internal/model"
)

func setupDB(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	os.Setenv("LEADSCOUT_DB", filepath.Join(dir, "diff.db"))
	if os.Getenv("LEADSCOUT_SCHEMA") == "" {
		for _, candidate := range []string{filepath.Join("..", "..", "..", "sql", "schema.sql")} {
			if _, err := os.Stat(candidate); err == nil {
				abs, _ := filepath.Abs(candidate)
				os.Setenv("LEADSCOUT_SCHEMA", abs)
			}
		}
	}
	if err := db.Initialize(); err != nil {
		t.Fatal(err)
	}
}

func lead(t *testing.T, sourceID, name string, score int, websiteStatus, phone string) int64 {
	t.Helper()
	ids, err := db.UpsertLeads([]*model.Lead{{
		Source: "test", SourceID: sourceID, Name: name, Country: "TR", City: "Afyonkarahisar",
		Category: "restaurant", WebsiteStatus: websiteStatus, Phone: strPtr(phone),
		SocialLinks: map[string]string{},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if score > 0 {
		conn, _ := db.Open()
		conn.Exec("UPDATE leads SET lead_score=? WHERE id=?", score, ids[0])
		conn.Close()
	}
	return ids[0]
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func run(t *testing.T, ids []int64) string {
	t.Helper()
	id, err := db.RecordSearchRun("TR", "Afyonkarahisar", "restaurant", 15, ids, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestDetectsNewAndGone(t *testing.T) {
	setupDB(t)
	a := lead(t, "a", "Alpha", 80, "missing", "555")
	first := run(t, []int64{a})
	b := lead(t, "b", "Beta", 80, "missing", "555")
	second := run(t, []int64{b})

	report, err := CompareRuns(first, second, 70)
	if err != nil {
		t.Fatal(err)
	}
	if report["new_count"] != 1 || report["gone_count"] != 1 {
		t.Errorf("new=%v gone=%v", report["new_count"], report["gone_count"])
	}
	if report["new"].([]map[string]any)[0]["name"] != "Beta" {
		t.Errorf("new name wrong")
	}
	if report["gone"].([]map[string]any)[0]["name"] != "Alpha" {
		t.Errorf("gone name wrong")
	}
}

func TestDetectsWebsiteGainedAndLost(t *testing.T) {
	setupDB(t)
	l := lead(t, "a", "Alpha", 80, "missing", "555")
	first := run(t, []int64{l})
	db.UpdateLead(l, map[string]any{"website_status": "healthy", "website": "https://alpha.example"})
	second := run(t, []int64{l})

	gained, _ := CompareRuns(first, second, 70)
	if gained["website_gained_count"] != 1 {
		t.Errorf("gained = %v", gained["website_gained_count"])
	}
	db.UpdateLead(l, map[string]any{"website_status": "missing", "website": ""})
	third := run(t, []int64{l})
	lost, _ := CompareRuns(second, third, 70)
	if lost["website_lost_count"] != 1 {
		t.Errorf("lost = %v", lost["website_lost_count"])
	}
}

func TestHighOpportunityAlertNeedsScoreSiteAndPhone(t *testing.T) {
	setupDB(t)
	good := lead(t, "good", "Good Lead", 80, "missing", "555")
	noPhone := lead(t, "np", "No Phone", 80, "missing", "")
	first := run(t, nil)
	second := run(t, []int64{good, noPhone})
	report, _ := CompareRuns(first, second, 70)
	names := map[string]bool{}
	for _, item := range report["alerts"].([]map[string]any) {
		names[item["name"].(string)] = true
	}
	if !names["Good Lead"] {
		t.Errorf("Good Lead should alert")
	}
	if names["No Phone"] {
		t.Errorf("No Phone should not alert")
	}
}

func TestAlertsOnlyFireForNewLeads(t *testing.T) {
	setupDB(t)
	l := lead(t, "a", "Existing", 80, "missing", "555")
	first := run(t, []int64{l})
	second := run(t, []int64{l})
	report, _ := CompareRuns(first, second, 70)
	if report["alert_count"] != 0 {
		t.Errorf("alert_count = %v", report["alert_count"])
	}
}

func TestRequiresBothIDs(t *testing.T) {
	setupDB(t)
	if _, err := CompareRuns("", "x", 70); err == nil {
		t.Errorf("expected error for empty previous")
	}
	if _, err := CompareRuns("x", "", 70); err == nil {
		t.Errorf("expected error for empty current")
	}
}

func TestFormatReportRendersSections(t *testing.T) {
	setupDB(t)
	a := lead(t, "a", "Alpha", 80, "missing", "555")
	first := run(t, []int64{a})
	b := lead(t, "b", "Beta", 80, "missing", "555")
	second := run(t, []int64{b})
	report, _ := CompareRuns(first, second, 70)
	text := FormatReport(report)
	for _, want := range []string{"Run diff:", "New businesses", "No longer seen"} {
		if !contains(text, want) {
			t.Errorf("missing %q in report", want)
		}
	}
}

func TestReportsVisibilityImprovement(t *testing.T) {
	setupDB(t)
	l := lead(t, "biz-1", "biz-1", 0, "missing", "555")
	db.UpdateLead(l, map[string]any{"ai_visibility_score": 30})
	first := run(t, []int64{l})
	db.UpdateLead(l, map[string]any{"ai_visibility_score": 72})
	second := run(t, []int64{l})
	report, _ := CompareRuns(first, second, 70)
	if report["visibility_improved_count"] != 1 {
		t.Fatalf("improved = %v", report["visibility_improved_count"])
	}
	if report["visibility_improved"].([]map[string]any)[0]["delta"] != 42 {
		t.Errorf("delta wrong")
	}
	if !contains(FormatReport(report), "Visibility improved") {
		t.Errorf("report missing visibility section")
	}
}

func TestReportsVisibilityRegression(t *testing.T) {
	setupDB(t)
	l := lead(t, "biz-2", "biz-2", 0, "missing", "555")
	db.UpdateLead(l, map[string]any{"ai_visibility_score": 80})
	first := run(t, []int64{l})
	db.UpdateLead(l, map[string]any{"ai_visibility_score": 55})
	second := run(t, []int64{l})
	report, _ := CompareRuns(first, second, 70)
	if report["visibility_regressed_count"] != 1 {
		t.Fatalf("regressed = %v", report["visibility_regressed_count"])
	}
	if report["visibility_regressed"].([]map[string]any)[0]["delta"] != -25 {
		t.Errorf("delta wrong")
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(h, n string) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}
	return -1
}
