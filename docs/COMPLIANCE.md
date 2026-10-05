# Compliance and Responsible Use

This is a technical plan, not legal advice.

- Respect source licenses and terms.
- Respect robots/rate limits for site checks.
- Collect only data needed for legitimate B2B outreach.
- Distinguish business contact data from personal data.
- Honor opt-outs and do-not-contact status.
- Keep source and timestamp metadata.
- Follow privacy/direct-marketing law for the relevant markets.
- Do not bypass access controls or anti-bot protections.
- Do not infer sensitive personal characteristics.
- Default to manual review before sending.

## What the code does

**Source attribution.** Discovery results keep their origin in `source` and
`source_refs` (OpenStreetMap node/way id, Overture place id) plus `created_at` /
`updated_at`, so any row can be traced back and attributed. OpenStreetMap data is
© OpenStreetMap contributors, licensed ODbL; Overture Maps data is licensed
CDLA-Permissive-2.0. Both require attribution when redistributed — keep
`source_refs` intact in exports and downstream copies. See `docs/DATA_SOURCES.md`.

**Opt-out.** `POST /api/leads/{id}/dnc` sets `do_not_contact`; such leads are
excluded from outreach and from new discovery lookups (`db.list_leads` filters
them). `messaging_eligibility` refuses to send to a `do_not_contact` lead.

**Erasure.** `DELETE /api/leads/{id}` hard-deletes a lead and its activities and
search links (`db.delete_lead`). This is the KVKK/GDPR right-to-erasure path.

**Retention.** `DELETE /api/retention/purge?days=N` (or `nightly.py
--retention-days N`) deletes leads that were never contacted and are older than
N days (`db.purge_stale_leads`). Contacted leads and do-not-contact rows are kept
so a sweep never erases an active relationship or an opt-out record.

**Politeness.** Auditing and enrichment respect `robots.txt`
(`LEADSCOUT_RESPECT_ROBOTS=1`), send an identifying User-Agent, and can be
spaced with `LEADSCOUT_AUDIT_MIN_INTERVAL` / `LEADSCOUT_FETCH_MIN_INTERVAL`.
Outbound URLs are checked against private/reserved address ranges (SSRF guard),
including on every redirect hop. See `lead_hunter/netguard.py`.

