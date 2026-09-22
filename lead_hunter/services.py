from __future__ import annotations

from typing import Any

from .audit import audit_url
from .db import get_lead, list_leads, record_search_run, update_lead, upsert_leads
from .outreach import build_message
from .providers.nominatim import geocode_area
from .providers.osm import CATEGORY_FILTERS, search_around

PIPELINE_STAGES = {"new", "reviewed", "contacted", "replied", "proposal", "won", "lost"}
MESSAGE_LANGUAGES = {"en", "tr", "ur", "sd", "de"}


def discover_businesses(
    *,
    city: str,
    category: str,
    country: str = "",
    radius_km: int = 20,
    max_results: int | None = None,
) -> dict[str, Any]:
    city = (city or "").strip()
    country = (country or "").strip()
    category = (category or "").strip()

    if not city or not category:
        raise ValueError("City/area and industry are required.")
    if category not in CATEGORY_FILTERS:
        raise ValueError(f"Unsupported industry: {category}")

    radius_km = max(1, min(100, int(radius_km or 20)))
    if max_results is not None:
        max_results = int(max_results)
        if max_results <= 0:
            max_results = None

    area = geocode_area(city, country)
    rows = search_around(
        area["lat"],
        area["lon"],
        radius_km,
        category,
        city=area["city"],
        country=area["country"],
        limit=max_results,
    )
    ids = upsert_leads(rows)
    search_id = record_search_run(
        country=area["country"],
        city=area["city"],
        category=category,
        radius_km=radius_km,
        lead_ids=ids,
    )
    return {
        "ok": True,
        "count": len(rows),
        "search_id": search_id,
        "ids": ids[:500],
        "ids_truncated": len(ids) > 500,
        "area": area,
        "category": category,
        "radius_km": radius_km,
        "result_cap": max_results,
    }


def audit_lead(lead_id: int) -> dict[str, Any]:
    lead = get_lead(int(lead_id))
    if not lead:
        raise LookupError("Lead not found.")
    if not lead.get("website"):
        raise ValueError("This lead has no website to audit.")

    result = audit_url(lead["website"])
    existing_socials = lead.get("social_links") or {}
    discovered_socials = result.get("social_links") or {}
    merged_socials = {**existing_socials, **discovered_socials}
    primary_social = lead.get("social_url")
    if not primary_social and merged_socials:
        primary_social = next(iter(merged_socials.values()))

    updates = {
        "website_status": result.get("website_status", "weak"),
        "performance_score": result.get("performance_score"),
        "seo_score": result.get("seo_score"),
        "mobile_ok": result.get("mobile_ok"),
        "has_cta": result.get("has_cta"),
        "has_booking": result.get("has_booking"),
        "has_https": result.get("has_https"),
        "social_links": merged_socials,
        "social_url": primary_social,
    }
    updated = update_lead(int(lead_id), updates)
    return {"lead": updated, "audit": result}


def draft_outreach(lead_id: int, lang: str = "en") -> dict[str, Any]:
    lead = get_lead(int(lead_id))
    if not lead:
        raise LookupError("Lead not found.")
    lang = lang if lang in MESSAGE_LANGUAGES else "en"
    return {"lead_id": int(lead_id), "lang": lang, "message": build_message(lead, lang)}


def set_pipeline_stage(lead_id: int, status: str) -> dict[str, Any]:
    if status not in PIPELINE_STAGES:
        raise ValueError(f"Invalid pipeline stage: {status}")
    lead = update_lead(int(lead_id), {"pipeline_status": status})
    if not lead:
        raise LookupError("Lead not found.")
    return {"lead": lead}


def mark_do_not_contact(lead_id: int) -> dict[str, Any]:
    lead = update_lead(int(lead_id), {"do_not_contact": 1})
    if not lead:
        raise LookupError("Lead not found.")
    return {"ok": True, "lead_id": int(lead_id)}


def query_leads(
    *,
    search_id: str = "",
    ids: list[int] | None = None,
    country: str = "",
    city: str = "",
    category: str = "",
    website_status: str = "",
    pipeline_status: str = "",
    min_score: int = 0,
    has_social: bool = False,
) -> list[dict[str, Any]]:
    filters: dict[str, str] = {}
    if search_id:
        filters["search_id"] = search_id
    if ids:
        filters["ids"] = ",".join(str(int(i)) for i in ids)
    if country:
        filters["country"] = country
    if city:
        filters["city"] = city
    if category:
        filters["category"] = category
    if website_status:
        filters["website_status"] = website_status
    if pipeline_status:
        filters["pipeline_status"] = pipeline_status
    if min_score:
        filters["min_score"] = str(int(min_score))
    if has_social:
        filters["has_social"] = "1"
    return list_leads(filters)
