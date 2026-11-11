"""PageSpeed Insights provider.

Lighthouse needs a local headless Chrome and takes tens of seconds. Google's
PageSpeed Insights API runs the same audit remotely and also returns CrUX field
data (real-user metrics), so it is a good fallback when Chrome is unavailable
or when field data is wanted.

Works without a key (the anonymous quota is small); set ``GOOGLE_PSI_API_KEY``
to use a key with a larger quota.
"""
from __future__ import annotations

import json
import os
import urllib.parse
import urllib.request
from typing import Any

from .. import netguard

ENDPOINT = "https://www.googleapis.com/pagespeedonline/v5/runPagespeed"
USER_AGENT = "LeadScout/7.0 (local business research tool)"
CATEGORIES = ("performance", "accessibility", "best-practices", "seo")


def configured() -> bool:
    # Opt-in: the anonymous quota is tiny and shared, so the fallback only runs
    # when explicitly enabled (or a key is set).
    if os.environ.get("GOOGLE_PSI_API_KEY", "").strip():
        return True
    return os.environ.get("LEADSCOUT_PAGESPEED", "").strip() == "1"


def _score(categories: dict[str, Any], name: str) -> int | None:
    value = (categories.get(name) or {}).get("score")
    return round(float(value) * 100) if value is not None else None


def _field_data(payload: dict[str, Any]) -> dict[str, Any] | None:
    """Extract CrUX field metrics from the loadingExperience block."""
    experience = payload.get("loadingExperience") or payload.get("originLoadingExperience") or {}
    metrics = experience.get("metrics") or {}
    if not metrics:
        return None
    out: dict[str, Any] = {}
    for key, value in metrics.items():
        if isinstance(value, dict):
            out[key] = {
                "percentile": value.get("percentile"),
                "category": value.get("category"),
            }
    overall = experience.get("overall_category")
    if overall:
        out["overall_category"] = overall
    return out or None


def parse_response(payload: dict[str, Any]) -> dict[str, Any]:
    """Map a runPagespeed response into the audit score shape."""
    report = payload.get("lighthouseResult") or {}
    categories = report.get("categories") or {}
    return {
        "performance_score": _score(categories, "performance"),
        "accessibility_score": _score(categories, "accessibility"),
        "best_practices_score": _score(categories, "best-practices"),
        "seo_score": _score(categories, "seo"),
        "lighthouse_version": report.get("lighthouseVersion"),
        "field_data": _field_data(payload),
        "source": "pagespeed",
    }


def analyze(url: str, *, strategy: str = "mobile", timeout: float = 60.0) -> dict[str, Any]:
    """Run PageSpeed Insights for ``url`` and return audit scores."""
    if not configured():
        raise RuntimeError("PageSpeed Insights is disabled (LEADSCOUT_PAGESPEED=0).")
    params: list[tuple[str, str]] = [("url", url), ("strategy", strategy)]
    params.extend(("category", c) for c in CATEGORIES)
    key = os.environ.get("GOOGLE_PSI_API_KEY", "").strip()
    if key:
        params.append(("key", key))
    request = urllib.request.Request(
        ENDPOINT + "?" + urllib.parse.urlencode(params),
        headers={"User-Agent": USER_AGENT, "Accept": "application/json"},
    )
    with netguard.build_public_opener(check_robots=False).open(request, timeout=timeout) as resp:
        payload = json.loads(resp.read().decode("utf-8", errors="replace"))
    if "error" in payload:
        message = (payload["error"] or {}).get("message") or "PageSpeed Insights returned an error."
        raise RuntimeError(message)
    return parse_response(payload)
