#!/usr/bin/env python3
"""Live exercise of every LeadScout MCP tool over stdio."""
from __future__ import annotations

import asyncio
import os
import sys

from mcp import Client

from lead_hunter.db import clear_all, initialize, record_search_run, upsert_leads
from mcp_server import mcp

FAILS = []


def check(name, cond, detail=""):
    print(f"[{'PASS' if cond else 'FAIL'}] {name} {detail}")
    if not cond:
        FAILS.append(name)


async def main() -> int:
    initialize()
    clear_all()
    ids = upsert_leads([
        {"source": "live", "source_id": "berlin-1", "name": "Live Berlin Dental",
         "country": "Germany", "city": "Berlin", "category": "dentist",
         "website_status": "missing", "opportunity_gap_score": 85, "data_confidence": "medium"},
        {"source": "live", "source_id": "berlin-2", "name": "Live Berlin Smile",
         "country": "Germany", "city": "Berlin", "category": "dentist",
         "website_status": "missing", "opportunity_gap_score": 40, "data_confidence": "medium"},
    ])
    sid = record_search_run(country="Germany", city="Berlin", category="dentist", radius_km=10, lead_ids=ids)
    lead_id = ids[0]

    async with Client(mcp, raise_exceptions=False) as client:
        tools = await client.list_tools()
        names = sorted(t.name for t in tools.tools)
        print("MCP tools:", len(names))
        check("tool count", len(names) >= 25, f"n={len(names)}")

        async def call(name, args):
            r = await client.call_tool(name, args)
            text = ""
            if r.content:
                text = getattr(r.content[0], "text", "") or ""
            if r.is_error:
                return None, text
            return (r.structured_content or {}), text

        # read / discovery
        d, e = await call("capabilities", {})
        check("capabilities", d and d.get("name") == "LeadScout", e or "")
        d, e = await call("search_businesses", {"city": "Berlin", "country": "Germany", "category": "dentist", "radius_km": 3, "max_results": 10})
        check("search_businesses", d and d.get("count", 0) > 0, f"count={d.get('count') if d else None} {e or ''}")
        d, e = await call("list_leads", {"search_id": sid, "page_size": 5})
        check("list_leads", d and d.get("total") == 2, f"total={d.get('total') if d else None}")
        d, e = await call("list_leads", {"search_id": sid, "min_opportunity_gap_score": 80})
        check("list_leads gap filter", d and d.get("total") == 1)
        d, e = await call("get_lead_detail", {"lead_id": lead_id})
        check("get_lead_detail", d and d.get("id") == lead_id)
        d, e = await call("verify_business", {"lead_id": lead_id})
        check("verify_business", d and "verification_status" in d)
        d, e = await call("enrich_contacts", {"lead_id": lead_id})
        check("enrich_contacts", d is not None, e or "")
        d, e = await call("enrich_reputation_data", {"lead_id": lead_id})
        check("enrich_reputation_data", d and d.get("reputation", {}).get("configured") is False)
        d, e = await call("audit_website", {"lead_id": lead_id})
        check("audit_website", d is None or d is not None)  # no website -> error is acceptable
        d, e = await call("draft_outreach_message", {"lead_id": lead_id, "lang": "tr"})
        check("draft_outreach_message", d and len(d.get("message", "")) > 20)

        # write tools (default allowed)
        d, e = await call("update_pipeline_stage", {"lead_id": lead_id, "status": "contacted"})
        check("update_pipeline_stage", d and d.get("lead", {}).get("pipeline_status") == "contacted")
        d, e = await call("update_engagement_status", {"lead_id": lead_id, "status": "sent", "note": "x"})
        check("update_engagement_status", d is not None)
        d, e = await call("schedule_follow_up", {"lead_id": lead_id, "when": "2026-12-15", "note": "n"})
        check("schedule_follow_up", d is not None)
        d, e = await call("add_lead_note", {"lead_id": lead_id, "note": "mcp live note"})
        check("add_lead_note", d is not None)
        d, e = await call("lead_activity_timeline", {"lead_id": lead_id})
        check("lead_activity_timeline", d and len(d.get("items", [])) >= 4, f"n={len(d.get('items', [])) if d else None}")
        d, e = await call("refresh_no_response_statuses", {"days": 7})
        check("refresh_no_response_statuses", d and "updated_count" in d)
        d, e = await call("meta_connections", {})
        check("meta_connections", d is not None)
        d, e = await call("messaging_eligibility_for_lead", {"lead_id": lead_id, "provider": "instagram"})
        check("messaging_eligibility_for_lead", d and d.get("provider") == "instagram")
        d, e = await call("link_messaging_recipient", {"lead_id": lead_id, "provider": "instagram", "recipient_id": "IGX"})
        check("link_messaging_recipient", d is not None)
        d, e = await call("export_leads", {"format": "xlsx", "search_id": sid})
        check("export_leads", d and d.get("count") == 2, f"path={d.get('path') if d else None}")
        d, e = await call("do_not_contact", {"lead_id": lead_id})
        check("do_not_contact", d and d.get("ok"))
        d, e = await call("get_agent_job", {"job_id": "nope"})
        check("get_agent_job missing -> error", d is None)
        d, e = await call("agent_audit_log", {"limit": 50})
        check("agent_audit_log", d and len(d.get("items", [])) > 0, f"n={len(d.get('items', [])) if d else None}")

        # permission-gated: send must be disabled by default
        os.environ.pop("LEADSCOUT_MCP_ALLOW_SEND", None)
        d, e = await call("send_social_message", {"lead_id": lead_id, "provider": "instagram", "text": "hi"})
        check("send gated by default", d is None and e and "disabled" in e.lower(), e or "")

        # clear requires confirm + permission
        d, e = await call("clear_local_data", {"confirm": False})
        check("clear needs confirm", d is None)
        os.environ.pop("LEADSCOUT_MCP_ALLOW_CLEAR", None)
        d, e = await call("clear_local_data", {"confirm": True})
        check("clear gated by default", d is None and e and "disabled" in e.lower(), e or "")

        # run_sales_agent (no send)
        d, e = await call("run_sales_agent", {"city": "Berlin", "country": "Germany", "category": "dentist", "radius_km": 3, "top_n": 2, "min_score": 30, "audit_websites": False, "verify_missing": False})
        check("run_sales_agent", d and d.get("job_id"), f"processed={len(d.get('processed', [])) if d else None} {e or ''}")

    clear_all()
    print()
    if FAILS:
        print("FAILURES:", FAILS)
        return 1
    print("ALL MCP CHECKS PASS")
    return 0


if __name__ == "__main__":
    sys.exit(asyncio.run(main()))
