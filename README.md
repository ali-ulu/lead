# Nishan

**Nishan** is a local-first global business-opportunity scout. Pick a country, city/area, industry and radius; Nishan finds local businesses, highlights missing or weak web presence, collects available contact/social channels, audits public websites and prepares human-reviewed outreach drafts.

## V3 highlights

- New **red + white** product identity. No AI-ULU branding.
- Global live discovery with OpenStreetMap / Overpass.
- English, Turkish, **Urdu** and **Sindhi** interface support.
- Automatic RTL layout for Urdu and Sindhi.
- Outreach drafts in English, Turkish, Urdu and Sindhi.
- Multi-social capture:
  - Instagram
  - Facebook
  - LinkedIn
  - X / Twitter
  - YouTube
  - TikTok
  - Telegram
  - WhatsApp
- Social channels are collected both from OpenStreetMap contact tags and, when a website audit is run, from public links found on the business website.
- Explainable 0–100 opportunity score.
- Website audit for HTTPS, mobile viewport, contact CTA, booking signals, response-time proxy, basic SEO/accessibility signals.
- Pipeline: `new → reviewed → contacted → replied → proposal → won/lost`.
- Do-not-contact handling.
- CSV export.
- SQLite local persistence.
- No API key, n8n, Apify or paid CRM required.

## Start

### Windows
Double-click `START_WINDOWS.bat`.

### macOS
Double-click `START_MAC.command`.

### Linux
Run:

```bash
./start.sh
```

Or directly:

```bash
python3 Nishan.py
```

Nishan opens at `http://127.0.0.1:8787`.

### Requirement
Python 3.11+ only. The core app has no third-party Python package dependency.

## Typical flow

1. Choose a country (optional), city/area, industry and radius.
2. Click **Find leads**.
3. Filter by:
   - no site found
   - weak website
   - social account available
   - opportunity score
   - pipeline stage
4. Open a lead.
5. Review contact information and discovered social channels.
6. If a website exists, run **Website check**. This can also enrich social channels from the public site.
7. Generate an EN/TR/UR/SD outreach draft.
8. Copy the draft or open an email client.
9. Move the lead through the pipeline.
10. Export the working set to CSV when needed.

## Data & accuracy

Discovery uses OpenStreetMap through public Overpass endpoints and location lookup through Nominatim. Coverage varies by country, city and category.

**“No site found” means the currently reviewed data source did not provide an independent website. It is not proof that no website exists anywhere online.**

Social accounts have the same limitation: Nishan shows accounts found in the available source tags or public website links. Missing social data is unknown, not proof of absence.

Website audit results are lightweight public-page heuristics. The performance score is a response-time proxy, not Google Lighthouse.

## Privacy & safety

- Lead data stays in local SQLite.
- No analytics or telemetry are built in.
- No secrets/API keys are required.
- The website auditor blocks localhost and private-network targets, including redirects to private addresses.
- Messages are never auto-sent.
- Leads can be marked do-not-contact.

The user remains responsible for applicable privacy, marketing and anti-spam law in the target market.

## Technical shape

- Python standard-library HTTP server
- SQLite
- OpenStreetMap / Overpass
- Nominatim geocoding
- Vanilla HTML/CSS/JS
- Provider-neutral data model
- Safe public-website audit
- GitHub Actions CI

## Open-data note

OpenStreetMap data is subject to the Open Database License (ODbL) and public service usage policies. Public Overpass/Nominatim instances are shared infrastructure; use them responsibly. For high-volume production use, run or purchase a compliant data service instead of overloading community endpoints.
