# LeadScout

**LeadScout 7.0** is a local-first sales opportunity system for finding local businesses, verifying web presence, enriching contact data, auditing websites, preparing outreach, and tracking what happened after contact.

## LeadScout 7.0

- **Multi-source discovery:** OpenStreetMap / Overpass + Overture Places.
- Results are merged and deduplicated by domain, phone, and name + proximity.
- **No application-level 250-result cap.**
- Search results expose provider counts, partial-result state and warnings.
- **Website verification:** "No site found" is cross-checked against Overture and, when configured, a web search before it becomes a strong sales signal. A keyless DuckDuckGo lookup runs first, with Google Programmable Search as a free fallback; Brave and self-hosted SearXNG are optional upgrades.
- **Contact enrichment:** public business pages are scanned for email, phone, booking links and social accounts.
- Instagram, Facebook, LinkedIn, X/Twitter, YouTube, TikTok, Telegram and WhatsApp discovery.
- **Deep website audit:** Lighthouse performance/accessibility/best-practices plus structured content analysis.
- **SEO / AEO / GEO / AI Visibility scorecard:** evidence-based 0–100 readiness scores with inspectable reasons.
- **Opportunity Gap:** combines weak digital visibility with commercial attractiveness to prioritize sales opportunities.
- Scores are readiness heuristics, not ranking, AI citation, or traffic predictions.
- **Reputation enrichment:** optional Google Places match can add rating, review count, website and phone.
- Opportunity, contactability and commercial-intent scoring; reputation signals feed the commercial score.
- CRM with separate pipeline and engagement state.
- Activity timeline, notes, last contact, last reply and follow-up date.
- Native XLSX + CSV export including CRM and intelligence fields.
- English / Turkish / Urdu / Sindhi UI, with RTL for Urdu/Sindhi.
- EN / TR / UR / SD / DE outreach drafts.
- REST / OpenAPI agent API.
- MCP stdio + Streamable HTTP.
- Autonomous agent workflow: search → verify → audit/enrich → visibility scoring → draft → Excel.
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

### Health check

Before a long run, check the install:

```bash
python -m lead_hunter.doctor        # or: leadscout-doctor
python -m lead_hunter.doctor --json # machine-readable
```

It reports missing modules (for example `duckdb`, which silently disables Overture discovery), database state, active discovery and web-search providers, and which optional keys are set. It exits non-zero when something is wrong. `GET /api/health` returns the same provider status and a `degraded` flag.

## Data sources

LeadScout uses OpenStreetMap and Overture Places by default. If a provider is unavailable, the search is marked partial (`degraded: true`) and warnings are returned instead of pretending the result is complete. A zero-result search is explicitly flagged when a provider was unavailable, so an empty area is not confused with missing data.

A missing website, email or social field means **not found in the reviewed sources**, not proof of absence.

## Caching and request politeness

Provider responses (Nominatim, Overpass, Overture, web search) are cached on disk when `LEADSCOUT_CACHE_TTL` is set, so re-scans are fast and shared public endpoints are not hammered. Overpass fallback tiles are fetched in parallel, Nominatim requests are spaced to its one-per-second policy, transient errors are retried, and a provider that keeps failing is skipped for a cooldown instead of stalling a long scan. See `.env.example` for the knobs.

## Nightly scans and change reports

`scripts/nightly.py` scans selected city/category pairs, exports one XLSX, and compares each run with the previous one so new businesses, websites gained or lost, and fresh high-opportunity leads (score ≥ 70, no website, phone present) are reported. See [docs/AUTOMATION.md](docs/AUTOMATION.md) for the report format and a cron example.

## Data quality and deduplication

The same business often arrives in several shapes across providers. Before a lead is stored it is normalized: phone numbers become E.164 (`+90 555…`, `0555…` and `555…` collapse to `+90555…`), websites are lowercased with the scheme, `www.`, tracking parameters and trailing slash removed, and categories are mapped to a canonical key (`hairdresser`/`berber`/`kuaför` → `barber`). Records are then matched on normalized domain, phone, or a fuzzy name (character ratio plus trigram, ignoring generic words like "salon"/"restoran") within 250 m, so OSM and Overture rows for one business merge instead of splitting. See `lead_hunter/normalize.py`.

## CRM

Sales pipeline:

`new → reviewed → contacted → replied → proposal → won/lost`

Engagement outcome:

`not_contacted / drafted / sent / delivered / replied / rejected / bounced / no_response`

This keeps “where is the deal?” separate from “what happened to the message?”.

## Optional verification & reputation providers

Core discovery still works without paid API keys.

For general-web verification, LeadScout uses a keyless DuckDuckGo lookup first, then falls back to Google Programmable Search when its free key is configured. This keeps serial queries working even if the keyless endpoint rate-limits.

```bash
# Default order (first provider that answers wins):
#   1. DuckDuckGo   — keyless, no account
#   2. Google Programmable Search — free 100 queries/day, needs a key + engine id
# LEADSCOUT_WEB_SEARCH=duckduckgo,google

# Google Programmable Search (Custom Search JSON API):
# https://developers.google.com/custom-search/v1/overview
# GOOGLE_CSE_API_KEY=...
# GOOGLE_CSE_ID=...

# Brave Search API (paid, best rate limits): https://brave.com/search/api/
# BRAVE_SEARCH_API_KEY=...

# Self-hosted SearXNG with JSON output enabled (no vendor lock-in):
# SEARXNG_URL=http://127.0.0.1:8080

# Disable web verification entirely:
# LEADSCOUT_WEB_SEARCH=off
```

Set `LEADSCOUT_WEB_SEARCH` to a comma-separated chain to choose and order providers explicitly, e.g. `searxng,google,duckduckgo`. When it is unset, the default chain is DuckDuckGo then Google. `off` disables web verification. If a provider fails (for example a rate limit), the next one in the chain is tried and the failures are reported in the result.

For reputation enrichment:

```bash
GOOGLE_PLACES_API_KEY=...
```

LeadScout uses Google Places Text Search only for leads you explicitly enrich or when the sales agent runs with reputation enrichment enabled. Google Places rating, user rating count and website fields are billable fields. Keep billing quotas configured in Google Cloud.

A discovered web result is not accepted blindly. LeadScout excludes major social/directory hosts, scores name/location agreement, and only promotes a high-confidence candidate to the lead website.

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

## Roadmap

Planned work, in priority order, lives in [docs/ROADMAP.md](docs/ROADMAP.md).

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


## Visibility scorecard

LeadScout audits each available website into five inspectable scores:

- **SEO**: indexability, HTTPS, mobile, title/meta/H1, canonical, performance and business/entity structured data.
- **AEO**: answer-oriented headings, content depth, semantic structure, explicit business facts and extractable answers.
- **GEO**: entity clarity, structured facts, cross-profile identity, external source links, factual density, authorship and freshness.
- **AI Visibility**: weighted readiness from SEO, AEO and GEO.
- **Opportunity Gap**: digital visibility weakness weighted with commercial opportunity.

Google's generative-search guidance still relies on normal SEO fundamentals; LeadScout therefore does not award magic points for llms.txt or deprecated FAQ rich-result tricks.

A verified business with no independent website receives zero owned-web readiness scores and a high Opportunity Gap. An unverified missing website remains unscored until verification.

For sales targeting, LeadScout can filter both directions:

- minimum `Opportunity Gap` to keep commercially interesting digital gaps,
- maximum `SEO`, `AEO`, `GEO`, or `AI Visibility / AIO` to intentionally find weak digital presence.

Example agent brief:

> Find Karachi dentists with Opportunity Gap >= 70 and AIO <= 40, verify/audit them, then prepare outreach only for the leads that still match those thresholds.
