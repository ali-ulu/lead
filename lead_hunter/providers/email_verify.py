"""Email verification: syntax, disposable/role/free detection and MX check.

Discovery and enrichment both produce candidate addresses, and a raw regex
match says nothing about whether the mailbox can receive mail. This provider
scores an address so outreach can skip the obvious dead ends.

The MX lookup uses ``dnspython`` when it is installed and degrades to
``mx_found=None`` ("not checked") otherwise, so the provider never hard-fails
on a minimal install.
"""
from __future__ import annotations

import re
from typing import Any

EMAIL_RE = re.compile(r"^[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}$", re.I)

# Retina/asset filenames such as "sprite@2x.png" or "icon@3x-480x358.jpg" look
# like addresses to a naive regex because the asset extension reads as a TLD.
# Reject any address whose final label is a known asset type.
ASSET_EXTENSIONS = {
    "png", "jpg", "jpeg", "gif", "svg", "webp", "ico", "bmp", "avif", "tiff",
    "css", "js", "mjs", "json", "xml", "txt", "pdf", "map", "webmanifest",
    "mp4", "webm", "mov", "avi", "mp3", "wav", "ogg",
    "woff", "woff2", "ttf", "otf", "eot",
    "zip", "gz", "tar", "rar", "7z",
}


def looks_like_email(address: str) -> bool:
    """Filter a regex match that is really a filename, not an address."""
    local, at, domain = (address or "").strip().lower().partition("@")
    if not at or not local or "." not in domain:
        return False
    return domain.rsplit(".", 1)[-1] not in ASSET_EXTENSIONS


DISPOSABLE_DOMAINS = {
    "mailinator.com", "guerrillamail.com", "10minutemail.com", "tempmail.com",
    "temp-mail.org", "throwawaymail.com", "yopmail.com", "trashmail.com",
    "sharklasers.com", "getnada.com", "maildrop.cc", "dispostable.com",
}

FREE_PROVIDERS = {
    "gmail.com", "googlemail.com", "yahoo.com", "yandex.com", "yandex.ru",
    "hotmail.com", "outlook.com", "live.com", "icloud.com", "aol.com",
    "gmx.com", "gmx.de", "mail.com", "protonmail.com", "proton.me", "zoho.com",
}

ROLE_LOCALPARTS = {
    "info", "contact", "sales", "support", "admin", "office", "hello",
    "help", "billing", "postmaster", "webmaster", "noreply", "no-reply",
    "abuse", "marketing", "hr", "jobs", "careers", "enquiries", "inquiries",
}


def _mx_lookup(domain: str, timeout: float = 5.0) -> bool | None:
    try:
        import dns.resolver  # type: ignore
    except Exception:
        return None
    try:
        resolver = dns.resolver.Resolver()
        resolver.lifetime = timeout
        answers = resolver.resolve(domain, "MX")
        return len(answers) > 0
    except Exception:
        return False


def verify_email(email: str, *, check_mx: bool = True, timeout: float = 5.0) -> dict[str, Any]:
    """Score a single address. ``status`` is valid, risky or invalid."""
    address = (email or "").strip().lower()
    if not EMAIL_RE.match(address) or not looks_like_email(address):
        return {
            "email": address, "valid_syntax": False, "status": "invalid",
            "score": 0, "reason": "Malformed address",
        }
    local, _, domain = address.partition("@")
    disposable = domain in DISPOSABLE_DOMAINS
    role = local in ROLE_LOCALPARTS
    free = domain in FREE_PROVIDERS
    mx_found = _mx_lookup(domain, timeout) if check_mx else None

    score = 100
    reasons = []
    if disposable:
        score -= 70
        reasons.append("Disposable domain")
    if role:
        score -= 15
        reasons.append("Role account")
    if free:
        score -= 5
        reasons.append("Free provider")
    if mx_found is False:
        score -= 60
        reasons.append("No MX record")
    score = max(0, score)

    if mx_found is False or disposable:
        status = "invalid"
    elif score >= 80:
        status = "valid"
    else:
        status = "risky"
    return {
        "email": address,
        "valid_syntax": True,
        "domain": domain,
        "disposable": disposable,
        "role_account": role,
        "free_provider": free,
        "mx_found": mx_found,
        "score": score,
        "status": status,
        "reason": "; ".join(reasons) or "Passed all checks",
    }
