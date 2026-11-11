package security

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ali-ulu/lead/leadscout-go/internal/db"
)

func useTempDB(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	os.Setenv("LEADSCOUT_DB", filepath.Join(dir, "security.db"))
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

func TestPermissionEnabled(t *testing.T) {
	os.Unsetenv("LEADSCOUT_ALLOW_SEND")
	if !PermissionEnabled("LEADSCOUT_ALLOW_SEND", true) {
		t.Errorf("default should be honored")
	}
	if PermissionEnabled("LEADSCOUT_ALLOW_SEND", false) {
		t.Errorf("default false should be honored")
	}
	os.Setenv("LEADSCOUT_ALLOW_SEND", "1")
	defer os.Unsetenv("LEADSCOUT_ALLOW_SEND")
	if !PermissionEnabled("LEADSCOUT_ALLOW_SEND", false) {
		t.Errorf("explicit 1 should enable")
	}
}

func TestRequirePermission(t *testing.T) {
	os.Setenv("LEADSCOUT_ALLOW_SEND", "0")
	defer os.Unsetenv("LEADSCOUT_ALLOW_SEND")
	if err := RequirePermission("LEADSCOUT_ALLOW_SEND", "Sending", false); err == nil {
		t.Errorf("disabled permission should error")
	}
	os.Setenv("LEADSCOUT_ALLOW_SEND", "1")
	if err := RequirePermission("LEADSCOUT_ALLOW_SEND", "Sending", false); err != nil {
		t.Errorf("enabled permission should pass: %v", err)
	}
}

func TestAuditAgentActionRecordsEntry(t *testing.T) {
	useTempDB(t)
	if err := AuditAgentAction("send", "lead:1", "ok", map[string]any{"channel": "facebook"}, "agent"); err != nil {
		t.Fatal(err)
	}
	entries, err := ListAgentAudit(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	if entries[0]["action"] != "send" || entries[0]["actor"] != "agent" {
		t.Errorf("entry wrong: %v", entries[0])
	}
}

func TestListAgentAuditClampsLimit(t *testing.T) {
	useTempDB(t)
	if _, err := ListAgentAudit(0); err != nil {
		t.Errorf("limit 0 should clamp and not error: %v", err)
	}
	if _, err := ListAgentAudit(100000); err != nil {
		t.Errorf("large limit should clamp and not error: %v", err)
	}
}
