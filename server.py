from __future__ import annotations

import json
import mimetypes
import os
import threading
import webbrowser
from http import HTTPStatus
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import parse_qs, urlparse

from lead_hunter.db import clear_all, get_lead, initialize, list_leads
from lead_hunter.exporters import csv_bytes, xlsx_bytes
from lead_hunter.providers.osm import CATEGORY_FILTERS
from lead_hunter.services import (
    audit_lead,
    discover_businesses,
    draft_outreach,
    mark_do_not_contact,
    set_pipeline_stage,
)

ROOT = Path(__file__).resolve().parent
STATIC = ROOT / "static"
HOST = os.environ.get("LEADSCOUT_HOST", os.environ.get("LEAD_HUNTER_HOST", "127.0.0.1"))
PORT = int(os.environ.get("LEADSCOUT_PORT", os.environ.get("LEAD_HUNTER_PORT", "8787")))
API_TOKEN = os.environ.get("LEADSCOUT_API_TOKEN", "").strip()


class Handler(BaseHTTPRequestHandler):
    server_version = "LeadScout/4.0"

    def log_message(self, fmt, *args):
        print(f"[leadscout] {self.address_string()} - {fmt % args}")

    def _authorized(self, path: str) -> bool:
        if not API_TOKEN or not path.startswith("/api/v1/"):
            return True
        return self.headers.get("Authorization", "") == f"Bearer {API_TOKEN}"

    def _json(self, payload, status=200):
        body = json.dumps(payload, ensure_ascii=False).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.send_header("Cache-Control", "no-store")
        self.end_headers()
        self.wfile.write(body)

    def _bytes(self, body: bytes, content_type: str, filename: str):
        self.send_response(200)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Disposition", f'attachment; filename="{filename}"')
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

    @staticmethod
    def _query(parsed):
        return {k: v[0] for k, v in parse_qs(parsed.query).items() if v}

    @staticmethod
    def _lead_id(path: str, suffix: str = "") -> int:
        clean = path
        if suffix and clean.endswith(suffix):
            clean = clean[: -len(suffix)]
        return int(clean.rstrip("/").rsplit("/", 1)[1])

    def do_OPTIONS(self):
        self.send_response(204)
        self.send_header("Access-Control-Allow-Origin", "http://127.0.0.1")
        self.send_header("Access-Control-Allow-Headers", "Content-Type, Authorization")
        self.send_header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
        self.end_headers()

    def do_GET(self):
        parsed = urlparse(self.path)
        path = parsed.path

        if not self._authorized(path):
            return self._json({"error": "unauthorized"}, 401)

        if path in {"/api/health", "/api/v1/health"}:
            return self._json({
                "ok": True,
                "name": "LeadScout",
                "version": "4.0.0",
                "provider": "OpenStreetMap / Overpass",
                "languages": ["en", "tr", "ur", "sd"],
                "agent_api": "/api/v1",
                "openapi": "/api/v1/openapi.json",
                "mcp": "mcp_server.py",
            })

        if path in {"/api/categories", "/api/v1/categories"}:
            return self._json({"items": sorted(CATEGORY_FILTERS.keys())})

        if path == "/api/v1/openapi.json":
            spec = (ROOT / "docs" / "openapi.json").read_bytes()
            self.send_response(200)
            self.send_header("Content-Type", "application/json; charset=utf-8")
            self.send_header("Content-Length", str(len(spec)))
            self.send_header("Cache-Control", "no-cache")
            self.end_headers()
            self.wfile.write(spec)
            return

        if path in {"/api/leads", "/api/v1/leads"}:
            return self._json({"items": list_leads(self._query(parsed))})

        if path in {"/api/export.csv", "/api/v1/export.csv"}:
            rows = list_leads(self._query(parsed))
            return self._bytes(csv_bytes(rows), "text/csv; charset=utf-8", "leadscout-leads.csv")

        if path in {"/api/export.xlsx", "/api/v1/export.xlsx"}:
            rows = list_leads(self._query(parsed))
            return self._bytes(
                xlsx_bytes(rows),
                "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
                "leadscout-leads.xlsx",
            )

        if path == "/api/message":
            query = parse_qs(parsed.query)
            try:
                lead_id = int(query.get("id", [""])[0])
            except ValueError:
                return self._json({"error": "invalid id"}, 400)
            try:
                return self._json(draft_outreach(lead_id, query.get("lang", ["en"])[0]))
            except LookupError as exc:
                return self._json({"error": str(exc)}, 404)

        if path.startswith("/api/v1/leads/") and path.endswith("/message"):
            try:
                lead_id = self._lead_id(path, "/message")
                lang = parse_qs(parsed.query).get("lang", ["en"])[0]
                return self._json(draft_outreach(lead_id, lang))
            except ValueError:
                return self._json({"error": "invalid id"}, 400)
            except LookupError as exc:
                return self._json({"error": str(exc)}, 404)

        if path.startswith("/api/leads/") or path.startswith("/api/v1/leads/"):
            try:
                lead_id = self._lead_id(path)
            except ValueError:
                return self._json({"error": "invalid id"}, 400)
            lead = get_lead(lead_id)
            return self._json(lead or {"error": "not found"}, 200 if lead else 404)

        return self._serve_static(path)

    def do_POST(self):
        parsed = urlparse(self.path)
        path = parsed.path
        payload = self._body_json()

        if not self._authorized(path):
            return self._json({"error": "unauthorized"}, 401)

        if path in {"/api/discover", "/api/v1/search"}:
            try:
                max_results = payload.get("max_results")
                result = discover_businesses(
                    city=str(payload.get("city") or ""),
                    country=str(payload.get("country") or ""),
                    category=str(payload.get("category") or ""),
                    radius_km=int(payload.get("radius_km") or 20),
                    max_results=int(max_results) if max_results not in (None, "", 0, "0") else None,
                )
                return self._json(result)
            except ValueError as exc:
                return self._json({"error": str(exc)}, 400)
            except Exception as exc:
                return self._json({"error": str(exc)}, 502)

        if (
            (path.startswith("/api/leads/") or path.startswith("/api/v1/leads/"))
            and path.endswith("/audit")
        ):
            try:
                return self._json(audit_lead(self._lead_id(path, "/audit")))
            except ValueError as exc:
                return self._json({"error": str(exc)}, 400)
            except LookupError as exc:
                return self._json({"error": str(exc)}, 404)
            except Exception as exc:
                return self._json({"error": str(exc)}, 502)

        if (
            (path.startswith("/api/leads/") or path.startswith("/api/v1/leads/"))
            and path.endswith("/status")
        ):
            try:
                return self._json(
                    set_pipeline_stage(
                        self._lead_id(path, "/status"),
                        str(payload.get("status") or "").strip(),
                    )
                )
            except ValueError as exc:
                return self._json({"error": str(exc)}, 400)
            except LookupError as exc:
                return self._json({"error": str(exc)}, 404)

        if (
            (path.startswith("/api/leads/") or path.startswith("/api/v1/leads/"))
            and path.endswith("/dnc")
        ):
            try:
                return self._json(mark_do_not_contact(self._lead_id(path, "/dnc")))
            except ValueError:
                return self._json({"error": "invalid id"}, 400)
            except LookupError as exc:
                return self._json({"error": str(exc)}, 404)

        if path in {"/api/clear", "/api/v1/clear"}:
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
    print(f"LeadScout 4.0 ready: {url}")
    if open_browser:
        threading.Timer(0.8, lambda: webbrowser.open(url)).start()
    ThreadingHTTPServer((HOST, PORT), Handler).serve_forever()


if __name__ == "__main__":
    main(open_browser=os.environ.get("LEADSCOUT_OPEN_BROWSER", "0") == "1")
