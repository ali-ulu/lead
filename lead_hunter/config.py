"""Central scoring configuration: weights, thresholds and env overrides.

The visibility and lead scores were previously hard-coded inside the scoring
functions, which made them impossible to tune and easy to drift apart (the
performance "weak" threshold lived in two places with two values). This module
is the single source of truth. Defaults preserve the shipped behaviour; the
weights can be overridden at runtime with a JSON env var so a deployment can
retune without a code change.
"""
from __future__ import annotations

import json
import os
from typing import Any

# --- shared thresholds -----------------------------------------------------

PERFORMANCE_WEAK = 40      # lead_score: "very low website performance"
PERFORMANCE_GOOD = 60      # visibility: "usable performance signal"
SEO_WEAK = 60              # lead_score: "weak SEO audit score"
SPEED_FLAG_MS = 2500       # audit quality flag: slow time-to-first-byte


# --- default weights -------------------------------------------------------

DEFAULT_VISIBILITY_WEIGHTS: dict[str, dict[str, float]] = {
    "seo": {
        "indexable": 15,
        "https": 8,
        "mobile": 8,
        "title": 10,
        "meta": 8,
        "h1": 8,
        "headings": 6,
        "canonical": 6,
        "schema": 12,
        "performance": 9,
        "lighthouse_share": 0.55,
    },
    "aeo": {
        "indexable": 10,
        "topic": 10,
        "hierarchy": 14,
        "questions": 14,
        "depth": 14,
        "schema": 16,
        "contact": 10,
        "semantic": 6,
        "summary": 6,
        "accessibility": 8,
    },
    "geo": {
        "indexable": 10,
        "schema": 18,
        "fields": 14,
        "identity": 10,
        "external": 10,
        "facts": 12,
        "depth": 10,
        "author": 8,
        "fresh": 8,
        "best_practices": 6,
    },
    "ai_visibility": {"seo": 0.35, "aeo": 0.30, "geo": 0.35, "noindex_penalty": 0.45},
    "opportunity_gap": {"visibility": 0.75, "commercial": 0.25},
}

DEFAULT_SCORE_WEIGHTS: dict[str, float] = {
    "no_website": 35,
    "low_performance": 15,
    "mobile_problem": 15,
    "no_cta": 10,
    "no_booking": 5,
    "weak_seo": 5,
    "no_https": 10,
    "phone": 8,
    "email": 8,
    "social": 5,
    "confidence_high": 4,
    "confidence_medium": 2,
}


def _env_json(name: str) -> dict[str, Any]:
    raw = os.environ.get(name, "").strip()
    if not raw:
        return {}
    try:
        value = json.loads(raw)
    except ValueError:
        return {}
    return value if isinstance(value, dict) else {}


def _deep_merge(base: dict[str, Any], override: dict[str, Any]) -> dict[str, Any]:
    merged = {k: (dict(v) if isinstance(v, dict) else v) for k, v in base.items()}
    for key, value in override.items():
        if isinstance(value, dict) and isinstance(merged.get(key), dict):
            merged[key] = {**merged[key], **value}
        else:
            merged[key] = value
    return merged


def visibility_weights() -> dict[str, dict[str, float]]:
    """Default visibility weights merged with ``LEADSCOUT_VISIBILITY_WEIGHTS``."""
    return _deep_merge(DEFAULT_VISIBILITY_WEIGHTS, _env_json("LEADSCOUT_VISIBILITY_WEIGHTS"))


def score_weights() -> dict[str, float]:
    """Default lead-score weights merged with ``LEADSCOUT_SCORE_WEIGHTS``."""
    return _deep_merge(DEFAULT_SCORE_WEIGHTS, _env_json("LEADSCOUT_SCORE_WEIGHTS"))
