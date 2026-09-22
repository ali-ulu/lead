from __future__ import annotations

import argparse
import os
from pathlib import Path
from typing import Any

from mcp.server.mcpserver import MCPServer

from lead_hunter.agent import get_job, run_sales_job
from lead_hunter.crm import add_note, list_activities, mark_stale_no_response, set_engagement, set_follow_up
from lead_hunter.db import clear_all, get_lead, initialize
from lead_hunter.exporters import csv_bytes, xlsx_bytes
from lead_hunter.oauth_meta import (
    link_recipient, list_connections, messaging_eligibility, send_message,
)
from lead_hunter.providers.osm import CATEGORY_FILTERS
from lead_hunter.security import audit_agent_action, list_agent_audit, require_permission
from lead_hunter.services import (
    audit_lead, discover_businesses, draft_outreach, enrich_lead,
    mark_do_not_contact, query_leads, set_pipeline_stage, verify_lead,
)

mcp=MCPServer("LeadScout")

@mcp.tool()
def capabilities() -> dict[str,Any]:
    """Describe LeadScout 5 capabilities."""
    initialize()
    return {
        "name":"LeadScout","version":"5.1.0","global_search":True,
        "providers":["osm","overture"],"result_cap":None,
        "radius_km":{"min":1,"max":100},"categories":sorted(CATEGORY_FILTERS),
        "languages":["en","tr","ur","sd","de"],
        "exports":["csv","xlsx"],
        "crm":True,"website_verification":True,"contact_enrichment":True,
        "lighthouse_when_installed":True,"meta_oauth":True,
        "autonomous_agent":True,
        "mcp_permissions":{
            "write_default":True,
            "send_default":False,
            "clear_default":False,
        },
        "pipeline_stages":["new","reviewed","contacted","replied","proposal","won","lost"],
        "engagement_statuses":["not_contacted","drafted","sent","delivered","replied","rejected","bounced","no_response"],
    }

@mcp.tool()
def search_businesses(city:str,category:str,country:str="",radius_km:int=20,max_results:int|None=None,
                      providers:list[str]|None=None) -> dict[str,Any]:
    """Search OSM + Overture and merge/dedupe leads."""
    initialize()
    return discover_businesses(city=city,country=country,category=category,radius_km=radius_km,
                               max_results=max_results,providers=providers)

@mcp.tool()
def list_leads(search_id:str="",ids:list[int]|None=None,country:str="",city:str="",category:str="",
               website_status:str="",pipeline_status:str="",engagement_status:str="",min_score:int=0,
               has_social:bool=False,contactable:bool=False,offset:int=0,page_size:int=100) -> dict[str,Any]:
    """List/filter leads with pagination."""
    initialize()
    rows=query_leads(search_id=search_id,ids=ids,country=country,city=city,category=category,
                     website_status=website_status,pipeline_status=pipeline_status,
                     engagement_status=engagement_status,min_score=min_score,
                     has_social=has_social,contactable=contactable)
    offset=max(0,int(offset)); page_size=max(1,min(500,int(page_size))); page=rows[offset:offset+page_size]
    return {"total":len(rows),"offset":offset,"page_size":page_size,
            "next_offset":offset+len(page) if offset+len(page)<len(rows) else None,"items":page}

@mcp.tool()
def get_lead_detail(lead_id:int) -> dict[str,Any]:
    initialize(); lead=get_lead(int(lead_id))
    if not lead: raise ValueError("Lead not found.")
    return lead

@mcp.tool()
def verify_business(lead_id:int) -> dict[str,Any]:
    """Cross-check a lead against Overture, especially before treating 'no site found' as verified."""
    initialize(); return verify_lead(int(lead_id))

@mcp.tool()
def enrich_contacts(lead_id:int) -> dict[str,Any]:
    """Crawl a public business website for email, phone, booking and social links."""
    initialize(); return enrich_lead(int(lead_id))

@mcp.tool()
def audit_website(lead_id:int) -> dict[str,Any]:
    """Run deep website audit; uses Lighthouse when installed and heuristic checks otherwise."""
    initialize(); return audit_lead(int(lead_id),enrich=True)

@mcp.tool()
def draft_outreach_message(lead_id:int,lang:str="en") -> dict[str,Any]:
    initialize(); return draft_outreach(int(lead_id),lang)

@mcp.tool()
def update_pipeline_stage(lead_id:int,status:str) -> dict[str,Any]:
    initialize(); require_permission("LEADSCOUT_MCP_ALLOW_WRITE","MCP CRM writes",default=True)
    result=set_pipeline_stage(int(lead_id),status)
    audit_agent_action("update_pipeline_stage",target=str(lead_id),metadata={"status":status})
    return result

@mcp.tool()
def update_engagement_status(lead_id:int,status:str,note:str="") -> dict[str,Any]:
    """Record whether outreach was sent, replied, rejected, bounced or had no response."""
    initialize(); require_permission("LEADSCOUT_MCP_ALLOW_WRITE","MCP CRM writes",default=True)
    result={"lead":set_engagement(int(lead_id),status,note)}
    audit_agent_action("update_engagement_status",target=str(lead_id),metadata={"status":status})
    return result

@mcp.tool()
def schedule_follow_up(lead_id:int,when:str|None,note:str="") -> dict[str,Any]:
    initialize(); require_permission("LEADSCOUT_MCP_ALLOW_WRITE","MCP CRM writes",default=True)
    result={"lead":set_follow_up(int(lead_id),when,note)}
    audit_agent_action("schedule_follow_up",target=str(lead_id),metadata={"when":when})
    return result

@mcp.tool()
def add_lead_note(lead_id:int,note:str) -> dict[str,Any]:
    initialize(); require_permission("LEADSCOUT_MCP_ALLOW_WRITE","MCP CRM writes",default=True)
    result={"activity":add_note(int(lead_id),note)}
    audit_agent_action("add_lead_note",target=str(lead_id))
    return result

@mcp.tool()
def lead_activity_timeline(lead_id:int) -> dict[str,Any]:
    initialize(); return {"items":list_activities(int(lead_id))}

@mcp.tool()
def refresh_no_response_statuses(days:int=7) -> dict[str,Any]:
    """Mark sent/delivered leads with no reply after N days as no_response."""
    initialize(); require_permission("LEADSCOUT_MCP_ALLOW_WRITE","MCP CRM writes",default=True)
    result=mark_stale_no_response(days)
    audit_agent_action("refresh_no_response_statuses",metadata={"days":days,"updated_count":result["updated_count"]})
    return result

@mcp.tool()
def meta_connections() -> dict[str,Any]:
    """List connected Facebook Page / Instagram Business accounts."""
    initialize(); return {"items":list_connections()}

@mcp.tool()
def messaging_eligibility_for_lead(lead_id:int,provider:str="instagram") -> dict[str,Any]:
    """Check whether LeadScout has an eligible Meta recipient id for this lead."""
    initialize(); return messaging_eligibility(int(lead_id),provider)

@mcp.tool()
def link_messaging_recipient(lead_id:int,provider:str,recipient_id:str) -> dict[str,Any]:
    """Associate a Meta Page/Instagram scoped recipient id with a lead after a real conversation exists."""
    initialize(); require_permission("LEADSCOUT_MCP_ALLOW_WRITE","MCP CRM writes",default=True)
    result=link_recipient(int(lead_id),provider,recipient_id)
    audit_agent_action("link_messaging_recipient",target=str(lead_id),metadata={"provider":provider})
    return result

@mcp.tool()
def send_social_message(lead_id:int,provider:str,text:str,recipient_id:str="",connection_id:int|None=None) -> dict[str,Any]:
    """Send through a connected Meta account when an eligible recipient/conversation id exists."""
    initialize()
    require_permission("LEADSCOUT_MCP_ALLOW_SEND","MCP message sending",default=False)
    result=send_message(lead_id=int(lead_id),provider=provider,recipient_id=recipient_id or None,
                        text=text,connection_id=connection_id)
    audit_agent_action("send_social_message",target=str(lead_id),metadata={"provider":provider,"connection_id":connection_id})
    return result

@mcp.tool()
def run_sales_agent(city:str,category:str,country:str="",radius_km:int=20,top_n:int=50,
                    min_score:int=40,lang:str="en",verify_missing:bool=True,
                    audit_websites:bool=True,send:bool=False,send_provider:str="instagram",
                    connection_id:int|None=None) -> dict[str,Any]:
    """Run search -> verify -> audit/enrich -> draft -> Excel; optional sending is separately gated."""
    initialize()
    require_permission("LEADSCOUT_MCP_ALLOW_WRITE","MCP agent writes",default=True)
    if send:
        require_permission("LEADSCOUT_MCP_ALLOW_SEND","MCP message sending",default=False)
    result=run_sales_job(city=city,category=category,country=country,radius_km=radius_km,
                         top_n=top_n,min_score=min_score,lang=lang,verify_missing=verify_missing,
                         audit_websites=audit_websites,send=send,send_provider=send_provider,
                         connection_id=connection_id)
    audit_agent_action("run_sales_agent",target=result.get("job_id",""),metadata={"city":city,"country":country,"category":category,"send":send})
    return result

@mcp.tool()
def get_agent_job(job_id:str) -> dict[str,Any]:
    initialize(); item=get_job(job_id)
    if not item: raise ValueError("Job not found.")
    return item

@mcp.tool()
def do_not_contact(lead_id:int) -> dict[str,Any]:
    initialize(); require_permission("LEADSCOUT_MCP_ALLOW_WRITE","MCP CRM writes",default=True)
    result=mark_do_not_contact(int(lead_id))
    audit_agent_action("do_not_contact",target=str(lead_id))
    return result

@mcp.tool()
def export_leads(format:str="xlsx",search_id:str="",ids:list[int]|None=None,country:str="",city:str="",
                 category:str="",website_status:str="",pipeline_status:str="",engagement_status:str="",
                 min_score:int=0,has_social:bool=False,contactable:bool=False) -> dict[str,Any]:
    initialize()
    rows=query_leads(search_id=search_id,ids=ids,country=country,city=city,category=category,
                     website_status=website_status,pipeline_status=pipeline_status,
                     engagement_status=engagement_status,min_score=min_score,
                     has_social=has_social,contactable=contactable)
    fmt=format.lower().strip()
    if fmt not in {"csv","xlsx"}: raise ValueError("format must be csv or xlsx")
    out=Path(__file__).resolve().parent/"exports"; out.mkdir(parents=True,exist_ok=True)
    path=out/f"leadscout-leads.{fmt}"; path.write_bytes(xlsx_bytes(rows) if fmt=="xlsx" else csv_bytes(rows))
    return {"format":fmt,"count":len(rows),"path":str(path)}

@mcp.tool()
def clear_local_data(confirm:bool=False) -> dict[str,Any]:
    """Clear cached leads and CRM history. Explicit confirm=true and permission are required."""
    initialize()
    if not confirm: raise ValueError("confirm=true is required.")
    require_permission("LEADSCOUT_MCP_ALLOW_CLEAR","MCP destructive clear",default=False)
    audit_agent_action("clear_local_data",status="requested")
    clear_all(); return {"ok":True}

@mcp.tool()
def agent_audit_log(limit:int=200) -> dict[str,Any]:
    """Read recent MCP write/send/clear audit events."""
    initialize(); return {"items":list_agent_audit(limit)}

def main() -> None:
    parser=argparse.ArgumentParser(description="LeadScout MCP server")
    parser.add_argument("--transport",choices=["stdio","streamable-http"],default="stdio")
    parser.add_argument("--host",default="127.0.0.1"); parser.add_argument("--port",type=int,default=8790)
    args=parser.parse_args(); initialize()
    if args.transport=="streamable-http":
        mcp.run(transport="streamable-http",host=args.host,port=args.port,stateless_http=True,json_response=True)
    else: mcp.run(transport="stdio")

if __name__=="__main__": main()
