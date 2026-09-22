# LeadScout

**LeadScout** is a local-first business opportunity scout. Choose a country, city/area, industry and radius; LeadScout discovers local businesses, highlights missing or weak web presence, collects contact/social channels, audits public websites and prepares human-reviewed outreach drafts.

## LeadScout 4.0

- Global live discovery with OpenStreetMap / Overpass.
- **No application-level 250-result cap.** Search returns the provider's complete result set unless an explicit `max_results` is requested through the agent API.
- English, Turkish, Urdu and Sindhi interface.
- Automatic RTL layout for Urdu and Sindhi.
- Social-channel capture from OSM tags and public business websites:
  - Instagram
  - Facebook
  - LinkedIn
  - X / Twitter
  - YouTube
  - TikTok
  - Telegram
  - WhatsApp
- Explainable 0–100 opportunity score.
- Public website audit.
- Local pipeline and do-not-contact state.
- Search history with one-click clearing.
- **Excel XLSX + CSV export.**
- **Versioned REST / OpenAPI agent API.**
- **MCP server for autonomous agents**, with stdio and Streamable HTTP transports.
- SQLite local persistence.
- No API key, n8n, Apify or paid CRM required for the core app.

## Start the app

### Windows
Double-click `START_WINDOWS.bat`.

### macOS
Double-click `START_MAC.command`.

### Linux

```bash
./start.sh
```

Or:

```bash
python3 LeadScout.py
```

The UI opens at:

```text
http://127.0.0.1:8787
```

Core requirement: Python 3.11+.

## Agent API

REST base:

```text
http://127.0.0.1:8787/api/v1
```

OpenAPI:

```text
http://127.0.0.1:8787/api/v1/openapi.json
```

Optional REST protection:

```bash
export LEADSCOUT_API_TOKEN="change-me"
python3 LeadScout.py
```

Full REST/MCP guide: [docs/AGENTS.md](docs/AGENTS.md).

## MCP

Install the official MCP Python SDK v2 line:

```bash
python -m pip install "mcp>=2,<3"
```

Local stdio:

```bash
python mcp_server.py
```

Streamable HTTP:

```bash
python mcp_server.py --transport streamable-http --host 127.0.0.1 --port 8790
```

MCP endpoint:

```text
http://127.0.0.1:8790/mcp
```

Tools exposed:

`capabilities`, `search_businesses`, `list_leads`, `get_lead_detail`, `audit_website`, `draft_outreach_message`, `update_pipeline_stage`, `do_not_contact`, `export_leads`, `clear_local_data`.

## Typical human workflow

1. Choose country, city/area, industry and radius.
2. Find leads.
3. Filter by site state, social availability, score or pipeline stage.
4. Open a lead.
5. Review phone, email and social channels.
6. Audit the website when one exists.
7. Generate an EN/TR/UR/SD outreach draft.
8. Move the lead through the pipeline.
9. Export the current result set as Excel or CSV.

## Data accuracy

Discovery uses OpenStreetMap through public Overpass endpoints and Nominatim for geocoding. Coverage varies by market.

**“No site found” means the reviewed sources did not provide an independent website. It is not proof that no website exists anywhere online.**

Missing email, phone or social data is also unknown, not proof of absence.

The website audit uses lightweight public-page heuristics. Its performance signal is a response-time proxy, not Google Lighthouse.

## Result limits

LeadScout itself no longer caps discovery at 250 records. Overpass queries use `out tags center qt;` without an output-count parameter. OpenStreetMap's Overpass infrastructure is still shared public infrastructure and can impose operational constraints such as timeouts or memory limits.

For very large production workloads, use a compliant dedicated data provider or self-hosted data pipeline rather than overloading public community endpoints.

## Export

The UI can download:

- `leadscout-leads.xlsx`
- `leadscout-leads.csv`

Excel output is a native XLSX workbook with frozen headers, filters, column sizing and dedicated social-channel columns.

## Privacy & safety

- Lead data stays in local SQLite.
- No analytics/telemetry.
- Website audit blocks localhost/private-network targets and private redirects.
- Messages are drafted only, never auto-sent.
- Do-not-contact state is hidden from normal lists.
- REST API binds locally by default and can require a bearer token.

## Verification

The repository contains three verification layers:

- normal CI: unit tests, Python compilation and frontend JavaScript syntax
- live provider smoke: Pakistan + other countries against real public data
- agent smoke: installs the official MCP SDK and calls LeadScout tools in-process
