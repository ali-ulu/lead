"""Photon (Komoot) geocoding provider.

Keyless fallback for Nominatim. Photon is OSM-backed and exposes a free public
endpoint, so it works out of the box when Nominatim is rate-limited or down.
"""
from __future__ import annotations

import json
import urllib.parse
import urllib.request
from typing import Any

from .. import cache
from .. import ratelimit

PHOTON_URL = "https://photon.komoot.io/api/"
USER_AGENT = "LeadScout/7.0 (local business research tool)"
# The public instance is best-effort; keep requests spaced out.
_MIN_INTERVAL = 1.0


def _row_to_area(row: dict[str, Any], city: str, country: str) -> dict[str, Any]:
    props = row.get("properties") or {}
    geometry = row.get("geometry") or {}
    coords = geometry.get("coordinates") or []
    lon = float(coords[0]) if len(coords) > 0 else 0.0
    lat = float(coords[1]) if len(coords) > 1 else 0.0
    extent = props.get("extent")
    bbox: dict[str, float] = {}
    if isinstance(extent, list) and len(extent) == 4:
        # Photon extent is [west, north, east, south].
        west, north, east, south = (float(x) for x in extent)
        bbox = {"south": south, "north": north, "west": west, "east": east}
    elif lat or lon:
        # A point result carries no extent; derive a small box so the caller
        # still gets a usable search area instead of a hard failure.
        pad = 0.05
        bbox = {"south": lat - pad, "north": lat + pad, "west": lon - pad, "east": lon + pad}
    name = props.get("name") or city
    return {
        "display_name": ", ".join(x for x in (name, props.get("city"), props.get("country")) if x) or city,
        "lat": lat,
        "lon": lon,
        "bbox": bbox,
        "country": props.get("country") or country,
        "country_code": (props.get("countrycode") or "").upper(),
        "city": props.get("city") or props.get("district") or props.get("county") or city,
    }


def geocode_area(city: str, country: str = "", timeout: int = 20) -> dict[str, Any]:
    query = ", ".join(x.strip() for x in (city, country) if x and x.strip())
    if not query:
        raise ValueError("City or area is required")
    cached = cache.get("photon", [query])
    if cached is not None:
        return cached
    params = urllib.parse.urlencode({"q": query, "limit": 1, "lang": "en"})
    req = urllib.request.Request(
        f"{PHOTON_URL}?{params}",
        headers={"User-Agent": USER_AGENT, "Accept": "application/json"},
    )

    def _do() -> dict[str, Any]:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            return json.loads(resp.read().decode("utf-8"))

    payload = ratelimit.call("photon", _do, min_interval=_MIN_INTERVAL)
    features = (payload or {}).get("features") or []
    if not features:
        raise LookupError(f"Location not found: {query}")
    result = _row_to_area(features[0], city, country)
    cache.put("photon", [query], result)
    return result
