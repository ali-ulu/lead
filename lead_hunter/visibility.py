from __future__ import annotations

from bisect import bisect_right
from typing import Any

from . import config


def _clamp(value: float) -> int:
    return max(0, min(100, round(value)))


def _add(reasons: list[str], condition: bool, points: float, positive: str, negative: str | None = None) -> float:
    if condition:
        reasons.append(f"+{points:g} {positive}")
        return points
    if negative:
        reasons.append(f"0/{points:g} {negative}")
    return 0


def _present(value: Any) -> bool:
    return isinstance(value, (int, float)) and not isinstance(value, bool)


def calculate_visibility(
    evidence: dict[str, Any],
    *,
    commercial_score: int = 0,
) -> dict[str, Any]:
    """Calculate inspectable readiness scores.

    These are readiness/quality heuristics, not ranking or citation predictions.
    Weights come from :mod:`lead_hunter.config` so they can be tuned without
    touching the formula.
    """
    w = config.visibility_weights()
    seo_w, aeo_w, geo_w = w["seo"], w["aeo"], w["geo"]
    aio_w, gap_w = w["ai_visibility"], w["opportunity_gap"]

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
    accessibility = evidence.get("accessibility_score")
    best_practices = evidence.get("best_practices_score")
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
    seo_heuristic = 0.0
    seo_heuristic += _add(reasons["seo"], indexable, seo_w["indexable"], "Page is indexable", "Page has a noindex signal")
    seo_heuristic += _add(reasons["seo"], https, seo_w["https"], "HTTPS", "HTTPS missing")
    seo_heuristic += _add(reasons["seo"], mobile, seo_w["mobile"], "Mobile viewport present", "Mobile viewport missing")
    seo_heuristic += _add(reasons["seo"], title, seo_w["title"], "Page title present", "Page title missing")
    seo_heuristic += _add(reasons["seo"], meta, seo_w["meta"], "Meta description present", "Meta description missing")
    seo_heuristic += _add(reasons["seo"], h1 == 1, seo_w["h1"], "Single H1", "H1 structure is missing or ambiguous")
    seo_heuristic += _add(reasons["seo"], h2_h3 >= 2, seo_w["headings"], "Supporting heading structure", "Thin heading structure")
    seo_heuristic += _add(reasons["seo"], canonical, seo_w["canonical"], "Canonical URL declared", "Canonical URL not detected")
    seo_heuristic += _add(reasons["seo"], business_schema or organization_schema, seo_w["schema"], "Business/entity structured data", "Business/entity structured data not detected")
    seo_heuristic += _add(
        reasons["seo"],
        _present(performance) and float(performance) >= config.PERFORMANCE_GOOD,
        seo_w["performance"],
        "Usable performance signal",
        "Performance signal is weak or unavailable",
    )
    if _present(lighthouse_seo):
        share = float(seo_w["lighthouse_share"])
        seo = _clamp(float(lighthouse_seo) * share + seo_heuristic * (1 - share))
        reasons["seo"].append(f"Lighthouse SEO contributes {round(share * 100)}% ({round(float(lighthouse_seo))}/100)")
    else:
        seo = _clamp(seo_heuristic)

    # AEO: answer extractability, hierarchy and explicit business facts.
    aeo = 0.0
    aeo += _add(reasons["aeo"], indexable, aeo_w["indexable"], "Content can be indexed", "Noindex blocks answer discovery")
    aeo += _add(reasons["aeo"], title and h1 >= 1, aeo_w["topic"], "Clear page topic", "Page topic is weakly defined")
    aeo += _add(reasons["aeo"], h2_h3 >= 3, aeo_w["hierarchy"], "Scannable answer hierarchy", "More descriptive subheadings would help")
    aeo += _add(reasons["aeo"], question_headings >= 1, aeo_w["questions"], "Question-oriented headings", "No question-oriented headings detected")
    aeo += _add(reasons["aeo"], word_count >= 350, aeo_w["depth"], "Enough explanatory content", "Content is thin")
    aeo += _add(reasons["aeo"], business_schema or organization_schema, aeo_w["schema"], "Entity structured data supports explicit answers", "Entity structured data not detected")
    aeo += _add(reasons["aeo"], contact_signal, aeo_w["contact"], "Concrete contact/action facts", "Contact/action facts are weak")
    aeo += _add(reasons["aeo"], semantic_main, aeo_w["semantic"], "Semantic main/article structure", "Semantic main/article structure not detected")
    aeo += _add(reasons["aeo"], meta, aeo_w["summary"], "Concise page summary available", "Meta summary missing")
    if _present(accessibility):
        aeo += _add(reasons["aeo"], float(accessibility) >= 90, aeo_w["accessibility"], "Accessible markup supports extraction", "Accessibility score is low")
    aeo = _clamp(aeo)

    # GEO: machine-readable entity clarity, factual density, sourcing and freshness.
    geo = 0.0
    geo += _add(reasons["geo"], indexable, geo_w["indexable"], "Content is crawlable/indexable", "Noindex reduces discoverability")
    geo += _add(reasons["geo"], business_schema or organization_schema, geo_w["schema"], "Explicit business/entity schema", "Entity schema not detected")
    geo += _add(reasons["geo"], len(schema_fields & {"name", "address", "telephone", "url", "logo"}) >= 3, geo_w["fields"], "Structured entity has multiple concrete fields", "Structured entity facts are sparse")
    geo += _add(reasons["geo"], "sameAs" in schema_fields or social_count >= 2, geo_w["identity"], "Entity identity is connected across profiles", "Cross-profile entity links are weak")
    geo += _add(reasons["geo"], external_links >= 2, geo_w["external"], "Outbound source/citation links detected", "Few external source links detected")
    geo += _add(reasons["geo"], fact_signals >= 3, geo_w["facts"], "Concrete factual/numeric signals", "Few concrete factual signals")
    geo += _add(reasons["geo"], word_count >= 500, geo_w["depth"], "Substantive crawlable content", "Content depth is limited")
    geo += _add(reasons["geo"], author_signal, geo_w["author"], "Authorship signal detected", "Authorship signal not detected")
    geo += _add(reasons["geo"], fresh_signal, geo_w["fresh"], "Freshness/date signal detected", "Freshness signal is weak")
    if _present(best_practices):
        geo += _add(reasons["geo"], float(best_practices) >= 90, geo_w["best_practices"], "Clean technical baseline for machine consumption", "Technical best-practices score is low")
    geo = _clamp(geo)

    ai_visibility = _clamp(seo * aio_w["seo"] + aeo * aio_w["aeo"] + geo * aio_w["geo"])
    reasons["ai_visibility"].append(f"{round(aio_w['seo'] * 100)}% SEO ({seo}) + {round(aio_w['aeo'] * 100)}% AEO ({aeo}) + {round(aio_w['geo'] * 100)}% GEO ({geo})")
    if not indexable:
        ai_visibility = _clamp(ai_visibility * aio_w["noindex_penalty"])
        reasons["ai_visibility"].append("Noindex penalty applied")

    # Sales opportunity: weak digital readiness + commercially attractive business.
    commercial = max(0, min(100, commercial_score))
    opportunity_gap = _clamp((100 - ai_visibility) * gap_w["visibility"] + commercial * gap_w["commercial"])
    reasons["opportunity_gap"].append(f"{round(gap_w['visibility'] * 100)}% digital visibility gap ({100 - ai_visibility}) + {round(gap_w['commercial'] * 100)}% commercial signal ({commercial})")

    return {
        "seo_score": seo,
        "aeo_score": aeo,
        "geo_score": geo,
        "ai_visibility_score": ai_visibility,
        "opportunity_gap_score": opportunity_gap,
        "score_confidence": {
            "seo": "lighthouse" if _present(lighthouse_seo) else "heuristic",
            "aeo": "lighthouse" if _present(accessibility) else "heuristic",
            "geo": "lighthouse" if _present(best_practices) else "heuristic",
            "ai_visibility": "composite",
        },
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
            "score_confidence": {
                "seo": "unverified", "aeo": "unverified", "geo": "unverified",
                "ai_visibility": "unverified",
            },
            "visibility_reasons": {
                "seo": ["Website not yet verified as absent."],
                "aeo": ["Website not yet verified as absent."],
                "geo": ["Website not yet verified as absent."],
                "ai_visibility": ["Website not yet verified as absent."],
                "opportunity_gap": ["Verify website absence before assigning a full digital gap."],
            },
            "visibility_signals": {"website_missing": True, "verified": False},
        }

    commercial = max(0, min(100, commercial_score))
    gap = _clamp(75 + commercial * 0.25)
    return {
        "seo_score": 0,
        "aeo_score": 0,
        "geo_score": 0,
        "ai_visibility_score": 0,
        "opportunity_gap_score": gap,
        "score_confidence": {
            "seo": "verified_missing", "aeo": "verified_missing",
            "geo": "verified_missing", "ai_visibility": "verified_missing",
        },
        "visibility_reasons": {
            "seo": ["Verified: no independent website available to optimize."],
            "aeo": ["Verified: no independent website available to answer from."],
            "geo": ["Verified: no independent website entity/content surface found."],
            "ai_visibility": ["Verified website absence creates a maximum owned-web visibility gap."],
            "opportunity_gap": [f"Verified website absence + commercial signal ({commercial_score})."],
        },
        "visibility_signals": {"website_missing": True, "verified": True},
    }


def percentile_rank(value: int | None, population: list[int]) -> int | None:
    """Position of ``value`` within ``population`` as a 0-100 percentile.

    Absolute readiness scores are not comparable across markets: a 60 in a city
    of strong sites is a weak position, the same 60 in a weak market is a
    leader. This returns the share of the sample at or below ``value`` so a
    report can say "top 20% here" without another scoring pass.
    """
    if value is None:
        return None
    sample = sorted(int(x) for x in population if x is not None)
    if not sample:
        return None
    return round(bisect_right(sample, int(value)) / len(sample) * 100)


def add_relative_scores(
    rows: list[dict[str, Any]],
    *,
    field: str = "ai_visibility_score",
) -> list[dict[str, Any]]:
    """Annotate each row with a market-relative percentile for ``field``.

    Rows without the score (unverified or un-audited) are left untouched so a
    missing measurement is never presented as a position.
    """
    population = [r.get(field) for r in rows if r.get(field) is not None]
    for row in rows:
        value = row.get(field)
        row[f"{field}_percentile"] = percentile_rank(value, population)
    return rows

