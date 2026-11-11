package crm

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
	os.Setenv("LEADSCOUT_DB", filepath.Join(dir, "crm.db"))
	if os.Getenv("LEADSCOUT_SCHEMA") == "" {
		candidate := filepath.Join("..", "..", "..", "sql", "schema.sql")
		if _, err := os.Stat(candidate); err == nil {
			abs, _ := filepath.Abs(candidate)
			os.Setenv("LEADSCOUT_SCHEMA", abs)
		}
	}
	if err := db.Initialize(); err != nil {
		t.Fatal(err)
	}
}

func makeLead(t *testing.T, sourceID string) int64 {
	t.Helper()
	ids, err := db.UpsertLeads([]*model.Lead{{
		Source: "test", SourceID: sourceID, Name: "Test Business", Country: "TR", City: "Istanbul",
		Category: "dentist", WebsiteStatus: "missing", SocialLinks: map[string]string{},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return ids[0]
}

func TestTimelineAndEngagement(t *testing.T) {
	useTempDB(t)
	leadID := makeLead(t, "1")
	if _, err := AddActivity(leadID, "note", "", "", "", "called owner", "", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := SetEngagement(leadID, "rejected", "not interested"); err != nil {
		t.Fatal(err)
	}
	rows, err := ListActivities(leadID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) < 2 {
		t.Errorf("activities = %d, want >= 2", len(rows))
	}
	lead, _ := db.GetLead(leadID)
	if lead.EngagementStatus != "rejected" {
		t.Errorf("engagement = %q, want rejected", lead.EngagementStatus)
	}
}

func TestInvalidActivityKindRejected(t *testing.T) {
	useTempDB(t)
	leadID := makeLead(t, "2")
	if _, err := AddActivity(leadID, "bogus", "", "", "", "x", "", nil); err == nil {
		t.Errorf("invalid kind should error")
	}
	if _, err := SetEngagement(leadID, "bogus", ""); err == nil {
		t.Errorf("invalid engagement should error")
	}
}

func TestStaleSentLeadBecomesNoResponse(t *testing.T) {
	useTempDB(t)
	leadID := makeLead(t, "3")
	if _, err := db.UpdateLead(leadID, map[string]any{
		"engagement_status": "sent",
		"last_contacted_at": "2026-01-01T00:00:00+00:00",
	}); err != nil {
		t.Fatal(err)
	}
	result, err := MarkStaleNoResponse(7)
	if err != nil {
		t.Fatal(err)
	}
	ids, _ := result["lead_ids"].([]int64)
	found := false
	for _, id := range ids {
		if id == leadID {
			found = true
		}
	}
	if !found {
		t.Errorf("stale lead not flagged: %v", ids)
	}
	lead, _ := db.GetLead(leadID)
	if lead.EngagementStatus != "no_response" {
		t.Errorf("engagement = %q, want no_response", lead.EngagementStatus)
	}
}

func TestDeliveryReceiptUpdatesMessageAndLead(t *testing.T) {
	useTempDB(t)
	leadID := makeLead(t, "4")
	if _, err := AddActivity(leadID, "message", "facebook", "sent", "out", "hello", "mid.123", nil); err != nil {
		t.Fatal(err)
	}
	updated, err := UpdateDeliveryStatus("mid.123", "delivered")
	if err != nil {
		t.Fatal(err)
	}
	if !updated {
		t.Fatalf("delivery not applied")
	}
	rows, _ := ListActivities(leadID)
	var message *Activity
	for i := range rows {
		if rows[i].ExternalID != nil && *rows[i].ExternalID == "mid.123" {
			message = &rows[i]
		}
	}
	if message == nil || message.Status == nil || *message.Status != "delivered" {
		t.Errorf("message status not updated: %+v", message)
	}
	lead, _ := db.GetLead(leadID)
	if lead.EngagementStatus != "delivered" {
		t.Errorf("engagement = %q, want delivered", lead.EngagementStatus)
	}
}

func TestAddNoteAppendsAndRejectsEmpty(t *testing.T) {
	useTempDB(t)
	leadID := makeLead(t, "5")
	if _, err := AddNote(leadID, ""); err == nil {
		t.Errorf("empty note should error")
	}
	if _, err := AddNote(leadID, "first"); err != nil {
		t.Fatal(err)
	}
	if _, err := AddNote(leadID, "second"); err != nil {
		t.Fatal(err)
	}
	lead, _ := db.GetLead(leadID)
	if lead.Notes == nil || *lead.Notes != "first\nsecond" {
		t.Errorf("notes = %v", lead.Notes)
	}
}
