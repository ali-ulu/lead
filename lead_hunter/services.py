from __future__ import annotations

from typing import Any

from .audit import audit_url
from .crm import add_activity
from .db import get_lead, list_leads, record_search_run, update_lead, upsert_leads
from .enrichment import enrich_website
from .intelligence import calculate_intelligence
from .merge import merge_leads
from .outreach import build_message
from .providers.nominatim import geocode_area
from .providers.osm import CATEGORY_FILTERS, _radius_bbox, search_around_detailed
from .providers.overture import search_bbox as search_overture_bbox

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
    return update_lead(lead_id,{
        "contactability_score":contact,"commercial_score":commercial,"intelligence_reasons":reasons
    }) or lead

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
    if lead.get("latitude") is None or lead.get("longitude") is None:
        return {"lead":lead,"verified":False,"reason":"missing_coordinates"}
    lat=float(lead["latitude"]); lon=float(lead["longitude"])
    delta=0.004
    try:
        rows=search_overture_bbox(
            lon-delta,lat-delta,lon+delta,lat+delta,lead.get("category") or "",
            city=lead.get("city") or "",country=lead.get("country") or "",limit=100,
        )
        merged=merge_leads([lead],rows)
        matched=next((x for x in merged if len(x.get("source_refs") or {})>=2),None)
        if matched:
            updates={k:matched.get(k) for k in ("website","phone","email","social_url","social_links","source_refs","website_status") if matched.get(k) is not None}
            updates["verification_status"]="verified_no_site" if not matched.get("website") else "cross_source"
            updates["verification_notes"]=list(dict.fromkeys((lead.get("verification_notes") or [])+["Cross-checked against Overture Places."]))
            updated=update_lead(int(lead_id),updates) or lead
            updated=_update_intelligence(int(lead_id),updated)
            return {"lead":updated,"verified":True,"matched_source":"overture"}
    except Exception as exc:
        return {"lead":lead,"verified":False,"reason":str(exc)}
    update_lead(int(lead_id),{"verification_notes":list(dict.fromkeys((lead.get("verification_notes") or [])+["No matching Overture place found nearby."]))})
    return {"lead":get_lead(int(lead_id)),"verified":False,"reason":"no_cross_source_match"}

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
    if has_social: filters["has_social"]="1"
    if contactable: filters["contactable"]="1"
    return list_leads(filters)
