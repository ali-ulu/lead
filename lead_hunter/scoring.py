from __future__ import annotations
from typing import Any

BOOKING_RELEVANT = {"dentist", "beauty", "hairdresser", "hotel", "restaurant", "clinic", "spa", "barber", "physiotherapy"}

def calculate_score(lead: dict[str, Any]) -> tuple[int, list[str]]:
    score = 0
    reasons: list[str] = []
    website = (lead.get("website") or "").strip()
    status = (lead.get("website_status") or "unknown").lower()
    if not website or status == "missing":
        score += 35; reasons.append("No known independent website (+35)")
    else:
        perf = lead.get("performance_score")
        if isinstance(perf, (int, float)) and perf < 40: score += 15; reasons.append("Very low website performance (+15)")
        if lead.get("mobile_ok") is False: score += 15; reasons.append("Mobile usability problem (+15)")
        if lead.get("has_cta") is False: score += 10; reasons.append("No clear conversion CTA (+10)")
        if (lead.get("category") or "").lower() in BOOKING_RELEVANT and lead.get("has_booking") is False: score += 5; reasons.append("No visible booking/reservation path (+5)")
        seo = lead.get("seo_score")
        if isinstance(seo, (int, float)) and seo < 60: score += 5; reasons.append("Weak SEO audit score (+5)")
        if lead.get("has_https") is False: score += 10; reasons.append("HTTPS missing (+10)")
    if lead.get("phone"): score += 8; reasons.append("Phone contact available (+8)")
    if lead.get("email"): score += 8; reasons.append("Email contact available (+8)")
    if lead.get("social_url"): score += 5; reasons.append("Social presence available (+5)")
    confidence = (lead.get("data_confidence") or "").lower()
    if confidence == "high": score += 4; reasons.append("High source confidence (+4)")
    elif confidence == "medium": score += 2; reasons.append("Medium source confidence (+2)")
    return min(100, score), reasons
