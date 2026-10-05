"""Canonical forms for phone numbers, websites, names and categories.

Providers hand back the same business in many shapes: ``+90 555 111 22 33``,
``0555 111 22 33`` and ``905551112233`` are one phone number; ``www.``,
``http://``, tracking parameters and a trailing slash are noise on a website.
Normalizing before comparison and storage is what lets duplicates collapse.
"""
from __future__ import annotations

import difflib
import re
import urllib.parse

_TR_DIGITS = {"tr", "tur", "turkey", "türkiye", "turkiye"}
_NON_DIGIT = re.compile(r"\D")
_TRACKING_PARAMS = {
    "utm_source", "utm_medium", "utm_campaign", "utm_term", "utm_content",
    "gclid", "fbclid", "yclid", "mc_cid", "mc_eid", "igshid", "ref", "ref_src",
}


def _region_key(region: str | None) -> str:
    return (region or "").strip().lower()


def normalize_phone(value: str | None, region: str = "TR") -> str | None:
    """Return an E.164-ish phone string, or None when there are no digits.

    A leading ``+`` is trusted. ``00`` is read as an international prefix. For a
    Turkish region, national forms (``05…``, ``5…``, ``90…``) are expanded to
    ``+90…``. Unknown regions keep the digit string so comparisons still work.
    """
    if value is None:
        return None
    raw = str(value).strip()
    if not raw:
        return None
    has_plus = raw.startswith("+")
    digits = _NON_DIGIT.sub("", raw)
    if not digits:
        return None
    if has_plus:
        return "+" + digits
    if digits.startswith("00"):
        return "+" + digits[2:]
    if _region_key(region) in _TR_DIGITS:
        if len(digits) == 12 and digits.startswith("90"):
            return "+" + digits
        if len(digits) == 11 and digits.startswith("0"):
            return "+90" + digits[1:]
        if len(digits) == 10 and digits.startswith("5"):
            return "+90" + digits
    return digits


def phone_key(value: str | None, region: str = "TR") -> str:
    """Digits-only key for equality checks (TR drops the country/trunk prefix)."""
    normalized = normalize_phone(value, region)
    if not normalized:
        return ""
    digits = _NON_DIGIT.sub("", normalized)
    if _region_key(region) in _TR_DIGITS and len(digits) == 12 and digits.startswith("90"):
        return digits[2:]
    return digits


def normalize_domain(value: str | None) -> str:
    if not value:
        return ""
    text = str(value).strip()
    if "://" not in text:
        text = "https://" + text
    try:
        host = (urllib.parse.urlsplit(text).hostname or "").lower().rstrip(".")
    except ValueError:
        return ""
    return host[4:] if host.startswith("www.") else host


def normalize_website(value: str | None) -> str | None:
    """Canonical URL: https, no ``www.``, no tracking params, no trailing slash."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    if "://" not in text:
        text = "https://" + text
    try:
        parts = urllib.parse.urlsplit(text)
    except ValueError:
        return value.strip()
    if parts.scheme not in ("http", "https"):
        return value.strip()
    host = (parts.hostname or "").lower().rstrip(".")
    if not host:
        return value.strip()
    if host.startswith("www."):
        host = host[4:]

    path = parts.path or ""
    if len(path) > 1 and path.endswith("/"):
        path = path.rstrip("/")
    if path == "/":
        path = ""

    query = urllib.parse.urlencode([
        (k, v) for k, v in urllib.parse.parse_qsl(parts.query, keep_blank_values=False)
        if k.lower() not in _TRACKING_PARAMS
    ])
    netloc = f"{host}:{parts.port}" if parts.port and parts.port not in (80, 443) else host
    url = f"https://{netloc}{path}"
    if query:
        url += f"?{query}"
    return url


def normalize_name(value: str | None) -> str:
    """Lowercased alphanumerics, with Turkish diacritics folded to ASCII."""
    if not value:
        return ""
    text = str(value).lower().strip()
    for src, dst in (("ı", "i"), ("İ", "i"), ("ş", "s"), ("ğ", "g"), ("ü", "u"),
                     ("ö", "o"), ("ç", "c"), ("â", "a"), ("î", "i"), ("û", "u")):
        text = text.replace(src, dst)
    return re.sub(r"[^a-z0-9]+", "", text)


def _trigrams(value: str) -> set[str]:
    padded = f"  {value} "
    return {padded[i:i + 3] for i in range(len(padded) - 2)}


# Generic descriptors that do not identify a business; ignored when comparing
# names so "Sarıtaş Döner" and "Sarıtaş Döner Salonu" match.
_GENERIC_TOKENS = {
    "salon", "salonu", "salonlar", "restaurant", "restoran", "lokanta", "cafe",
    "kafe", "hotel", "otel", "market", "magaza", "store", "shop", "center",
    "merkez", "merkezi", "clinic", "klinik", "poliklinik", "ltd", "sti",
    "limited", "as", "inc", "co", "company", "the",
}


def _core_name(value: str | None) -> str:
    if not value:
        return ""
    words = re.split(r"[^0-9A-Za-zÀ-ÿĞğİıŞşÇçÖöÜüÂâÎîÛû]+", str(value))
    kept = [w for w in words if w and normalize_name(w) not in _GENERIC_TOKENS]
    core = normalize_name(" ".join(kept))
    return core or normalize_name(value)


def name_similarity(a: str | None, b: str | None) -> float:
    """0..1 similarity blending character-ratio and trigram Jaccard."""
    na, nb = _core_name(a), _core_name(b)
    if not na or not nb:
        return 0.0
    if na == nb:
        return 1.0
    ratio = difflib.SequenceMatcher(None, na, nb).ratio()
    ta, tb = _trigrams(na), _trigrams(nb)
    union = ta | tb
    jaccard = len(ta & tb) / len(union) if union else 0.0
    return max(ratio, jaccard)


# Canonical category key -> aliases (English + Turkish common names).
CATEGORY_ALIASES: dict[str, tuple[str, ...]] = {
    "barber": ("barber", "berber", "hairdresser", "kuaför", "kuafor", "erkek kuafor"),
    "beauty": ("beauty", "güzellik", "guzellik", "beauty_salon", "estetik"),
    "dentist": ("dentist", "dental_clinic", "dişçi", "disci", "dis hekimi", "diş hekimi"),
    "doctor": ("doctor", "doctors", "doktor", "hekim"),
    "clinic": ("clinic", "medical_clinic", "klinik", "poliklinik"),
    "restaurant": ("restaurant", "restoran", "lokanta", "yemek"),
    "cafe": ("cafe", "café", "kafe", "kahve"),
    "hotel": ("hotel", "otel"),
    "pharmacy": ("pharmacy", "eczane"),
    "veterinary": ("veterinary", "veteriner"),
    "florist": ("florist", "flower_shop", "çiçekçi", "cicekci"),
    "gym": ("gym", "fitness", "spor salonu"),
    "lawyer": ("lawyer", "avukat", "hukuk"),
    "accountant": ("accountant", "muhasebe", "mali müşavir"),
    "real_estate": ("real_estate", "emlak", "gayrimenkul"),
    "car_repair": ("car_repair", "oto tamir", "tamirci", "servis"),
}

_ALIAS_TO_KEY: dict[str, str] = {}
for _key, _aliases in CATEGORY_ALIASES.items():
    _ALIAS_TO_KEY[_key] = _key
    for _alias in _aliases:
        _ALIAS_TO_KEY[normalize_name(_alias)] = _key


def category_key(category: str | None) -> str:
    """Map a category name (English or Turkish) to its canonical key."""
    name = normalize_name(category)
    if not name:
        return ""
    return _ALIAS_TO_KEY.get(name, name)
