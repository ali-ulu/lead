from __future__ import annotations

import hashlib
import hmac
import json
import os
import secrets
import urllib.parse
import urllib.request
from datetime import datetime, timedelta, timezone
from pathlib import Path
from typing import Any

from cryptography.fernet import Fernet

from .crm import add_activity, update_delivery_status
from .db import connect, get_lead, update_lead

ROOT = Path(__file__).resolve().parents[1]
KEY_PATH = ROOT / "data" / "token.key"

FACEBOOK_DEFAULT_SCOPES = "pages_show_list,pages_messaging,pages_manage_metadata,pages_read_engagement"
INSTAGRAM_DEFAULT_SCOPES = "instagram_business_basic,instagram_business_manage_messages"


def _graph_version() -> str:
    return os.environ.get("META_GRAPH_VERSION", "").strip()


def _facebook_base() -> str:
    version = _graph_version()
    return f"https://graph.facebook.com/{version}" if version else "https://graph.facebook.com"


def _instagram_base() -> str:
    version = os.environ.get("META_INSTAGRAM_GRAPH_VERSION", _graph_version()).strip()
    return f"https://graph.instagram.com/{version}" if version else "https://graph.instagram.com"


def _fernet() -> Fernet:
    configured = os.environ.get("LEADSCOUT_TOKEN_KEY", "").strip()
    if configured:
        key = configured.encode("utf-8")
    else:
        KEY_PATH.parent.mkdir(parents=True, exist_ok=True)
        if KEY_PATH.exists():
            key = KEY_PATH.read_bytes().strip()
        else:
            key = Fernet.generate_key()
            KEY_PATH.write_bytes(key)
            try:
                os.chmod(KEY_PATH, 0o600)
            except OSError:
                pass
    return Fernet(key)


def _encrypt(token: str) -> str:
    return _fernet().encrypt(token.encode("utf-8")).decode("ascii")


def _decrypt(token: str) -> str:
    return _fernet().decrypt(token.encode("ascii")).decode("utf-8")


def _request_json(
    url: str,
    *,
    method: str = "GET",
    data: dict[str, Any] | None = None,
    token: str = "",
    form: bool = False,
) -> dict[str, Any]:
    headers = {"Accept": "application/json", "User-Agent": "LeadScout/5.1"}
    body = None
    if method == "POST":
        if form:
            body = urllib.parse.urlencode(data or {}).encode("utf-8")
            headers["Content-Type"] = "application/x-www-form-urlencoded"
        else:
            body = json.dumps(data or {}).encode("utf-8")
            headers["Content-Type"] = "application/json"
    if token:
        sep = "&" if "?" in url else "?"
        url = f"{url}{sep}access_token={urllib.parse.quote(token)}"
    req = urllib.request.Request(url, data=body, headers=headers, method=method)
    with urllib.request.urlopen(req, timeout=25) as resp:
        return json.loads(resp.read().decode("utf-8"))


def meta_configured(provider: str = "") -> bool:
    provider = provider.lower().strip()
    if provider == "instagram":
        app_id = os.environ.get("META_INSTAGRAM_APP_ID", os.environ.get("META_APP_ID", "")).strip()
        secret = os.environ.get("META_INSTAGRAM_APP_SECRET", os.environ.get("META_APP_SECRET", "")).strip()
        return bool(app_id and secret)
    return bool(os.environ.get("META_APP_ID", "").strip() and os.environ.get("META_APP_SECRET", "").strip())


def create_oauth_state(provider: str) -> str:
    provider = provider.lower().strip()
    if provider not in {"facebook", "instagram"}:
        raise ValueError("Unsupported OAuth provider.")
    state = secrets.token_urlsafe(32)
    with connect() as conn:
        conn.execute("INSERT INTO oauth_states(state,provider) VALUES (?,?)", (state, provider))
        conn.commit()
    return state


def _consume_state(state: str, provider: str) -> None:
    with connect() as conn:
        row = conn.execute(
            "SELECT state,provider,created_at FROM oauth_states WHERE state=?",
            (state,),
        ).fetchone()
        if not row or row["provider"] != provider:
            raise ValueError("Invalid OAuth state.")
        conn.execute("DELETE FROM oauth_states WHERE state=?", (state,))
        conn.commit()


def facebook_oauth_start_url() -> str:
    app_id = os.environ.get("META_APP_ID", "").strip()
    if not app_id:
        raise RuntimeError("META_APP_ID is not configured.")
    redirect = os.environ.get(
        "META_FACEBOOK_REDIRECT_URI",
        "http://127.0.0.1:8787/api/v1/oauth/meta/facebook/callback",
    ).strip()
    scopes = os.environ.get("META_FACEBOOK_SCOPES", FACEBOOK_DEFAULT_SCOPES).strip()
    state = create_oauth_state("facebook")
    params = urllib.parse.urlencode({
        "client_id": app_id,
        "redirect_uri": redirect,
        "state": state,
        "scope": scopes,
        "response_type": "code",
    })
    version = _graph_version()
    prefix = f"https://www.facebook.com/{version}/dialog/oauth" if version else "https://www.facebook.com/dialog/oauth"
    return f"{prefix}?{params}"


def instagram_oauth_start_url() -> str:
    app_id = os.environ.get("META_INSTAGRAM_APP_ID", os.environ.get("META_APP_ID", "")).strip()
    if not app_id:
        raise RuntimeError("META_INSTAGRAM_APP_ID or META_APP_ID is not configured.")
    redirect = os.environ.get(
        "META_INSTAGRAM_REDIRECT_URI",
        "http://127.0.0.1:8787/api/v1/oauth/meta/instagram/callback",
    ).strip()
    scopes = os.environ.get("META_INSTAGRAM_SCOPES", INSTAGRAM_DEFAULT_SCOPES).strip()
    state = create_oauth_state("instagram")
    params = urllib.parse.urlencode({
        "client_id": app_id,
        "redirect_uri": redirect,
        "state": state,
        "scope": scopes,
        "response_type": "code",
    })
    return f"https://www.instagram.com/oauth/authorize?{params}"


# Backward-compatible alias: old /oauth/meta/start means Facebook Login.
def oauth_start_url() -> str:
    return facebook_oauth_start_url()


def _save_connection(
    provider: str,
    account_id: str,
    account_name: str,
    token: str,
    metadata: dict[str, Any],
    expires_in: int | None = None,
) -> None:
    enc = _encrypt(token)
    expires_at = None
    if expires_in:
        expires_at = (datetime.now(timezone.utc) + timedelta(seconds=int(expires_in))).isoformat()
    with connect() as conn:
        conn.execute(
            """
            INSERT INTO oauth_connections(provider,account_id,account_name,access_token,token_expires_at,metadata)
            VALUES (?,?,?,?,?,?)
            ON CONFLICT(provider,account_id) DO UPDATE SET
              account_name=excluded.account_name,
              access_token=excluded.access_token,
              token_expires_at=excluded.token_expires_at,
              metadata=excluded.metadata,
              updated_at=CURRENT_TIMESTAMP
            """,
            (
                provider, account_id, account_name, enc, expires_at,
                json.dumps(metadata, ensure_ascii=False),
            ),
        )
        conn.commit()


def handle_facebook_callback(code: str, state: str) -> list[dict[str, Any]]:
    if not meta_configured("facebook"):
        raise RuntimeError("Facebook OAuth is not configured.")
    _consume_state(state, "facebook")
    app_id = os.environ["META_APP_ID"].strip()
    secret = os.environ["META_APP_SECRET"].strip()
    redirect = os.environ.get(
        "META_FACEBOOK_REDIRECT_URI",
        "http://127.0.0.1:8787/api/v1/oauth/meta/facebook/callback",
    ).strip()
    params = urllib.parse.urlencode({
        "client_id": app_id,
        "client_secret": secret,
        "redirect_uri": redirect,
        "code": code,
    })
    token_data = _request_json(f"{_facebook_base()}/oauth/access_token?{params}")
    user_token = token_data.get("access_token")
    if not user_token:
        raise RuntimeError("Meta did not return a Facebook access token.")

    pages = _request_json(
        f"{_facebook_base()}/me/accounts?fields=id,name,access_token,tasks,instagram_business_account{{id,username}}",
        token=user_token,
    ).get("data", [])
    saved = []
    for page in pages:
        page_id = str(page.get("id") or "")
        page_token = page.get("access_token") or user_token
        if page_id:
            _save_connection(
                "facebook", page_id, str(page.get("name") or page_id), page_token,
                {"kind": "facebook_page", "tasks": page.get("tasks") or []},
            )
            saved.append({"provider": "facebook", "account_id": page_id, "account_name": page.get("name")})

        # Keep linked-Page Instagram as a compatibility route.
        ig = page.get("instagram_business_account") or {}
        ig_id = str(ig.get("id") or "")
        if ig_id:
            _save_connection(
                "instagram", ig_id, str(ig.get("username") or ig_id), page_token,
                {
                    "kind": "instagram_via_facebook",
                    "page_id": page_id,
                    "username": ig.get("username"),
                    "api_base": "facebook",
                },
            )
            saved.append({"provider": "instagram", "account_id": ig_id, "account_name": ig.get("username")})
    return saved


def handle_instagram_callback(code: str, state: str) -> list[dict[str, Any]]:
    if not meta_configured("instagram"):
        raise RuntimeError("Instagram OAuth is not configured.")
    _consume_state(state, "instagram")
    app_id = os.environ.get("META_INSTAGRAM_APP_ID", os.environ.get("META_APP_ID", "")).strip()
    secret = os.environ.get("META_INSTAGRAM_APP_SECRET", os.environ.get("META_APP_SECRET", "")).strip()
    redirect = os.environ.get(
        "META_INSTAGRAM_REDIRECT_URI",
        "http://127.0.0.1:8787/api/v1/oauth/meta/instagram/callback",
    ).strip()

    short = _request_json(
        "https://api.instagram.com/oauth/access_token",
        method="POST",
        form=True,
        data={
            "client_id": app_id,
            "client_secret": secret,
            "grant_type": "authorization_code",
            "redirect_uri": redirect,
            "code": code,
        },
    )
    short_token = str(short.get("access_token") or "")
    if not short_token:
        raise RuntimeError("Instagram did not return an access token.")

    long_data = _request_json(
        "https://graph.instagram.com/access_token?"
        + urllib.parse.urlencode({
            "grant_type": "ig_exchange_token",
            "client_secret": secret,
            "access_token": short_token,
        })
    )
    token = str(long_data.get("access_token") or short_token)
    expires_in = int(long_data.get("expires_in") or short.get("expires_in") or 0) or None

    profile = _request_json(
        f"{_instagram_base()}/me?fields=id,username",
        token=token,
    )
    account_id = str(profile.get("id") or short.get("user_id") or "")
    if not account_id:
        raise RuntimeError("Instagram account id was not returned.")
    username = str(profile.get("username") or account_id)
    _save_connection(
        "instagram",
        account_id,
        username,
        token,
        {
            "kind": "instagram_login",
            "username": profile.get("username"),
            "api_base": "instagram",
            "scopes": os.environ.get("META_INSTAGRAM_SCOPES", INSTAGRAM_DEFAULT_SCOPES),
        },
        expires_in=expires_in,
    )
    return [{"provider": "instagram", "account_id": account_id, "account_name": username}]


def handle_callback(code: str, state: str) -> list[dict[str, Any]]:
    # Legacy callback retained for existing local setups.
    return handle_facebook_callback(code, state)


def list_connections() -> list[dict[str, Any]]:
    with connect() as conn:
        rows = conn.execute(
            "SELECT id,provider,account_id,account_name,token_expires_at,metadata,created_at,updated_at "
            "FROM oauth_connections ORDER BY provider,account_name"
        ).fetchall()
    out = []
    for row in rows:
        item = dict(row)
        try:
            item["metadata"] = json.loads(item.get("metadata") or "{}")
        except Exception:
            item["metadata"] = {}
        out.append(item)
    return out


def _connection(provider: str, connection_id: int | None = None) -> tuple[dict[str, Any], str]:
    with connect() as conn:
        if connection_id is not None:
            row = conn.execute(
                "SELECT * FROM oauth_connections WHERE id=? AND provider=?",
                (int(connection_id), provider),
            ).fetchone()
        else:
            row = conn.execute(
                "SELECT * FROM oauth_connections WHERE provider=? ORDER BY updated_at DESC LIMIT 1",
                (provider,),
            ).fetchone()
    if not row:
        raise LookupError(f"No connected {provider} account.")
    item = dict(row)
    try:
        item["metadata"] = json.loads(item.get("metadata") or "{}")
    except Exception:
        item["metadata"] = {}
    return item, _decrypt(item["access_token"])


def disconnect(connection_id: int) -> None:
    with connect() as conn:
        conn.execute("DELETE FROM oauth_connections WHERE id=?", (int(connection_id),))
        conn.commit()


def messaging_eligibility(lead_id: int, provider: str) -> dict[str, Any]:
    provider = provider.lower().strip()
    lead = get_lead(int(lead_id))
    if not lead:
        raise LookupError("Lead not found.")
    ids = lead.get("messaging_ids") or {}
    recipient = str(ids.get(provider) or "")
    if provider == "instagram":
        rule = "Instagram API can reply only when the Instagram user has already messaged the connected professional account."
    elif provider == "facebook":
        rule = "Messenger requires an eligible Page-scoped recipient and messaging-window/permission eligibility."
    else:
        raise ValueError("provider must be facebook or instagram")
    return {
        "provider": provider,
        "eligible": bool(recipient and not lead.get("do_not_contact")),
        "recipient_id": recipient or None,
        "do_not_contact": bool(lead.get("do_not_contact")),
        "rule": rule,
    }


def link_recipient(lead_id: int, provider: str, recipient_id: str) -> dict[str, Any]:
    provider = provider.lower().strip()
    if provider not in {"facebook", "instagram"}:
        raise ValueError("provider must be facebook or instagram")
    lead = get_lead(int(lead_id))
    if not lead:
        raise LookupError("Lead not found.")
    recipient_id = (recipient_id or "").strip()
    if not recipient_id:
        raise ValueError("recipient_id is required")
    ids = lead.get("messaging_ids") or {}
    ids[provider] = recipient_id
    updated = update_lead(int(lead_id), {"messaging_ids": ids}) or lead
    add_activity(
        int(lead_id), kind="oauth", channel=provider, status="recipient_linked",
        metadata={"recipient_id": recipient_id},
    )
    return {"lead": updated, "eligibility": messaging_eligibility(int(lead_id), provider)}


def _send_endpoint(conn: dict[str, Any], provider: str) -> str:
    sender_id = urllib.parse.quote(str(conn["account_id"]))
    meta = conn.get("metadata") or {}
    if provider == "instagram" and meta.get("api_base") == "instagram":
        return f"{_instagram_base()}/{sender_id}/messages"
    return f"{_facebook_base()}/{sender_id}/messages"


def send_message(
    *,
    lead_id: int,
    provider: str,
    recipient_id: str | None,
    text: str,
    connection_id: int | None = None,
) -> dict[str, Any]:
    provider = provider.lower().strip()
    if provider not in {"facebook", "instagram"}:
        raise ValueError("provider must be facebook or instagram")
    lead = get_lead(int(lead_id))
    if not lead:
        raise LookupError("Lead not found.")
    if lead.get("do_not_contact"):
        raise ValueError("Lead is marked do-not-contact.")

    ids = lead.get("messaging_ids") or {}
    recipient = (recipient_id or ids.get(provider) or "").strip()
    if not recipient:
        rule = messaging_eligibility(int(lead_id), provider)["rule"]
        raise ValueError(
            f"No eligible {provider} recipient/conversation id is stored for this lead. {rule} "
            "A public username/profile URL is not a sendable recipient id."
        )
    text = (text or "").strip()
    if not text:
        raise ValueError("Message cannot be empty.")

    conn, token = _connection(provider, connection_id)
    result = _request_json(
        _send_endpoint(conn, provider),
        method="POST",
        token=token,
        data={"recipient": {"id": recipient}, "message": {"text": text}},
    )
    external_id = str(result.get("message_id") or result.get("id") or "")
    add_activity(
        int(lead_id),
        kind="message",
        channel=provider,
        status="sent",
        direction="out",
        body=text,
        external_id=external_id,
        metadata={
            "connection_id": conn["id"],
            "recipient_id": recipient,
            "response": result,
            "platform_rule": messaging_eligibility(int(lead_id), provider)["rule"],
        },
    )
    update_lead(
        int(lead_id),
        {"messaging_ids": {**ids, provider: recipient}, "engagement_status": "sent"},
    )
    return {
        "ok": True,
        "provider": provider,
        "recipient_id": recipient,
        "external_id": external_id,
        "response": result,
    }


def verify_webhook_signature(raw_body: bytes, signature: str, provider: str = "facebook") -> bool:
    provider = provider.lower().strip()
    secret = (
        os.environ.get("META_INSTAGRAM_APP_SECRET", os.environ.get("META_APP_SECRET", "")).strip()
        if provider == "instagram"
        else os.environ.get("META_APP_SECRET", "").strip()
    )
    if not secret or not signature or not signature.startswith("sha256="):
        return False
    expected = "sha256=" + hmac.new(secret.encode("utf-8"), raw_body, hashlib.sha256).hexdigest()
    return hmac.compare_digest(expected, signature)


def receive_webhook_event(provider: str, lead_id: int, sender_id: str, text: str, external_id: str = "") -> dict[str, Any]:
    lead = get_lead(int(lead_id))
    if not lead:
        raise LookupError("Lead not found.")
    ids = lead.get("messaging_ids") or {}
    ids[provider] = sender_id
    update_lead(int(lead_id), {"messaging_ids": ids})
    return add_activity(
        int(lead_id), kind="reply", channel=provider, status="received",
        direction="in", body=text, external_id=external_id,
    )


def _lead_for_sender(provider: str, sender_id: str) -> dict[str, Any] | None:
    with connect() as conn:
        try:
            row = conn.execute(
                "SELECT * FROM leads WHERE json_extract(messaging_ids, ?) = ? LIMIT 1",
                (f"$.{provider}", str(sender_id)),
            ).fetchone()
        except Exception:
            row = None
    if not row:
        return None
    return get_lead(int(row["id"]))


def handle_webhook_payload(provider: str, payload: dict[str, Any]) -> dict[str, Any]:
    provider = provider if provider in {"facebook", "instagram"} else "facebook"
    handled = 0
    unmatched = 0
    status_updates = 0

    for entry in payload.get("entry") or []:
        events = list(entry.get("messaging") or [])
        for change in entry.get("changes") or []:
            value = change.get("value") or {}
            events.extend(value.get("messages") or [])

        for event in events:
            # Messenger delivery/read events.
            delivery = event.get("delivery") or {}
            mids = delivery.get("mids") or []
            for mid in mids:
                if update_delivery_status(str(mid), "delivered"):
                    status_updates += 1

            if event.get("read"):
                # Read receipts often do not include a message id; preserve as an account event.
                sender = (event.get("sender") or {}).get("id")
                lead = _lead_for_sender(provider, str(sender)) if sender else None
                if lead:
                    add_activity(int(lead["id"]), kind="status", channel=provider, status="read")
                    status_updates += 1

            sender = (event.get("sender") or {}).get("id") or event.get("from")
            message = event.get("message") or {}
            if isinstance(message, str):
                text = message
                mid = str(event.get("id") or "")
                is_echo = False
            else:
                text = str(message.get("text") or "")
                mid = str(message.get("mid") or event.get("id") or "")
                is_echo = bool(message.get("is_echo"))
            if is_echo or not sender or not text:
                continue
            lead = _lead_for_sender(provider, str(sender))
            if not lead:
                unmatched += 1
                continue
            receive_webhook_event(provider, int(lead["id"]), str(sender), text, mid)
            handled += 1

    return {
        "ok": True,
        "handled": handled,
        "unmatched": unmatched,
        "status_updates": status_updates,
    }
