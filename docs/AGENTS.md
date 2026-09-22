# LeadScout 7.0 — Agent Integration

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
3. cross-source + optional general-web verification
4. contact/social enrichment
5. optional rating/review reputation enrichment
6. website audit
7. SEO / AEO / GEO / AI visibility + Opportunity Gap scoring
8. intelligence scoring
9. outreach draft
9. CRM activity
10. Excel export

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
- `enrich_reputation_data`
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


## Meta send eligibility

LeadScout exposes Facebook and Instagram OAuth separately.

- Instagram: official API messaging starts from an existing conversation; the Instagram user must have messaged the connected professional account before the API can send a reply.
- Facebook Messenger: the recipient must be an eligible Page-scoped user and the send must satisfy Meta's messaging-window/permission rules.
- A discovered social profile URL is research data, not a recipient id.
- `messaging_eligibility_for_lead` checks whether LeadScout has a stored eligible recipient id.
- `link_messaging_recipient` exists for associating a real conversation id with a lead.
- Actual MCP sending additionally requires `LEADSCOUT_MCP_ALLOW_SEND=1`.

CRM message lifecycle can record `sent`, `delivered`, `replied`, `rejected`, `bounced`, and `no_response`. Signed Meta webhook events update matched replies/delivery events automatically; `refresh_no_response_statuses` can mark old sent/delivered leads with no reply.


## Visibility-aware agent filtering

`list_leads` supports minimum SEO, AEO, GEO, AI Visibility and Opportunity Gap filters.

`run_sales_agent` supports `min_opportunity_gap`. Leads below that post-audit threshold are skipped before outreach.

Example:

> Find Karachi dentists, verify missing websites, audit existing sites, and only prepare outreach for leads with Opportunity Gap >= 70.
