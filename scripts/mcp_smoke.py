#!/usr/bin/env python3
from __future__ import annotations

import asyncio
from pathlib import Path

from mcp import Client

from lead_hunter.db import clear_all, initialize, record_search_run, upsert_leads
from mcp_server import mcp


async def main() -> None:
    initialize()
    clear_all()
    ids = upsert_leads([
        {
            "source": "mcp-smoke",
            "source_id": "karachi-demo",
            "name": "Karachi MCP Demo",
            "country": "Pakistan",
            "city": "Karachi",
            "category": "beauty",
            "website_status": "missing",
            "social_links": {"instagram": "https://instagram.com/example"},
            "data_confidence": "medium",
            "opportunity_gap_score": 88,
        },
        {
            "source": "mcp-smoke",
            "source_id": "berlin-demo",
            "name": "Berlin MCP Demo",
            "country": "Germany",
            "city": "Berlin",
            "category": "dentist",
            "website_status": "missing",
            "social_links": {},
            "data_confidence": "medium",
        },
    ])
    search_id = record_search_run(
        country="Pakistan",
        city="Karachi",
        category="beauty",
        radius_km=20,
        lead_ids=[ids[0]],
    )

    async with Client(mcp, raise_exceptions=True) as client:
        result = await client.call_tool("capabilities", {})
        if result.is_error:
            raise SystemExit("capabilities tool returned an error")
        data = result.structured_content or {}
        if data.get("name") != "LeadScout":
            raise SystemExit(f"unexpected MCP identity: {data!r}")

        listed = await client.call_tool(
            "list_leads",
            {"search_id": search_id, "page_size": 5},
        )
        if listed.is_error:
            raise SystemExit("list_leads tool returned an error")
        payload = listed.structured_content or {}
        if payload.get("total") != 1:
            raise SystemExit(f"search_id isolation failed: {payload!r}")
        if payload["items"][0]["name"] != "Karachi MCP Demo":
            raise SystemExit(f"wrong search result: {payload!r}")

        gap_filtered = await client.call_tool(
            "list_leads",
            {"search_id": search_id, "min_opportunity_gap_score": 80, "page_size": 5},
        )
        if gap_filtered.is_error:
            raise SystemExit("visibility-filtered list_leads returned an error")
        gap_payload = gap_filtered.structured_content or {}
        if gap_payload.get("total") != 1:
            raise SystemExit(f"opportunity gap filter failed: {gap_payload!r}")

        reputation = await client.call_tool(
            "enrich_reputation_data",
            {"lead_id": ids[0]},
        )
        if reputation.is_error:
            raise SystemExit("enrich_reputation_data tool returned an error")
        rep_payload = reputation.structured_content or {}
        if rep_payload.get("reputation", {}).get("configured") is not False:
            raise SystemExit(f"unexpected reputation fallback: {rep_payload!r}")

        exported = await client.call_tool(
            "export_leads",
            {"format": "xlsx", "search_id": search_id},
        )
        if exported.is_error:
            raise SystemExit("export_leads tool returned an error")
        export_payload = exported.structured_content or {}
        if export_payload.get("count") != 1:
            raise SystemExit(f"scoped MCP export failed: {export_payload!r}")
        path = export_payload.get("path")
        if not path or not Path(path).is_file():
            raise SystemExit(f"MCP export file missing: {export_payload!r}")

        print("MCP PASS: search isolation, reputation tool, pagination and XLSX export")

    clear_all()


if __name__ == "__main__":
    asyncio.run(main())
