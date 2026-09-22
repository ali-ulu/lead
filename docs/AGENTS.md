# LeadScout 5.1 — Agent Integration

LeadScout has two agent surfaces:

1. REST/OpenAPI
2. MCP via stdio or Streamable HTTP

## REST

Base:

```text
http://127.0.0.1:8787/api/v1
```

OpenAPI:

```text
http://127.0.0.1:8787/api/v1/openapi.json
```

Set `LEADSCOUT_API_TOKEN` to require Bearer auth on protected v1 endpoints.

## MCP

Install:

```bash
python -m pip install -e ".[agents]"
```

stdio:

```bash
python mcp_server.py
```

Streamable HTTP:

```bash
python mcp_server.py --transport streamable-http --host 127.0.0.1 --port 8790
```

Endpoint: `http://127.0.0.1:8790/mcp`

## Agent workflow

The high-level autonomous workflow is:

1. multi-source discovery
2. dedupe
3. cross-source verification
4. contact/social enrichment
5. website audit
6. intelligence scoring
7. outreach draft
8. CRM activity
9. Excel export

Example intent:

> Find Karachi restaurants, verify the strongest opportunities, enrich their contact channels, audit existing websites, prepare Urdu drafts for the best 30 leads, and export Excel.

## Tool groups

Discovery/read:
- `capabilities`
- `search_businesses`
- `list_leads`
- `get_lead_detail`
- `verify_business`
- `enrich_contacts`
- `audit_website`
- `lead_activity_timeline`
- `meta_connections`
- messaging eligibility inspection
- `get_agent_job`

Workflow/write:
- `draft_outreach_message`
- `update_pipeline_stage`
- `update_engagement_status`
- `schedule_follow_up`
- `add_lead_note`
- `do_not_contact`
- `export_leads`
- `run_sales_agent`

## Security

Risky agent actions are separately permission-gated. Sending, destructive clearing and other writes can be disabled independently. Agent actions are recorded in the local audit log.

Website audit/enrichment blocks private/local network targets.

## Meta messaging

Facebook and Instagram OAuth are separate.

Social handles found during research are discovery data, not message recipient IDs. Sending requires:

- a connected Page/professional account,
- applicable platform permissions,
- an API-eligible recipient/conversation ID,
- LeadScout send permission enabled.

Incoming reply/delivery events can update the CRM timeline when stored recipient or external message IDs match.

Autonomous sending is not implicitly enabled.
