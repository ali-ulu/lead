from __future__ import annotations

import json
import math
import os
import re
import urllib.request
from typing import Any

ENDPOINT="https://places.googleapis.com/v1/places:searchText"
USER_AGENT="LeadScout/6.0 reputation-enricher"

def configured() -> bool:
    return bool(os.environ.get("GOOGLE_PLACES_API_KEY","").strip())

def _norm(value: str) -> set[str]:
    return {
        token for token in re.findall(r"[a-z0-9]+",(value or "").lower())
        if len(token)>=3 and token not in {"the","and","for","clinic","restaurant","hotel","salon","shop","services"}
    }

def _distance_km(lat1: float, lon1: float, lat2: float, lon2: float) -> float:
    radius=6371.0088
    p1=math.radians(lat1); p2=math.radians(lat2)
    dp=math.radians(lat2-lat1); dl=math.radians(lon2-lon1)
    a=math.sin(dp/2)**2+math.cos(p1)*math.cos(p2)*math.sin(dl/2)**2
    return radius*2*math.atan2(math.sqrt(a),math.sqrt(max(0.0,1-a)))

def _match_score(place: dict[str,Any], *, name: str, lat: float|None, lon: float|None) -> float:
    display=((place.get("displayName") or {}).get("text") or "")
    a=_norm(name); b=_norm(display)
    name_score=len(a & b)/max(1,len(a)) if a else 0.0
    score=name_score*0.82
    loc=place.get("location") or {}
    if lat is not None and lon is not None and loc.get("latitude") is not None and loc.get("longitude") is not None:
        distance=_distance_km(lat,lon,float(loc["latitude"]),float(loc["longitude"]))
        if distance<=0.25: score+=0.18
        elif distance<=1.0: score+=0.12
        elif distance<=3.0: score+=0.06
    return min(1.0,score)

def enrich_reputation(
    *,
    name: str,
    city: str="",
    country: str="",
    latitude: float|None=None,
    longitude: float|None=None,
) -> dict[str,Any]:
    key=os.environ.get("GOOGLE_PLACES_API_KEY","").strip()
    if not key:
        return {"configured":False,"matched":False,"reason":"GOOGLE_PLACES_API_KEY is not configured."}

    text_query=", ".join(x for x in [name,city,country] if x)
    payload: dict[str,Any]={"textQuery":text_query,"maxResultCount":5}
    if latitude is not None and longitude is not None:
        payload["locationBias"]={
            "circle":{
                "center":{"latitude":float(latitude),"longitude":float(longitude)},
                "radius":5000.0,
            }
        }

    field_mask=",".join([
        "places.id",
        "places.displayName",
        "places.formattedAddress",
        "places.location",
        "places.rating",
        "places.userRatingCount",
        "places.websiteUri",
        "places.nationalPhoneNumber",
    ])
    req=urllib.request.Request(
        ENDPOINT,
        data=json.dumps(payload).encode("utf-8"),
        method="POST",
        headers={
            "Content-Type":"application/json",
            "Accept":"application/json",
            "X-Goog-Api-Key":key,
            "X-Goog-FieldMask":field_mask,
            "User-Agent":USER_AGENT,
        },
    )
    with urllib.request.urlopen(req,timeout=20) as resp:
        data=json.loads(resp.read().decode("utf-8"))

    ranked=[]
    for place in data.get("places") or []:
        score=_match_score(place,name=name,lat=latitude,lon=longitude)
        ranked.append((score,place))
    ranked.sort(key=lambda item:item[0],reverse=True)
    if not ranked or ranked[0][0]<0.55:
        return {
            "configured":True,
            "matched":False,
            "query":text_query,
            "candidate_count":len(ranked),
        }

    score,place=ranked[0]
    return {
        "configured":True,
        "matched":True,
        "match_confidence":round(score,3),
        "place_id":place.get("id"),
        "display_name":((place.get("displayName") or {}).get("text") or ""),
        "formatted_address":place.get("formattedAddress"),
        "rating":place.get("rating"),
        "review_count":place.get("userRatingCount"),
        "website":place.get("websiteUri"),
        "phone":place.get("nationalPhoneNumber"),
        "source":"google_places",
    }
