from __future__ import annotations
import json
import math
import urllib.parse
import urllib.request
from typing import Any

OVERPASS_URLS = (
    "https://overpass-api.de/api/interpreter",
    "https://overpass.kumi.systems/api/interpreter",
)
USER_AGENT = "LeadScout/4.0 (local business research tool)"

DENSE_CATEGORIES = {"restaurant", "cafe", "beauty", "hairdresser", "barber", "hotel"}

CATEGORY_FILTERS: dict[str, list[tuple[str, str]]] = {
    "dentist": [("amenity", "dentist")],
    "clinic": [("amenity", "clinic"), ("healthcare", "clinic")],
    "doctor": [("amenity", "doctors"), ("healthcare", "doctor")],
    "physiotherapy": [("healthcare", "physiotherapist")],
    "veterinary": [("amenity", "veterinary")],
    "restaurant": [("amenity", "restaurant")],
    "cafe": [("amenity", "cafe")],
    "hotel": [("tourism", "hotel"), ("tourism", "guest_house")],
    "beauty": [("shop", "beauty")],
    "hairdresser": [("shop", "hairdresser")],
    "barber": [("shop", "hairdresser")],
    "spa": [("leisure", "spa"), ("shop", "beauty")],
    "real_estate": [("office", "estate_agent")],
    "accountant": [("office", "accountant")],
    "lawyer": [("office", "lawyer")],
    "insurance": [("office", "insurance")],
    "travel_agency": [("shop", "travel_agency")],
    "car_repair": [("shop", "car_repair")],
    "car_dealer": [("shop", "car")],
    "electrician": [("craft", "electrician")],
    "plumber": [("craft", "plumber")],
    "photographer": [("craft", "photographer")],
    "architect": [("office", "architect")],
    "gym": [("leisure", "fitness_centre")],
    "bakery": [("shop", "bakery")],
    "florist": [("shop", "florist")],
}

SOCIAL_TAGS: dict[str, tuple[tuple[str, ...], str]] = {
    "instagram": (("contact:instagram","instagram","brand:instagram","operator:instagram"), "https://instagram.com/"),
    "facebook": (("contact:facebook","facebook","brand:facebook","operator:facebook"), "https://facebook.com/"),
    "linkedin": (("contact:linkedin","linkedin","brand:linkedin","operator:linkedin"), "https://linkedin.com/"),
    "x": (("contact:twitter","twitter","contact:x","x"), "https://x.com/"),
    "youtube": (("contact:youtube","youtube","brand:youtube"), "https://youtube.com/"),
    "tiktok": (("contact:tiktok","tiktok"), "https://tiktok.com/@"),
    "telegram": (("contact:telegram","telegram"), "https://t.me/"),
    "whatsapp": (("contact:whatsapp","whatsapp"), "https://wa.me/"),
}

def _tag(tags: dict[str, Any], *names: str):
    for name in names:
        if tags.get(name):
            return tags[name]
    return None

def _normalize_social(value: str, prefix: str) -> str:
    value = (value or "").strip()
    if not value:
        return ""
    if value.startswith(("http://","https://")):
        return value
    handle = value.lstrip("@/ ")
    if prefix.endswith("wa.me/"):
        handle = "".join(ch for ch in handle if ch.isdigit())
    return prefix + handle

def _socials(tags: dict[str, Any]) -> dict[str, str]:
    found: dict[str, str] = {}
    for platform, (keys, prefix) in SOCIAL_TAGS.items():
        value = _tag(tags, *keys)
        if value:
            url = _normalize_social(str(value), prefix)
            if url:
                found[platform] = url
    return found

def _social(tags: dict[str, Any]):
    socials = _socials(tags)
    for platform in ("instagram","facebook","linkedin","x","youtube","tiktok","whatsapp","telegram"):
        if socials.get(platform):
            return socials[platform]
    return None

def _build_query(south: float, west: float, north: float, east: float, category: str, timeout: int) -> str:
    filters = CATEGORY_FILTERS.get(category)
    if not filters:
        raise ValueError(f"Unsupported category: {category}")
    bbox = f"{south},{west},{north},{east}"
    parts = [f'nwr["{key}"="{value}"]({bbox});' for key, value in filters]
    return f'[out:json][timeout:{timeout}];({"".join(parts)});out tags center qt;'

def _build_around_query(lat: float, lon: float, radius_m: int, category: str, timeout: int) -> str:
    filters = CATEGORY_FILTERS.get(category)
    if not filters:
        raise ValueError(f"Unsupported category: {category}")
    radius_m = max(1000, min(100000, int(radius_m)))
    parts = [f'nwr["{key}"="{value}"](around:{radius_m},{lat},{lon});' for key, value in filters]
    return f'[out:json][timeout:{timeout}];({"".join(parts)});out tags center qt;'

def _fetch(query: str, timeout: int) -> dict[str, Any]:
    payload = urllib.parse.urlencode({"data": query}).encode()
    last_error: Exception | None = None
    for endpoint in OVERPASS_URLS:
        try:
            req = urllib.request.Request(
                endpoint,
                data=payload,
                headers={"User-Agent": USER_AGENT, "Content-Type": "application/x-www-form-urlencoded"},
            )
            with urllib.request.urlopen(req, timeout=timeout + 8) as resp:
                return json.loads(resp.read().decode("utf-8"))
        except Exception as exc:
            last_error = exc
    raise RuntimeError(f"OpenStreetMap query failed: {last_error}")

def _normalize(data: dict[str, Any], category: str, city: str, country: str, limit: int | None = None) -> list[dict[str, Any]]:
    out: list[dict[str, Any]] = []
    seen: set[str] = set()
    for element in data.get("elements", []):
        tags = element.get("tags") or {}
        name = tags.get("name") or tags.get("brand")
        if not name:
            continue
        source_id = f"{element.get('type')}:{element.get('id')}"
        if source_id in seen:
            continue
        seen.add(source_id)
        center = element.get("center") or {}
        website = _tag(tags, "website", "contact:website", "url")
        socials = _socials(tags)
        primary_social = next(iter(socials.values()), None)
        out.append({
            "source": "osm",
            "source_id": source_id,
            "name": name,
            "country": tags.get("addr:country") or country,
            "city": _tag(tags, "addr:city", "addr:town", "addr:village", "addr:municipality") or city,
            "category": category,
            "latitude": element.get("lat", center.get("lat")),
            "longitude": element.get("lon", center.get("lon")),
            "website": website,
            "phone": _tag(tags, "phone", "contact:phone", "mobile", "contact:mobile"),
            "email": _tag(tags, "email", "contact:email"),
            "social_url": primary_social,
            "social_links": socials,
            "website_status": "unknown" if website else "missing",
            "data_confidence": "medium",
        })
        if limit is not None and len(out) >= limit:
            break
    return out

def search_bbox(south: float, west: float, north: float, east: float, category: str, city: str = "", country: str = "", timeout: int = 35, limit: int | None = None) -> list[dict[str, Any]]:
    return _normalize(_fetch(_build_query(south, west, north, east, category, timeout), timeout), category, city, country, limit)


def _distance_km(lat1: float, lon1: float, lat2: float, lon2: float) -> float:
    earth_km = 6371.0088
    p1 = math.radians(lat1)
    p2 = math.radians(lat2)
    dp = math.radians(lat2 - lat1)
    dl = math.radians(lon2 - lon1)
    a = math.sin(dp / 2) ** 2 + math.cos(p1) * math.cos(p2) * math.sin(dl / 2) ** 2
    return 2 * earth_km * math.atan2(math.sqrt(a), math.sqrt(1 - a))


def _radius_bbox(lat: float, lon: float, radius_km: float) -> tuple[float, float, float, float]:
    lat_delta = radius_km / 111.32
    cos_lat = max(0.15, abs(math.cos(math.radians(lat))))
    lon_delta = radius_km / (111.32 * cos_lat)
    return lat - lat_delta, lon - lon_delta, lat + lat_delta, lon + lon_delta


def _tile_boxes(south: float, west: float, north: float, east: float, grid: int = 2) -> list[tuple[float, float, float, float]]:
    lat_step = (north - south) / grid
    lon_step = (east - west) / grid
    boxes = []
    for row in range(grid):
        for col in range(grid):
            boxes.append((
                south + row * lat_step,
                west + col * lon_step,
                south + (row + 1) * lat_step,
                west + (col + 1) * lon_step,
            ))
    return boxes


def _search_tiled_around(
    lat: float,
    lon: float,
    radius_km: int,
    category: str,
    city: str,
    country: str,
    timeout: int,
    limit: int | None,
) -> list[dict[str, Any]]:
    south, west, north, east = _radius_bbox(lat, lon, radius_km)
    merged: dict[str, dict[str, Any]] = {}

    for tile_s, tile_w, tile_n, tile_e in _tile_boxes(south, west, north, east, grid=2):
        data = _fetch(_build_query(tile_s, tile_w, tile_n, tile_e, category, timeout), timeout)
        for row in _normalize(data, category, city, country, None):
            row_lat = row.get("latitude")
            row_lon = row.get("longitude")
            if row_lat is not None and row_lon is not None:
                if _distance_km(lat, lon, float(row_lat), float(row_lon)) > radius_km:
                    continue
            merged[row["source_id"]] = row
            if limit is not None and len(merged) >= limit:
                return list(merged.values())[:limit]

    return list(merged.values()) if limit is None else list(merged.values())[:limit]


def search_around(lat: float, lon: float, radius_km: int, category: str, city: str = "", country: str = "", timeout: int = 35, limit: int | None = None) -> list[dict[str, Any]]:
    radius_km = max(1, min(100, int(radius_km)))

    # Fast path: ask Overpass once for the whole radius with no output-count cap.
    # This is both faster for users and lighter on shared public infrastructure.
    # If a dense market times out or the provider refuses the large query,
    # transparently fall back to smaller tiled bounding-box queries.
    try:
        return _normalize(
            _fetch(_build_around_query(lat, lon, radius_km * 1000, category, timeout), timeout),
            category,
            city,
            country,
            limit,
        )
    except RuntimeError:
        return _search_tiled_around(lat, lon, radius_km, category, city, country, timeout, limit)
