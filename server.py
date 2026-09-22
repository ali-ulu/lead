from __future__ import annotations

import json
import mimetypes
import os
import threading
import urllib.parse
import webbrowser
from http import HTTPStatus
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import parse_qs, urlparse

from lead_hunter.agent import get_job, run_sales_job
from lead_hunter.crm import add_note, list_activities, mark_stale_no_response, set_engagement, set_follow_up
from lead_hunter.db import clear_all, get_lead, get_search_run, initialize, list_leads
from lead_hunter.exporters import csv_bytes, xlsx_bytes
from lead_hunter.oauth_meta import (
    disconnect, facebook_oauth_start_url, handle_callback, handle_facebook_callback,
    handle_instagram_callback, handle_webhook_payload, instagram_oauth_start_url,
    link_recipient, list_connections, messaging_eligibility, meta_configured,
    oauth_start_url, send_message, verify_webhook_signature,
)
from lead_hunter.providers.osm import CATEGORY_FILTERS
from lead_hunter.services import (
    audit_lead, discover_businesses, draft_outreach, enrich_lead, enrich_reputation,
    mark_do_not_contact, set_pipeline_stage, verify_lead,
)

ROOT = Path(__file__).resolve().parent
STATIC = ROOT / "static"
HOST = os.environ.get("LEADSCOUT_HOST", os.environ.get("LEAD_HUNTER_HOST", "127.0.0.1"))
PORT = int(os.environ.get("LEADSCOUT_PORT", os.environ.get("LEAD_HUNTER_PORT", "8787")))
API_TOKEN = os.environ.get("LEADSCOUT_API_TOKEN", "").strip()

class Handler(BaseHTTPRequestHandler):
    server_version = "LeadScout/7.0"

    def log_message(self, fmt, *args):
        print(f"[leadscout] {self.address_string()} - {fmt % args}")

    def _authorized(self, path: str) -> bool:
        public_prefixes=("/api/v1/oauth/meta/","/api/v1/webhooks/meta")
        if any(path.startswith(x) for x in public_prefixes): return True
        if not API_TOKEN or not path.startswith("/api/v1/"): return True
        return self.headers.get("Authorization","")==f"Bearer {API_TOKEN}"

    def _json(self,payload,status=200):
        body=json.dumps(payload,ensure_ascii=False).encode("utf-8")
        self.send_response(status); self.send_header("Content-Type","application/json; charset=utf-8")
        self.send_header("Content-Length",str(len(body))); self.send_header("Cache-Control","no-store")
        self.end_headers(); self.wfile.write(body)

    def _text(self,text:str,status=200):
        body=text.encode("utf-8")
        self.send_response(status); self.send_header("Content-Type","text/plain; charset=utf-8")
        self.send_header("Content-Length",str(len(body))); self.end_headers(); self.wfile.write(body)

    def _bytes(self,body:bytes,content_type:str,filename:str):
        self.send_response(200); self.send_header("Content-Type",content_type)
        self.send_header("Content-Disposition",f'attachment; filename="{filename}"')
        self.send_header("Content-Length",str(len(body))); self.send_header("Cache-Control","no-store")
        self.end_headers(); self.wfile.write(body)

    def _body_bytes(self):
        try:
            length=int(self.headers.get("Content-Length","0"))
            return self.rfile.read(length) if length>0 else b""
        except Exception:
            return b""

    @staticmethod
    def _json_from_bytes(raw: bytes):
        try:
            return json.loads(raw.decode("utf-8")) if raw else {}
        except Exception:
            return {}

    @staticmethod
    def _query(parsed): return {k:v[0] for k,v in parse_qs(parsed.query).items() if v}

    @staticmethod
    def _lead_id(path:str,suffix:str="")->int:
        clean=path[:-len(suffix)] if suffix and path.endswith(suffix) else path
        return int(clean.rstrip("/").rsplit("/",1)[1])

    def _redirect(self,url:str):
        self.send_response(302); self.send_header("Location",url); self.end_headers()

    def do_OPTIONS(self):
        self.send_response(204)
        self.send_header("Access-Control-Allow-Origin","http://127.0.0.1")
        self.send_header("Access-Control-Allow-Headers","Content-Type, Authorization")
        self.send_header("Access-Control-Allow-Methods","GET, POST, OPTIONS")
        self.end_headers()

    def do_GET(self):
        parsed=urlparse(self.path); path=parsed.path
        if not self._authorized(path): return self._json({"error":"unauthorized"},401)

        if path in {"/api/health","/api/v1/health"}:
            return self._json({
                "ok":True,"name":"LeadScout","version":"7.0.0",
                "providers":["OpenStreetMap/Overpass","Overture Places","Brave/SearXNG web verification","Google Places reputation (optional)"],
                "languages":["en","tr","ur","sd","de"],"agent_api":"/api/v1",
                "openapi":"/api/v1/openapi.json","mcp":"mcp_server.py",
                "meta_oauth_configured":{
                    "facebook":meta_configured("facebook"),
                    "instagram":meta_configured("instagram"),
                },
            })
        if path in {"/api/categories","/api/v1/categories"}:
            return self._json({"items":sorted(CATEGORY_FILTERS.keys())})
        if path=="/api/v1/openapi.json":
            spec=(ROOT/"docs"/"openapi.json").read_bytes()
            self.send_response(200); self.send_header("Content-Type","application/json; charset=utf-8")
            self.send_header("Content-Length",str(len(spec))); self.end_headers(); self.wfile.write(spec); return
        if path in {"/api/leads","/api/v1/leads"}:
            return self._json({"items":list_leads(self._query(parsed))})
        if path in {"/api/export.csv","/api/v1/export.csv"}:
            return self._bytes(csv_bytes(list_leads(self._query(parsed))),"text/csv; charset=utf-8","leadscout-leads.csv")
        if path in {"/api/export.xlsx","/api/v1/export.xlsx"}:
            return self._bytes(xlsx_bytes(list_leads(self._query(parsed))),
                "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet","leadscout-leads.xlsx")
        if path=="/api/message":
            q=parse_qs(parsed.query)
            try: return self._json(draft_outreach(int(q.get("id",[""])[0]),q.get("lang",["en"])[0]))
            except ValueError: return self._json({"error":"invalid id"},400)
            except LookupError as exc: return self._json({"error":str(exc)},404)

        if path=="/api/v1/webhooks/meta":
            q=parse_qs(parsed.query)
            verify=os.environ.get("META_WEBHOOK_VERIFY_TOKEN","").strip()
            if q.get("hub.mode",[""])[0]=="subscribe" and verify and q.get("hub.verify_token",[""])[0]==verify:
                return self._text(q.get("hub.challenge",[""])[0])
            return self._text("forbidden",403)
        if path in {"/api/v1/oauth/meta/start","/api/v1/oauth/meta/facebook/start"}:
            try: return self._redirect(facebook_oauth_start_url())
            except Exception as exc: return self._json({"error":str(exc)},400)
        if path=="/api/v1/oauth/meta/instagram/start":
            try: return self._redirect(instagram_oauth_start_url())
            except Exception as exc: return self._json({"error":str(exc)},400)

        if path in {"/api/v1/oauth/meta/callback","/api/v1/oauth/meta/facebook/callback"}:
            q=parse_qs(parsed.query)
            if q.get("error"): return self._redirect("/?meta=facebook_error")
            try:
                handle_facebook_callback(q.get("code",[""])[0],q.get("state",[""])[0])
                return self._redirect("/?meta=facebook_connected")
            except Exception:
                return self._redirect("/?meta=facebook_error")

        if path=="/api/v1/oauth/meta/instagram/callback":
            q=parse_qs(parsed.query)
            if q.get("error"): return self._redirect("/?meta=instagram_error")
            try:
                handle_instagram_callback(q.get("code",[""])[0],q.get("state",[""])[0])
                return self._redirect("/?meta=instagram_connected")
            except Exception:
                return self._redirect("/?meta=instagram_error")
        if path=="/api/v1/oauth/connections":
            return self._json({"items":list_connections()})
        if path.startswith("/api/v1/searches/"):
            item=get_search_run(path.rsplit("/",1)[1])
            return self._json(item or {"error":"not found"},200 if item else 404)
        if path.startswith("/api/v1/agent/jobs/"):
            item=get_job(path.rsplit("/",1)[1])
            return self._json(item or {"error":"not found"},200 if item else 404)
        if path.startswith("/api/v1/leads/") and path.endswith("/activities"):
            try: return self._json({"items":list_activities(self._lead_id(path,"/activities"))})
            except ValueError: return self._json({"error":"invalid id"},400)
        if path.startswith("/api/v1/leads/") and path.endswith("/messaging-eligibility"):
            try:
                q=parse_qs(parsed.query); provider=q.get("provider",["instagram"])[0]
                return self._json(messaging_eligibility(self._lead_id(path,"/messaging-eligibility"),provider))
            except (ValueError,LookupError) as exc: return self._json({"error":str(exc)},400)
        if path.startswith("/api/v1/leads/") and path.endswith("/message"):
            try:
                lead_id=self._lead_id(path,"/message"); lang=parse_qs(parsed.query).get("lang",["en"])[0]
                return self._json(draft_outreach(lead_id,lang))
            except ValueError: return self._json({"error":"invalid id"},400)
            except LookupError as exc: return self._json({"error":str(exc)},404)
        if path.startswith("/api/leads/") or path.startswith("/api/v1/leads/"):
            try: lead_id=self._lead_id(path)
            except ValueError: return self._json({"error":"invalid id"},400)
            lead=get_lead(lead_id)
            return self._json(lead or {"error":"not found"},200 if lead else 404)

        return self._serve_static(path)

    def do_POST(self):
        parsed=urlparse(self.path); path=parsed.path
        raw_body=self._body_bytes()
        payload=self._json_from_bytes(raw_body)
        if not self._authorized(path): return self._json({"error":"unauthorized"},401)

        if path=="/api/v1/webhooks/meta":
            try:
                provider=parse_qs(parsed.query).get("provider",["facebook"])[0]
                signature=self.headers.get("X-Hub-Signature-256","")
                allow_unsigned=os.environ.get("LEADSCOUT_META_ALLOW_UNSIGNED_WEBHOOK","0")=="1"
                if not allow_unsigned and not verify_webhook_signature(raw_body,signature,provider):
                    return self._json({"error":"invalid webhook signature"},401)
                return self._json(handle_webhook_payload(provider,payload))
            except Exception as exc:
                return self._json({"error":str(exc)},400)

        if path in {"/api/discover","/api/v1/search"}:
            try:
                raw=payload.get("max_results")
                return self._json(discover_businesses(
                    city=str(payload.get("city") or ""),country=str(payload.get("country") or ""),
                    category=str(payload.get("category") or ""),radius_km=int(payload.get("radius_km") or 20),
                    max_results=int(raw) if raw not in (None,"",0,"0") else None,
                    providers=payload.get("providers") if isinstance(payload.get("providers"),list) else None,
                ))
            except ValueError as exc: return self._json({"error":str(exc)},400)
            except Exception as exc: return self._json({"error":str(exc)},502)

        for suffix,func in (("/audit",audit_lead),("/enrich",enrich_lead),("/verify",verify_lead),("/reputation",enrich_reputation)):
            if (path.startswith("/api/leads/") or path.startswith("/api/v1/leads/")) and path.endswith(suffix):
                try: return self._json(func(self._lead_id(path,suffix)))
                except ValueError as exc: return self._json({"error":str(exc)},400)
                except LookupError as exc: return self._json({"error":str(exc)},404)
                except Exception as exc: return self._json({"error":str(exc)},502)

        if (path.startswith("/api/leads/") or path.startswith("/api/v1/leads/")) and path.endswith("/status"):
            try: return self._json(set_pipeline_stage(self._lead_id(path,"/status"),str(payload.get("status") or "").strip()))
            except ValueError as exc: return self._json({"error":str(exc)},400)
            except LookupError as exc: return self._json({"error":str(exc)},404)
        if path.startswith("/api/v1/leads/") and path.endswith("/engagement"):
            try: return self._json({"lead":set_engagement(self._lead_id(path,"/engagement"),str(payload.get("status") or ""),str(payload.get("note") or ""))})
            except (ValueError,LookupError) as exc: return self._json({"error":str(exc)},400)
        if path.startswith("/api/v1/leads/") and path.endswith("/follow-up"):
            try: return self._json({"lead":set_follow_up(self._lead_id(path,"/follow-up"),payload.get("when"),str(payload.get("note") or ""))})
            except (ValueError,LookupError) as exc: return self._json({"error":str(exc)},400)
        if path.startswith("/api/v1/leads/") and path.endswith("/notes"):
            try: return self._json({"activity":add_note(self._lead_id(path,"/notes"),str(payload.get("note") or ""))})
            except (ValueError,LookupError) as exc: return self._json({"error":str(exc)},400)
        if path.startswith("/api/v1/leads/") and path.endswith("/messaging-recipient"):
            try:
                return self._json(link_recipient(
                    self._lead_id(path,"/messaging-recipient"),
                    str(payload.get("provider") or ""),
                    str(payload.get("recipient_id") or ""),
                ))
            except (ValueError,LookupError) as exc: return self._json({"error":str(exc)},400)

        if path.startswith("/api/v1/leads/") and path.endswith("/send"):
            try:
                return self._json(send_message(
                    lead_id=self._lead_id(path,"/send"),provider=str(payload.get("provider") or ""),
                    recipient_id=payload.get("recipient_id"),text=str(payload.get("text") or ""),
                    connection_id=int(payload["connection_id"]) if payload.get("connection_id") else None,
                ))
            except (ValueError,LookupError) as exc: return self._json({"error":str(exc)},400)
            except Exception as exc: return self._json({"error":str(exc)},502)
        if (path.startswith("/api/leads/") or path.startswith("/api/v1/leads/")) and path.endswith("/dnc"):
            try: return self._json(mark_do_not_contact(self._lead_id(path,"/dnc")))
            except Exception as exc: return self._json({"error":str(exc)},400)

        if path=="/api/v1/crm/refresh-no-response":
            try:
                return self._json(mark_stale_no_response(int(payload.get("days") or 7)))
            except Exception as exc:
                return self._json({"error":str(exc)},400)

        if path=="/api/v1/agent/run":
            try: return self._json(run_sales_job(**payload))
            except Exception as exc: return self._json({"error":str(exc)},502)
        if path.startswith("/api/v1/oauth/connections/") and path.endswith("/disconnect"):
            try:
                cid=int(path.split("/")[-2]); disconnect(cid); return self._json({"ok":True})
            except Exception as exc: return self._json({"error":str(exc)},400)
        if path in {"/api/clear","/api/v1/clear"}:
            if path=="/api/v1/clear" and payload.get("confirm") is not True:
                return self._json({"error":"confirm=true is required"},400)
            clear_all(); return self._json({"ok":True})
        return self._json({"error":"not found"},404)

    def _serve_static(self,path):
        if path=="/": path="/index.html"
        candidate=(STATIC/path.lstrip("/")).resolve()
        if STATIC.resolve() not in candidate.parents and candidate!=STATIC.resolve():
            return self.send_error(HTTPStatus.FORBIDDEN)
        if not candidate.is_file(): return self.send_error(HTTPStatus.NOT_FOUND)
        body=candidate.read_bytes(); self.send_response(HTTPStatus.OK)
        self.send_header("Content-Type",mimetypes.guess_type(candidate.name)[0] or "application/octet-stream")
        self.send_header("Content-Length",str(len(body))); self.send_header("Cache-Control","no-cache")
        self.end_headers(); self.wfile.write(body)

def main(open_browser: bool=False):
    initialize(); url=f"http://{HOST}:{PORT}"; print(f"LeadScout 7.0 ready: {url}")
    if open_browser: threading.Timer(0.8,lambda:webbrowser.open(url)).start()
    ThreadingHTTPServer((HOST,PORT),Handler).serve_forever()

if __name__=="__main__":
    main(open_browser=os.environ.get("LEADSCOUT_OPEN_BROWSER","0")=="1")
