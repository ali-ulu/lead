from __future__ import annotations

import hashlib
import math
from typing import Any

from . import normalize

NAME_SIMILARITY = 0.86
NEARBY_METERS = 250.0

def _norm_text(value: str | None) -> str:
    return normalize.normalize_name(value)

def _norm_phone(value: str | None, region: str = "TR") -> str:
    return normalize.phone_key(value, region)

def _domain(value: str | None) -> str:
    return normalize.normalize_domain(value)

def _distance_m(a: dict[str, Any], b: dict[str, Any]) -> float:
    try:
        lat1,lon1=float(a["latitude"]),float(a["longitude"])
        lat2,lon2=float(b["latitude"]),float(b["longitude"])
    except Exception:
        return 1e9
    r=6371000.0
    p1,p2=math.radians(lat1),math.radians(lat2)
    dp=math.radians(lat2-lat1); dl=math.radians(lon2-lon1)
    x=math.sin(dp/2)**2+math.cos(p1)*math.cos(p2)*math.sin(dl/2)**2
    return 2*r*math.atan2(math.sqrt(x),math.sqrt(1-x))

def _region(a: dict[str, Any], b: dict[str, Any]) -> str:
    return str(a.get("country") or b.get("country") or "TR")

def _same_business(a: dict[str, Any], b: dict[str, Any]) -> bool:
    da,db=_domain(a.get("website")),_domain(b.get("website"))
    if da and db and da==db: return True
    region=_region(a,b)
    pa,pb=_norm_phone(a.get("phone"),region),_norm_phone(b.get("phone"),region)
    if pa and pb and len(pa)>=7 and pa==pb: return True
    if normalize.name_similarity(a.get("name"), b.get("name"))>=NAME_SIMILARITY and _distance_m(a,b)<=NEARBY_METERS:
        return True
    return False

def _canonical_id(lead: dict[str, Any]) -> str:
    domain=_domain(lead.get("website"))
    phone=_norm_phone(lead.get("phone"), _region(lead, lead))
    if domain: key=f"domain:{domain}"
    elif phone: key=f"phone:{phone}"
    else:
        lat=round(float(lead.get("latitude") or 0),4)
        lon=round(float(lead.get("longitude") or 0),4)
        key=f"namegeo:{normalize.normalize_name(lead.get('name'))}:{lat}:{lon}"
    return hashlib.sha256(key.encode("utf-8")).hexdigest()[:32]

def merge_leads(*provider_sets: list[dict[str, Any]]) -> list[dict[str, Any]]:
    merged: list[dict[str, Any]] = []
    for rows in provider_sets:
        for candidate in rows:
            target = next((x for x in merged if _same_business(x,candidate)), None)
            if target is None:
                item=dict(candidate)
                item["source"]="multi"
                item["source_refs"]=dict(candidate.get("source_refs") or {candidate.get("source","unknown"):candidate.get("source_id")})
                item["social_links"]=dict(candidate.get("social_links") or {})
                item["verification_notes"]=list(candidate.get("verification_notes") or [])
                merged.append(item)
                continue

            refs=target.setdefault("source_refs",{})
            refs.update(candidate.get("source_refs") or {candidate.get("source","unknown"):candidate.get("source_id")})
            target.setdefault("verification_notes",[]).append(f"Matched across {', '.join(sorted(refs))}")
            target["verification_status"]="cross_source" if len(refs)>=2 else target.get("verification_status","unverified")

            for field in ("website","phone","email","social_url"):
                if not target.get(field) and candidate.get(field):
                    target[field]=candidate[field]
            socials=target.setdefault("social_links",{})
            socials.update(candidate.get("social_links") or {})
            if not target.get("social_url") and socials:
                target["social_url"]=next(iter(socials.values()))
            if target.get("website"):
                target["website_status"]="unknown"
            if candidate.get("data_confidence")=="high":
                target["data_confidence"]="high"
            for coord in ("latitude","longitude"):
                if target.get(coord) is None and candidate.get(coord) is not None:
                    target[coord]=candidate[coord]

    for lead in merged:
        region=str(lead.get("country") or "TR")
        lead["phone"]=normalize.normalize_phone(lead.get("phone"),region)
        lead["website"]=normalize.normalize_website(lead.get("website"))
        if lead.get("website") and lead.get("website_status") in (None,"missing"):
            lead["website_status"]="unknown"
        lead["category"]=normalize.category_key(lead.get("category")) or lead.get("category")
        lead["source"]="multi"
        lead["source_id"]=_canonical_id(lead)
        lead["source_refs"]=lead.get("source_refs") or {}
    return merged
