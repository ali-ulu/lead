from __future__ import annotations

import json
import os
from typing import Any

from .db import connect


def permission_enabled(name: str) -> bool:
    return os.environ.get(name, "0").strip() == "1"


def require_permission(name: str, description: str) -> None:
    if not permission_enabled(name):
        raise ValueError(f"{description} is disabled. Set {name}=1 to enable it.")


def audit_agent_action(
    action: str,
    *,
    target: str = "",
    status: str = "ok",
    metadata: dict[str, Any] | None = None,
    actor: str = "mcp",
) -> None:
    with connect() as conn:
        conn.execute(
            "INSERT INTO agent_audit_log(actor,action,target,status,metadata) VALUES (?,?,?,?,?)",
            (
                actor,
                action,
                target or None,
                status,
                json.dumps(metadata or {}, ensure_ascii=False),
            ),
        )
        conn.commit()


def list_agent_audit(limit: int = 200) -> list[dict[str, Any]]:
    limit=max(1,min(1000,int(limit)))
    with connect() as conn:
        rows=conn.execute(
            "SELECT * FROM agent_audit_log ORDER BY created_at DESC,id DESC LIMIT ?",
            (limit,),
        ).fetchall()
    out=[]
    for row in rows:
        item=dict(row)
        try:
            item["metadata"]=json.loads(item.get("metadata") or "{}")
        except Exception:
            item["metadata"]={}
        out.append(item)
    return out
