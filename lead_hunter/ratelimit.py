"""Provider-agnostic retry, rate limiting and circuit breaking.

Public endpoints fail transiently (504s, read timeouts) and rate-limit bursts
of serial queries. Wrapping a call with `call()` adds:

* a per-key token bucket that spaces requests out,
* bounded retries with backoff for transient errors,
* a circuit breaker that skips a provider for a cooldown after repeated
  failures, so one dead endpoint does not stall a long scan.
"""
from __future__ import annotations

import threading
import time
from typing import Any, Callable, TypeVar

T = TypeVar("T")


def _int_env(name: str, default: int) -> int:
    import os
    raw = os.environ.get(name, "").strip()
    if not raw:
        return default
    try:
        return int(float(raw))
    except ValueError:
        return default


class _Bucket:
    def __init__(self, min_interval: float) -> None:
        self.min_interval = min_interval
        self._next = 0.0
        self._lock = threading.Lock()

    def wait(self) -> None:
        with self._lock:
            now = time.monotonic()
            if now < self._next:
                time.sleep(self._next - now)
                now = time.monotonic()
            self._next = now + self.min_interval


class _Breaker:
    def __init__(self, threshold: int, cooldown: float) -> None:
        self.threshold = threshold
        self.cooldown = cooldown
        self.failures = 0
        self.open_until = 0.0
        self._lock = threading.Lock()

    def allow(self) -> bool:
        with self._lock:
            return time.monotonic() >= self.open_until

    def record_success(self) -> None:
        with self._lock:
            self.failures = 0
            self.open_until = 0.0

    def record_failure(self) -> None:
        with self._lock:
            self.failures += 1
            if self.threshold and self.failures >= self.threshold:
                self.open_until = time.monotonic() + self.cooldown


_buckets: dict[str, _Bucket] = {}
_breakers: dict[str, _Breaker] = {}
_registry_lock = threading.Lock()


def configure(
    key: str,
    *,
    min_interval: float | None = None,
    failure_threshold: int | None = None,
    cooldown: float | None = None,
) -> None:
    """Adjust defaults for a provider key (used by tests)."""
    with _registry_lock:
        bucket = _buckets.setdefault(key, _Bucket(0.0))
        if min_interval is not None:
            bucket.min_interval = min_interval
        breaker = _breakers.setdefault(key, _Breaker(0, 0.0))
        if failure_threshold is not None:
            breaker.threshold = failure_threshold
        if cooldown is not None:
            breaker.cooldown = cooldown


def reset() -> None:
    with _registry_lock:
        _buckets.clear()
        _breakers.clear()


def _bucket_for(key: str) -> _Bucket:
    with _registry_lock:
        return _buckets.setdefault(key, _Bucket(0.0))


def _breaker_for(key: str) -> _Breaker:
    with _registry_lock:
        return _breakers.setdefault(
            key,
            _Breaker(_int_env("LEADSCOUT_CIRCUIT_THRESHOLD", 4), _int_env("LEADSCOUT_CIRCUIT_COOLDOWN", 60)),
        )


class CircuitOpen(RuntimeError):
    """Raised when a provider is skipped because its breaker is open."""


def call(
    key: str,
    fn: Callable[[], T],
    *,
    min_interval: float | None = None,
    retries: int | None = None,
    backoff: float = 0.5,
    breaker: bool = True,
) -> T:
    if min_interval is None:
        min_interval = _int_env("LEADSCOUT_MIN_INTERVAL_MS", 0) / 1000.0
    if retries is None:
        retries = max(0, _int_env("LEADSCOUT_HTTP_RETRIES", 2))

    if breaker and not _breaker_for(key).allow():
        raise CircuitOpen(f"{key} is temporarily unavailable after repeated failures.")

    bucket = _bucket_for(key)
    if min_interval > bucket.min_interval:
        bucket.min_interval = min_interval
    bucket.wait()

    for attempt in range(retries + 1):
        try:
            value = fn()
        except Exception as exc:
            transient = isinstance(exc, (OSError, TimeoutError))
            if transient and attempt < retries:
                time.sleep(backoff * (2 ** attempt))
                continue
            # Non-transient errors (bot-check pages, missing keys, bad input)
            # must not be retried: repeating them wastes quota and can worsen
            # the rate limit that caused them.
            if breaker:
                _breaker_for(key).record_failure()
            raise
        else:
            if breaker:
                _breaker_for(key).record_success()
            return value

    raise RuntimeError(f"{key} failed without an exception")
