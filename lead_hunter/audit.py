from __future__ import annotations
import re
import ssl
import time
import socket
import ipaddress
import urllib.error
import urllib.parse
import urllib.request
from html.parser import HTMLParser
from typing import Any

USER_AGENT = "Mozilla/5.0 (compatible; AI-ULU-Lead-Hunter/1.0; local website audit)"

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

    def handle_starttag(self, tag, attrs):
        tag = tag.lower()
        d = {k.lower(): (v or "") for k, v in attrs}
        if tag == "meta":
            if d.get("name", "").lower() == "viewport":
                self.viewport = True
            if d.get("name", "").lower() == "description" and d.get("content", "").strip():
                self.meta_description = True
        if tag == "a":
            href = d.get("href", "").lower()
            textish = " ".join(d.values()).lower()
            self.tel = self.tel or href.startswith("tel:")
            self.contact = self.contact or any(x in href for x in ("contact", "iletisim", "iletişim", "kontakt", "contato"))
            self.booking = self.booking or any(x in href + " " + textish for x in ("book", "appointment", "reservation", "randevu", "termin", "reserve"))
        if tag == "form":
            self.forms += 1
        if tag == "h1":
            self.h1 += 1
        if tag == "link" and "stylesheet" in d.get("rel", "").lower():
            self.stylesheets += 1
        if tag == "script" and d.get("src"):
            self.scripts += 1
        if tag == "img":
            self.images += 1
            if d.get("alt", "").strip():
                self.alt_images += 1

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
            p = SignalParser(); p.feed(body)
            final = resp.geturl()
            _assert_public_host(final)
            title_match = re.search(r"<title[^>]*>(.*?)</title>", body, flags=re.I | re.S)
            title = re.sub(r"\s+", " ", title_match.group(1)).strip() if title_match else ""
            years = [int(y) for y in re.findall(r"(?:©|copyright[^0-9]{0,15})(20\d{2})", body, flags=re.I)]
            latest_year = max(years) if years else None
            seo_score = 100
            if not title: seo_score -= 30
            if not p.meta_description: seo_score -= 25
            if p.h1 == 0: seo_score -= 20
            if not p.viewport: seo_score -= 15
            seo_score = max(0, seo_score)
            quality_flags: list[str] = []
            if not p.viewport: quality_flags.append("No mobile viewport")
            if not (p.tel or p.contact or p.forms): quality_flags.append("No clear contact CTA")
            if not p.booking: quality_flags.append("No booking/reservation signal")
            if not final.startswith("https://"): quality_flags.append("HTTPS missing")
            if elapsed > 2500: quality_flags.append("Slow first response")
            if not title: quality_flags.append("Missing page title")
            if not p.meta_description: quality_flags.append("Missing meta description")
            if p.images and p.alt_images / max(1, p.images) < 0.5: quality_flags.append("Many images lack alt text")
            if latest_year and latest_year <= 2022: quality_flags.append(f"Old copyright signal ({latest_year})")
            weak = len(quality_flags) >= 2 or (not p.viewport) or (not final.startswith("https://"))
            return {
                "reachable": True,
                "status": getattr(resp, "status", 200),
                "final_url": final,
                "has_https": final.startswith("https://"),
                "response_ms": elapsed,
                "performance_score": _speed_proxy(elapsed),
                "seo_score": seo_score,
                "mobile_ok": p.viewport,
                "has_cta": p.tel or p.contact or p.forms > 0,
                "has_booking": p.booking,
                "has_tel_link": p.tel,
                "has_contact_path": p.contact,
                "form_count": p.forms,
                "title": title,
                "latest_copyright_year": latest_year,
                "quality_flags": quality_flags,
                "website_status": "weak" if weak else "healthy",
                "note": "Performance is a response-time proxy, not a Lighthouse score.",
            }
    except (urllib.error.URLError, TimeoutError, ssl.SSLError) as exc:
        return {"reachable": False, "error": str(exc), "website_status": "weak"}
