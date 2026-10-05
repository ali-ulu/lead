from __future__ import annotations
import json
import urllib.parse
import urllib.request
import os
from typing import Any

from .. import cache
from .. import ratelimit

NOMINATIM_URL = "https://nominatim.openstreetmap.org/search"
USER_AGENT = "LeadScout/4.0 (local business research tool)"
# Nominatim's usage policy asks for at most one request per second.
_MIN_INTERVAL = float(os.environ.get("LEADSCOUT_NOMINATIM_MIN_INTERVAL", "1.0") or 1.0)

def geocode_area(city: str, country: str = "", timeout: int = 20) -> dict[str, Any]:
    query = ", ".join(x.strip() for x in (city, country) if x and x.strip())
    if not query:
        raise ValueError("City or area is required")
    cached = cache.get("nominatim", [query])
    if cached is not None:
        return cached
    params = urllib.parse.urlencode({"q": query, "format": "jsonv2", "limit": 1, "addressdetails": 1})
    req = urllib.request.Request(
        f"{NOMINATIM_URL}?{params}",
        headers={"User-Agent": USER_AGENT, "Accept-Language": "en"},
    )

    def _do() -> list[dict[str, Any]]:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            return json.loads(resp.read().decode("utf-8"))

    # Nominatim's usage policy asks for at most one request per second.
    rows = ratelimit.call("nominatim", _do, min_interval=_MIN_INTERVAL)
    if not rows:
        raise LookupError(f"Location not found: {query}")
    row = rows[0]
    bbox = [float(x) for x in row["boundingbox"]]
    address = row.get("address") or {}
    result = {
        "display_name": row.get("display_name") or query,
        "lat": float(row["lat"]),
        "lon": float(row["lon"]),
        "bbox": {"south": bbox[0], "north": bbox[1], "west": bbox[2], "east": bbox[3]},
        "country": address.get("country") or country,
        "country_code": (address.get("country_code") or "").upper(),
        "city": address.get("city") or address.get("town") or address.get("municipality") or address.get("county") or city,
    }
    cache.put("nominatim", [query], result)
    return result
