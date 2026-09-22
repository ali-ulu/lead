from __future__ import annotations

import json
import os
import urllib.request
from typing import Any

STAC_URL = "https://stac.overturemaps.org/catalog.json"
FALLBACK_RELEASE = "2026-08-19.0"

CATEGORY_ALIASES: dict[str, tuple[str, ...]] = {
    "dentist": ("dentist", "dental_clinic"),
    "clinic": ("clinic", "medical_clinic"),
    "doctor": ("doctor", "medical_clinic"),
    "physiotherapy": ("physiotherapist", "physical_therapy"),
    "veterinary": ("veterinarian", "veterinary_clinic"),
    "restaurant": ("restaurant", "casual_eatery"),
    "cafe": ("cafe", "coffee_shop"),
    "hotel": ("hotel", "lodging"),
    "beauty": ("beauty_salon", "beauty"),
    "hairdresser": ("hair_salon", "hairdresser"),
    "barber": ("barber_shop", "hair_salon"),
    "spa": ("spa",),
    "real_estate": ("real_estate_agency", "real_estate"),
    "accountant": ("accountant", "accounting"),
    "lawyer": ("lawyer", "law_firm"),
    "insurance": ("insurance_agency", "insurance"),
    "travel_agency": ("travel_agency",),
    "car_repair": ("auto_repair", "car_repair"),
    "car_dealer": ("car_dealer", "auto_dealer"),
    "electrician": ("electrician",),
    "plumber": ("plumber",),
    "photographer": ("photographer", "photography"),
    "architect": ("architect", "architecture_firm"),
    "gym": ("gym", "fitness_center"),
    "bakery": ("bakery",),
    "florist": ("florist", "flower_shop"),
}

def latest_release() -> str:
    configured = os.environ.get("OVERTURE_RELEASE", "").strip()
    if configured:
        return configured
    try:
        req = urllib.request.Request(STAC_URL, headers={"User-Agent": "LeadScout/5.0"})
        with urllib.request.urlopen(req, timeout=12) as resp:
            data = json.loads(resp.read().decode("utf-8"))
        latest = data.get("latest")
        if isinstance(latest, str) and latest:
            return latest.strip("/")
        if isinstance(latest, dict):
            href = latest.get("href") or latest.get("id")
            if href:
                return str(href).rstrip("/").rsplit("/", 1)[-1]
    except Exception:
        pass
    return FALLBACK_RELEASE

def _json(value: Any, fallback):
    if value is None:
        return fallback
    if isinstance(value, type(fallback)):
        return value
    try:
        return json.loads(value)
    except Exception:
        return fallback

def _category_sql(category: str) -> str:
    aliases = CATEGORY_ALIASES.get(category, (category,))
    safe = [x.replace("'", "''") for x in aliases]
    checks = []
    for alias in safe:
        checks.append(f"basic_category = '{alias}'")
        checks.append(f"taxonomy.primary = '{alias}'")
        checks.append(f"list_contains(taxonomy.hierarchy, '{alias}')")
    return "(" + " OR ".join(checks) + ")"

def search_bbox(
    west: float,
    south: float,
    east: float,
    north: float,
    category: str,
    city: str = "",
    country: str = "",
    limit: int | None = None,
) -> list[dict[str, Any]]:
    try:
        import duckdb
    except ImportError as exc:
        raise RuntimeError("Overture provider requires duckdb. Install the project dependencies.") from exc

    release = latest_release()
    source = os.environ.get(
        "OVERTURE_PLACES_GLOB",
        f"s3://overturemaps-us-west-2/release/{release}/theme=places/type=place/*",
    )
    where_category = _category_sql(category)
    limit_sql = f" LIMIT {int(limit)}" if limit and int(limit) > 0 else ""

    sql = f"""
        SELECT
          id,
          names.primary AS name,
          basic_category,
          taxonomy.primary AS taxonomy_primary,
          confidence,
          operating_status,
          CAST(websites AS JSON) AS websites_json,
          CAST(socials AS JSON) AS socials_json,
          CAST(emails AS JSON) AS emails_json,
          CAST(phones AS JSON) AS phones_json,
          CAST(addresses AS JSON) AS addresses_json,
          bbox.xmin AS longitude,
          bbox.ymin AS latitude
        FROM read_parquet(?, filename=true, hive_partitioning=1)
        WHERE bbox.xmin BETWEEN ? AND ?
          AND bbox.ymin BETWEEN ? AND ?
          AND (operating_status IS NULL OR operating_status <> 'permanently_closed')
          AND {where_category}
        {limit_sql}
    """

    con = duckdb.connect(database=":memory:")
    try:
        con.execute("INSTALL httpfs; LOAD httpfs;")
        con.execute("SET s3_region='us-west-2';")
        rows = con.execute(sql, [source, float(west), float(east), float(south), float(north)]).fetchall()
        columns = [d[0] for d in con.description]
    finally:
        con.close()

    out: list[dict[str, Any]] = []
    for values in rows:
        row = dict(zip(columns, values))
        name = row.get("name")
        if not name:
            continue
        websites = _json(row.get("websites_json"), [])
        socials_raw = _json(row.get("socials_json"), [])
        emails = _json(row.get("emails_json"), [])
        phones = _json(row.get("phones_json"), [])
        addresses = _json(row.get("addresses_json"), [])

        socials: dict[str, str] = {}
        for url in socials_raw or []:
            lower = str(url).lower()
            platform = "social"
            for key, needle in (
                ("instagram","instagram.com"),("facebook","facebook.com"),("linkedin","linkedin.com"),
                ("x","x.com"),("x","twitter.com"),("youtube","youtube.com"),("tiktok","tiktok.com"),
                ("telegram","t.me"),("whatsapp","wa.me"),
            ):
                if needle in lower:
                    platform = key
                    break
            socials.setdefault(platform, str(url))

        address = addresses[0] if addresses else {}
        out.append({
            "source": "overture",
            "source_id": str(row["id"]),
            "source_refs": {"overture": str(row["id"])},
            "name": str(name),
            "country": (address.get("country") if isinstance(address, dict) else None) or country,
            "city": (address.get("locality") if isinstance(address, dict) else None) or city,
            "category": category,
            "latitude": float(row["latitude"]) if row.get("latitude") is not None else None,
            "longitude": float(row["longitude"]) if row.get("longitude") is not None else None,
            "website": str(websites[0]) if websites else None,
            "phone": str(phones[0]) if phones else None,
            "email": str(emails[0]) if emails else None,
            "social_url": next(iter(socials.values()), None),
            "social_links": socials,
            "website_status": "unknown" if websites else "missing",
            "verification_status": "cross_source" if websites or phones or emails or socials else "unverified",
            "data_confidence": "high" if (row.get("confidence") or 0) >= 0.75 else "medium",
            "overture_confidence": float(row.get("confidence") or 0),
        })
    return out
