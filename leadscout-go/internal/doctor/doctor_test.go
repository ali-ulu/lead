package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func useTempDB(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	os.Setenv("LEADSCOUT_DB", filepath.Join(dir, "doctor.db"))
	if os.Getenv("LEADSCOUT_SCHEMA") == "" {
		candidate := filepath.Join("..", "..", "..", "sql", "schema.sql")
		if _, err := os.Stat(candidate); err == nil {
			abs, _ := filepath.Abs(candidate)
			os.Setenv("LEADSCOUT_SCHEMA", abs)
		}
	}
}

func TestReportShapeAndDatabaseCheck(t *testing.T) {
	useTempDB(t)
	report := RunChecks()
	for _, key := range []string{"runtime", "node", "database", "providers", "config", "categories", "problems"} {
		if _, ok := report[key]; !ok {
			t.Errorf("report missing %q", key)
		}
	}
	if report["runtime"] != "go" {
		t.Errorf("runtime = %v", report["runtime"])
	}
	database := report["database"].(map[string]any)
	if writable, _ := database["writable"].(bool); !writable {
		t.Errorf("database should be writable")
	}
	tables, _ := database["tables"].([]string)
	found := false
	for _, name := range tables {
		if name == "leads" {
			found = true
		}
	}
	if !found {
		t.Errorf("leads table missing: %v", tables)
	}
	if database["lead_count"] != 0 {
		t.Errorf("lead_count = %v, want 0", database["lead_count"])
	}
}

func TestOvertureReportedAsDisabledProblem(t *testing.T) {
	useTempDB(t)
	report := RunChecks()
	providers := report["providers"].(map[string]any)
	if ok, _ := providers["overture"].(map[string]any)["ok"].(bool); ok {
		t.Errorf("overture should be reported as unavailable in the Go build")
	}
	problems, _ := report["problems"].([]string)
	if !anyContains(problems, "Overture") {
		t.Errorf("problems should mention Overture: %v", problems)
	}
}

func TestWebSearchOffIsReported(t *testing.T) {
	useTempDB(t)
	os.Setenv("LEADSCOUT_WEB_SEARCH", "off")
	defer os.Unsetenv("LEADSCOUT_WEB_SEARCH")
	report := RunChecks()
	providers := report["providers"].(map[string]any)
	if ok, _ := providers["web_search"].(map[string]any)["ok"].(bool); ok {
		t.Errorf("web_search should be off")
	}
	problems, _ := report["problems"].([]string)
	if !anyContains(problems, "Web verification") {
		t.Errorf("problems should mention web verification: %v", problems)
	}
}

func TestFormatReportRendersSections(t *testing.T) {
	useTempDB(t)
	text := FormatReport(RunChecks())
	for _, section := range []string{"Database:", "Providers:", "Config:"} {
		if !strings.Contains(text, section) {
			t.Errorf("report missing %q", section)
		}
	}
}

func anyContains(items []string, needle string) bool {
	for _, item := range items {
		if strings.Contains(item, needle) {
			return true
		}
	}
	return false
}
