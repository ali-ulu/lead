from __future__ import annotations

import html
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
            "X-Subscription-Token":key,
            "User-Agent":USER_AGENT,
        },
    )
    with urllib.request.urlopen(req,timeout=20) as resp:
        data=json.loads(resp.read().decode("utf-8"))
    return list((data.get("web") or {}).get("results") or [])

def _google_search(query: str, *, count: int=10) -> list[dict[str,Any]]:
    key=os.environ.get("GOOGLE_CSE_API_KEY","").strip()
    cx=os.environ.get("GOOGLE_CSE_ID","").strip()
    if not key or not cx:
        raise RuntimeError("GOOGLE_CSE_API_KEY and GOOGLE_CSE_ID are not configured.")
    params={"key":key,"cx":cx,"q":query,"num":str(max(1,min(10,int(count))))}
    url="https://www.googleapis.com/customsearch/v1?"+urllib.parse.urlencode(params)
    req=urllib.request.Request(url,headers={"Accept":"application/json","User-Agent":USER_AGENT})
    with urllib.request.urlopen(req,timeout=20) as resp:
        data=json.loads(resp.read().decode("utf-8"))
    return [
        {"url":item.get("link") or "","title":item.get("title") or "","description":item.get("snippet") or ""}
        for item in (data.get("items") or [])
    ]


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


def _clean_html(value: str) -> str:
    return re.sub(r"\s+", " ", html.unescape(re.sub(r"<[^>]+>", "", value or ""))).strip()


def _duckduckgo_search(query: str, *, count: int=10) -> list[dict[str,Any]]:
    # Keyless HTML endpoint. No API key or account is required; results are
    # parsed from the markup because the Instant Answer API does not return links.
    data=urllib.parse.urlencode({"q":query}).encode("utf-8")
    req=urllib.request.Request(
        "https://html.duckduckgo.com/html/",
        data=data,
        headers={
            "User-Agent": "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120 Safari/537.36",
            "Accept": "text/html",
            "Content-Type": "application/x-www-form-urlencoded",
        },
    )
    with urllib.request.urlopen(req,timeout=20) as resp:
        page=resp.read().decode("utf-8","replace")

    lowered=page.lower()
    if "/anomaly" in page or "anomaly-modal" in lowered or "unusual traffic" in lowered or "captcha" in lowered:
        # DuckDuckGo serves a bot-check page instead of results once a burst of
        # serial queries trips its rate limiter; raise so the chain falls through.
        raise RuntimeError("DuckDuckGo returned a bot-check page; try another provider.")

    results: list[dict[str,Any]] = []
    pattern=re.compile(
        r'<a[^>]*class="result__a"[^>]*href="([^"]+)"[^>]*>(.*?)</a>'
        r'(?:.*?class="result__snippet"[^>]*>(.*?)</a>)?',
        re.S,
    )
    for href, title, snippet in pattern.findall(page):
        url=html.unescape(href)
        if url.startswith("//duckduckgo.com/l/"):
            target=urllib.parse.parse_qs(urllib.parse.urlparse("https:"+url).query).get("uddg")
            url=target[0] if target else url
        elif url.startswith("/l/"):
            target=urllib.parse.parse_qs(urllib.parse.urlparse(url).query).get("uddg")
            url=target[0] if target else url
        if not url.startswith(("http://","https://")):
            continue
        results.append({
            "url":url,
            "title":_clean_html(title),
            "description":_clean_html(snippet or ""),
        })
        if len(results)>=max(1,min(20,int(count))):
            break
    return results


def _explicit_chain() -> list[str] | None:
    raw=os.environ.get("LEADSCOUT_WEB_SEARCH","").strip().lower()
    if not raw:
        return None
    items=[part.strip() for part in raw.split(",") if part.strip()]
    if not items:
        return None
    if items[0] in {"off","none","disabled"}:
        return []
    return items


def _available(provider: str) -> bool:
    if provider=="brave":
        return bool(os.environ.get("BRAVE_SEARCH_API_KEY","").strip())
    if provider=="searxng":
        return bool(os.environ.get("SEARXNG_URL","").strip())
    if provider=="google":
        return bool(os.environ.get("GOOGLE_CSE_API_KEY","").strip() and os.environ.get("GOOGLE_CSE_ID","").strip())
    if provider=="duckduckgo":
        return True
    return False


def configured_providers() -> list[str]:
    explicit=_explicit_chain()
    if explicit is not None:
        return [p for p in explicit if _available(p)]
    # Default: a free, keyless DuckDuckGo lookup first, then Google Programmable
    # Search when its key is present, so serial queries keep working if the
    # keyless endpoint rate-limits.
    return [p for p in ("duckduckgo","google") if _available(p)]


def configured_provider() -> str:
    providers=configured_providers()
    return providers[0] if providers else ""


def _run_provider(provider: str, query: str, *, count: int, country_code: str) -> list[dict[str,Any]]:
    if provider=="brave":
        return _brave_search(query,count=count,country_code=country_code)
    if provider=="searxng":
        return _searxng_search(query,count=count)
    if provider=="google":
        return _google_search(query,count=count)
    return _duckduckgo_search(query,count=count)


def verify_business_web(
    *,
    name: str,
    city: str="",
    country: str="",
    country_code: str="",
    count: int=10,
) -> dict[str,Any]:
    providers=configured_providers()
    if not providers:
        return {
            "configured":False,
            "provider":"",
            "verified":False,
            "reason":"No web-search provider configured.",
            "candidates":[],
        }

    query=" ".join(x for x in [f'"{name}"',city,country,"official website"] if x).strip()
    provider=""
    raw: list[dict[str,Any]]=[]
    errors: list[dict[str,str]]=[]
    for candidate in providers:
        try:
            got=_run_provider(candidate,query,count=count,country_code=country_code)
        except Exception as exc:
            errors.append({"provider":candidate,"error":str(exc)})
            continue
        if got:
            raw=got
            provider=candidate
            break
        # A provider that answers with nothing may be soft-blocking; let the
        # next provider in the chain try before giving up.
        provider=candidate

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
    result = {
        "configured":True,
        "provider":provider,
        "providers":providers,
        "query":query,
        "verified":verified,
        "website":best["url"] if verified else None,
        "confidence":best["score"] if best else 0.0,
        "best":best,
        "candidates":candidates[:5],
    }
    if errors:
        result["errors"]=errors
    return result
