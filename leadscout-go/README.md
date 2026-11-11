# LeadScout 7.0 (Go)

A provider-neutral, local-first rewrite of the Python LeadScout 7.0 sales
opportunity system in Go. It preserves the original behavior: multi-source
discovery, website verification, contact/reputation enrichment, deep website
audit, SEO/AEO/GEO/AI-visibility scoring, CRM, outreach drafting, the REST
agent API, the MCP server and the autonomous agent workflow.

## Requirements

- Go 1.23.4 or newer.

## Build and run

```bash
cd leadscout-go
go build -o leadscout ./cmd/leadscout

# Web UI + REST API (default http://127.0.0.1:8787)
LEADSCOUT_ROOT=.. ./leadscout

# Install health check
./leadscout -doctor
./leadscout -doctor -json

# MCP server (stdio, or streamable HTTP on http://127.0.0.1:8790/mcp)
./leadscout -mcp
./leadscout -mcp -mcp-transport streamable-http -mcp-port 8790
```

The SQLite schema is embedded, so no extra setup is needed. The database
defaults to `data/lead_hunter.db` under `LEADSCOUT_ROOT` and can be overridden
with `LEADSCOUT_DB`.

## Tests

```bash
go test ./...
```

## Layout

| Package | Responsibility |
| --- | --- |
| `internal/model` | Shared lead record and area types |
| `internal/config` | Scoring weights, thresholds and env overrides |
| `internal/db` | SQLite storage, migrations and queries |
| `internal/providers/{osm,nominatim,googleplaces,websearch}` | Discovery, geocoding, reputation and web search |
| `internal/normalize`, `internal/merge` | Field normalization and dedupe |
| `internal/audit`, `internal/enrichment` | Website audit (Lighthouse/heuristics) and contact enrichment |
| `internal/visibility`, `internal/scoring`, `internal/intelligence` | Readiness scores and commercial intelligence |
| `internal/aipresence` | Opt-in answer-style search measurement |
| `internal/crm`, `internal/outreach`, `internal/diff` | Pipeline/engagement, drafts and nightly change diffs |
| `internal/security` | Agent write/send/clear gates and audit log |
| `internal/services`, `internal/agent` | Orchestration services and the autonomous sales job |
| `internal/oauthmeta` | Meta (Facebook/Instagram) OAuth, webhooks and messaging |
| `internal/exporters` | CSV and XLSX output |
| `internal/server` | REST API and static UI |
| `internal/mcp` | MCP server (stdio + streamable HTTP) |
| `internal/doctor` | Install health checks |
| `cmd/leadscout` | Entry point (web, doctor, MCP) |

## Behavior and permission gates

- Website sending is off by default. Autonomous sending requires
  `LEADSCOUT_AGENT_SEND=1`; the MCP tool requires `LEADSCOUT_MCP_ALLOW_SEND=1`.
- MCP CRM writes default on (`LEADSCOUT_MCP_ALLOW_WRITE`), destructive clear
  defaults off (`LEADSCOUT_MCP_ALLOW_CLEAR`).
- AI presence measurement is opt-in (`LEADSCOUT_AI_PRESENCE=1`) and spends
  search quota.

## Known differences from the Python build

- **Overture discovery is unavailable.** It requires DuckDB, which is not
  bundled in the Go build. Discovery falls back to OpenStreetMap, and both
  `-doctor` and `GET /api/health` report the provider as unavailable and mark
  the install `degraded`. Verification of "no website found" still runs against
  web search.
