#!/usr/bin/env python3
from __future__ import annotations

import json

from lead_hunter.providers.overture import latest_release, search_bbox


CASES = [
    ("Berlin", "Germany", 13.32, 52.47, 13.46, 52.56, "restaurant"),
    ("Karachi", "Pakistan", 66.93, 24.79, 67.16, 24.98, "restaurant"),
]


def main() -> int:
    all_results = []
    for city, country, west, south, east, north, category in CASES:
        rows = search_bbox(
            west, south, east, north, category,
            city=city, country=country, limit=50,
        )
        payload = {
            "release": latest_release(),
            "city": city,
            "country": country,
            "category": category,
            "count": len(rows),
            "with_website": sum(bool(x.get("website")) for x in rows),
            "with_phone": sum(bool(x.get("phone")) for x in rows),
            "with_email": sum(bool(x.get("email")) for x in rows),
            "with_social": sum(bool(x.get("social_links")) for x in rows),
            "samples": [x.get("name") for x in rows[:5]],
        }
        print("OVERTURE_SMOKE", json.dumps(payload, ensure_ascii=False), flush=True)
        if not rows:
            print(f"FAIL: Overture returned zero {city} {category} records", flush=True)
            return 2
        all_results.append(payload)

    print("PASS", json.dumps(all_results, ensure_ascii=False), flush=True)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
