from __future__ import annotations
from typing import Any

HIGH_VALUE = {"dentist","clinic","lawyer","real_estate","accountant","hotel","beauty","spa","physiotherapy","car_dealer"}
BOOKING_VALUE = {"dentist","clinic","physiotherapy","beauty","hairdresser","barber","spa","hotel","restaurant"}

def calculate_intelligence(lead: dict[str,Any]) -> tuple[int,int,list[str]]:
    contact=0; commercial=0; reasons=[]
    if lead.get("phone"): contact+=25; reasons.append("Phone available")
    if lead.get("email"): contact+=30; reasons.append("Email available")
    socials=lead.get("social_links") or {}
    if socials: contact+=min(30,10+5*len(socials)); reasons.append(f"{len(socials)} social channel(s)")
    if lead.get("website"): contact+=10
    if lead.get("verification_status") in {"cross_source","verified"}: contact+=5; reasons.append("Cross-source verified")
    contact=min(100,contact)

    category=(lead.get("category") or "").lower()
    if category in HIGH_VALUE: commercial+=25; reasons.append("Higher-value service category")
    if lead.get("website_status")=="missing": commercial+=30; reasons.append("No site found in verified sources")
    elif lead.get("website_status")=="weak": commercial+=25; reasons.append("Weak website opportunity")
    if lead.get("mobile_ok") is False: commercial+=10
    if lead.get("has_cta") is False: commercial+=10
    if category in BOOKING_VALUE and lead.get("has_booking") is False: commercial+=8
    rating=lead.get("rating"); reviews=lead.get("review_count")
    if isinstance(rating,(int,float)) and rating>=4: commercial+=7
    if isinstance(reviews,int) and reviews>=20: commercial+=5
    if socials: commercial+=5
    commercial=min(100,commercial)
    return contact,commercial,reasons
