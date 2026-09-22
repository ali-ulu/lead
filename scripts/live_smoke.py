#!/usr/bin/env python3
"""End-to-end live provider verification for LeadScout 4.0."""
from __future__ import annotations

import json
import time

from lead_hunter.db import clear_all, initialize
from lead_hunter.services import discover_businesses, query_leads

VERIFY_RELEASE = "4.0.0"

CASES = [
    ("Karachi", "Pakistan", "restaurant", 20),
    ("Karachi", "Pakistan", "beauty", 20),
    ("Istanbul", "Türkiye", "dentist", 15),
    ("Berlin", "Germany", "dentist", 15),
    ("London", "United Kingdom", "restaurant", 15),
]


def main() -> int:
    initialize()
    clear_all()
    results = []

    for city, country, category, radius in CASES:
        try:
            result = discover_businesses(
                city=city,
                country=country,
                category=category,
                radius_km=radius,
                max_results=None,
            )
            search_id = result["search_id"]
            leads = query_leads(search_id=search_id)

            if len(leads) != result["count"]:
                raise RuntimeError(
                    f"search_id mismatch: discovery={result['count']} stored={len(leads)}"
                )

            row = {
                "city": city,
                "country": country,
                "category": category,
                "radius_km": radius,
                "search_id": search_id,
                "count": len(leads),
                "with_website": sum(bool(x.get("website")) for x in leads),
                "with_phone": sum(bool(x.get("phone")) for x in leads),
                "with_email": sum(bool(x.get("email")) for x in leads),
                "with_social": sum(bool(x.get("social_links")) for x in leads),
                "ids_preview_count": len(result.get("ids") or []),
                "ids_truncated": bool(result.get("ids_truncated")),
                "sample_names": [x["name"] for x in leads[:5]],
            }
        except Exception as exc:
            row = {
                "city": city,
                "country": country,
                "category": category,
                "radius_km": radius,
                "error": repr(exc),
                "count": 0,
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

    if not any(x.get("count", 0) > 250 for x in results):
        print("FAIL: no dense case exceeded the former 250-result application cap", flush=True)
        return 5

    dense = [x for x in results if x.get("count", 0) > 500]
    if dense and not all(x.get("ids_truncated") for x in dense):
        print("FAIL: large searches did not expose truncated ID previews as designed", flush=True)
        return 6

    verified_cases = sum(1 for x in results if x.get("count", 0) > 0)
    if verified_cases != len(results):
        print(
            f"FAIL: only {verified_cases}/{len(results)} live market/category cases returned data",
            flush=True,
        )
        return 4

    print(
        f"PASS: LeadScout {VERIFY_RELEASE} completed {verified_cases}/{len(results)} "
        "live end-to-end searches, exceeded 250 results, and verified search_id isolation",
        flush=True,
    )
    clear_all()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
