package db

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ali-ulu/lead/leadscout-go/internal/model"
)

// testSchema points LEADSCOUT_SCHEMA at the repo schema when running from the
// package directory inside leadscout-go.
func useTempDB(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	os.Setenv("LEADSCOUT_DB", filepath.Join(dir, "test.db"))
	if os.Getenv("LEADSCOUT_SCHEMA") == "" {
		for _, candidate := range []string{"schema.sql", filepath.Join("..", "..", "..", "sql", "schema.sql")} {
			if _, err := os.Stat(candidate); err == nil {
				abs, _ := filepath.Abs(candidate)
				os.Setenv("LEADSCOUT_SCHEMA", abs)
				break
			}
		}
	}
	if err := Initialize(); err != nil {
		t.Fatalf("initialize: %v", err)
	}
}

func discovered() *model.Lead {
	return &model.Lead{
		Source: "test", SourceID: "karachi-1", Name: "Karachi One",
		Country: "Pakistan", City: "Karachi", Category: "dentist",
		Website: model.Str("https://example.com/"), Phone: model.Str("+905000000001"),
		WebsiteStatus: "unknown", VerificationStatus: "unverified",
	}
}

func leadBySourceID(t *testing.T, sourceID string) *model.Lead {
	t.Helper()
	conn, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	row := conn.QueryRow("SELECT "+leadColumns+" FROM leads WHERE source_id=?", sourceID)
	lead, err := scanLead(row)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	return lead
}

func patch(t *testing.T, sqlText string) {
	t.Helper()
	conn, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Exec(sqlText); err != nil {
		t.Fatalf("patch %q: %v", sqlText, err)
	}
}

func TestRediscoveryKeepsVisibilityScores(t *testing.T) {
	useTempDB(t)
	if _, err := UpsertLeads([]*model.Lead{discovered()}); err != nil {
		t.Fatal(err)
	}
	patch(t, "UPDATE leads SET seo_score=88, aeo_score=90, geo_score=82, ai_visibility_score=86, opportunity_gap_score=24 WHERE source_id='karachi-1'")
	if _, err := UpsertLeads([]*model.Lead{discovered()}); err != nil {
		t.Fatal(err)
	}
	row := leadBySourceID(t, "karachi-1")
	if *row.SEOScore != 88 || *row.AEOScore != 90 || *row.GEOScore != 82 || *row.AIVisibilityScore != 86 || *row.OpportunityGapScore != 24 {
		t.Errorf("scores changed: %+v", row)
	}
}

func TestRediscoveryKeepsVerificationAndReputation(t *testing.T) {
	useTempDB(t)
	UpsertLeads([]*model.Lead{discovered()})
	patch(t, "UPDATE leads SET verification_status='verified', rating=4.6, review_count=210, audit_engine='heuristic', has_https=1 WHERE source_id='karachi-1'")
	UpsertLeads([]*model.Lead{discovered()})
	row := leadBySourceID(t, "karachi-1")
	if row.VerificationStatus != "verified" {
		t.Errorf("verification = %q", row.VerificationStatus)
	}
	if row.Rating == nil || *row.Rating != 4.6 {
		t.Errorf("rating = %v", row.Rating)
	}
	if row.ReviewCount == nil || *row.ReviewCount != 210 {
		t.Errorf("review_count = %v", row.ReviewCount)
	}
	if row.AuditEngine == nil || *row.AuditEngine != "heuristic" {
		t.Errorf("audit_engine = %v", row.AuditEngine)
	}
	if row.HasHTTPS == nil || !*row.HasHTTPS {
		t.Errorf("has_https = %v", row.HasHTTPS)
	}
}

func TestRediscoveryDoesNotDowngradeWebsiteStatus(t *testing.T) {
	useTempDB(t)
	UpsertLeads([]*model.Lead{discovered()})
	patch(t, "UPDATE leads SET website_status='weak' WHERE source_id='karachi-1'")
	incoming := discovered()
	incoming.WebsiteStatus = "missing"
	UpsertLeads([]*model.Lead{incoming})
	if got := leadBySourceID(t, "karachi-1").WebsiteStatus; got != "weak" {
		t.Errorf("website_status = %q, want weak", got)
	}
}

func TestMissingDecisionNotReverted(t *testing.T) {
	useTempDB(t)
	nosite := discovered()
	nosite.Website = nil
	nosite.WebsiteStatus = "missing"
	UpsertLeads([]*model.Lead{nosite})
	if got := leadBySourceID(t, "karachi-1").WebsiteStatus; got != "missing" {
		t.Fatalf("initial status = %q", got)
	}
	again := *nosite
	again.WebsiteStatus = "unknown"
	UpsertLeads([]*model.Lead{&again})
	if got := leadBySourceID(t, "karachi-1").WebsiteStatus; got != "missing" {
		t.Errorf("status = %q, want missing", got)
	}
}

func TestLowIncomingScoreDoesNotOverwriteHigh(t *testing.T) {
	useTempDB(t)
	UpsertLeads([]*model.Lead{discovered()})
	patch(t, "UPDATE leads SET seo_score=95, ai_visibility_score=93 WHERE source_id='karachi-1'")
	incoming := discovered()
	incoming.SEOScore = model.Int(5)
	incoming.AIVisibilityScore = model.Int(3)
	UpsertLeads([]*model.Lead{incoming})
	row := leadBySourceID(t, "karachi-1")
	if *row.SEOScore != 95 || *row.AIVisibilityScore != 93 {
		t.Errorf("scores changed: seo=%d aio=%d", *row.SEOScore, *row.AIVisibilityScore)
	}
}

func TestCRMFieldsSurviveRediscovery(t *testing.T) {
	useTempDB(t)
	UpsertLeads([]*model.Lead{discovered()})
	patch(t, "UPDATE leads SET pipeline_status='proposal', engagement_status='replied', notes='Discussed by phone', follow_up_at='2026-12-01' WHERE source_id='karachi-1'")
	incoming := discovered()
	incoming.PipelineStatus = "new"
	incoming.EngagementStatus = "not_contacted"
	incoming.Notes = model.Str("")
	UpsertLeads([]*model.Lead{incoming})
	row := leadBySourceID(t, "karachi-1")
	if row.PipelineStatus != "proposal" {
		t.Errorf("pipeline = %q", row.PipelineStatus)
	}
	if row.EngagementStatus != "replied" {
		t.Errorf("engagement = %q", row.EngagementStatus)
	}
	if row.Notes == nil || *row.Notes != "Discussed by phone" {
		t.Errorf("notes = %v", row.Notes)
	}
	if row.FollowUpAt == nil || *row.FollowUpAt != "2026-12-01" {
		t.Errorf("follow_up = %v", row.FollowUpAt)
	}
}

func TestIdentityFieldsStillUpdate(t *testing.T) {
	useTempDB(t)
	UpsertLeads([]*model.Lead{discovered()})
	patch(t, "UPDATE leads SET seo_score=10, email='old@example.com' WHERE source_id='karachi-1'")
	incoming := discovered()
	incoming.Phone = model.Str("+905555555555")
	incoming.Email = model.Str("new@example.com")
	incoming.Name = "Karachi One Renamed"
	UpsertLeads([]*model.Lead{incoming})
	row := leadBySourceID(t, "karachi-1")
	if *row.Phone != "+905555555555" {
		t.Errorf("phone = %q", *row.Phone)
	}
	if *row.Email != "new@example.com" {
		t.Errorf("email = %q", *row.Email)
	}
	if row.Name != "Karachi One Renamed" {
		t.Errorf("name = %q", row.Name)
	}
	if *row.SEOScore != 10 {
		t.Errorf("seo = %d", *row.SEOScore)
	}
}

func TestRediscoveryKeepsEnrichedEmailAndPhone(t *testing.T) {
	useTempDB(t)
	UpsertLeads([]*model.Lead{discovered()})
	patch(t, "UPDATE leads SET email='found@example.com', phone='+491234567' WHERE source_id='karachi-1'")
	incoming := discovered()
	incoming.Email = nil
	incoming.Phone = nil
	UpsertLeads([]*model.Lead{incoming})
	row := leadBySourceID(t, "karachi-1")
	if row.Email == nil || *row.Email != "found@example.com" {
		t.Errorf("email = %v", row.Email)
	}
	if row.Phone == nil || *row.Phone != "+491234567" {
		t.Errorf("phone = %v", row.Phone)
	}
}

func TestRediscoveryKeepsCoordinatesWhenSourceOmits(t *testing.T) {
	useTempDB(t)
	UpsertLeads([]*model.Lead{discovered()})
	patch(t, "UPDATE leads SET latitude=52.52, longitude=13.405 WHERE source_id='karachi-1'")
	incoming := discovered()
	incoming.Latitude = nil
	incoming.Longitude = nil
	UpsertLeads([]*model.Lead{incoming})
	row := leadBySourceID(t, "karachi-1")
	if row.Latitude == nil || *row.Latitude != 52.52 || row.Longitude == nil || *row.Longitude != 13.405 {
		t.Errorf("coords = %v,%v", row.Latitude, row.Longitude)
	}
}

func TestDiscoveryStillFillsEmptyContactFields(t *testing.T) {
	useTempDB(t)
	UpsertLeads([]*model.Lead{discovered()})
	patch(t, "UPDATE leads SET email=NULL, phone=NULL WHERE source_id='karachi-1'")
	incoming := discovered()
	incoming.Email = model.Str("new@example.com")
	incoming.Phone = model.Str("+905000000009")
	UpsertLeads([]*model.Lead{incoming})
	row := leadBySourceID(t, "karachi-1")
	if row.Email == nil || *row.Email != "new@example.com" {
		t.Errorf("email = %v", row.Email)
	}
	if row.Phone == nil || *row.Phone != "+905000000009" {
		t.Errorf("phone = %v", row.Phone)
	}
}

func TestMissingCategoryIsClearedOnRediscovery(t *testing.T) {
	useTempDB(t)
	UpsertLeads([]*model.Lead{discovered()})
	patch(t, "UPDATE leads SET category='wrong_category' WHERE source_id='karachi-1'")
	incoming := discovered()
	incoming.Category = ""
	UpsertLeads([]*model.Lead{incoming})
	if got := leadBySourceID(t, "karachi-1").Category; got != "" {
		t.Errorf("category = %q, want empty", got)
	}
}
