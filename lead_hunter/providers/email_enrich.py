"""Waterfall email enrichment: try Hunter, then PDL, then Clearbit.

Website crawling only finds an address when the business publishes one. When
it does not, these hosted providers can supply a contact from the domain. They
are tried in order and the first usable address wins, so a single configured
key still produces a result and multiple keys increase coverage.

Every provider is key-gated (``HUNTER_API_KEY``, ``PEOPLE_DATA_LABS_API_KEY``,
``CLEARBIT_API_KEY``). A provider that is not configured is skipped; a
provider that answers without an email falls through to the next one.
"""
from __future__ import annotations

import json
import os
import urllib.parse
import urllib.request
from typing import Any, Callable

from .. import cache
from .. import ratelimit

USER_AGENT = "LeadScout/7.0 (local business research tool)"


def _get_json(url: str, headers: dict[str, str], timeout: float) -> dict[str, Any]:
    req = urllib.request.Request(url, headers={"User-Agent": USER_AGENT, "Accept": "application/json", **headers})
    with urllib.request.urlopen(req, timeout=timeout) as resp:
        return json.loads(resp.read().decode("utf-8", errors="replace"))


def _hunter(domain: str, timeout: float) -> dict[str, Any] | None:
    key = os.environ.get("HUNTER_API_KEY", "").strip()
    if not key:
        raise RuntimeError("HUNTER_API_KEY is not configured.")
    url = "https://api.hunter.io/v2/domain-search?" + urllib.parse.urlencode({"domain": domain, "api_key": key})
    data = _get_json(url, {}, timeout).get("data") or {}
    emails = [e for e in (data.get("emails") or []) if (e or {}).get("value")]
    if not emails:
        return None
    best = max(emails, key=lambda e: e.get("confidence") or 0)
    return {
        "email": best["value"],
        "source": "hunter",
        "confidence": best.get("confidence"),
        "type": best.get("type"),
        "first_name": best.get("first_name"),
        "last_name": best.get("last_name"),
        "position": best.get("position"),
    }


def _pdl(domain: str, timeout: float) -> dict[str, Any] | None:
    key = os.environ.get("PEOPLE_DATA_LABS_API_KEY", "").strip()
    if not key:
        raise RuntimeError("PEOPLE_DATA_LABS_API_KEY is not configured.")
    url = "https://api.peopledatalabs.com/v5/company/enrich?" + urllib.parse.urlencode({"website": domain})
    data = _get_json(url, {"X-Api-Key": key}, timeout)
    # Company records expose an email only on some plans; parse defensively.
    candidate = data.get("email")
    if not candidate:
        for item in data.get("emails") or []:
            if isinstance(item, dict) and item.get("address"):
                candidate = item["address"]
                break
    if not candidate:
        return None
    return {"email": candidate, "source": "pdl", "confidence": None, "company": data.get("name")}


def _clearbit(domain: str, timeout: float) -> dict[str, Any] | None:
    key = os.environ.get("CLEARBIT_API_KEY", "").strip()
    if not key:
        raise RuntimeError("CLEARBIT_API_KEY is not configured.")
    url = "https://company.clearbit.com/v2/companies/find?" + urllib.parse.urlencode({"domain": domain})
    data = _get_json(url, {"Authorization": f"Bearer {key}"}, timeout)
    candidate = data.get("email") or (data.get("company") or {}).get("email")
    if not candidate:
        return None
    return {"email": candidate, "source": "clearbit", "confidence": None, "company": data.get("name")}


def _providers() -> list[tuple[str, Callable[[str, float], dict[str, Any] | None]]]:
    # Resolve at call time so tests (and future plugins) can swap providers.
    return [("hunter", _hunter), ("pdl", _pdl), ("clearbit", _clearbit)]


def enrich_domain(domain: str, *, timeout: float = 20.0) -> dict[str, Any]:
    """Run the waterfall for ``domain`` and return the first address found.

    The result always carries ``attempts`` (one entry per provider) so a caller
    can see which keys are configured and why a provider was skipped.
    """
    domain = (domain or "").strip().lower().removeprefix("www.")
    if not domain or "." not in domain:
        raise ValueError("A registrable domain is required")
    cached = cache.get("email_enrich", [domain])
    if cached is not None:
        return cached

    attempts: list[dict[str, Any]] = []
    for name, provider in _providers():
        try:
            match = ratelimit.call(f"email_enrich_{name}", lambda p=provider: p(domain, timeout), min_interval=0.0)
        except Exception as exc:
            attempts.append({"provider": name, "status": "error", "error": str(exc)})
            continue
        if match:
            attempts.append({"provider": name, "status": "matched"})
            result = {"found": True, "domain": domain, "attempts": attempts, **match}
            cache.put("email_enrich", [domain], result)
            return result
        attempts.append({"provider": name, "status": "no_match"})

    result = {"found": False, "domain": domain, "attempts": attempts}
    cache.put("email_enrich", [domain], result)
    return result
