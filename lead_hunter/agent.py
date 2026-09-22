from __future__ import annotations

import json
import os
import uuid
from pathlib import Path
from typing import Any

from .crm import add_activity
from .db import connect, get_lead
from .exporters import xlsx_bytes
from .oauth_meta import send_message
from .services import audit_lead, discover_businesses, draft_outreach, query_leads, verify_lead

ROOT = Path(__file__).resolve().parents[1]

def _job_write(job_id: str, *, status: str, request: dict[str,Any] | None=None,
               result: dict[str,Any] | None=None, error: str | None=None) -> None:
    with connect() as conn:
        if request is not None:
            conn.execute(
                "INSERT INTO agent_jobs(id,status,request_json,result_json,error) VALUES (?,?,?,?,?) "
                "ON CONFLICT(id) DO UPDATE SET status=excluded.status,request_json=excluded.request_json,"
                "result_json=excluded.result_json,error=excluded.error,updated_at=CURRENT_TIMESTAMP",
                (job_id,status,json.dumps(request,ensure_ascii=False),json.dumps(result,ensure_ascii=False) if result is not None else None,error),
            )
        else:
            conn.execute(
                "UPDATE agent_jobs SET status=?,result_json=?,error=?,updated_at=CURRENT_TIMESTAMP WHERE id=?",
                (status,json.dumps(result,ensure_ascii=False) if result is not None else None,error,job_id),
            )
        conn.commit()

def get_job(job_id: str) -> dict[str,Any] | None:
    with connect() as conn:
        row=conn.execute("SELECT * FROM agent_jobs WHERE id=?",(job_id,)).fetchone()
    if not row: return None
    item=dict(row)
    for key in ("request_json","result_json"):
        try: item[key[:-5] if key.endswith("_json") else key]=json.loads(item.get(key) or "{}")
        except Exception: item[key[:-5] if key.endswith("_json") else key]={}
    return item

def run_sales_job(
    *,
    city: str,
    category: str,
    country: str="",
    radius_km: int=20,
    top_n: int=50,
    min_score: int=40,
    lang: str="en",
    verify_missing: bool=True,
    audit_websites: bool=True,
    send: bool=False,
    send_provider: str="instagram",
    connection_id: int | None=None,
) -> dict[str,Any]:
    request=locals().copy()
    job_id=uuid.uuid4().hex
    _job_write(job_id,status="running",request=request)
    try:
        search=discover_businesses(
            city=city,country=country,category=category,radius_km=radius_km,max_results=None
        )
        rows=query_leads(search_id=search["search_id"],min_score=min_score)
        candidates=rows[:max(1,int(top_n))]

        processed=[]
        sent=0
        for lead in candidates:
            lead_id=int(lead["id"])
            if verify_missing and lead.get("website_status")=="missing":
                try: verify_lead(lead_id)
                except Exception: pass
            current=get_lead(lead_id) or lead
            if audit_websites and current.get("website"):
                try: audit_lead(lead_id,enrich=True)
                except Exception: pass
            current=get_lead(lead_id) or current
            draft=draft_outreach(lead_id,lang)
            action={"lead_id":lead_id,"name":current.get("name"),"score":current.get("lead_score"),
                    "engagement_status":current.get("engagement_status"),"message":draft["message"],"sent":False}
            if send:
                if os.environ.get("LEADSCOUT_AGENT_SEND","0")!="1":
                    action["send_error"]="LEADSCOUT_AGENT_SEND=1 is required for autonomous sending."
                else:
                    try:
                        result=send_message(
                            lead_id=lead_id,provider=send_provider,recipient_id=None,
                            text=draft["message"],connection_id=connection_id,
                        )
                        action["sent"]=True; action["send_result"]=result; sent+=1
                    except Exception as exc:
                        action["send_error"]=str(exc)
            processed.append(action)
            add_activity(lead_id,kind="agent",status="processed",metadata={"job_id":job_id})

        export_rows=query_leads(search_id=search["search_id"],min_score=min_score)
        out_dir=ROOT/"exports"; out_dir.mkdir(parents=True,exist_ok=True)
        export_path=out_dir/f"leadscout-agent-{job_id[:8]}.xlsx"
        export_path.write_bytes(xlsx_bytes(export_rows))

        result={
            "job_id":job_id,"search":search,"candidate_count":len(candidates),"processed":processed,
            "sent_count":sent,"export_path":str(export_path),"partial":search.get("partial",False),
            "warnings":search.get("warnings",[]),
        }
        _job_write(job_id,status="completed",result=result)
        return result
    except Exception as exc:
        _job_write(job_id,status="failed",error=str(exc))
        raise
