from __future__ import annotations

import json
from datetime import datetime, timedelta, timezone
from typing import Any

from .db import connect, get_lead, update_lead

ENGAGEMENT = {"not_contacted","drafted","sent","delivered","replied","rejected","bounced","no_response"}
ACTIVITY_KINDS = {"note","message","reply","status","follow_up","oauth","agent","audit","enrichment","verification","reputation"}

def _utcnow() -> str:
    return datetime.now(timezone.utc).isoformat()

def add_activity(
    lead_id: int,
    *,
    kind: str,
    channel: str = "",
    status: str = "",
    direction: str = "",
    body: str = "",
    external_id: str = "",
    metadata: dict[str,Any] | None = None,
) -> dict[str,Any]:
    if kind not in ACTIVITY_KINDS:
        raise ValueError(f"Invalid activity kind: {kind}")
    if not get_lead(int(lead_id)):
        raise LookupError("Lead not found.")
    with connect() as conn:
        cur=conn.execute(
            "INSERT INTO lead_activities(lead_id,kind,channel,status,direction,body,external_id,metadata) VALUES (?,?,?,?,?,?,?,?)",
            (int(lead_id),kind,channel or None,status or None,direction or None,body or None,external_id or None,
             json.dumps(metadata or {},ensure_ascii=False)),
        )
        conn.commit()
        activity_id=int(cur.lastrowid)
    if kind=="message" and direction=="out":
        fields={"last_contacted_at":_utcnow()}
        if status in ENGAGEMENT: fields["engagement_status"]=status
        update_lead(int(lead_id),fields)
    if kind=="reply" or (kind=="message" and direction=="in"):
        update_lead(int(lead_id),{"last_reply_at":_utcnow(),"engagement_status":"replied","pipeline_status":"replied"})
    return get_activity(activity_id) or {}

def get_activity(activity_id: int) -> dict[str,Any] | None:
    with connect() as conn:
        row=conn.execute("SELECT * FROM lead_activities WHERE id=?",(int(activity_id),)).fetchone()
    if not row: return None
    item=dict(row)
    try: item["metadata"]=json.loads(item.get("metadata") or "{}")
    except Exception: item["metadata"]={}
    return item

def list_activities(lead_id: int) -> list[dict[str,Any]]:
    with connect() as conn:
        rows=conn.execute("SELECT * FROM lead_activities WHERE lead_id=? ORDER BY created_at DESC,id DESC",(int(lead_id),)).fetchall()
    out=[]
    for row in rows:
        item=dict(row)
        try: item["metadata"]=json.loads(item.get("metadata") or "{}")
        except Exception: item["metadata"]={}
        out.append(item)
    return out

def set_engagement(lead_id: int,status: str,note: str="") -> dict[str,Any]:
    if status not in ENGAGEMENT:
        raise ValueError(f"Invalid engagement status: {status}")
    fields={"engagement_status":status}
    if status=="replied": fields["last_reply_at"]=_utcnow()
    if status in {"sent","delivered"}: fields["last_contacted_at"]=_utcnow()
    lead=update_lead(int(lead_id),fields)
    if not lead: raise LookupError("Lead not found.")
    add_activity(int(lead_id),kind="status",status=status,body=note)
    return lead

def set_follow_up(lead_id: int,when: str | None,note: str="") -> dict[str,Any]:
    lead=update_lead(int(lead_id),{"follow_up_at":when})
    if not lead: raise LookupError("Lead not found.")
    add_activity(int(lead_id),kind="follow_up",status="scheduled" if when else "cleared",body=note,metadata={"follow_up_at":when})
    return lead

def add_note(lead_id: int,note: str) -> dict[str,Any]:
    note=(note or "").strip()
    if not note: raise ValueError("Note cannot be empty.")
    lead=get_lead(int(lead_id))
    if not lead: raise LookupError("Lead not found.")
    existing=(lead.get("notes") or "").strip()
    update_lead(int(lead_id),{"notes":(existing+"\n"+note).strip()})
    return add_activity(int(lead_id),kind="note",body=note)


def update_delivery_status(external_id: str, status: str) -> bool:
    external_id=(external_id or "").strip()
    if not external_id:
        return False
    with connect() as conn:
        row=conn.execute(
            "SELECT id,lead_id FROM lead_activities WHERE external_id=? ORDER BY id DESC LIMIT 1",
            (external_id,),
        ).fetchone()
        if not row:
            return False
        conn.execute(
            "UPDATE lead_activities SET status=? WHERE id=?",
            (status,int(row["id"])),
        )
        conn.commit()
    if status in {"delivered","replied"}:
        fields={"engagement_status":status}
        if status=="delivered":
            fields["last_contacted_at"]=_utcnow()
        else:
            fields["last_reply_at"]=_utcnow()
        update_lead(int(row["lead_id"]),fields)
    return True


def mark_stale_no_response(days: int = 7) -> dict[str,Any]:
    days=max(1,min(365,int(days)))
    cutoff=(datetime.now(timezone.utc)-timedelta(days=days)).isoformat()
    with connect() as conn:
        rows=conn.execute(
            """
            SELECT id FROM leads
            WHERE do_not_contact=0
              AND engagement_status IN ('sent','delivered')
              AND last_contacted_at IS NOT NULL
              AND last_contacted_at < ?
              AND last_reply_at IS NULL
            """,
            (cutoff,),
        ).fetchall()
    updated=[]
    for row in rows:
        lead_id=int(row["id"])
        update_lead(lead_id,{"engagement_status":"no_response"})
        add_activity(
            lead_id,
            kind="status",
            status="no_response",
            metadata={"rule":"stale_no_response","days":days,"cutoff":cutoff},
        )
        updated.append(lead_id)
    return {"updated_count":len(updated),"lead_ids":updated,"days":days,"cutoff":cutoff}
