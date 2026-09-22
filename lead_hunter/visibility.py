from __future__ import annotations

from typing import Any


def _clamp(value: float) -> int:
    return max(0, min(100, round(value)))


def _add(reasons: list[str], condition: bool, points: int, positive: str, negative: str | None = None) -> int:
    if condition:
        reasons.append(f"+{points} {positive}")
        return points
    if negative:
        reasons.append(f"0/{points} {negative}")
    return 0


def calculate_visibility(
    evidence: dict[str, Any],
    *,
    commercial_score: int = 0,
) -> dict[str, Any]:
    """Calculate inspectable readiness scores.

    These are readiness/quality heuristics, not ranking or citation predictions.
    """

    indexable = bool(evidence.get("indexable", True))
    title = bool(evidence.get("title"))
    meta = bool(evidence.get("meta_description"))
    h1 = int(evidence.get("h1_count") or 0)
    h2_h3 = int(evidence.get("h2_h3_count") or 0)
    canonical = bool(evidence.get("canonical"))
    https = bool(evidence.get("has_https"))
    mobile = bool(evidence.get("mobile_ok"))
    performance = evidence.get("performance_score")
    lighthouse_seo = evidence.get("lighthouse_seo_score")
    word_count = int(evidence.get("word_count") or 0)
    question_headings = int(evidence.get("question_headings") or 0)
    business_schema = bool(evidence.get("business_schema"))
    organization_schema = bool(evidence.get("organization_schema"))
    schema_fields = set(evidence.get("schema_fields") or [])
    external_links = int(evidence.get("external_links") or 0)
    fact_signals = int(evidence.get("fact_signals") or 0)
    author_signal = bool(evidence.get("author_signal"))
    fresh_signal = bool(evidence.get("fresh_signal"))
    semantic_main = bool(evidence.get("semantic_main"))
    social_count = int(evidence.get("social_count") or 0)
    contact_signal = bool(evidence.get("contact_signal"))

    reasons: dict[str, list[str]] = {"seo": [], "aeo": [], "geo": [], "ai_visibility": [], "opportunity_gap": []}

    # SEO: technical discoverability + page fundamentals + local entity markup.
    seo_heuristic = 0
    seo_heuristic += _add(reasons["seo"], indexable, 15, "Page is indexable", "Page has a noindex signal")
    seo_heuristic += _add(reasons["seo"], https, 8, "HTTPS", "HTTPS missing")
    seo_heuristic += _add(reasons["seo"], mobile, 8, "Mobile viewport present", "Mobile viewport missing")
    seo_heuristic += _add(reasons["seo"], title, 10, "Page title present", "Page title missing")
    seo_heuristic += _add(reasons["seo"], meta, 8, "Meta description present", "Meta description missing")
    seo_heuristic += _add(reasons["seo"], h1 == 1, 8, "Single H1", "H1 structure is missing or ambiguous")
    seo_heuristic += _add(reasons["seo"], h2_h3 >= 2, 6, "Supporting heading structure", "Thin heading structure")
    seo_heuristic += _add(reasons["seo"], canonical, 6, "Canonical URL declared", "Canonical URL not detected")
    seo_heuristic += _add(reasons["seo"], business_schema or organization_schema, 12, "Business/entity structured data", "Business/entity structured data not detected")
    seo_heuristic += _add(
        reasons["seo"],
        isinstance(performance, (int, float)) and performance >= 60,
        9,
        "Usable performance signal",
        "Performance signal is weak or unavailable",
    )
    if isinstance(lighthouse_seo, (int, float)):
        seo = _clamp(float(lighthouse_seo) * 0.55 + seo_heuristic * 0.45)
        reasons["seo"].append(f"Lighthouse SEO contributes 55% ({round(float(lighthouse_seo))}/100)")
    else:
        seo = _clamp(seo_heuristic)

    # AEO: answer extractability, hierarchy and explicit business facts.
    aeo = 0
    aeo += _add(reasons["aeo"], indexable, 10, "Content can be indexed", "Noindex blocks answer discovery")
    aeo += _add(reasons["aeo"], title and h1 >= 1, 10, "Clear page topic", "Page topic is weakly defined")
    aeo += _add(reasons["aeo"], h2_h3 >= 3, 14, "Scannable answer hierarchy", "More descriptive subheadings would help")
    aeo += _add(reasons["aeo"], question_headings >= 1, 14, "Question-oriented headings", "No question-oriented headings detected")
    aeo += _add(reasons["aeo"], word_count >= 350, 14, "Enough explanatory content", "Content is thin")
    aeo += _add(reasons["aeo"], business_schema or organization_schema, 16, "Entity structured data supports explicit answers", "Entity structured data not detected")
    aeo += _add(reasons["aeo"], contact_signal, 10, "Concrete contact/action facts", "Contact/action facts are weak")
    aeo += _add(reasons["aeo"], semantic_main, 6, "Semantic main/article structure", "Semantic main/article structure not detected")
    aeo += _add(reasons["aeo"], meta, 6, "Concise page summary available", "Meta summary missing")
    aeo = _clamp(aeo)

    # GEO: machine-readable entity clarity, factual density, sourcing and freshness.
    geo = 0
    geo += _add(reasons["geo"], indexable, 10, "Content is crawlable/indexable", "Noindex reduces discoverability")
    geo += _add(reasons["geo"], business_schema or organization_schema, 18, "Explicit business/entity schema", "Entity schema not detected")
    geo += _add(reasons["geo"], len(schema_fields & {"name", "address", "telephone", "url", "logo"}) >= 3, 14, "Structured entity has multiple concrete fields", "Structured entity facts are sparse")
    geo += _add(reasons["geo"], "sameAs" in schema_fields or social_count >= 2, 10, "Entity identity is connected across profiles", "Cross-profile entity links are weak")
    geo += _add(reasons["geo"], external_links >= 2, 10, "Outbound source/citation links detected", "Few external source links detected")
    geo += _add(reasons["geo"], fact_signals >= 3, 12, "Concrete factual/numeric signals", "Few concrete factual signals")
    geo += _add(reasons["geo"], word_count >= 500, 10, "Substantive crawlable content", "Content depth is limited")
    geo += _add(reasons["geo"], author_signal, 8, "Authorship signal detected", "Authorship signal not detected")
    geo += _add(reasons["geo"], fresh_signal, 8, "Freshness/date signal detected", "Freshness signal is weak")
    geo = _clamp(geo)

    ai_visibility = _clamp(seo * 0.35 + aeo * 0.30 + geo * 0.35)
    reasons["ai_visibility"].append(f"35% SEO ({seo}) + 30% AEO ({aeo}) + 35% GEO ({geo})")
    if not indexable:
        ai_visibility = _clamp(ai_visibility * 0.45)
        reasons["ai_visibility"].append("Noindex penalty applied")

    # Sales opportunity: weak digital readiness + commercially attractive business.
    opportunity_gap = _clamp((100 - ai_visibility) * 0.75 + max(0, min(100, commercial_score)) * 0.25)
    reasons["opportunity_gap"].append(f"75% digital visibility gap ({100-ai_visibility}) + 25% commercial signal ({max(0,min(100,commercial_score))})")

    return {
        "seo_score": seo,
        "aeo_score": aeo,
        "geo_score": geo,
        "ai_visibility_score": ai_visibility,
        "opportunity_gap_score": opportunity_gap,
        "visibility_reasons": reasons,
        "visibility_signals": evidence,
        "score_note": "Readiness heuristics, not search ranking or AI citation predictions.",
    }


def missing_website_visibility(*, commercial_score: int = 0, verified: bool = False) -> dict[str, Any]:
    if not verified:
        return {
            "seo_score": None,
            "aeo_score": None,
            "geo_score": None,
            "ai_visibility_score": None,
            "opportunity_gap_score": None,
            "visibility_reasons": {
                "seo": ["Website not yet verified as absent."],
                "aeo": ["Website not yet verified as absent."],
                "geo": ["Website not yet verified as absent."],
                "ai_visibility": ["Website not yet verified as absent."],
                "opportunity_gap": ["Verify website absence before assigning a full digital gap."],
            },
            "visibility_signals": {"website_missing": True, "verified": False},
        }

    gap = _clamp(75 + max(0, min(100, commercial_score)) * 0.25)
    return {
        "seo_score": 0,
        "aeo_score": 0,
        "geo_score": 0,
        "ai_visibility_score": 0,
        "opportunity_gap_score": gap,
        "visibility_reasons": {
            "seo": ["Verified: no independent website available to optimize."],
            "aeo": ["Verified: no independent website available to answer from."],
            "geo": ["Verified: no independent website entity/content surface found."],
            "ai_visibility": ["Verified website absence creates a maximum owned-web visibility gap."],
            "opportunity_gap": [f"Verified website absence + commercial signal ({commercial_score})."],
        },
        "visibility_signals": {"website_missing": True, "verified": True},
    }
