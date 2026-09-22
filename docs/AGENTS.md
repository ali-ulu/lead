# LeadScout Agent Integration

LeadScout can be driven by agents in two ways:

1. **REST / OpenAPI** for any agent or automation platform that can call HTTP.
2. **MCP** for hosts that support Model Context Protocol.

## REST API

Start the normal app:

```bash
python3 LeadScout.py
```

Base URL:

```text
http://127.0.0.1:8787/api/v1
```

OpenAPI document:

```text
http://127.0.0.1:8787/api/v1/openapi.json
```

Optional local API token:

```bash
export LEADSCOUT_API_TOKEN="change-me"
python3 LeadScout.py
```

When set, requests to `/api/v1/*` require:

```http
Authorization: Bearer change-me
```

### Typical REST agent flow

Search Karachi restaurants:

```bash
curl -X POST http://127.0.0.1:8787/api/v1/search \
  -H "Content-Type: application/json" \
  -d '{"country":"Pakistan","city":"Karachi","category":"restaurant","radius_km":20}'
```

The response includes all stored lead IDs from that search. No application-level result cap is applied unless `max_results` is explicitly sent.

Read those leads:

```text
GET /api/v1/leads?ids=12,13,14
```

Audit one website:

```text
POST /api/v1/leads/12/audit
```

Draft outreach:

```text
GET /api/v1/leads/12/message?lang=ur
```

Move pipeline:

```http
POST /api/v1/leads/12/status
Content-Type: application/json

{"status":"contacted"}
```

Download Excel:

```text
GET /api/v1/export.xlsx?ids=12,13,14
```

## MCP

LeadScout uses the official MCP Python SDK v2 line.

Install the optional agent dependency:

```bash
python -m pip install "mcp>=2,<3"
```

### Local stdio

```bash
python mcp_server.py
```

The MCP host launches this process and communicates over stdio.

Example host configuration shape:

```json
{
  "mcpServers": {
    "leadscout": {
      "command": "python",
      "args": ["/absolute/path/to/lead/mcp_server.py"]
    }
  }
}
```

### Streamable HTTP

```bash
python mcp_server.py --transport streamable-http --host 127.0.0.1 --port 8790
```

Endpoint:

```text
http://127.0.0.1:8790/mcp
```

## MCP tools

- `capabilities`
- `search_businesses`
- `list_leads`
- `get_lead_detail`
- `audit_website`
- `draft_outreach_message`
- `update_pipeline_stage`
- `do_not_contact`
- `export_leads`
- `clear_local_data`

### Example autonomous flow

An agent can:

1. call `search_businesses(city="Karachi", country="Pakistan", category="beauty", radius_km=20)`
2. receive the full search's lead IDs
3. call `list_leads(ids=[...], min_score=50, page_size=100)`
4. inspect the strongest leads
5. call `audit_website(lead_id=...)` for leads that have websites
6. call `draft_outreach_message(lead_id=..., lang="ur")`
7. call `update_pipeline_stage(..., status="reviewed")`
8. export the working set with `export_leads(format="xlsx", ids=[...])`

The MCP list tool is paginated to keep individual model/tool responses manageable. Pagination does **not** cap discovery or the number of leads stored.

## Safety defaults

- The app binds to localhost by default.
- REST bearer auth is optional but recommended if exposing the REST server beyond a trusted local machine.
- MCP Streamable HTTP binds to localhost by default.
- Website audit blocks localhost/private-network targets and private redirects.
- Outreach is drafted only. LeadScout does not auto-send messages.
- Do-not-contact state is honored by normal lead lists.
