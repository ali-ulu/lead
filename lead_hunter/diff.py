"""Run-to-run comparison for scheduled scans.

The value of a nightly scan is not the first result set, it is what changed:
businesses that appeared, websites that were gained or lost, and new
high-opportunity leads worth contacting. This module compares two stored search
runs and produces a machine- and human-readable report.
"""
from __future__ import annotations

from typing import Any

import lead_hunter.db as db

OPPORTUNITY_SCORE = 70


def _contactable(lead: dict[str, Any]) -> bool:
    return bool(
        str(lead.get("phone") or "").strip()
        or str(lead.get("email") or "").strip()
        or lead.get("social_links")
    )


def _is_high_opportunity(lead: dict[str, Any], *, min_score: int) -> bool:
    return (
        int(lead.get("lead_score") or 0) >= min_score
        and str(lead.get("website_status") or "") == "missing"
        and bool(str(lead.get("phone") or "").strip())
    )


def _brief(lead: dict[str, Any]) -> dict[str, Any]:
    return {
        "id": lead.get("id"),
        "name": lead.get("name"),
        "category": lead.get("category"),
        "city": lead.get("city"),
        "lead_score": lead.get("lead_score"),
        "website_status": lead.get("website_status"),
        "phone": lead.get("phone"),
        "email": lead.get("email"),
    }


def compare_runs(
    previous_search_id: str,
    current_search_id: str,
    *,
    min_score: int = OPPORTUNITY_SCORE,
) -> dict[str, Any]:
    if not previous_search_id or not current_search_id:
        raise ValueError("Both previous and current search ids are required.")

    previous_ids = set(db.lead_ids_for_search(previous_search_id))
    current_ids = set(db.lead_ids_for_search(current_search_id))
    previous_snap = db.snapshots_for_search(previous_search_id)
    current_snap = db.snapshots_for_search(current_search_id)
    previous = {lead["id"]: lead for lead in db.leads_by_ids(list(previous_ids))}
    current = {lead["id"]: lead for lead in db.leads_by_ids(list(current_ids))}

    new_ids = current_ids - previous_ids
    gone_ids = previous_ids - current_ids
    common_ids = current_ids & previous_ids

    website_gained = []
    website_lost = []
    for lead_id in common_ids:
        before = str((previous_snap.get(lead_id) or {}).get("website_status") or "")
        after = str((current_snap.get(lead_id) or {}).get("website_status") or "")
        if before in {"missing", "dead"} and after in {"unknown", "healthy", "weak"}:
            website_gained.append(current[lead_id])
        elif before in {"unknown", "healthy", "weak"} and after in {"missing", "dead"}:
            website_lost.append(current[lead_id])

    alerts = []
    for lead_id in sorted(new_ids):
        lead = current.get(lead_id)
        if not lead:
            continue
        snap = current_snap.get(lead_id) or {}
        merged = {**lead, **{k: v for k, v in snap.items() if v is not None}}
        if _is_high_opportunity(merged, min_score=min_score):
            alerts.append(lead)

    report = {
        "previous_search_id": previous_search_id,
        "current_search_id": current_search_id,
        "previous_count": len(previous_ids),
        "current_count": len(current_ids),
        "new_count": len(new_ids),
        "gone_count": len(gone_ids),
        "website_gained_count": len(website_gained),
        "website_lost_count": len(website_lost),
        "alert_count": len(alerts),
        "new": [_brief(current[i]) for i in sorted(new_ids) if i in current],
        "gone": [_brief(previous[i]) for i in sorted(gone_ids) if i in previous],
        "website_gained": [_brief(x) for x in website_gained],
        "website_lost": [_brief(x) for x in website_lost],
        "alerts": [_brief(x) for x in alerts],
    }
    return report


def format_report(report: dict[str, Any]) -> str:
    lines = [
        f"Run diff: {report['previous_search_id'][:8]} -> {report['current_search_id'][:8]}",
        f"  leads: {report['previous_count']} -> {report['current_count']}",
        f"  new: {report['new_count']}  gone: {report['gone_count']}",
        f"  websites gained: {report['website_gained_count']}  lost: {report['website_lost_count']}",
        f"  high-opportunity alerts: {report['alert_count']}",
    ]
    for label, key in (
        ("New businesses", "new"),
        ("No longer seen", "gone"),
        ("Websites gained", "website_gained"),
        ("Websites lost", "website_lost"),
        ("High-opportunity alerts", "alerts"),
    ):
        items = report.get(key) or []
        if not items:
            continue
        lines.append(f"  {label}:")
        for item in items[:20]:
            lines.append(f"    - {item['name']} ({item['category']}, {item['city']}) score={item['lead_score']}")
        if len(items) > 20:
            lines.append(f"    ... and {len(items) - 20} more")
    return "\n".join(lines)
