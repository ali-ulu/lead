# Data Sources

## Primary: Overture Maps Places

Use for global batch discovery where possible. Keep release-specific schema mapping inside an adapter.

Desired normalized fields: name, category/taxonomy, geometry, address, websites, phones, emails, socials, confidence/source metadata.

## Secondary: OpenStreetMap / Overpass

Useful tags include `name`, `amenity`, `shop`, `office`, `tourism`, `website`, `contact:website`, `phone`, `contact:phone`, `email`, `contact:email`.

Public Overpass endpoints are shared infrastructure. Use politely, rate-limit requests, and do not assume unlimited capacity.

## Optional paid enrichment

Only after open-data discovery proves useful. Paid sources should enrich a short list, not power every initial query.

## Data quality rules

- store source and retrieval timestamp
- missing data means unknown, not necessarily absent
- cross-check high-value leads before outreach
- preserve raw evidence separately from generated summaries
