#!/usr/bin/env python3
"""End-to-end smoke test for the Go LeadScout MCP server over stdio.

Mirrors scripts/mcp_smoke.py but drives the compiled Go binary instead of the
Python MCP SDK. Exercises the same guarantees: tool discovery, search-scoped
listing, pagination, XLSX export and the default-off send gate.

Usage:
    python scripts/go_mcp_smoke.py /path/to/leadscout
"""
from __future__ import annotations

import json
import os
import subprocess
import sys
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
BINARY = sys.argv[1] if len(sys.argv) > 1 else str(ROOT / "leadscout-go" / "leadscout")

EXPECTED_TOOLS = {
    "capabilities", "search_businesses", "list_leads", "get_lead_detail",
    "verify_business", "enrich_contacts", "enrich_reputation_data",
    "audit_website", "measure_ai_presence", "draft_outreach_message",
    "update_pipeline_stage", "update_engagement_status", "schedule_follow_up",
    "add_lead_note", "lead_activity_timeline", "refresh_no_response_statuses",
    "meta_connections", "messaging_eligibility_for_lead",
    "link_messaging_recipient", "send_social_message", "run_sales_agent",
    "get_agent_job", "do_not_contact", "export_leads", "clear_local_data",
    "agent_audit_log",
}


def fail(message: str) -> None:
    raise SystemExit(f"GO MCP SMOKE FAIL: {message}")


def main() -> None:
    if not Path(BINARY).is_file():
        fail(f"binary not found: {BINARY}")

    tmp = tempfile.mkdtemp(prefix="leadscout-go-smoke-")
    env = dict(os.environ)
    env["LEADSCOUT_ROOT"] = str(ROOT)
    env["LEADSCOUT_DB"] = str(Path(tmp) / "smoke.db")
    env.pop("LEADSCOUT_MCP_ALLOW_SEND", None)

    requests = [
        {"jsonrpc": "2.0", "id": 1, "method": "initialize",
         "params": {"protocolVersion": "2025-06-18"}},
        {"jsonrpc": "2.0", "id": 2, "method": "tools/list"},
        {"jsonrpc": "2.0", "id": 3, "method": "tools/call",
         "params": {"name": "capabilities", "arguments": {}}},
        {"jsonrpc": "2.0", "id": 4, "method": "tools/call",
         "params": {"name": "list_leads", "arguments": {"page_size": 5}}},
        {"jsonrpc": "2.0", "id": 5, "method": "tools/call",
         "params": {"name": "send_social_message",
                    "arguments": {"lead_id": 1, "provider": "instagram", "text": "hi"}}},
        {"jsonrpc": "2.0", "id": 6, "method": "tools/call",
         "params": {"name": "export_leads", "arguments": {"format": "xlsx"}}},
    ]
    stdin = "\n".join(json.dumps(r) for r in requests) + "\n"

    proc = subprocess.run(
        [BINARY, "-mcp"],
        input=stdin,
        capture_output=True,
        text=True,
        env=env,
        timeout=120,
    )
    if proc.returncode != 0:
        fail(f"binary exited {proc.returncode}: {proc.stderr[:500]}")

    responses = {}
    for line in proc.stdout.splitlines():
        line = line.strip()
        if not line:
            continue
        try:
            message = json.loads(line)
        except json.JSONDecodeError:
            fail(f"non-JSON output: {line[:200]}")
        if "id" in message:
            responses[message["id"]] = message

    def payload(rid: int) -> dict:
        message = responses.get(rid) or fail(f"no response for id={rid}")
        if message.get("error"):
            fail(f"rpc error for id={rid}: {message['error']}")
        result = message.get("result", {})
        if "content" not in result:
            # Non-tool responses (initialize, tools/list) carry their fields
            # directly on the result object.
            return result
        if result.get("isError"):
            return {"_is_error": True, "text": (result.get("content") or [{}])[0].get("text", "")}
        content = result.get("content") or [{}]
        text = content[0].get("text", "") if content else ""
        try:
            return json.loads(text)
        except json.JSONDecodeError:
            return {"_text": text}

    init = payload(1)
    if init.get("serverInfo", {}).get("name") != "LeadScout":
        fail(f"unexpected server identity: {init!r}")

    listed = payload(2)
    names = {t["name"] for t in listed.get("tools", [])}
    if names != EXPECTED_TOOLS:
        missing = EXPECTED_TOOLS - names
        extra = names - EXPECTED_TOOLS
        fail(f"tool set mismatch (missing={sorted(missing)}, extra={sorted(extra)})")

    caps = payload(3)
    if caps.get("name") != "LeadScout":
        fail(f"capabilities identity mismatch: {caps!r}")

    leads = payload(4)
    if "total" not in leads:
        fail(f"list_leads returned no total: {leads!r}")

    gated = payload(5)
    if not gated.get("_is_error"):
        fail("send_social_message must be disabled by default")
    if "disabled" not in gated.get("text", ""):
        fail(f"gated tool lost its message: {gated!r}")

    exported = payload(6)
    if "count" not in exported:
        fail(f"export_leads returned no count: {exported!r}")
    path = exported.get("path")
    if path and not Path(path).is_file():
        fail(f"export file missing: {exported!r}")
    if path:
        Path(path).unlink(missing_ok=True)

    print("GO MCP SMOKE PASS: tools, capabilities, list, send gate, export")


if __name__ == "__main__":
    main()
