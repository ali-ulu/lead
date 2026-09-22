from __future__ import annotations

import base64
import json
import os
import secrets
import urllib.parse
import urllib.request
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from cryptography.fernet import Fernet

from .crm import add_activity
from .db import connect, get_lead, update_lead

ROOT = Path(__file__).resolve().parents[1]
KEY_PATH = ROOT / "data" / "token.key"

DEFAULT_SCOPES = (
    "pages_show_list,"
    "pages_messaging,"
    "pages_manage_metadata,"
    "instagram_basic,"
    "instagram_manage_messages"
)

def _graph_version() -> str:
    return os.environ.get("META_GRAPH_VERSION", "").strip()

def _graph_base() -> str:
    version = _graph_version()
    return f"https://graph.facebook.com/{version}" if version else "https://graph.facebook.com"

def _fernet() -> Fernet:
    configured=os.environ.get("LEADSCOUT_TOKEN_KEY","").strip()
    if configured:
        key=configured.encode("utf-8")
    else:
        KEY_PATH.parent.mkdir(parents=True,exist_ok=True)
        if KEY_PATH.exists():
            key=KEY_PATH.read_bytes().strip()
        else:
            key=Fernet.generate_key()
            KEY_PATH.write_bytes(key)
            try: os.chmod(KEY_PATH,0o600)
            except OSError: pass
    return Fernet(key)

def _encrypt(token: str) -> str:
    return _fernet().encrypt(token.encode("utf-8")).decode("ascii")

def _decrypt(token: str) -> str:
    return _fernet().decrypt(token.encode("ascii")).decode("utf-8")

def _request_json(url: str, *, method: str="GET", data: dict[str,Any] | None=None, token: str="") -> dict[str,Any]:
    headers={"Accept":"application/json","User-Agent":"LeadScout/5.0"}
    body=None
    if method=="POST":
        body=json.dumps(data or {}).encode("utf-8")
        headers["Content-Type"]="application/json"
    if token:
        sep="&" if "?" in url else "?"
        url=f"{url}{sep}access_token={urllib.parse.quote(token)}"
    req=urllib.request.Request(url,data=body,headers=headers,method=method)
    with urllib.request.urlopen(req,timeout=25) as resp:
        return json.loads(resp.read().decode("utf-8"))

def meta_configured() -> bool:
    return bool(os.environ.get("META_APP_ID","").strip() and os.environ.get("META_APP_SECRET","").strip())

def create_oauth_state() -> str:
    state=secrets.token_urlsafe(32)
    with connect() as conn:
        conn.execute("INSERT INTO oauth_states(state,provider) VALUES (?,?)",(state,"meta"))
        conn.commit()
    return state

def _consume_state(state: str) -> None:
    with connect() as conn:
        row=conn.execute("SELECT state FROM oauth_states WHERE state=? AND provider='meta'",(state,)).fetchone()
        if not row: raise ValueError("Invalid or expired OAuth state.")
        conn.execute("DELETE FROM oauth_states WHERE state=?",(state,))
        conn.commit()

def oauth_start_url() -> str:
    app_id=os.environ.get("META_APP_ID","").strip()
    if not app_id: raise RuntimeError("META_APP_ID is not configured.")
    redirect=os.environ.get("META_REDIRECT_URI","http://127.0.0.1:8787/api/v1/oauth/meta/callback").strip()
    scopes=os.environ.get("META_SCOPES",DEFAULT_SCOPES).strip()
    state=create_oauth_state()
    params=urllib.parse.urlencode({
        "client_id":app_id,
        "redirect_uri":redirect,
        "state":state,
        "scope":scopes,
        "response_type":"code",
    })
    return f"https://www.facebook.com/dialog/oauth?{params}"

def _save_connection(provider: str, account_id: str, account_name: str, token: str, metadata: dict[str,Any]) -> None:
    enc=_encrypt(token)
    with connect() as conn:
        conn.execute(
            """
            INSERT INTO oauth_connections(provider,account_id,account_name,access_token,metadata)
            VALUES (?,?,?,?,?)
            ON CONFLICT(provider,account_id) DO UPDATE SET
              account_name=excluded.account_name,
              access_token=excluded.access_token,
              metadata=excluded.metadata,
              updated_at=CURRENT_TIMESTAMP
            """,
            (provider,account_id,account_name,enc,json.dumps(metadata,ensure_ascii=False)),
        )
        conn.commit()

def handle_callback(code: str, state: str) -> list[dict[str,Any]]:
    if not meta_configured(): raise RuntimeError("Meta OAuth is not configured.")
    _consume_state(state)
    app_id=os.environ["META_APP_ID"].strip()
    secret=os.environ["META_APP_SECRET"].strip()
    redirect=os.environ.get("META_REDIRECT_URI","http://127.0.0.1:8787/api/v1/oauth/meta/callback").strip()
    params=urllib.parse.urlencode({
        "client_id":app_id,"client_secret":secret,"redirect_uri":redirect,"code":code
    })
    token_data=_request_json(f"{_graph_base()}/oauth/access_token?{params}")
    user_token=token_data.get("access_token")
    if not user_token: raise RuntimeError("Meta did not return an access token.")

    pages=_request_json(
        f"{_graph_base()}/me/accounts?fields=id,name,access_token,instagram_business_account{{id,username}}",
        token=user_token,
    ).get("data",[])
    saved=[]
    for page in pages:
        page_id=str(page.get("id") or "")
        page_token=page.get("access_token") or user_token
        if page_id:
            meta={"kind":"facebook_page"}
            _save_connection("facebook",page_id,str(page.get("name") or page_id),page_token,meta)
            saved.append({"provider":"facebook","account_id":page_id,"account_name":page.get("name")})
        ig=page.get("instagram_business_account") or {}
        ig_id=str(ig.get("id") or "")
        if ig_id:
            meta={"kind":"instagram_business","page_id":page_id,"username":ig.get("username")}
            _save_connection("instagram",ig_id,str(ig.get("username") or ig_id),page_token,meta)
            saved.append({"provider":"instagram","account_id":ig_id,"account_name":ig.get("username")})
    return saved

def list_connections() -> list[dict[str,Any]]:
    with connect() as conn:
        rows=conn.execute("SELECT id,provider,account_id,account_name,token_expires_at,metadata,created_at,updated_at FROM oauth_connections ORDER BY provider,account_name").fetchall()
    out=[]
    for row in rows:
        item=dict(row)
        try: item["metadata"]=json.loads(item.get("metadata") or "{}")
        except Exception: item["metadata"]={}
        out.append(item)
    return out

def _connection(provider: str, connection_id: int | None=None) -> tuple[dict[str,Any],str]:
    with connect() as conn:
        if connection_id is not None:
            row=conn.execute("SELECT * FROM oauth_connections WHERE id=? AND provider=?",(int(connection_id),provider)).fetchone()
        else:
            row=conn.execute("SELECT * FROM oauth_connections WHERE provider=? ORDER BY updated_at DESC LIMIT 1",(provider,)).fetchone()
    if not row: raise LookupError(f"No connected {provider} account.")
    item=dict(row)
    try: item["metadata"]=json.loads(item.get("metadata") or "{}")
    except Exception: item["metadata"]={}
    return item,_decrypt(item["access_token"])

def disconnect(connection_id: int) -> None:
    with connect() as conn:
        conn.execute("DELETE FROM oauth_connections WHERE id=?",(int(connection_id),)); conn.commit()

def send_message(
    *,
    lead_id: int,
    provider: str,
    recipient_id: str | None,
    text: str,
    connection_id: int | None=None,
) -> dict[str,Any]:
    provider=provider.lower().strip()
    if provider not in {"facebook","instagram"}: raise ValueError("provider must be facebook or instagram")
    lead=get_lead(int(lead_id))
    if not lead: raise LookupError("Lead not found.")
    if lead.get("do_not_contact"): raise ValueError("Lead is marked do-not-contact.")
    ids=lead.get("messaging_ids") or {}
    recipient=(recipient_id or ids.get(provider) or "").strip()
    if not recipient:
        raise ValueError(
            f"No {provider} recipient id is stored for this lead. "
            "Meta messaging APIs require an eligible recipient/conversation id; a public username alone is not enough."
        )
    text=(text or "").strip()
    if not text: raise ValueError("Message cannot be empty.")
    conn,token=_connection(provider,connection_id)
    sender_id=conn["account_id"]
    result=_request_json(
        f"{_graph_base()}/{urllib.parse.quote(str(sender_id))}/messages",
        method="POST",
        token=token,
        data={"recipient":{"id":recipient},"message":{"text":text}},
    )
    external_id=str(result.get("message_id") or result.get("id") or "")
    add_activity(
        int(lead_id),kind="message",channel=provider,status="sent",direction="out",
        body=text,external_id=external_id,metadata={"connection_id":conn["id"],"recipient_id":recipient,"response":result},
    )
    update_lead(int(lead_id),{"messaging_ids":{**ids,provider:recipient},"engagement_status":"sent"})
    return {"ok":True,"provider":provider,"recipient_id":recipient,"external_id":external_id,"response":result}

def receive_webhook_event(provider: str, lead_id: int, sender_id: str, text: str, external_id: str="") -> dict[str,Any]:
    lead=get_lead(int(lead_id))
    if not lead: raise LookupError("Lead not found.")
    ids=lead.get("messaging_ids") or {}
    ids[provider]=sender_id
    update_lead(int(lead_id),{"messaging_ids":ids})
    return add_activity(int(lead_id),kind="reply",channel=provider,status="received",direction="in",body=text,external_id=external_id)
