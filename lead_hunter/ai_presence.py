"""Opt-in measurement of whether a business surfaces in AI-style answers.

"AI Visibility" in the readiness scorecard is a composite heuristic, not a
measurement. This module adds the missing measurement: it runs the same
category/city questions a person would ask an assistant through the configured
web-search chain and records whether the business name or domain appears in the
returned sources. The result is stored separately from the heuristic score so
the two are never confused.

It is opt-in (``LEADSCOUT_AI_PRESENCE=1``) because it spends search-provider
quota: each lead costs one or more queries.
"""
from __future__ import annotations

import os
import re
from typing import Any

from .providers.web_search import _run_provider, configured_providers


def enabled() -> bool:
    return os.environ.get("LEADSCOUT_AI_PRESENCE", "0").strip() == "1"


def _tokens(value: str) -> set[str]:
    return {
        token for token in re.findall(r"[a-z0-9]+", (value or "").lower())
        if len(token) >= 4
    }


def _questions(category: str, city: str, country: str) -> list[str]:
    place = " ".join(x for x in (city, country) if x).strip()
    sector = (category or "business").replace("_", " ")
    if not place:
        return []
    return [
        f"best {sector} in {place}",
        f"recommended {sector} {place}",
    ]


def measure_ai_presence(
    *,
    name: str,
    category: str,
    city: str,
    country: str = "",
    website: str = "",
    count: int = 10,
) -> dict[str, Any]:
    """Return how often this business appears in answer-style search results."""
    if not enabled():
        return {"configured": False, "reason": "LEADSCOUT_AI_PRESENCE is not enabled."}
    providers = configured_providers()
    if not providers:
        return {"configured": False, "reason": "No web-search provider configured."}

    questions = _questions(category, city, country)
    if not questions:
        return {"configured": False, "reason": "City/category are required to measure AI presence."}

    name_tokens = _tokens(name)
    domain = ""
    if website:
        from urllib.parse import urlparse
        domain = (urlparse(website).hostname or "").lower().removeprefix("www.")

    hits: list[dict[str, Any]] = []
    queries_run = 0
    for question in questions:
        results = None
        for provider in providers:
            try:
                results = _run_provider(provider, question, count=count, country_code="")
            except Exception:
                continue
            if results:
                break
        if not results:
            continue
        queries_run += 1
        ranked = None
        for index, item in enumerate(results, start=1):
            url = str(item.get("url") or "")
            title = str(item.get("title") or "")
            snippet = str(item.get("description") or item.get("content") or "")
            hay = " ".join([url, title, snippet]).lower()
            host = ""
            try:
                from urllib.parse import urlparse
                host = (urlparse(url).hostname or "").lower().removeprefix("www.")
            except Exception:
                host = ""
            matched = bool(domain and domain in hay) or bool(
                name_tokens and name_tokens.issubset(_tokens(title + " " + snippet))
            )
            if matched:
                ranked = {"rank": index, "url": url, "host": host, "title": title}
                break
        hits.append({"question": question, "cited": ranked is not None, "citation": ranked})

    if queries_run == 0:
        return {"configured": True, "measured": False, "reason": "Search providers returned no results."}

    cited = sum(1 for hit in hits if hit["cited"])
    best_rank = min((h["citation"]["rank"] for h in hits if h["citation"]), default=None)
    return {
        "configured": True,
        "measured": True,
        "queries": queries_run,
        "cited_count": cited,
        "mention_rate": round(cited / queries_run, 3),
        "best_rank": best_rank,
        "hits": hits,
    }
