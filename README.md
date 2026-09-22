# LeadScout

**LeadScout 5.1** is a local-first sales opportunity system for finding local businesses, verifying web presence, enriching contact data, auditing websites, preparing outreach, and tracking what happened after contact.

## LeadScout 5.1

- **Multi-source discovery:** OpenStreetMap / Overpass + Overture Places.
- Results are merged and deduplicated by domain, phone, and name + proximity.
- **No application-level 250-result cap.**
- Search results expose provider counts, partial-result state and warnings.
- **Website verification:** "No site found" can be cross-checked against Overture before it becomes a strong sales signal.
- **Contact enrichment:** public business pages are scanned for email, phone, booking links and social accounts.
- Instagram, Facebook, LinkedIn, X/Twitter, YouTube, TikTok, Telegram and WhatsApp discovery.
- **Deep website audit:** Lighthouse performance/accessibility/best-practices/SEO when available, with safe heuristic fallback.
- Opportunity, contactability and commercial-intent scoring.
- CRM with separate pipeline and engagement state.
- Activity timeline, notes, last contact, last reply and follow-up date.
- Native XLSX + CSV export including CRM and intelligence fields.
- English / Turkish / Urdu / Sindhi UI, with RTL for Urdu/Sindhi.
- EN / TR / UR / SD / DE outreach drafts.
- REST / OpenAPI agent API.
- MCP stdio + Streamable HTTP.
- Autonomous agent workflow: search → verify → audit/enrich → draft → Excel.
- Scoped agent write/send/clear permissions and persistent agent audit log.
- Facebook Page + Instagram professional-account OAuth flows.
- OAuth tokens encrypted locally.
- Meta reply/delivery lifecycle support where webhook events can be matched.

## Start

Windows: double-click `START_WINDOWS.bat`  
macOS: double-click `START_MAC.command`  
Linux:

```bash
./start.sh
```

The launcher creates a local `.venv`, installs Python dependencies and, when npm is available, installs local Lighthouse tooling.

UI:

```text
http://127.0.0.1:8787
```

## Data sources

LeadScout uses OpenStreetMap and Overture Places by default. If a provider is unavailable, the search is marked partial and warnings are returned instead of pretending the result is complete.

A missing website, email or social field means **not found in the reviewed sources**, not proof of absence.

## CRM

Sales pipeline:

`new → reviewed → contacted → replied → proposal → won/lost`

Engagement outcome:

`not_contacted / drafted / sent / delivered / replied / rejected / bounced / no_response`

This keeps “where is the deal?” separate from “what happened to the message?”.

## Meta connection

Facebook and Instagram OAuth are separate. Configure account-specific credentials through environment variables in `.env.example`.

Important: a public Instagram/Facebook username or profile URL is not automatically a sendable recipient ID. Instagram's official messaging API can reply only after the Instagram user has messaged the connected professional account. Messenger requires an eligible Page-scoped recipient and the applicable messaging window/permission. LeadScout therefore never treats a discovered profile as permission to cold-DM it and does not invent or bypass recipient IDs.

## Agent API

REST:

```text
http://127.0.0.1:8787/api/v1
```

OpenAPI:

```text
http://127.0.0.1:8787/api/v1/openapi.json
```

MCP stdio:

```bash
python -m pip install -e ".[agents]"
python mcp_server.py
```

MCP Streamable HTTP:

```bash
python mcp_server.py --transport streamable-http --host 127.0.0.1 --port 8790
```

Endpoint: `http://127.0.0.1:8790/mcp`

Full guide: [docs/AGENTS.md](docs/AGENTS.md)

## Verification

The repository has independent checks for:

- unit tests
- Python compilation
- frontend JavaScript syntax
- REST + Excel smoke
- MCP stdio + Streamable HTTP smoke
- live OSM + Overture combined discovery
- Overture-only live checks in Karachi and Berlin

Public-data coverage still varies by market. Provider warnings and partial results are exposed rather than hidden.
