package mcp

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pythonTools is the tool surface exported by the Python mcp_server.py build.
var pythonTools = []string{
	"capabilities", "search_businesses", "list_leads", "get_lead_detail", "verify_business",
	"enrich_contacts", "enrich_reputation_data", "audit_website", "measure_ai_presence",
	"draft_outreach_message", "update_pipeline_stage", "update_engagement_status",
	"schedule_follow_up", "add_lead_note", "lead_activity_timeline", "refresh_no_response_statuses",
	"meta_connections", "messaging_eligibility_for_lead", "link_messaging_recipient",
	"send_social_message", "run_sales_agent", "get_agent_job", "do_not_contact",
	"export_leads", "clear_local_data", "agent_audit_log",
}

func useTempDB(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	os.Setenv("LEADSCOUT_DB", filepath.Join(dir, "mcp.db"))
	os.Setenv("LEADSCOUT_ROOT", dir)
	if os.Getenv("LEADSCOUT_SCHEMA") == "" {
		candidate := filepath.Join("..", "..", "..", "sql", "schema.sql")
		if _, err := os.Stat(candidate); err == nil {
			abs, _ := filepath.Abs(candidate)
			os.Setenv("LEADSCOUT_SCHEMA", abs)
		}
	}
}

func TestToolSurfaceMatchesPython(t *testing.T) {
	s := New()
	got := map[string]bool{}
	for _, tool := range s.Tools() {
		got[tool.Name] = true
		if tool.Description == "" {
			t.Errorf("tool %q has no description", tool.Name)
		}
		if tool.InputSchema["type"] != "object" {
			t.Errorf("tool %q schema is not an object", tool.Name)
		}
	}
	if len(s.Tools()) != len(pythonTools) {
		t.Errorf("tool count = %d, want %d", len(s.Tools()), len(pythonTools))
	}
	for _, name := range pythonTools {
		if !got[name] {
			t.Errorf("missing tool %q", name)
		}
	}
}

func TestInitializeAndToolsList(t *testing.T) {
	s := New()
	init := call(t, s, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`)
	result, _ := init["result"].(map[string]any)
	if result["protocolVersion"] != "2025-06-18" {
		t.Errorf("protocolVersion = %v", result["protocolVersion"])
	}
	serverInfo, _ := result["serverInfo"].(map[string]any)
	if serverInfo["name"] != "LeadScout" {
		t.Errorf("serverInfo = %v", serverInfo)
	}

	list := call(t, s, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	listResult, _ := list["result"].(map[string]any)
	tools, _ := listResult["tools"].([]map[string]any)
	if len(tools) != len(pythonTools) {
		t.Errorf("tools/list returned %d tools", len(tools))
	}
}

func TestUnknownMethodAndTool(t *testing.T) {
	s := New()
	resp := call(t, s, `{"jsonrpc":"2.0","id":3,"method":"does/not/exist"}`)
	if resp["error"] == nil {
		t.Errorf("unknown method should error")
	}
	resp = call(t, s, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"nope"}}`)
	errObj, _ := resp["error"].(map[string]any)
	if errObj == nil || !strings.Contains(errObj["message"].(string), "unknown tool") {
		t.Errorf("unknown tool should error: %v", resp)
	}
}

func TestCapabilitiesTool(t *testing.T) {
	useTempDB(t)
	s := New()
	resp := call(t, s, `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"capabilities"}}`)
	result, _ := resp["result"].(map[string]any)
	if isErr, _ := result["isError"].(bool); isErr {
		t.Fatalf("capabilities errored: %v", result)
	}
	content, _ := result["content"].([]map[string]any)
	if len(content) == 0 {
		t.Fatalf("no content returned")
	}
	text := content[0]["text"].(string)
	if !strings.Contains(text, "LeadScout") || !strings.Contains(text, "ai_visibility") {
		t.Errorf("capabilities payload = %s", text)
	}
}

func TestClearLocalDataRequiresConfirm(t *testing.T) {
	useTempDB(t)
	s := New()
	resp := call(t, s, `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"clear_local_data","arguments":{}}}`)
	result, _ := resp["result"].(map[string]any)
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Errorf("clear without confirm should report an error")
	}
}

func TestSendPermissionGate(t *testing.T) {
	useTempDB(t)
	os.Unsetenv("LEADSCOUT_MCP_ALLOW_SEND")
	s := New()
	resp := call(t, s, `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"send_social_message","arguments":{"lead_id":1,"provider":"instagram","text":"hi"}}}`)
	result, _ := resp["result"].(map[string]any)
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Errorf("send should be blocked by default permission")
	}
}

func TestServeStdioHandlesNotifications(t *testing.T) {
	useTempDB(t)
	s := New()
	in := strings.NewReader(
		`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" +
			`{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n")
	var out bytes.Buffer
	if err := s.ServeStdio(in, &out); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected exactly one response line, got %d: %q", len(lines), out.String())
	}
	if !strings.Contains(lines[0], `"id":1`) {
		t.Errorf("response = %s", lines[0])
	}
}

func call(t *testing.T, s *Server, payload string) map[string]any {
	t.Helper()
	var req rpcRequest
	if err := json.Unmarshal([]byte(payload), &req); err != nil {
		t.Fatalf("bad payload: %v", err)
	}
	result, rpcErr := s.handle(req)
	out := map[string]any{}
	if rpcErr != nil {
		out["error"] = map[string]any{"code": rpcErr.Code, "message": rpcErr.Message}
	} else {
		out["result"] = result
	}
	return out
}
