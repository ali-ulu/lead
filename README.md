# AI-ULU Lead Hunter

A ready-to-use local application for finding businesses in any city whose web presence is missing or weak, auditing public websites, scoring opportunities, preparing outreach drafts, and tracking a lightweight sales pipeline.

## What it does

- Searches live OpenStreetMap/Overpass data by **country + city/area + industry + radius**.
- Works globally. It is not tied to Türkiye or to any single city.
- Flags businesses with **no known website** immediately.
- Audits public websites for HTTPS, mobile viewport, contact CTA, booking/reservation signals, response-time proxy, title/meta/H1 signals, and a few aging/accessibility hints.
- Recalculates an explainable 0–100 lead score after an audit.
- Keeps phone, email, social, website and map links together.
- Generates human-review outreach drafts in **English, Turkish and German**.
- Tracks pipeline stages: `new → reviewed → contacted → replied → proposal → won/lost`.
- Exports the current lead set to CSV.
- Stores everything locally in SQLite.
- Requires **no API key, no n8n, no Apify, no paid CRM**.

## Start

### Windows
Double-click `START_WINDOWS.bat`.

### macOS
Double-click `START_MAC.command` (or run `./START_MAC.command`).

### Linux
Run `./start.sh`.

The app opens automatically at `http://127.0.0.1:8787`.

### Requirement
Python 3.11+ only. There are no third-party Python packages to install.

## Use

1. Enter a country (optional), city/area, industry and radius.
2. Click **Find live leads**.
3. Filter to **No website** or inspect website-bearing leads.
4. Open a lead and click **Audit website** to classify weak sites and recalculate the score.
5. Generate EN/TR/DE outreach text, copy it or open an email draft.
6. Move the lead through the local pipeline and export CSV whenever needed.

## Data & accuracy

Discovery uses OpenStreetMap through public Overpass endpoints and location lookup through Nominatim. Coverage and contact fields vary by country and city. “No website” means no website was present in the open-data record, not proof that a website does not exist anywhere on the internet. Website audits are public-page heuristics; the performance score shown in-app is a response-time proxy, not a Google Lighthouse score.

The application does not auto-send messages. This is intentional: outreach stays human-reviewed, and the user remains responsible for local marketing, privacy, anti-spam and do-not-contact rules.

## Privacy & security

- SQLite data stays on the local machine.
- No analytics or telemetry are built in.
- No secrets or API keys are required.
- Website audit blocks localhost/private-network targets to avoid accidental local-network probing.
- A lead can be marked **Do not contact**, which hides it from normal lists.

## Technical shape

- Python standard-library HTTP server
- SQLite
- OpenStreetMap / Overpass
- Nominatim geocoding
- Vanilla HTML/CSS/JS UI
- Provider-neutral internal model so Overture/Google/other enrichment can be added without rewriting the product

## License / data note

OpenStreetMap data is subject to the Open Database License (ODbL) and public service usage policies. For high-volume or commercial-scale querying, operate your own compliant data pipeline or provider rather than hammering public community endpoints.
