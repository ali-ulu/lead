from __future__ import annotations

import re
import ssl
import urllib.parse
import urllib.request
from html.parser import HTMLParser
from typing import Any

from .audit import _assert_public_host, normalize_url

USER_AGENT = "Mozilla/5.0 (compatible; LeadScout/5.0; contact-enrichment)"
EMAIL_RE = re.compile(r"\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b", re.I)
PHONE_RE = re.compile(r"(?:\+?\d[\d\s().-]{7,}\d)")
SOCIAL_HOSTS = {
    "instagram": ("instagram.com",), "facebook": ("facebook.com","fb.com"),
    "linkedin": ("linkedin.com",), "x": ("x.com","twitter.com"),
    "youtube": ("youtube.com","youtu.be"), "tiktok": ("tiktok.com",),
    "telegram": ("t.me","telegram.me"), "whatsapp": ("wa.me","api.whatsapp.com","whatsapp.com"),
}
FOLLOW_HINTS = ("contact","iletisim","iletişim","kontakt","about","reservation","booking","appointment","randevu","termin")

class PageParser(HTMLParser):
    def __init__(self, base: str):
        super().__init__()
        self.base=base
        self.links: list[str]=[]
        self.socials: dict[str,str]={}
        self.mailtos: list[str]=[]
        self.tels: list[str]=[]
        self.booking_links: list[str]=[]

    def handle_starttag(self, tag, attrs):
        if tag.lower()!="a": return
        d={k.lower():(v or "") for k,v in attrs}
        href=d.get("href","").strip()
        if not href: return
        if href.startswith("mailto:"):
            self.mailtos.append(href[7:].split("?")[0]); return
        if href.startswith("tel:"):
            self.tels.append(href[4:]); return
        url=urllib.parse.urljoin(self.base,href)
        parsed=urllib.parse.urlparse(url)
        host=(parsed.hostname or "").lower()
        if parsed.scheme not in {"http","https"}: return
        self.links.append(url)
        lower=url.lower()
        for platform,hosts in SOCIAL_HOSTS.items():
            if any(host==h or host.endswith("."+h) for h in hosts):
                self.socials.setdefault(platform,url)
        if any(h in lower for h in ("book","reservation","appointment","randevu","termin","calendly")):
            self.booking_links.append(url)

def _fetch(url: str, timeout: float=10.0) -> tuple[str,str]:
    url=normalize_url(url); _assert_public_host(url)
    req=urllib.request.Request(url,headers={"User-Agent":USER_AGENT,"Accept":"text/html"})
    with urllib.request.urlopen(req,timeout=timeout,context=ssl.create_default_context()) as resp:
        final=resp.geturl(); _assert_public_host(final)
        raw=resp.read(1_200_000)
        return final, raw.decode(resp.headers.get_content_charset() or "utf-8",errors="replace")

def enrich_website(url: str, max_pages: int=4) -> dict[str,Any]:
    start=normalize_url(url)
    root=urllib.parse.urlparse(start)
    root_host=(root.hostname or "").lower()
    queue=[start]; seen=set()
    emails:set[str]=set(); phones:set[str]=set(); socials:dict[str,str]={}; booking:set[str]=set()
    pages=[]; errors=[]
    while queue and len(pages)<max_pages:
        current=queue.pop(0)
        if current in seen: continue
        seen.add(current)
        try:
            final,html=_fetch(current)
        except Exception as exc:
            errors.append(f"{current}: {exc}"); continue
        pages.append(final)
        parser=PageParser(final); parser.feed(html)
        emails.update(parser.mailtos); emails.update(EMAIL_RE.findall(html))
        phones.update(parser.tels)
        for raw in PHONE_RE.findall(html):
            digits="".join(ch for ch in raw if ch.isdigit() or ch=="+")
            if len("".join(ch for ch in digits if ch.isdigit()))>=8:
                phones.add(digits)
        socials.update(parser.socials); booking.update(parser.booking_links)
        for link in parser.links:
            p=urllib.parse.urlparse(link)
            if (p.hostname or "").lower().removeprefix("www.") != root_host.removeprefix("www."):
                continue
            low=link.lower()
            if any(h in low for h in FOLLOW_HINTS) and link not in seen and link not in queue:
                queue.append(link)

    clean_emails=sorted(x.strip().lower() for x in emails if "@" in x)[:20]
    clean_phones=sorted(x.strip() for x in phones if x.strip())[:20]
    return {
        "pages_scanned":pages,
        "emails":clean_emails,
        "phones":clean_phones,
        "social_links":socials,
        "booking_links":sorted(booking)[:20],
        "errors":errors[:10],
        "complete":bool(pages),
    }
