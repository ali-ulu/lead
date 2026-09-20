# Architecture

## Principles

1. Provider-neutral ingestion.
2. Evidence before AI.
3. Local-first MVP.
4. Replaceable discovery/audit/outreach/storage components.
5. Human gate before send.

## Logical architecture

```text
Web UI
  |
API Core
  |-------------------------------|
Discovery                         Audit
  |                               |
Overture + OSM/Overpass        Crawler + Lighthouse
  |                               |
  +---------- Normalize / Evidence+
                  |
              Dedupe
                  |
              Score Engine
                  |
              SQLite -> PostgreSQL
                  |
              Draft / Pipeline
```

## Shared lead model

Identity: `source`, `source_id`, `name`, `country`, `city`, `category`, coordinates.

Contact: `website`, `phone`, `email`, `social_url`.

Audit evidence: website status, performance, SEO, accessibility, mobile, CTA, booking, HTTPS.

Sales state: score, score reasons, pipeline state, do-not-contact.

## Dedupe order

1. normalized phone
2. normalized domain
3. stable provider id
4. fallback: normalized name + rounded coordinates

Never merge solely on business name.
