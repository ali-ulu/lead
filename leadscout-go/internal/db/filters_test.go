package db

import (
	"testing"

	"github.com/ali-ulu/lead/leadscout-go/internal/model"
)

func seedTwo(t *testing.T) []int64 {
	t.Helper()
	ids, err := UpsertLeads([]*model.Lead{
		{Source: "test", SourceID: "karachi-1", Name: "Karachi One", Country: "Pakistan", City: "Karachi", Category: "dentist", WebsiteStatus: "missing", SocialLinks: map[string]string{}, DataConfidence: "medium"},
		{Source: "test", SourceID: "berlin-1", Name: "Berlin One", Country: "Germany", City: "Berlin", Category: "dentist", WebsiteStatus: "missing", SocialLinks: map[string]string{}, DataConfidence: "medium"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return ids
}

func names(rows []*model.Lead) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Name
	}
	return out
}

func TestIDsFilterIsolates(t *testing.T) {
	useTempDB(t)
	ids := seedTwo(t)
	rows, _ := ListLeads(map[string]string{"ids": itoa(ids[0])})
	if got := names(rows); len(got) != 1 || got[0] != "Karachi One" {
		t.Errorf("got %v", got)
	}
}

func TestSearchIDIsolates(t *testing.T) {
	useTempDB(t)
	ids := seedTwo(t)
	searchID, err := RecordSearchRun("Pakistan", "Karachi", "dentist", 20, []int64{ids[0]}, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := ListLeads(map[string]string{"search_id": searchID})
	if got := names(rows); len(got) != 1 || got[0] != "Karachi One" {
		t.Errorf("got %v", got)
	}
}

func TestUnknownSearchIDReturnsEmpty(t *testing.T) {
	useTempDB(t)
	seedTwo(t)
	rows, _ := ListLeads(map[string]string{"search_id": "does-not-exist"})
	if len(rows) != 0 {
		t.Errorf("got %d rows", len(rows))
	}
}

func TestInvalidIDsDoNotLeakHistory(t *testing.T) {
	useTempDB(t)
	seedTwo(t)
	rows, _ := ListLeads(map[string]string{"ids": "not-an-id"})
	if len(rows) != 0 {
		t.Errorf("got %d rows", len(rows))
	}
}

func TestClearAllRemovesLeadsAndRuns(t *testing.T) {
	useTempDB(t)
	ids := seedTwo(t)
	searchID, _ := RecordSearchRun("Pakistan", "Karachi", "dentist", 20, ids, nil, false, nil)
	if err := ClearAll(); err != nil {
		t.Fatal(err)
	}
	if rows, _ := ListLeads(map[string]string{}); len(rows) != 0 {
		t.Errorf("leads remain")
	}
	if rows, _ := ListLeads(map[string]string{"search_id": searchID}); len(rows) != 0 {
		t.Errorf("search leads remain")
	}
	conn, _ := Open()
	defer conn.Close()
	var runs, links int
	conn.QueryRow("SELECT COUNT(*) FROM search_runs").Scan(&runs)
	conn.QueryRow("SELECT COUNT(*) FROM search_run_leads").Scan(&links)
	if runs != 0 || links != 0 {
		t.Errorf("runs=%d links=%d", runs, links)
	}
}

func TestOpportunityGapFilterable(t *testing.T) {
	useTempDB(t)
	ids, err := UpsertLeads([]*model.Lead{{
		Source: "test", SourceID: "gap-1", Name: "Gap Business", Country: "Pakistan",
		City: "Karachi", Category: "dentist", Website: model.Str("https://example.test"),
		WebsiteStatus: "weak", SEOScore: model.Int(30), AEOScore: model.Int(20),
		GEOScore: model.Int(15), AIVisibilityScore: model.Int(22), OpportunityGapScore: model.Int(86),
		VisibilityReasons: map[string][]string{"seo": {"weak"}},
		VisibilitySignals: map[string]any{"indexable": true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	lead, _ := GetLead(ids[0])
	if *lead.OpportunityGapScore != 86 {
		t.Errorf("gap = %d", *lead.OpportunityGapScore)
	}
	if got := lead.VisibilityReasons["seo"]; len(got) != 1 || got[0] != "weak" {
		t.Errorf("reasons = %v", lead.VisibilityReasons)
	}
	rows, _ := ListLeads(map[string]string{"min_opportunity_gap_score": "80"})
	if len(rows) != 1 || rows[0].ID != ids[0] {
		t.Errorf("min 80 filter: %d rows", len(rows))
	}
	rows, _ = ListLeads(map[string]string{"min_opportunity_gap_score": "90"})
	if len(rows) != 0 {
		t.Errorf("min 90 filter: %d rows", len(rows))
	}
}

func TestMaxVisibilityFilters(t *testing.T) {
	useTempDB(t)
	UpsertLeads([]*model.Lead{
		{Source: "test", SourceID: "gap-1", Name: "Weak Visibility Business", Country: "Pakistan", City: "Karachi", Category: "dentist", Website: model.Str("https://weak.test"), WebsiteStatus: "weak", SEOScore: model.Int(30), AEOScore: model.Int(20), GEOScore: model.Int(15), AIVisibilityScore: model.Int(22), OpportunityGapScore: model.Int(86)},
		{Source: "test", SourceID: "gap-2", Name: "Strong Visibility Business", Country: "Pakistan", City: "Karachi", Category: "dentist", Website: model.Str("https://strong.test"), WebsiteStatus: "healthy", SEOScore: model.Int(88), AEOScore: model.Int(82), GEOScore: model.Int(79), AIVisibilityScore: model.Int(83), OpportunityGapScore: model.Int(22)},
	})
	rows, _ := ListLeads(map[string]string{"max_ai_visibility_score": "40"})
	if len(rows) != 1 || rows[0].SourceID != "gap-1" {
		t.Errorf("max aio filter: %v", names(rows))
	}
	rows, _ = ListLeads(map[string]string{"max_seo_score": "35", "max_geo_score": "20"})
	if len(rows) != 1 || rows[0].SourceID != "gap-1" {
		t.Errorf("max seo/geo filter: %v", names(rows))
	}
}

func itoa(i int64) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	neg := i < 0
	if neg {
		i = -i
	}
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	if neg {
		return "-" + string(b)
	}
	return string(b)
}
