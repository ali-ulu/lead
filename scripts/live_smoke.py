#!/usr/bin/env python3
"""Live provider smoke test.

Uses Nishan's real geocoder/provider code against public endpoints.
This is intentionally not part of every CI run.
"""
from __future__ import annotations
import json
import time
from lead_hunter.providers.nominatim import geocode_area
from lead_hunter.providers.osm import search_around\n\nVERIFY_RELEASE = "3.1.0"

CASES = [
    ("Karachi", "Pakistan", "dentist", 20),
    ("Karachi", "Pakistan", "restaurant", 20),
    ("Karachi", "Pakistan", "beauty", 20),
    ("Istanbul", "Türkiye", "dentist", 15),
    ("Berlin", "Germany", "dentist", 15),
    ("London", "United Kingdom", "restaurant", 15),
]

def main() -> int:
    results = []
    geocodes = {}
    for city, country, category, radius in CASES:
        key = (city, country)
        try:
            if key not in geocodes:
                geocodes[key] = geocode_area(city, country, timeout=25)
                time.sleep(1.2)
            area = geocodes[key]
            leads = search_around(
                area["lat"], area["lon"], radius, category,
                city=area["city"], country=area["country"], timeout=40, limit=250,
            )
            row = {
                "city": city,
                "country": country,
                "category": category,
                "radius_km": radius,
                "geocoded_city": area["city"],
                "count": len(leads),
                "with_website": sum(bool(x.get("website")) for x in leads),
                "with_phone": sum(bool(x.get("phone")) for x in leads),
                "with_email": sum(bool(x.get("email")) for x in leads),
                "with_social": sum(bool(x.get("social_links")) for x in leads),
                "sample_names": [x["name"] for x in leads[:5]],
            }
        except Exception as exc:
            row = {
                "city": city, "country": country, "category": category,
                "radius_km": radius, "error": repr(exc), "count": 0,
            }
        results.append(row)
        print("SMOKE", json.dumps(row, ensure_ascii=False), flush=True)
        time.sleep(2)

    print("SUMMARY", json.dumps(results, ensure_ascii=False), flush=True)

    pakistan = [x for x in results if x["country"] == "Pakistan"]
    other_markets = [x for x in results if x["country"] != "Pakistan"]
    if not any(x.get("count", 0) > 0 for x in pakistan):
        print("FAIL: Karachi/Pakistan returned no live business data", flush=True)
        return 3
    if not any(x.get("count", 0) > 0 for x in other_markets):
        print("FAIL: no non-Pakistan market returned data", flush=True)
        return 2
    verified_markets = sum(1 for x in results if x.get("count", 0) > 0)
    if verified_markets < 4:
        print(f"FAIL: only {verified_markets} live market/category cases returned data", flush=True)
        return 4
    print(f"PASS: {verified_markets}/{len(results)} live cases returned data", flush=True)
    return 0

if __name__ == "__main__":
    raise SystemExit(main())
