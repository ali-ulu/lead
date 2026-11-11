package agent

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
	os.Setenv("LEADSCOUT_DB", filepath.Join(dir, "agent.db"))
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

func TestJobRoundTrip(t *testing.T) {
	useTempDB(t)
	request := map[string]any{"city": "Istanbul", "category": "dentist"}
	if err := jobWriteState("job-1", "running", request, nil, ""); err != nil {
		t.Fatal(err)
	}
	job, err := GetJob("job-1")
	if err != nil {
		t.Fatal(err)
	}
	if job["status"] != "running" {
		t.Errorf("status = %v, want running", job["status"])
	}

	result := map[string]any{"candidate_count": 3}
	if err := jobWriteState("job-1", "completed", nil, result, ""); err != nil {
		t.Fatal(err)
	}
	job, _ = GetJob("job-1")
	if job["status"] != "completed" {
		t.Errorf("status = %v, want completed", job["status"])
	}
	res, _ := job["result"].(map[string]any)
	if res["candidate_count"] != float64(3) && res["candidate_count"] != 3 {
		t.Errorf("result = %v", job["result"])
	}
}

func TestGetJobMissingReturnsNil(t *testing.T) {
	useTempDB(t)
	job, err := GetJob("does-not-exist")
	if err != nil {
		t.Errorf("missing job should not error: %v", err)
	}
	if job != nil {
		t.Errorf("missing job should be nil, got %v", job)
	}
}

func TestVisibilityValue(t *testing.T) {
	lead := &model.Lead{
		SEOScore:          model.Int(70),
		AEOScore:          model.Int(60),
		GEOScore:          model.Int(50),
		AIVisibilityScore: model.Int(40),
	}
	cases := map[string]int{"seo_score": 70, "aeo_score": 60, "geo_score": 50, "ai_visibility_score": 40}
	for field, want := range cases {
		got := visibilityValue(lead, field)
		if got == nil || *got != want {
			t.Errorf("visibilityValue(%q) = %v, want %d", field, got, want)
		}
	}
	if visibilityValue(lead, "unknown") != nil {
		t.Errorf("unknown field should return nil")
	}
}
