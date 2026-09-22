from __future__ import annotations

from typing import Any

from .audit import audit_url
from .crm import add_activity
from .db import get_lead, list_leads, record_search_run, update_lead, upsert_leads
from .enrichment import enrich_website
from .intelligence import calculate_intelligence
from .visibility import calculate_visibility, missing_website_visibility
from .merge import merge_leads
from .outreach import build_message
from .providers.nominatim import geocode_area
from .providers.osm import CATEGORY_FILTERS, _radius_bbox, search_around_detailed
from .providers.overture import search_bbox as search_overture_bbox
from .providers.web_search import verify_business_web
from .providers.google_places import enrich_reputation as google_reputation

PIPELINE_STAGES = {"new","reviewed","contacted","replied","proposal","won","lost"}
MESSAGE_LANGUAGES = {"en","tr","ur","sd","de"}

def _intelligent(rows: list[dict[str,Any]]) -> list[dict[str,Any]]:
    for lead in rows:
        refs=lead.get("source_refs") or {}
        if len(refs)>=2:
            lead["verification_status"]="verified_no_site" if not lead.get("website") else "cross_source"
            notes=list(lead.get("verification_notes") or [])
            notes.append("Business matched across independent place sources.")
            lead["verification_notes"]=list(dict.fromkeys(notes))
        contact,commercial,reasons=calculate_intelligence(lead)
        lead["contactability_score"]=contact
        lead["commercial_score"]=commercial
        lead["intelligence_reasons"]=reasons
    return rows

def discover_businesses(
    *, city: str, category: str, country: str="", radius_km: int=20,
    max_results: int | None=None, providers: list[str] | None=None,
) -> dict[str,Any]:
    city=(city or "").strip(); country=(country or "").strip(); category=(category or "").strip()
    if not city or not category: raise ValueError("City/area and industry are required.")
    if category not in CATEGORY_FILTERS: raise ValueError(f"Unsupported industry: {category}")
    radius_km=max(1,min(100,int(radius_km or 20)))
    if max_results is not None:
        max_results=int(max_results)
        if max_results<=0: max_results=None

    wanted=set(providers or ["osm","overture"])
    area=geocode_area(city,country)
    provider_rows: list[list[dict[str,Any]]] = []
    summary: dict[str,Any]={}; warnings=[]; partial=False

    if "osm" in wanted:
        osm=search_around_detailed(
            area["lat"],area["lon"],radius_km,category,
            city=area["city"],country=area["country"],limit=max_results,
        )
        provider_rows.append(osm["rows"])
        summary["osm"]={"count":len(osm["rows"]),"partial":bool(osm.get("partial"))}
        if osm.get("partial"): partial=True
        warnings.extend(osm.get("warnings") or [])

    if "overture" in wanted:
        south,west,north,east=_radius_bbox(area["lat"],area["lon"],radius_km)
        try:
            overture=search_overture_bbox(
                west,south,east,north,category,
                city=area["city"],country=area["country"],limit=max_results,
            )
            provider_rows.append(overture)
            summary["overture"]={"count":len(overture),"partial":False}
        except Exception as exc:
            partial=True
            warnings.append(f"Overture unavailable: {exc}")
            summary["overture"]={"count":0,"partial":True,"error":str(exc)}

    if not provider_rows:
        raise RuntimeError("No discovery provider is enabled.")

    rows=_intelligent(merge_leads(*provider_rows))
    if max_results is not None: rows=rows[:max_results]
    ids=upsert_leads(rows)
    search_id=record_search_run(
        country=area["country"],city=area["city"],category=category,radius_km=radius_km,
        lead_ids=ids,provider_summary=summary,partial=partial,warnings=warnings,
    )
    return {
        "ok":True,"count":len(rows),"search_id":search_id,"ids":ids[:500],"ids_truncated":len(ids)>500,
        "area":area,"category":category,"radius_km":radius_km,"result_cap":max_results,
        "providers":summary,"partial":partial,"warnings":warnings,
    }

def _update_intelligence(lead_id: int, lead: dict[str,Any]) -> dict[str,Any]:
    contact,commercial,reasons=calculate_intelligence(lead)
    updates: dict[str,Any] = {
        "contactability_score":contact,
        "commercial_score":commercial,
        "intelligence_reasons":reasons,
    }

    signals=lead.get("visibility_signals") or {}
    if lead.get("website") and signals:
        visibility=calculate_visibility(signals,commercial_score=commercial)
        updates.update({
            "seo_score":visibility.get("seo_score"),
            "aeo_score":visibility.get("aeo_score"),
            "geo_score":visibility.get("geo_score"),
            "ai_visibility_score":visibility.get("ai_visibility_score"),
            "opportunity_gap_score":visibility.get("opportunity_gap_score"),
            "visibility_reasons":visibility.get("visibility_reasons") or {},
            "visibility_signals":visibility.get("visibility_signals") or signals,
        })
    elif not lead.get("website"):
        visibility=missing_website_visibility(
            commercial_score=commercial,
            verified=lead.get("verification_status")=="verified_no_site",
        )
        updates.update(visibility)

    return update_lead(lead_id,updates) or lead

def enrich_lead(lead_id: int) -> dict[str,Any]:
    lead=get_lead(int(lead_id))
    if not lead: raise LookupError("Lead not found.")
    if not lead.get("website"):
        return {"lead":lead,"enrichment":{"complete":False,"reason":"no_website"}}
    result=enrich_website(lead["website"])
    socials={**(lead.get("social_links") or {}),**(result.get("social_links") or {})}
    updates={
        "email":lead.get("email") or next(iter(result.get("emails") or []),None),
        "phone":lead.get("phone") or next(iter(result.get("phones") or []),None),
        "social_links":socials,
        "social_url":lead.get("social_url") or next(iter(socials.values()),None),
        "verification_status":"verified" if result.get("complete") else lead.get("verification_status"),
    }
    updated=update_lead(int(lead_id),updates) or lead
    updated=_update_intelligence(int(lead_id),updated)
    add_activity(int(lead_id),kind="enrichment",status="complete" if result.get("complete") else "partial",
                 metadata={"pages_scanned":result.get("pages_scanned",[]),"booking_links":result.get("booking_links",[])})
    return {"lead":updated,"enrichment":result}

def verify_lead(lead_id: int) -> dict[str,Any]:
    lead=get_lead(int(lead_id))
    if not lead: raise LookupError("Lead not found.")

    checks: list[dict[str,Any]]=[]
    updated=lead

    # Cross-check the place record against Overture when coordinates exist.
    if lead.get("latitude") is not None and lead.get("longitude") is not None:
        lat=float(lead["latitude"]); lon=float(lead["longitude"])
        delta=0.004
        try:
            rows=search_overture_bbox(
                lon-delta,lat-delta,lon+delta,lat+delta,lead.get("category") or "",
                city=lead.get("city") or "",country=lead.get("country") or "",limit=100,
            )
            merged=merge_leads([updated],rows)
            matched=next((x for x in merged if len(x.get("source_refs") or {})>=2),None)
            if matched:
                updates={
                    k:matched.get(k) for k in
                    ("website","phone","email","social_url","social_links","source_refs","website_status")
                    if matched.get(k) is not None
                }
                updates["verification_status"]="cross_source"
                updates["verification_notes"]=list(dict.fromkeys(
                    (updated.get("verification_notes") or [])+
                    ["Cross-checked against Overture Places."]
                ))
                updated=update_lead(int(lead_id),updates) or updated
                checks.append({"provider":"overture","matched":True})
            else:
                checks.append({"provider":"overture","matched":False})
        except Exception as exc:
            checks.append({"provider":"overture","matched":False,"error":str(exc)})

    # If a website is still missing, use an optional real web-search provider.
    web_check=None
    if not updated.get("website"):
        try:
            web_check=verify_business_web(
                name=updated.get("name") or "",
                city=updated.get("city") or "",
                country=updated.get("country") or "",
            )
            if web_check.get("verified") and web_check.get("website"):
                website=str(web_check["website"])
                notes=list(updated.get("verification_notes") or [])
                notes.append(
                    f"Official-site candidate verified via {web_check.get('provider')}: {website}"
                )
                updated=update_lead(int(lead_id),{
                    "website":website,
                    "website_status":"unknown",
                    "verification_status":"web_verified",
                    "verification_notes":list(dict.fromkeys(notes)),
                }) or updated
            elif web_check.get("configured"):
                notes=list(updated.get("verification_notes") or [])
                notes.append(
                    f"No high-confidence official website found via {web_check.get('provider')}."
                )
                updated=update_lead(int(lead_id),{
                    "verification_status":"verified_no_site",
                    "verification_notes":list(dict.fromkeys(notes)),
                }) or updated
            checks.append({
                "provider":web_check.get("provider") or "web_search",
                "matched":bool(web_check.get("verified")),
                "configured":bool(web_check.get("configured")),
            })
        except Exception as exc:
            web_check={"configured":True,"verified":False,"error":str(exc)}
            checks.append({"provider":"web_search","matched":False,"error":str(exc)})

    updated=_update_intelligence(int(lead_id),updated)
    add_activity(
        int(lead_id),kind="verification",
        status=updated.get("verification_status") or "unverified",
        metadata={"checks":checks,"web":web_check or {}},
    )
    return {
        "lead":updated,
        "verified":updated.get("verification_status") in {"cross_source","web_verified","verified_no_site","verified"},
        "verification_status":updated.get("verification_status"),
        "checks":checks,
        "web_verification":web_check,
    }


def enrich_reputation(lead_id: int) -> dict[str,Any]:
    lead=get_lead(int(lead_id))
    if not lead: raise LookupError("Lead not found.")

    result=google_reputation(
        name=lead.get("name") or "",
        city=lead.get("city") or "",
        country=lead.get("country") or "",
        latitude=float(lead["latitude"]) if lead.get("latitude") is not None else None,
        longitude=float(lead["longitude"]) if lead.get("longitude") is not None else None,
    )

    if not result.get("matched"):
        add_activity(
            int(lead_id),kind="reputation",status="not_matched",
            metadata=result,
        )
        return {"lead":lead,"reputation":result}

    source_refs=dict(lead.get("source_refs") or {})
    if result.get("place_id"):
        source_refs["google_places"]=result["place_id"]
    notes=list(lead.get("verification_notes") or [])
    notes.append(
        f"Reputation matched via Google Places ({result.get('match_confidence',0):.2f} confidence)."
    )
    updates={
        "rating":result.get("rating"),
        "review_count":result.get("review_count"),
        "source_refs":source_refs,
        "verification_notes":list(dict.fromkeys(notes)),
    }
    if not lead.get("website") and result.get("website"):
        updates["website"]=result["website"]
        updates["website_status"]="unknown"
        updates["verification_status"]="reputation_verified"
    if not lead.get("phone") and result.get("phone"):
        updates["phone"]=result["phone"]

    updated=update_lead(int(lead_id),updates) or lead
    updated=_update_intelligence(int(lead_id),updated)
    add_activity(
        int(lead_id),kind="reputation",status="matched",
        metadata={
            "source":"google_places",
            "rating":result.get("rating"),
            "review_count":result.get("review_count"),
            "place_id":result.get("place_id"),
        },
    )
    return {"lead":updated,"reputation":result}

def audit_lead(lead_id: int, enrich: bool=True) -> dict[str,Any]:
    lead=get_lead(int(lead_id))
    if not lead: raise LookupError("Lead not found.")
    if not lead.get("website"): raise ValueError("This lead has no website to audit.")
    result=audit_url(lead["website"])
    existing=lead.get("social_links") or {}; discovered=result.get("social_links") or {}
    socials={**existing,**discovered}
    updates={
        "website_status":result.get("website_status","weak"),
        "performance_score":result.get("performance_score"),"seo_score":result.get("seo_score"),
        "aeo_score":result.get("aeo_score"),"geo_score":result.get("geo_score"),
        "ai_visibility_score":result.get("ai_visibility_score"),
        "opportunity_gap_score":result.get("opportunity_gap_score"),
        "visibility_reasons":result.get("visibility_reasons") or {},
        "visibility_signals":result.get("visibility_signals") or {},
        "accessibility_score":result.get("accessibility_score"),"mobile_ok":result.get("mobile_ok"),
        "has_cta":result.get("has_cta"),"has_booking":result.get("has_booking"),"has_https":result.get("has_https"),
        "audit_engine":result.get("audit_engine","heuristic"),
        "social_links":socials,"social_url":lead.get("social_url") or next(iter(socials.values()),None),
    }
    updated=update_lead(int(lead_id),updates) or lead
    enrichment=None
    if enrich:
        try:
            payload=enrich_lead(int(lead_id)); updated=payload["lead"]; enrichment=payload["enrichment"]
        except Exception as exc:
            enrichment={"complete":False,"error":str(exc)}
    updated=_update_intelligence(int(lead_id),updated)
    add_activity(int(lead_id),kind="audit",status=updated.get("website_status") or "",metadata={"engine":result.get("audit_engine","heuristic")})
    return {"lead":updated,"audit":result,"enrichment":enrichment}

def draft_outreach(lead_id: int, lang: str="en") -> dict[str,Any]:
    lead=get_lead(int(lead_id))
    if not lead: raise LookupError("Lead not found.")
    lang=lang if lang in MESSAGE_LANGUAGES else "en"
    message=build_message(lead,lang)
    add_activity(int(lead_id),kind="message",channel="draft",status="drafted",direction="out",body=message)
    if lead.get("engagement_status")=="not_contacted":
        update_lead(int(lead_id),{"engagement_status":"drafted"})
    return {"lead_id":int(lead_id),"lang":lang,"message":message}

def set_pipeline_stage(lead_id: int,status: str) -> dict[str,Any]:
    if status not in PIPELINE_STAGES: raise ValueError(f"Invalid pipeline stage: {status}")
    lead=update_lead(int(lead_id),{"pipeline_status":status})
    if not lead: raise LookupError("Lead not found.")
    add_activity(int(lead_id),kind="status",status=status,metadata={"field":"pipeline_status"})
    return {"lead":lead}

def mark_do_not_contact(lead_id: int) -> dict[str,Any]:
    lead=update_lead(int(lead_id),{"do_not_contact":1})
    if not lead: raise LookupError("Lead not found.")
    add_activity(int(lead_id),kind="status",status="do_not_contact")
    return {"ok":True,"lead_id":int(lead_id)}

def query_leads(
    *, search_id: str="", ids: list[int] | None=None, country: str="", city: str="", category: str="",
    website_status: str="", pipeline_status: str="", engagement_status: str="", min_score: int=0,
    min_seo_score: int=0, min_aeo_score: int=0, min_geo_score: int=0,
    min_ai_visibility_score: int=0, min_opportunity_gap_score: int=0,
    max_seo_score: int=0, max_aeo_score: int=0, max_geo_score: int=0,
    max_ai_visibility_score: int=0,
    has_social: bool=False, contactable: bool=False,
) -> list[dict[str,Any]]:
    filters: dict[str,str]={}
    if search_id: filters["search_id"]=search_id
    if ids: filters["ids"]=",".join(str(int(i)) for i in ids)
    for key,value in {
        "country":country,"city":city,"category":category,"website_status":website_status,
        "pipeline_status":pipeline_status,"engagement_status":engagement_status,
    }.items():
        if value: filters[key]=value
    if min_score: filters["min_score"]=str(int(min_score))
    for key,value in {
        "min_seo_score":min_seo_score,
        "min_aeo_score":min_aeo_score,
        "min_geo_score":min_geo_score,
        "min_ai_visibility_score":min_ai_visibility_score,
        "min_opportunity_gap_score":min_opportunity_gap_score,
        "max_seo_score":max_seo_score,
        "max_aeo_score":max_aeo_score,
        "max_geo_score":max_geo_score,
        "max_ai_visibility_score":max_ai_visibility_score,
    }.items():
        if value: filters[key]=str(int(value))
    if has_social: filters["has_social"]="1"
    if contactable: filters["contactable"]="1"
    return list_leads(filters)
