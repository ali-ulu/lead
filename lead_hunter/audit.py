from __future__ import annotations
import re
import json
import os
import shutil
import subprocess
import ssl
import time
import socket
import ipaddress
import urllib.error
import urllib.parse
import urllib.request
from html.parser import HTMLParser
from pathlib import Path
from typing import Any
from datetime import datetime, timezone

from .visibility import calculate_visibility

USER_AGENT = "Mozilla/5.0 (compatible; LeadScout/5.0; local website audit)"

SOCIAL_HOSTS = {
    "instagram": ("instagram.com",),
    "facebook": ("facebook.com", "fb.com"),
    "linkedin": ("linkedin.com",),
    "x": ("x.com", "twitter.com"),
    "youtube": ("youtube.com", "youtu.be"),
    "tiktok": ("tiktok.com",),
    "telegram": ("t.me", "telegram.me"),
    "whatsapp": ("wa.me", "api.whatsapp.com", "whatsapp.com"),
}

class SignalParser(HTMLParser):
    def __init__(self):
        super().__init__()
        self.viewport = False
        self.tel = False
        self.booking = False
        self.contact = False
        self.forms = 0
        self.h1 = 0
        self.meta_description = False
        self.stylesheets = 0
        self.scripts = 0
        self.images = 0
        self.alt_images = 0
        self.social_links: dict[str, str] = {}
        self.canonical = ""
        self.robots_noindex = False
        self.meta_author = False
        self.h2 = 0
        self.h3 = 0
        self.semantic_main = False
        self.hrefs: list[str] = []
        self.heading_texts: list[str] = []
        self.text_parts: list[str] = []
        self.jsonld_blocks: list[str] = []
        self.time_values: list[str] = []
        self._jsonld = False
        self._jsonld_buffer: list[str] = []
        self._heading_tag = ""
        self._heading_buffer: list[str] = []

    def handle_starttag(self, tag, attrs):
        tag = tag.lower()
        d = {k.lower(): (v or "") for k, v in attrs}
        if tag == "meta":
            name = d.get("name", "").lower()
            if name == "viewport":
                self.viewport = True
            if name == "description" and d.get("content", "").strip():
                self.meta_description = True
            if name in {"robots", "googlebot"} and "noindex" in d.get("content", "").lower():
                self.robots_noindex = True
            if name == "author" and d.get("content", "").strip():
                self.meta_author = True
        if tag == "a":
            raw_href = d.get("href", "").strip()
            if raw_href:
                self.hrefs.append(raw_href)
            if "author" in d.get("rel", "").lower():
                self.meta_author = True
            href = raw_href.lower()
            textish = " ".join(d.values()).lower()
            self.tel = self.tel or href.startswith("tel:")
            self.contact = self.contact or any(x in href for x in ("contact", "iletisim", "iletişim", "kontakt", "contato", "رابطہ"))
            self.booking = self.booking or any(x in href + " " + textish for x in ("book", "appointment", "reservation", "randevu", "termin", "reserve"))
            if raw_href.startswith(("http://", "https://")):
                host = (urllib.parse.urlparse(raw_href).hostname or "").lower()
                for platform, hosts in SOCIAL_HOSTS.items():
                    if any(host == item or host.endswith("." + item) for item in hosts):
                        self.social_links.setdefault(platform, raw_href)
        if tag == "form":
            self.forms += 1
        if tag == "h1":
            self.h1 += 1
        if tag == "h2":
            self.h2 += 1
        if tag == "h3":
            self.h3 += 1
        if tag in {"h1","h2","h3","h4"}:
            self._heading_tag = tag
            self._heading_buffer = []
        if tag in {"main","article"}:
            self.semantic_main = True
        if tag == "time" and d.get("datetime"):
            self.time_values.append(d.get("datetime", ""))
        if tag == "link":
            rel = d.get("rel", "").lower()
            if "stylesheet" in rel:
                self.stylesheets += 1
            if "canonical" in rel and d.get("href"):
                self.canonical = d.get("href", "").strip()
        if tag == "script":
            if d.get("src"):
                self.scripts += 1
            if "application/ld+json" in d.get("type", "").lower():
                self._jsonld = True
                self._jsonld_buffer = []
        if tag == "img":
            self.images += 1
            if d.get("alt", "").strip():
                self.alt_images += 1

    def handle_endtag(self, tag):
        tag = tag.lower()
        if self._jsonld and tag == "script":
            block = "".join(self._jsonld_buffer).strip()
            if block:
                self.jsonld_blocks.append(block)
            self._jsonld = False
            self._jsonld_buffer = []
        if self._heading_tag and tag == self._heading_tag:
            text = re.sub(r"\\s+", " ", " ".join(self._heading_buffer)).strip()
            if text:
                self.heading_texts.append(text)
            self._heading_tag = ""
            self._heading_buffer = []

    def handle_data(self, data):
        text = re.sub(r"\\s+", " ", data or "").strip()
        if not text:
            return
        if self._jsonld:
            self._jsonld_buffer.append(data)
            return
        self.text_parts.append(text)
        if self._heading_tag:
            self._heading_buffer.append(text)

def _walk_jsonld(value: Any, types: set[str], fields: set[str]) -> None:
    if isinstance(value, list):
        for item in value:
            _walk_jsonld(item, types, fields)
        return
    if not isinstance(value, dict):
        return
    raw_type = value.get("@type")
    if isinstance(raw_type, str):
        types.add(raw_type)
    elif isinstance(raw_type, list):
        types.update(str(x) for x in raw_type)
    fields.update(str(k) for k, v in value.items() if v not in (None, "", [], {}))
    for child in value.values():
        if isinstance(child, (dict, list)):
            _walk_jsonld(child, types, fields)

def _structured_data(blocks: list[str]) -> tuple[list[str], list[str]]:
    types: set[str] = set()
    fields: set[str] = set()
    for block in blocks:
        try:
            _walk_jsonld(json.loads(block), types, fields)
        except Exception:
            continue
    return sorted(types), sorted(fields)

def normalize_url(url: str) -> str:
    url = (url or "").strip()
    if not url:
        raise ValueError("Website URL is empty")
    if not urllib.parse.urlparse(url).scheme:
        url = "https://" + url
    parsed = urllib.parse.urlparse(url)
    if parsed.scheme not in {"http", "https"}:
        raise ValueError("Only http/https URLs are supported")
    return url

def _speed_proxy(ms: int) -> int:
    if ms > 5000: return 20
    if ms > 3000: return 35
    if ms > 2000: return 50
    if ms > 1200: return 65
    if ms > 700: return 78
    return 90

def _assert_public_host(url: str) -> None:
    parsed = urllib.parse.urlparse(url)
    host = parsed.hostname
    if not host:
        raise ValueError("Invalid website hostname")
    if host.lower() in {"localhost", "localhost.localdomain"}:
        raise ValueError("Local/private websites are not audited")
    try:
        infos = socket.getaddrinfo(host, parsed.port or (443 if parsed.scheme == "https" else 80), type=socket.SOCK_STREAM)
    except socket.gaierror as exc:
        raise ValueError(f"Website hostname could not be resolved: {exc}") from exc
    for info in infos:
        ip = ipaddress.ip_address(info[4][0])
        if ip.is_private or ip.is_loopback or ip.is_link_local or ip.is_reserved or ip.is_multicast:
            raise ValueError("Local/private websites are not audited")

class PublicRedirectHandler(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        target = urllib.parse.urljoin(req.full_url, newurl)
        _assert_public_host(target)
        return super().redirect_request(req, fp, code, msg, headers, target)


def _lighthouse(url: str, timeout: int = 90) -> dict[str, Any] | None:
    configured = os.environ.get("LEADSCOUT_LIGHTHOUSE_BIN", "").strip()
    binary = configured or shutil.which("lighthouse")
    command: list[str] | None = [binary] if binary else None

    if not command:
        root = Path(__file__).resolve().parents[1]
        local = root / "node_modules" / ".bin" / ("lighthouse.cmd" if os.name == "nt" else "lighthouse")
        if local.exists():
            command = [str(local)]

    # Self-use convenience: if Node/npm exists, the first deep audit can fetch
    # the pinned Lighthouse runtime without a separate manual install step.
    if not command and os.environ.get("LEADSCOUT_LIGHTHOUSE_AUTO_NPX", "1") == "1":
        npx = shutil.which("npx")
        if npx:
            command = [npx, "--yes", "lighthouse@13.5.0"]

    if not command:
        return None

    try:
        proc = subprocess.run(
            command + [
                url,
                "--quiet",
                "--output=json",
                "--output-path=stdout",
                "--only-categories=performance,accessibility,best-practices,seo",
                "--chrome-flags=--headless --no-sandbox --disable-gpu",
            ],
            capture_output=True,
            text=True,
            timeout=timeout,
            check=False,
        )
        if proc.returncode != 0 or not proc.stdout.strip():
            return None
        report=json.loads(proc.stdout)
        cats=report.get("categories") or {}
        def score(name: str):
            value=(cats.get(name) or {}).get("score")
            return round(float(value)*100) if value is not None else None
        return {
            "performance_score":score("performance"),
            "accessibility_score":score("accessibility"),
            "best_practices_score":score("best-practices"),
            "seo_score":score("seo"),
            "lighthouse_version":report.get("lighthouseVersion"),
        }
    except Exception:
        return None

def audit_url(url: str, timeout: float = 12.0) -> dict[str, Any]:
    url = normalize_url(url)
    _assert_public_host(url)
    req = urllib.request.Request(url, headers={"User-Agent": USER_AGENT, "Accept": "text/html,application/xhtml+xml"})
    opener = urllib.request.build_opener(
        urllib.request.HTTPSHandler(context=ssl.create_default_context()),
        PublicRedirectHandler(),
    )
    start = time.perf_counter()
    try:
        with opener.open(req, timeout=timeout) as resp:
            raw = resp.read(1_800_000)
            body = raw.decode(resp.headers.get_content_charset() or "utf-8", errors="replace")
            elapsed = round((time.perf_counter() - start) * 1000)
            parser = SignalParser()
            parser.feed(body)
            final = resp.geturl()
            _assert_public_host(final)
            title_match = re.search(r"<title[^>]*>(.*?)</title>", body, flags=re.I | re.S)
            title = re.sub(r"\s+", " ", title_match.group(1)).strip() if title_match else ""
            years = [int(y) for y in re.findall(r"(?:©|copyright[^0-9]{0,15})(20\d{2})", body, flags=re.I)]
            latest_year = max(years) if years else None
            seo_score = 100
            if not title: seo_score -= 30
            if not parser.meta_description: seo_score -= 25
            if parser.h1 == 0: seo_score -= 20
            if not parser.viewport: seo_score -= 15
            seo_score = max(0, seo_score)
            quality_flags: list[str] = []
            if not parser.viewport: quality_flags.append("No mobile viewport")
            if not (parser.tel or parser.contact or parser.forms): quality_flags.append("No clear contact CTA")
            if not parser.booking: quality_flags.append("No booking/reservation signal")
            if not final.startswith("https://"): quality_flags.append("HTTPS missing")
            if elapsed > 2500: quality_flags.append("Slow first response")
            if not title: quality_flags.append("Missing page title")
            if not parser.meta_description: quality_flags.append("Missing meta description")
            if parser.images and parser.alt_images / max(1, parser.images) < 0.5: quality_flags.append("Many images lack alt text")
            if latest_year and latest_year <= 2022: quality_flags.append(f"Old copyright signal ({latest_year})")
            lighthouse = _lighthouse(final)
            performance_score = (lighthouse or {}).get("performance_score")
            accessibility_score = (lighthouse or {}).get("accessibility_score")
            lighthouse_seo = (lighthouse or {}).get("seo_score")
            if performance_score is None:
                performance_score = _speed_proxy(elapsed)
            if lighthouse_seo is not None:
                seo_score = lighthouse_seo
            weak = (
                len(quality_flags) >= 2
                or (not parser.viewport)
                or (not final.startswith("https://"))
                or (performance_score is not None and performance_score < 45)
                or (seo_score is not None and seo_score < 55)
            )
            return {
                "reachable": True,
                "status": getattr(resp, "status", 200),
                "final_url": final,
                "has_https": final.startswith("https://"),
                "response_ms": elapsed,
                "performance_score": performance_score,
                "accessibility_score": accessibility_score,
                "best_practices_score": (lighthouse or {}).get("best_practices_score"),
                "seo_score": seo_score,
                "audit_engine": "lighthouse+heuristic" if lighthouse else "heuristic",
                "mobile_ok": parser.viewport,
                "has_cta": parser.tel or parser.contact or parser.forms > 0,
                "has_booking": parser.booking,
                "has_tel_link": parser.tel,
                "has_contact_path": parser.contact,
                "form_count": parser.forms,
                "title": title,
                "latest_copyright_year": latest_year,
                "quality_flags": quality_flags,
                "social_links": parser.social_links,
                "website_status": "weak" if weak else "healthy",
                "note": "Lighthouse scores are used when a local Lighthouse binary is available; otherwise performance falls back to a response-time proxy.",
            }
    except (urllib.error.URLError, TimeoutError, ssl.SSLError) as exc:
        return {"reachable": False, "error": str(exc), "website_status": "weak", "social_links": {}}
