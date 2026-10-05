"""Tiny JSON response cache for public data providers.

Public endpoints (Nominatim, Overpass, Overture, keyless web search) are shared
infrastructure that rate-limits or times out under repeated scans. Caching
identical responses makes re-scans fast and keeps the providers happy.

The cache is disabled unless `LEADSCOUT_CACHE_TTL` is set to a positive number
of seconds, so tests and first-time users never see stale data.
"""
from __future__ import annotations

import hashlib
import json
import os
import tempfile
import time
from pathlib import Path
from typing import Any, Callable

DEFAULT_TTL = 6 * 60 * 60
_DEFAULT_DIR = Path(tempfile.gettempdir()) / "leadscout-cache"


def cache_dir() -> Path:
    # Resolve at call time so tests and long-running processes can point the
    # cache elsewhere via LEADSCOUT_CACHE_DIR.
    configured = os.environ.get("LEADSCOUT_CACHE_DIR", "").strip()
    return Path(configured) if configured else _DEFAULT_DIR


def ttl_seconds() -> int:
    raw = os.environ.get("LEADSCOUT_CACHE_TTL", "").strip()
    if not raw:
        return 0
    try:
        value = int(float(raw))
    except ValueError:
        return 0
    return max(0, value)


def enabled() -> bool:
    return ttl_seconds() > 0


def _key(kind: str, parts: list[Any]) -> str:
    blob = json.dumps([kind, parts], sort_keys=True, ensure_ascii=False, default=str)
    return f"{kind}-{hashlib.sha256(blob.encode('utf-8')).hexdigest()[:24]}"


def _path(key: str) -> Path:
    return cache_dir() / f"{key}.json"


def get(kind: str, parts: list[Any]) -> Any | None:
    if not enabled():
        return None
    path = _path(_key(kind, parts))
    try:
        stat = path.stat()
    except OSError:
        return None
    if time.time() - stat.st_mtime > ttl_seconds():
        return None
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except (OSError, ValueError):
        return None


def put(kind: str, parts: list[Any], value: Any) -> None:
    if not enabled():
        return
    try:
        cache_dir().mkdir(parents=True, exist_ok=True)
        tmp = _path(_key(kind, parts)).with_suffix(".tmp")
        tmp.write_text(json.dumps(value, ensure_ascii=False, default=str), encoding="utf-8")
        tmp.replace(_path(_key(kind, parts)))
    except OSError:
        # A cache write must never break a search.
        return


def get_or_call(kind: str, parts: list[Any], produce: Callable[[], Any]) -> Any:
    cached = get(kind, parts)
    if cached is not None:
        return cached
    value = produce()
    if value is not None:
        put(kind, parts, value)
    return value


def clear() -> None:
    try:
        for item in cache_dir().glob("*.json"):
            item.unlink(missing_ok=True)
    except OSError:
        return
