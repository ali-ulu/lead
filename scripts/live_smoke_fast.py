#!/usr/bin/env python3
from __future__ import annotations

import json

from lead_hunter.db import clear_all, initialize
from lead_hunter.services import discover_businesses, query_leads

CASES = [
    ("Karachi", "Pakistan", "dentist", 20, 1),
    ("Istanbul", "Türkiye", "restaurant", 8, 251),
    ("Berlin", "Germany", "dentist", 10, 1),
]


def main() -> int:
    initialize()
    clear_all()
    rows = []

    for city, country, category, radius_km, minimum in CASES:
        result = discover_businesses(
            city=city,
            country=country,
            category=category,
            radius_km=radius_km,
            max_results=None,
        )
        leads = query_leads(search_id=result["search_id"])
        row = {
            "city": city,
            "country": country,
            "category": category,
            "radius_km": radius_km,
            "count": len(leads),
            "search_id": result["search_id"],
            "minimum": minimum,
            "providers": result.get("providers") or {},
            "partial": bool(result.get("partial")),
            "warnings": result.get("warnings") or [],
            "sample": [x["name"] for x in leads[:3]],
        }
        rows.append(row)
        print("FAST_SMOKE", json.dumps(row, ensure_ascii=False), flush=True)

        if len(leads) < minimum:
            print(
                f"FAIL: {city}/{category} returned {len(leads)}, expected >= {minimum}",
                flush=True,
            )
            return 2
        providers = result.get("providers") or {}
        if "osm" not in providers or "overture" not in providers:
            print(f"FAIL: multi-source provider summary missing for {city}: {providers}", flush=True)
            return 3
        if providers.get("overture", {}).get("count", 0) <= 0:
            print(f"FAIL: Overture returned no data inside combined search for {city}", flush=True)
            return 4

    print("PASS", json.dumps(rows, ensure_ascii=False), flush=True)
    clear_all()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
