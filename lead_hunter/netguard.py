"""Outbound-network guards: SSRF protection, robots.txt and rate limiting.

Enrichment and auditing fetch URLs that come from third-party data (OSM tags,
Overture records, web-search results). Those URLs are untrusted: a crafted
record must not be able to make the server read ``http://169.254.169.254/`` or
an internal admin panel. Every outbound fetch goes through ``assert_public_url``
and a redirect handler that re-checks each hop.
"""
from __future__ import annotations

import http.client
import ipaddress
import os
import socket
import threading
import time
import urllib.parse
import urllib.request
from typing import Any

USER_AGENT = "LeadScout/7.0 (+https://github.com/ali-ulu/lead; polite local scanner)"

_PRIVATE_HOSTNAMES = {
    "localhost", "localhost.localdomain", "ip6-localhost", "ip6-loopback",
    "metadata", "metadata.google.internal",
}


def _is_public_ip(ip: ipaddress.IPv4Address | ipaddress.IPv6Address) -> bool:
    return not (ip.is_private or ip.is_loopback or ip.is_link_local or ip.is_reserved
                or ip.is_multicast or ip.is_unspecified)


def assert_public_url(url: str, *, allow_unresolvable: bool = False) -> str:
    """Reject URLs that resolve to a local, private or reserved address.

    Raises ValueError whose message starts with "Local/private" or
    "Website hostname could not be resolved". ``allow_unresolvable`` lets
    callers treat a dead domain as a finding instead of an error.
    """
    parsed = urllib.parse.urlparse(url)
    host = parsed.hostname
    if not host:
        raise ValueError("Invalid website hostname")
    if host.lower() in _PRIVATE_HOSTNAMES:
        raise ValueError("Local/private websites are not audited")
    port = parsed.port or (443 if parsed.scheme == "https" else 80)
    try:
        infos = socket.getaddrinfo(host, port, type=socket.SOCK_STREAM)
    except socket.gaierror as exc:
        raise ValueError(f"Website hostname could not be resolved: {exc}") from exc
    for info in infos:
        ip = ipaddress.ip_address(info[4][0])
        if not _is_public_ip(ip):
            raise ValueError("Local/private websites are not audited")
    return url


class _GuardHTTPConnection(http.client.HTTPConnection):
    """An HTTP(S) connection that re-checks the address it actually connects to.

    ``assert_public_url`` validates DNS, but the connection resolves the name a
    second time; a name that flips between a public and a private answer
    (DNS rebinding) would otherwise slip through. Checking the peer address on
    the live socket closes that window.
    """

    def connect(self) -> None:
        self.sock = self._create_connection(
            (self.host, self.port), self.timeout, self.source_address,
        )
        try:
            peer = self.sock.getpeername()[0]
            ip = ipaddress.ip_address(peer)
            if not _is_public_ip(ip):
                raise ValueError("Local/private websites are not audited")
        except ValueError:
            self.sock.close()
            raise


class _GuardHTTPSConnection(http.client.HTTPSConnection):
    def connect(self) -> None:
        self.sock = self._create_connection(
            (self.host, self.port), self.timeout, self.source_address,
        )
        try:
            peer = self.sock.getpeername()[0]
            ip = ipaddress.ip_address(peer)
            if not _is_public_ip(ip):
                raise ValueError("Local/private websites are not audited")
        except ValueError:
            self.sock.close()
            raise
        if self._tunnel_host:
            self._tunnel()
        self.sock = self._context.wrap_socket(self.sock, server_hostname=self.host)


def build_public_opener(*, check_robots: bool = True) -> urllib.request.OpenerDirector:
    """An opener that re-validates every redirect hop and the connected peer."""
    return urllib.request.build_opener(
        _PublicRedirectHandler(check_robots=check_robots),
        _GuardHTTPHandler(), _GuardHTTPSHandler(),
    )


class _GuardHTTPHandler(urllib.request.HTTPHandler):
    def http_open(self, req):
        return self.do_open(_GuardHTTPConnection, req)


class _GuardHTTPSHandler(urllib.request.HTTPSHandler):
    def https_open(self, req):
        return self.do_open(_GuardHTTPSConnection, req)


class _PublicRedirectHandler(urllib.request.HTTPRedirectHandler):
    def __init__(self, *, check_robots: bool = True) -> None:
        super().__init__()
        self._check_robots = check_robots

    def redirect_request(self, req, fp, code, msg, headers, newurl):
        target = urllib.parse.urljoin(req.full_url, newurl)
        assert_public_url(target)
        # A redirect is a fresh fetch of a new URL, so it must pass the same
        # robots check as the original request. robots.txt itself opts out to
        # avoid recursing back into _fetch_robots.
        if (self._check_robots
                and os.environ.get("LEADSCOUT_RESPECT_ROBOTS", "1") == "1"
                and not robots_allows(target)):
            raise ValueError("Blocked by robots.txt")
        return super().redirect_request(req, fp, code, msg, headers, target)


# --- robots.txt ------------------------------------------------------------

_ROBOTS_CACHE: dict[str, tuple[float, list[str]]] = {}
_ROBOTS_LOCK = threading.Lock()
_ROBOTS_TTL = 3600.0


def _parse_robots(body: str, user_agent: str = USER_AGENT) -> list[str]:
    """Disallow paths for our UA, falling back to the '*' group."""
    blocks: list[tuple[str, list[str]]] = []
    current: tuple[str, list[str]] | None = None
    for line in body.splitlines():
        line = line.split("#", 1)[0].strip()
        if not line or ":" not in line:
            continue
        field, value = line.split(":", 1)
        field, value = field.strip().lower(), value.strip()
        if field == "user-agent":
            current = (value, [])
            blocks.append(current)
        elif field == "disallow" and current is not None:
            current[1].append(value)
    specific = [b for b in blocks if b[0] != "*" and "leadscout" in b[0].lower()]
    chosen = specific or [b for b in blocks if b[0] == "*"]
    disallows: list[str] = []
    for _, rules in chosen:
        disallows.extend(r for r in rules if r)
    return disallows


def robots_allows(url: str, timeout: float = 5.0) -> bool:
    """Best-effort robots.txt check. Unknown/unreachable robots.txt allows."""
    parsed = urllib.parse.urlparse(url)
    if not parsed.scheme or not parsed.hostname:
        return True
    origin = f"{parsed.scheme}://{parsed.netloc}"
    now = time.monotonic()
    with _ROBOTS_LOCK:
        cached = _ROBOTS_CACHE.get(origin)
    if cached and now - cached[0] < _ROBOTS_TTL:
        disallows = cached[1]
    else:
        disallows = _fetch_robots(origin, timeout)
        with _ROBOTS_LOCK:
            _ROBOTS_CACHE[origin] = (now, disallows)
    path = parsed.path or "/"
    if parsed.query:
        path = f"{path}?{parsed.query}"
    for rule in disallows:
        if rule == "/":
            return False
        if rule.endswith("*") and path.startswith(rule[:-1]):
            return False
        if path.startswith(rule):
            return False
    return True


def _fetch_robots(origin: str, timeout: float) -> list[str]:
    robots_url = f"{origin}/robots.txt"
    try:
        assert_public_url(robots_url)
        req = urllib.request.Request(robots_url, headers={"User-Agent": USER_AGENT})
        with build_public_opener(check_robots=False).open(req, timeout=timeout) as resp:
            if resp.status != 200:
                return []
            body = resp.read(512_000).decode("utf-8", errors="replace")
    except Exception:
        return []
    return _parse_robots(body)


# --- rate limiting ---------------------------------------------------------

class RateLimiter:
    """Thread-safe minimum-interval limiter for one key."""

    def __init__(self, min_interval: float = 0.0) -> None:
        self.min_interval = max(0.0, min_interval)
        self._next = 0.0
        self._lock = threading.Lock()

    def wait(self) -> None:
        if self.min_interval <= 0:
            return
        with self._lock:
            now = time.monotonic()
            if now < self._next:
                time.sleep(self._next - now)
                now = time.monotonic()
            self._next = now + self.min_interval


def env_float(name: str, default: float) -> float:
    raw = os.environ.get(name, "").strip()
    if not raw:
        return default
    try:
        return float(raw)
    except ValueError:
        return default


def default_audit_interval() -> float:
    return env_float("LEADSCOUT_AUDIT_MIN_INTERVAL", 0.0)


def default_fetch_interval() -> float:
    return env_float("LEADSCOUT_FETCH_MIN_INTERVAL", 0.0)


def host_of(url: str) -> str:
    return urllib.parse.urlparse(url).hostname or url
