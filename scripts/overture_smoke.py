#!/usr/bin/env python3
from __future__ import annotations

import json

from lead_hunter.providers.overture import latest_release, search_bbox


def main() -> int:
    # Central Berlin, deliberately compact bbox.
    rows = search_bbox(
        13.32, 52.47, 13.46, 52.56,
        "restaurant",
        city="Berlin",
        country="Germany",
        limit=50,
    )
    payload = {
        "release": latest_release(),
        "count": len(rows),
        "with_website": sum(bool(x.get("website")) for x in rows),
        "with_phone": sum(bool(x.get("phone")) for x in rows),
        "with_email": sum(bool(x.get("email")) for x in rows),
        "with_social": sum(bool(x.get("social_links")) for x in rows),
        "samples": [x.get("name") for x in rows[:5]],
    }
    print("OVERTURE_SMOKE", json.dumps(payload, ensure_ascii=False), flush=True)
    if not rows:
        print("FAIL: Overture returned zero Berlin restaurants", flush=True)
        return 2
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
