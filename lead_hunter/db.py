from __future__ import annotations
import json
import sqlite3
from pathlib import Path
from typing import Any
from .scoring import calculate_score

ROOT = Path(__file__).resolve().parents[1]
DB_PATH = ROOT / "data" / "lead_hunter.db"
SCHEMA_PATH = ROOT / "sql" / "schema.sql"

def connect() -> sqlite3.Connection:
    DB_PATH.parent.mkdir(parents=True, exist_ok=True)
    conn = sqlite3.connect(DB_PATH)
    conn.row_factory = sqlite3.Row
    return conn

def initialize() -> None:
    with connect() as conn:
        conn.executescript(SCHEMA_PATH.read_text(encoding="utf-8"))

def _decode(row: sqlite3.Row | None) -> dict[str, Any] | None:
    if not row:
        return None
    item = dict(row)
    item["score_reasons"] = json.loads(item.get("score_reasons") or "[]")
    for key in ("mobile_ok", "has_cta", "has_booking", "has_https"):
        if item.get(key) is not None:
            item[key] = bool(item[key])
    return item

def upsert_leads(rows: list[dict[str, Any]]) -> list[int]:
    ids: list[int] = []
    columns = ["source","source_id","name","country","city","category","latitude","longitude","website","phone","email","social_url","website_status","performance_score","seo_score","accessibility_score","mobile_ok","has_cta","has_booking","has_https","data_confidence","lead_score","score_reasons","pipeline_status"]
    with connect() as conn:
        for lead in rows:
            score, reasons = calculate_score(lead)
            lead["lead_score"] = score
            lead["score_reasons"] = json.dumps(reasons, ensure_ascii=False)
            lead.setdefault("pipeline_status", "new")
            values = [lead.get(c) for c in columns]
            updates = ",".join(f"{c}=excluded.{c}" for c in columns if c not in {"source", "source_id", "pipeline_status"}) + ",updated_at=CURRENT_TIMESTAMP"
            conn.execute(f"INSERT INTO leads ({','.join(columns)}) VALUES ({','.join('?' for _ in columns)}) ON CONFLICT(source,source_id) DO UPDATE SET {updates}",values)
            row = conn.execute("SELECT id FROM leads WHERE source=? AND source_id=?", (lead["source"], lead["source_id"])).fetchone()
            if row:
                ids.append(int(row["id"]))
        conn.commit()
    return ids

def list_leads(filters: dict[str, str]) -> list[dict[str, Any]]:
    clauses = ["do_not_contact = 0"]
    args: list[Any] = []
    for field in ("country", "city", "category", "pipeline_status"):
        value = filters.get(field, "").strip()
        if value:
            clauses.append(f"LOWER({field}) LIKE LOWER(?)")
            args.append(f"%{value}%")
    status = filters.get("website_status", "").strip()
    if status:
        clauses.append("website_status = ?")
        args.append(status)
    min_score = filters.get("min_score", "").strip()
    if min_score:
        try:
            clauses.append("lead_score >= ?")
            args.append(int(min_score))
        except ValueError:
            pass
    sql = f"SELECT * FROM leads WHERE {' AND '.join(clauses)} ORDER BY lead_score DESC, updated_at DESC, name ASC"
    with connect() as conn:
        return [_decode(row) for row in conn.execute(sql, args).fetchall() if row]

def get_lead(lead_id: int) -> dict[str, Any] | None:
    with connect() as conn:
        return _decode(conn.execute("SELECT * FROM leads WHERE id=?", (lead_id,)).fetchone())

def update_lead(lead_id: int, fields: dict[str, Any]) -> dict[str, Any] | None:
    allowed = {"website","phone","email","social_url","website_status","performance_score","seo_score","accessibility_score","mobile_ok","has_cta","has_booking","has_https","data_confidence","pipeline_status","do_not_contact"}
    clean = {k: v for k, v in fields.items() if k in allowed}
    if not clean:
        return get_lead(lead_id)
    with connect() as conn:
        setters = ",".join(f"{k}=?" for k in clean)
        conn.execute(f"UPDATE leads SET {setters}, updated_at=CURRENT_TIMESTAMP WHERE id=?", [*clean.values(), lead_id])
        row = conn.execute("SELECT * FROM leads WHERE id=?", (lead_id,)).fetchone()
        if not row:
            return None
        lead = _decode(row)
        assert lead is not None
        score, reasons = calculate_score(lead)
        conn.execute("UPDATE leads SET lead_score=?, score_reasons=?, updated_at=CURRENT_TIMESTAMP WHERE id=?", (score, json.dumps(reasons, ensure_ascii=False), lead_id))
        conn.commit()
    return get_lead(lead_id)

def clear_all() -> None:
    with connect() as conn:
        conn.execute("DELETE FROM leads")
        conn.commit()
