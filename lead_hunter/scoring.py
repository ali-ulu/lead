from __future__ import annotations
from typing import Any

from . import config

BOOKING_RELEVANT = {"dentist", "beauty", "hairdresser", "hotel", "restaurant", "clinic", "spa", "barber", "physiotherapy"}

def calculate_score(lead: dict[str, Any]) -> tuple[int, list[str]]:
    w = config.score_weights()
    score = 0.0
    reasons: list[str] = []
    website = (lead.get("website") or "").strip()
    status = (lead.get("website_status") or "unknown").lower()
    if not website or status == "missing":
        score += w["no_website"]; reasons.append(f"No known independent website (+{w['no_website']:g})")
    else:
        perf = lead.get("performance_score")
        if isinstance(perf, (int, float)) and perf < config.PERFORMANCE_WEAK:
            score += w["low_performance"]; reasons.append(f"Very low website performance (+{w['low_performance']:g})")
        if lead.get("mobile_ok") is False:
            score += w["mobile_problem"]; reasons.append(f"Mobile usability problem (+{w['mobile_problem']:g})")
        if lead.get("has_cta") is False:
            score += w["no_cta"]; reasons.append(f"No clear conversion CTA (+{w['no_cta']:g})")
        if (lead.get("category") or "").lower() in BOOKING_RELEVANT and lead.get("has_booking") is False:
            score += w["no_booking"]; reasons.append(f"No visible booking/reservation path (+{w['no_booking']:g})")
        seo = lead.get("seo_score")
        if isinstance(seo, (int, float)) and seo < config.SEO_WEAK:
            score += w["weak_seo"]; reasons.append(f"Weak SEO audit score (+{w['weak_seo']:g})")
        if lead.get("has_https") is False:
            score += w["no_https"]; reasons.append(f"HTTPS missing (+{w['no_https']:g})")
    if lead.get("phone"): score += w["phone"]; reasons.append(f"Phone contact available (+{w['phone']:g})")
    if lead.get("email"): score += w["email"]; reasons.append(f"Email contact available (+{w['email']:g})")
    if lead.get("social_url"): score += w["social"]; reasons.append(f"Social presence available (+{w['social']:g})")
    confidence = (lead.get("data_confidence") or "").lower()
    if confidence == "high":
        score += w["confidence_high"]; reasons.append(f"High source confidence (+{w['confidence_high']:g})")
    elif confidence == "medium":
        score += w["confidence_medium"]; reasons.append(f"Medium source confidence (+{w['confidence_medium']:g})")
    return min(100, round(score)), reasons
