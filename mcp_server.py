from __future__ import annotations

import argparse
from pathlib import Path
from typing import Any

from mcp.server.mcpserver import MCPServer

from lead_hunter.db import clear_all, get_lead, initialize
from lead_hunter.exporters import csv_bytes, xlsx_bytes
from lead_hunter.providers.osm import CATEGORY_FILTERS
from lead_hunter.services import (
    audit_lead,
    discover_businesses,
    draft_outreach,
    mark_do_not_contact,
    query_leads,
    set_pipeline_stage,
)

mcp = MCPServer("LeadScout")


@mcp.tool()
def capabilities() -> dict[str, Any]:
    """Describe LeadScout capabilities and supported categories/languages."""
    initialize()
    return {
        "name": "LeadScout",
        "version": "4.0.0",
        "search": {
            "global": True,
            "application_result_cap": None,
            "radius_km": {"min": 1, "max": 100},
            "categories": sorted(CATEGORY_FILTERS.keys()),
        },
        "languages": ["en", "tr", "ur", "sd", "de"],
        "social_channels": [
            "instagram", "facebook", "linkedin", "x",
            "youtube", "tiktok", "telegram", "whatsapp",
        ],
        "exports": ["csv", "xlsx"],
        "pipeline_stages": [
            "new", "reviewed", "contacted", "replied",
            "proposal", "won", "lost",
        ],
    }


@mcp.tool()
def search_businesses(
    city: str,
    category: str,
    country: str = "",
    radius_km: int = 20,
    max_results: int | None = None,
) -> dict[str, Any]:
    """Search live business data and store discovered leads locally.

    Omit max_results for no application-level result cap. Use list_leads to read
    large result sets in pages after this tool returns the matching lead IDs.
    """
    initialize()
    return discover_businesses(
        city=city,
        country=country,
        category=category,
        radius_km=radius_km,
        max_results=max_results,
    )


@mcp.tool()
def list_leads(
    ids: list[int] | None = None,
    country: str = "",
    city: str = "",
    category: str = "",
    website_status: str = "",
    pipeline_status: str = "",
    min_score: int = 0,
    has_social: bool = False,
    offset: int = 0,
    page_size: int = 100,
) -> dict[str, Any]:
    """List/filter stored leads with pagination for agent-friendly responses."""
    initialize()
    rows = query_leads(
        ids=ids,
        country=country,
        city=city,
        category=category,
        website_status=website_status,
        pipeline_status=pipeline_status,
        min_score=min_score,
        has_social=has_social,
    )
    offset = max(0, int(offset))
    page_size = max(1, min(500, int(page_size)))
    page = rows[offset : offset + page_size]
    return {
        "total": len(rows),
        "offset": offset,
        "page_size": page_size,
        "next_offset": offset + len(page) if offset + len(page) < len(rows) else None,
        "items": page,
    }


@mcp.tool()
def get_lead_detail(lead_id: int) -> dict[str, Any]:
    """Get the complete stored record for one lead."""
    initialize()
    lead = get_lead(int(lead_id))
    if not lead:
        raise ValueError("Lead not found.")
    return lead


@mcp.tool()
def audit_website(lead_id: int) -> dict[str, Any]:
    """Audit a lead's public website and enrich social-media links."""
    initialize()
    return audit_lead(int(lead_id))


@mcp.tool()
def draft_outreach_message(lead_id: int, lang: str = "en") -> dict[str, Any]:
    """Draft a human-review outreach message in EN/TR/UR/SD/DE."""
    initialize()
    return draft_outreach(int(lead_id), lang)


@mcp.tool()
def update_pipeline_stage(lead_id: int, status: str) -> dict[str, Any]:
    """Move a lead to a pipeline stage."""
    initialize()
    return set_pipeline_stage(int(lead_id), status)


@mcp.tool()
def do_not_contact(lead_id: int) -> dict[str, Any]:
    """Mark a lead do-not-contact so normal lead lists hide it."""
    initialize()
    return mark_do_not_contact(int(lead_id))


@mcp.tool()
def export_leads(
    format: str = "xlsx",
    ids: list[int] | None = None,
    country: str = "",
    city: str = "",
    category: str = "",
    website_status: str = "",
    pipeline_status: str = "",
    min_score: int = 0,
    has_social: bool = False,
) -> dict[str, Any]:
    """Export filtered leads to a local CSV or Excel XLSX file and return its path."""
    initialize()
    rows = query_leads(
        ids=ids,
        country=country,
        city=city,
        category=category,
        website_status=website_status,
        pipeline_status=pipeline_status,
        min_score=min_score,
        has_social=has_social,
    )
    fmt = format.lower().strip()
    if fmt not in {"csv", "xlsx"}:
        raise ValueError("format must be csv or xlsx")

    out_dir = Path(__file__).resolve().parent / "exports"
    out_dir.mkdir(parents=True, exist_ok=True)
    path = out_dir / f"leadscout-leads.{fmt}"
    path.write_bytes(xlsx_bytes(rows) if fmt == "xlsx" else csv_bytes(rows))
    return {"format": fmt, "count": len(rows), "path": str(path)}


@mcp.tool()
def clear_local_data() -> dict[str, Any]:
    """Clear locally cached leads and pipeline data."""
    initialize()
    clear_all()
    return {"ok": True}


def main() -> None:
    parser = argparse.ArgumentParser(description="LeadScout MCP server")
    parser.add_argument(
        "--transport",
        choices=["stdio", "streamable-http"],
        default="stdio",
    )
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=8790)
    args = parser.parse_args()

    initialize()
    if args.transport == "streamable-http":
        mcp.run(
            transport="streamable-http",
            host=args.host,
            port=args.port,
            stateless_http=True,
            json_response=True,
        )
    else:
        mcp.run(transport="stdio")


if __name__ == "__main__":
    main()
