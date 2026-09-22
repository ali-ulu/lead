from __future__ import annotations

import json
import os
import re
import urllib.parse
import urllib.request
from typing import Any

USER_AGENT = "LeadScout/6.0 web-verifier"

DIRECTORY_HOSTS = {
    "google.com","googleusercontent.com","maps.apple.com","bing.com","yelp.com",
    "tripadvisor.com","foursquare.com","yellowpages.com","justdial.com",
    "facebook.com","instagram.com","linkedin.com","x.com","twitter.com",
    "tiktok.com","youtube.com","wikipedia.org","mapquest.com",
}

def _tokens(value: str) -> set[str]:
    return {
        token for token in re.findall(r"[a-z0-9]+", (value or "").lower())
        if len(token) >= 3 and token not in {
            "the","and","for","clinic","restaurant","hotel","salon","shop",
            "services","service","official","website","karachi","istanbul","berlin",
        }
    }

def _host(url: str) -> str:
    try:
        return (urllib.parse.urlparse(url).hostname or "").lower().removeprefix("www.")
    except Exception:
        return ""

def _directory(url: str) -> bool:
    host=_host(url)
    return any(host==x or host.endswith("."+x) for x in DIRECTORY_HOSTS)

def _score_result(result: dict[str,Any], *, name: str, city: str, country: str) -> float:
    url=str(result.get("url") or "")
    title=str(result.get("title") or "")
    description=str(result.get("description") or result.get("content") or "")
    if not url.startswith(("http://","https://")) or _directory(url):
        return 0.0

    needle=_tokens(name)
    hay=_tokens(" ".join([title,_host(url),description]))
    if not needle:
        return 0.0

    overlap=len(needle & hay)/max(1,len(needle))
    score=overlap*0.72
    text=(title+" "+description).lower()
    if city and city.lower() in text:
        score+=0.12
    if country and country.lower() in text:
        score+=0.06
    if any(word in text for word in ("official","contact","about us")):
        score+=0.05
    host=_host(url)
    if any(tok in host for tok in needle):
        score+=0.12
    return min(1.0,score)

def _brave_search(query: str, *, count: int=10, country_code: str="") -> list[dict[str,Any]]:
    key=os.environ.get("BRAVE_SEARCH_API_KEY","").strip()
    if not key:
        raise RuntimeError("BRAVE_SEARCH_API_KEY is not configured.")
    params={"q":query,"count":str(max(1,min(20,int(count))))}
    if country_code and len(country_code)==2:
        params["country"]=country_code.upper()
    url="https://api.search.brave.com/res/v1/web/search?"+urllib.parse.urlencode(params)
    req=urllib.request.Request(
        url,
        headers={
            "Accept":"application/json",
            "Accept-Encoding":"gzip",
            "X-Subscription-Token":key,
            "User-Agent":USER_AGENT,
        },
    )
    with urllib.request.urlopen(req,timeout=20) as resp:
        data=json.loads(resp.read().decode("utf-8"))
    return list((data.get("web") or {}).get("results") or [])

def _searxng_search(query: str, *, count: int=10) -> list[dict[str,Any]]:
    base=os.environ.get("SEARXNG_URL","").strip().rstrip("/")
    if not base:
        raise RuntimeError("SEARXNG_URL is not configured.")
    url=base+"/search?"+urllib.parse.urlencode({
        "q":query,
        "format":"json",
        "safesearch":"1",
    })
    req=urllib.request.Request(url,headers={"Accept":"application/json","User-Agent":USER_AGENT})
    with urllib.request.urlopen(req,timeout=20) as resp:
        data=json.loads(resp.read().decode("utf-8"))
    return list(data.get("results") or [])[:max(1,min(20,int(count)))]

def configured_provider() -> str:
    if os.environ.get("BRAVE_SEARCH_API_KEY","").strip():
        return "brave"
    if os.environ.get("SEARXNG_URL","").strip():
        return "searxng"
    return ""

def verify_business_web(
    *,
    name: str,
    city: str="",
    country: str="",
    country_code: str="",
    count: int=10,
) -> dict[str,Any]:
    provider=configured_provider()
    if not provider:
        return {
            "configured":False,
            "provider":"",
            "verified":False,
            "reason":"No web-search provider configured.",
            "candidates":[],
        }

    query=" ".join(x for x in [f'"{name}"',city,country,"official website"] if x).strip()
    if provider=="brave":
        raw=_brave_search(query,count=count,country_code=country_code)
    else:
        raw=_searxng_search(query,count=count)

    candidates=[]
    for item in raw:
        url=str(item.get("url") or "").strip()
        if not url:
            continue
        score=_score_result(item,name=name,city=city,country=country)
        candidates.append({
            "url":url,
            "title":str(item.get("title") or ""),
            "description":str(item.get("description") or item.get("content") or ""),
            "score":round(score,3),
            "host":_host(url),
        })
    candidates.sort(key=lambda x:x["score"],reverse=True)
    best=candidates[0] if candidates else None
    verified=bool(best and best["score"]>=0.58)
    return {
        "configured":True,
        "provider":provider,
        "query":query,
        "verified":verified,
        "website":best["url"] if verified else None,
        "confidence":best["score"] if best else 0.0,
        "best":best,
        "candidates":candidates[:5],
    }
