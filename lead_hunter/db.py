from __future__ import annotations
import json
import sqlite3
import uuid
from pathlib import Path
from typing import Any
from .scoring import calculate_score

ROOT = Path(__file__).resolve().parents[1]
DB_PATH = ROOT / "data" / "lead_hunter.db"
SCHEMA_PATH = ROOT / "sql" / "schema.sql"

LEAD_MIGRATIONS = {
    "source_refs": "TEXT NOT NULL DEFAULT '{}'",
    "messaging_ids": "TEXT NOT NULL DEFAULT '{}'",
    "verification_status": "TEXT NOT NULL DEFAULT 'unverified'",
    "verification_notes": "TEXT NOT NULL DEFAULT '[]'",
    "audit_engine": "TEXT",
    "rating": "REAL",
    "review_count": "INTEGER",
    "contactability_score": "INTEGER NOT NULL DEFAULT 0",
    "commercial_score": "INTEGER NOT NULL DEFAULT 0",
    "intelligence_reasons": "TEXT NOT NULL DEFAULT '[]'",
    "engagement_status": "TEXT NOT NULL DEFAULT 'not_contacted'",
    "last_contacted_at": "TEXT",
    "last_reply_at": "TEXT",
    "follow_up_at": "TEXT",
    "notes": "TEXT",
}

def connect() -> sqlite3.Connection:
    DB_PATH.parent.mkdir(parents=True, exist_ok=True)
    conn = sqlite3.connect(DB_PATH)
    conn.row_factory = sqlite3.Row
    return conn

def initialize() -> None:
    with connect() as conn:
        conn.executescript(SCHEMA_PATH.read_text(encoding="utf-8"))
        lead_cols = {row["name"] for row in conn.execute("PRAGMA table_info(leads)").fetchall()}
        for name, ddl in LEAD_MIGRATIONS.items():
            if name not in lead_cols:
                conn.execute(f"ALTER TABLE leads ADD COLUMN {name} {ddl}")
        search_cols = {row["name"] for row in conn.execute("PRAGMA table_info(search_runs)").fetchall()}
        for name, ddl in {
            "provider_summary": "TEXT NOT NULL DEFAULT '{}'",
            "partial": "INTEGER NOT NULL DEFAULT 0",
            "warnings": "TEXT NOT NULL DEFAULT '[]'",
        }.items():
            if name not in search_cols:
                conn.execute(f"ALTER TABLE search_runs ADD COLUMN {name} {ddl}")
        conn.commit()

def _loads(value: Any, fallback):
    if isinstance(value, type(fallback)):
        return value
    try:
        return json.loads(value or json.dumps(fallback))
    except Exception:
        return fallback

def _decode(row: sqlite3.Row | None) -> dict[str, Any] | None:
    if not row:
        return None
    item = dict(row)
    for key, fallback in {
        "score_reasons": [], "social_links": {}, "messaging_ids": {}, "source_refs": {},
        "verification_notes": [], "intelligence_reasons": [],
    }.items():
        item[key] = _loads(item.get(key), fallback)
    for key in ("mobile_ok","has_cta","has_booking","has_https","do_not_contact"):
        if item.get(key) is not None:
            item[key] = bool(item[key])
    return item

def _jsonify(clean: dict[str, Any]) -> dict[str, Any]:
    for key in ("social_links","messaging_ids","source_refs"):
        if isinstance(clean.get(key), dict):
            clean[key] = json.dumps(clean[key], ensure_ascii=False, sort_keys=True)
    for key in ("score_reasons","verification_notes","intelligence_reasons"):
        if isinstance(clean.get(key), list):
            clean[key] = json.dumps(clean[key], ensure_ascii=False)
    return clean

def upsert_leads(rows: list[dict[str, Any]]) -> list[int]:
    ids: list[int] = []
    columns = [
        "source","source_id","source_refs","name","country","city","category","latitude","longitude",
        "website","phone","email","social_url","social_links","messaging_ids","website_status",
        "verification_status","verification_notes","performance_score","seo_score","accessibility_score",
        "mobile_ok","has_cta","has_booking","has_https","audit_engine","rating","review_count",
        "data_confidence","contactability_score","commercial_score","intelligence_reasons",
        "lead_score","score_reasons","pipeline_status","engagement_status","follow_up_at","notes",
    ]
    with connect() as conn:
        for raw in rows:
            lead = dict(raw)
            score, reasons = calculate_score(raw)
            lead["lead_score"] = score
            lead["score_reasons"] = reasons
            lead.setdefault("pipeline_status", "new")
            lead.setdefault("engagement_status", "not_contacted")
            lead.setdefault("website_status", "unknown")
            lead.setdefault("verification_status", "unverified")
            lead.setdefault("data_confidence", "unknown")
            lead.setdefault("contactability_score", 0)
            lead.setdefault("commercial_score", 0)
            lead.setdefault("source_refs", {lead.get("source","unknown"): lead.get("source_id")})
            lead.setdefault("social_links", {})
            lead.setdefault("messaging_ids", {})
            lead.setdefault("verification_notes", [])
            lead.setdefault("intelligence_reasons", [])
            lead = _jsonify(lead)
            values = [lead.get(c) for c in columns]
            protected = {"source","source_id","pipeline_status","engagement_status","follow_up_at","notes"}
            updates = ",".join(f"{c}=excluded.{c}" for c in columns if c not in protected) + ",updated_at=CURRENT_TIMESTAMP"
            conn.execute(
                f"INSERT INTO leads ({','.join(columns)}) VALUES ({','.join('?' for _ in columns)}) "
                f"ON CONFLICT(source,source_id) DO UPDATE SET {updates}", values,
            )
            row = conn.execute("SELECT id FROM leads WHERE source=? AND source_id=?", (lead["source"],lead["source_id"])).fetchone()
            if row: ids.append(int(row["id"]))
        conn.commit()
    return ids

def record_search_run(*, country: str, city: str, category: str, radius_km: int, lead_ids: list[int],
                      provider_summary: dict | None = None, partial: bool = False, warnings: list[str] | None = None) -> str:
    search_id = uuid.uuid4().hex
    with connect() as conn:
        conn.execute(
            "INSERT INTO search_runs(id,country,city,category,radius_km,result_count,provider_summary,partial,warnings) VALUES (?,?,?,?,?,?,?,?,?)",
            (search_id,country,city,category,int(radius_km),len(lead_ids),
             json.dumps(provider_summary or {},ensure_ascii=False),int(partial),json.dumps(warnings or [],ensure_ascii=False)),
        )
        conn.executemany("INSERT OR IGNORE INTO search_run_leads(search_id,lead_id) VALUES (?,?)",
                         [(search_id,int(i)) for i in lead_ids])
        conn.commit()
    return search_id

def list_leads(filters: dict[str, str]) -> list[dict[str, Any]]:
    clauses = ["do_not_contact = 0"]; args: list[Any] = []
    for field in ("country","city","category","pipeline_status","engagement_status"):
        value = filters.get(field,"").strip()
        if value: clauses.append(f"LOWER({field}) LIKE LOWER(?)"); args.append(f"%{value}%")
    search_id = filters.get("search_id","").strip()
    if search_id: clauses.append("id IN (SELECT lead_id FROM search_run_leads WHERE search_id=?)"); args.append(search_id)
    ids_raw = filters.get("ids","").strip()
    if ids_raw:
        ids=[]
        for p in ids_raw.split(","):
            try: ids.append(int(p))
            except ValueError: pass
        if ids:
            clauses.append(f"id IN ({','.join('?' for _ in ids)})"); args.extend(ids)
        else: clauses.append("1=0")
    status=filters.get("website_status","").strip()
    if status: clauses.append("website_status=?"); args.append(status)
    if filters.get("has_social")=="1": clauses.append("social_links != '{}' AND social_links != ''")
    if filters.get("contactable")=="1": clauses.append("(phone IS NOT NULL OR email IS NOT NULL OR social_links != '{}')")
    min_score=filters.get("min_score","").strip()
    if min_score:
        try: clauses.append("lead_score>=?"); args.append(int(min_score))
        except ValueError: pass
    sql=f"SELECT * FROM leads WHERE {' AND '.join(clauses)} ORDER BY lead_score DESC, commercial_score DESC, updated_at DESC, name ASC"
    with connect() as conn:
        return [_decode(r) for r in conn.execute(sql,args).fetchall() if r]

def get_lead(lead_id: int) -> dict[str, Any] | None:
    with connect() as conn:
        return _decode(conn.execute("SELECT * FROM leads WHERE id=?",(lead_id,)).fetchone())

def update_lead(lead_id: int, fields: dict[str, Any]) -> dict[str, Any] | None:
    allowed = {
        "website","phone","email","social_url","social_links","messaging_ids","source_refs","website_status",
        "verification_status","verification_notes","performance_score","seo_score","accessibility_score",
        "mobile_ok","has_cta","has_booking","has_https","audit_engine","rating","review_count",
        "data_confidence","contactability_score","commercial_score","intelligence_reasons",
        "pipeline_status","engagement_status","last_contacted_at","last_reply_at","follow_up_at","notes","do_not_contact"
    }
    clean=_jsonify({k:v for k,v in fields.items() if k in allowed})
    if not clean: return get_lead(lead_id)
    with connect() as conn:
        conn.execute(f"UPDATE leads SET {','.join(f'{k}=?' for k in clean)},updated_at=CURRENT_TIMESTAMP WHERE id=?",
                     [*clean.values(),lead_id])
        row=conn.execute("SELECT * FROM leads WHERE id=?",(lead_id,)).fetchone()
        if not row: return None
        lead=_decode(row); score,reasons=calculate_score(lead or {})
        conn.execute("UPDATE leads SET lead_score=?,score_reasons=?,updated_at=CURRENT_TIMESTAMP WHERE id=?",
                     (score,json.dumps(reasons,ensure_ascii=False),lead_id)); conn.commit()
    return get_lead(lead_id)

def get_search_run(search_id: str) -> dict[str, Any] | None:
    with connect() as conn:
        row=conn.execute("SELECT * FROM search_runs WHERE id=?",(search_id,)).fetchone()
    if not row: return None
    item=dict(row); item["provider_summary"]=_loads(item.get("provider_summary"),{}); item["warnings"]=_loads(item.get("warnings"),[])
    item["partial"]=bool(item.get("partial")); return item

def clear_all() -> None:
    with connect() as conn:
        conn.execute("DELETE FROM lead_activities"); conn.execute("DELETE FROM search_run_leads")
        conn.execute("DELETE FROM search_runs"); conn.execute("DELETE FROM agent_jobs"); conn.execute("DELETE FROM leads"); conn.commit()
