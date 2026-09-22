from __future__ import annotations
import csv
import io
import json
import mimetypes
import os
import threading
import webbrowser
from http import HTTPStatus
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import parse_qs, urlparse

from lead_hunter.audit import audit_url
from lead_hunter.db import clear_all, get_lead, initialize, list_leads, update_lead, upsert_leads
from lead_hunter.outreach import build_message
from lead_hunter.providers.nominatim import geocode_area
from lead_hunter.providers.osm import CATEGORY_FILTERS, search_around

ROOT = Path(__file__).resolve().parent
STATIC = ROOT / "static"
HOST = os.environ.get("LEAD_HUNTER_HOST", "127.0.0.1")
PORT = int(os.environ.get("LEAD_HUNTER_PORT", "8787"))

class Handler(BaseHTTPRequestHandler):
    server_version = "Nishan/3.1"

    def log_message(self, fmt, *args):
        print(f"[nishan] {self.address_string()} - {fmt % args}")

    def _json(self, payload, status=200):
        body = json.dumps(payload, ensure_ascii=False).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.send_header("Cache-Control", "no-store")
        self.end_headers()
        self.wfile.write(body)

    def _body_json(self):
        try:
            length = int(self.headers.get("Content-Length", "0"))
            if length <= 0:
                return {}
            return json.loads(self.rfile.read(length).decode("utf-8"))
        except Exception:
            return {}

    def do_GET(self):
        parsed = urlparse(self.path)
        if parsed.path == "/api/health":
            return self._json({
                "ok": True,
                "name": "Nishan",
                "version": "3.1.0",
                "provider": "OpenStreetMap / Overpass",
                "languages": ["en", "tr", "ur", "sd"],
            })
        if parsed.path == "/api/categories":
            return self._json({"items": sorted(CATEGORY_FILTERS.keys())})
        if parsed.path == "/api/leads":
            query = {k: v[0] for k, v in parse_qs(parsed.query).items() if v}
            return self._json({"items": list_leads(query)})
        if parsed.path.startswith("/api/leads/"):
            try:
                lead_id = int(parsed.path.rsplit("/", 1)[1])
            except ValueError:
                return self._json({"error": "invalid id"}, 400)
            lead = get_lead(lead_id)
            return self._json(lead or {"error": "not found"}, 200 if lead else 404)
        if parsed.path == "/api/message":
            query = parse_qs(parsed.query)
            try:
                lead_id = int(query.get("id", [""])[0])
            except ValueError:
                return self._json({"error": "invalid id"}, 400)
            lang = query.get("lang", ["en"])[0]
            lang = lang if lang in {"en", "tr", "ur", "sd", "de"} else "en"
            lead = get_lead(lead_id)
            if not lead:
                return self._json({"error": "not found"}, 404)
            return self._json({"message": build_message(lead, lang)})
        if parsed.path == "/api/export.csv":
            query = {k: v[0] for k, v in parse_qs(parsed.query).items() if v}
            rows = list_leads(query)
            buf = io.StringIO()
            fields = [
                "id","name","country","city","category","lead_score","website_status",
                "website","phone","email","social_links","pipeline_status","source","source_id"
            ]
            writer = csv.DictWriter(buf, fieldnames=fields, extrasaction="ignore")
            writer.writeheader()
            for row in rows:
                item = dict(row)
                item["social_links"] = json.dumps(item.get("social_links") or {}, ensure_ascii=False)
                writer.writerow(item)
            body = buf.getvalue().encode("utf-8-sig")
            self.send_response(200)
            self.send_header("Content-Type", "text/csv; charset=utf-8")
            self.send_header("Content-Disposition", 'attachment; filename="nishan-leads.csv"')
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            return
        return self._serve_static(parsed.path)

    def do_POST(self):
        parsed = urlparse(self.path)
        payload = self._body_json()

        if parsed.path == "/api/discover":
            city = str(payload.get("city") or "").strip()
            country = str(payload.get("country") or "").strip()
            category = str(payload.get("category") or "").strip()
            if not city or not category:
                return self._json({"error": "City/area and industry are required."}, 400)
            if category not in CATEGORY_FILTERS:
                return self._json({"error": "Unsupported industry."}, 400)
            try:
                area = geocode_area(city, country)
                radius_km = max(3, min(50, int(payload.get("radius_km") or 20)))
                rows = search_around(
                    area["lat"], area["lon"], radius_km, category,
                    city=area["city"], country=area["country"]
                )
                ids = upsert_leads(rows)
                return self._json({"ok": True, "count": len(rows), "ids": ids, "area": area})
            except Exception as exc:
                return self._json({"error": str(exc)}, 502)

        if parsed.path.startswith("/api/leads/") and parsed.path.endswith("/audit"):
            parts = parsed.path.strip("/").split("/")
            try:
                lead_id = int(parts[2])
            except Exception:
                return self._json({"error": "invalid id"}, 400)
            lead = get_lead(lead_id)
            if not lead:
                return self._json({"error": "not found"}, 404)
            if not lead.get("website"):
                return self._json({"error": "This lead has no website to audit."}, 400)

            result = audit_url(lead["website"])
            existing_socials = lead.get("social_links") or {}
            discovered_socials = result.get("social_links") or {}
            merged_socials = {**existing_socials, **discovered_socials}
            primary_social = lead.get("social_url")
            if not primary_social and merged_socials:
                primary_social = next(iter(merged_socials.values()))

            updates = {
                "website_status": result.get("website_status", "weak"),
                "performance_score": result.get("performance_score"),
                "seo_score": result.get("seo_score"),
                "mobile_ok": result.get("mobile_ok"),
                "has_cta": result.get("has_cta"),
                "has_booking": result.get("has_booking"),
                "has_https": result.get("has_https"),
                "social_links": merged_socials,
                "social_url": primary_social,
            }
            lead = update_lead(lead_id, updates)
            return self._json({"lead": lead, "audit": result})

        if parsed.path.startswith("/api/leads/") and parsed.path.endswith("/status"):
            parts = parsed.path.strip("/").split("/")
            try:
                lead_id = int(parts[2])
            except Exception:
                return self._json({"error": "invalid id"}, 400)
            status = str(payload.get("status") or "").strip()
            allowed = {"new", "reviewed", "contacted", "replied", "proposal", "won", "lost"}
            if status not in allowed:
                return self._json({"error": "invalid status"}, 400)
            lead = update_lead(lead_id, {"pipeline_status": status})
            return self._json({"lead": lead})

        if parsed.path.startswith("/api/leads/") and parsed.path.endswith("/dnc"):
            parts = parsed.path.strip("/").split("/")
            try:
                lead_id = int(parts[2])
            except Exception:
                return self._json({"error": "invalid id"}, 400)
            update_lead(lead_id, {"do_not_contact": 1})
            return self._json({"ok": True})

        if parsed.path == "/api/clear":
            clear_all()
            return self._json({"ok": True})
        return self._json({"error": "not found"}, 404)

    def _serve_static(self, path):
        if path == "/":
            path = "/index.html"
        candidate = (STATIC / path.lstrip("/")).resolve()
        if STATIC.resolve() not in candidate.parents and candidate != STATIC.resolve():
            return self.send_error(HTTPStatus.FORBIDDEN)
        if not candidate.is_file():
            return self.send_error(HTTPStatus.NOT_FOUND)
        body = candidate.read_bytes()
        self.send_response(HTTPStatus.OK)
        self.send_header("Content-Type", mimetypes.guess_type(candidate.name)[0] or "application/octet-stream")
        self.send_header("Content-Length", str(len(body)))
        self.send_header("Cache-Control", "no-cache")
        self.end_headers()
        self.wfile.write(body)

def main(open_browser: bool = False):
    initialize()
    url = f"http://{HOST}:{PORT}"
    print(f"Nishan v3.1 ready: {url}")
    if open_browser:
        threading.Timer(0.8, lambda: webbrowser.open(url)).start()
    ThreadingHTTPServer((HOST, PORT), Handler).serve_forever()

if __name__ == "__main__":
    main(open_browser=os.environ.get("LEAD_HUNTER_OPEN_BROWSER", "0") == "1")
