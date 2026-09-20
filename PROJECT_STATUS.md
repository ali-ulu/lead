# Project Status — Ready for Use

**Version:** 1.0.0

The repository contains a complete local MVP that can be used immediately with no paid service and no API key.

Implemented:

- Live global location lookup
- Live local-business discovery from OpenStreetMap/Overpass
- 26 business categories
- Adjustable 10/20/30/50 km search radius
- Local SQLite persistence and deduplication
- Missing/unknown/weak/healthy website states
- Public website audit and score recalculation
- Explainable lead scoring
- EN/TR/DE outreach drafts
- Contact links and map link
- Pipeline states
- Do-not-contact handling
- CSV export
- Windows/macOS/Linux launchers
- CI + unit tests

Known data limitation: open-data coverage varies by market. A missing website/email/phone means the selected source did not provide it; it is not guaranteed to be absent everywhere online.
