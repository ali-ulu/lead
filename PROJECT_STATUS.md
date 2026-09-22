# LeadScout — Project Status

**Version:** 5.1.0  
**State:** Local sales/agent release

Implemented:

- OSM + Overture multi-source live discovery
- Global country/city/category/radius search
- No application-level 250-result cap
- Provider-level partial-result warnings
- Cross-source deduplication and source references
- Website/no-website cross-checking
- Public website contact enrichment
- Email, phone, booking and 8 social-channel extraction
- Lighthouse audit when available + safe heuristic fallback
- SSRF/private-network protections
- Opportunity, contactability and commercial scoring
- Pipeline stages
- Engagement states: not_contacted / drafted / sent / delivered / replied / rejected / bounced / no_response
- Activity timeline
- Notes and follow-up date
- Last contact / last reply tracking
- Delivery/reply webhook lifecycle support
- Do-not-contact
- XLSX + CSV with CRM/intelligence fields
- EN / TR / UR / SD interface
- RTL Urdu/Sindhi
- REST/OpenAPI
- MCP stdio + Streamable HTTP
- Autonomous sales-agent workflow
- Scoped agent write/send/clear permissions
- Persistent agent audit log
- Facebook OAuth
- Instagram professional-account OAuth
- Encrypted local OAuth tokens
- Meta messaging for eligible recipient/conversation IDs
- Windows/macOS/Linux bootstrap launchers
- Normal CI + MCP smoke + Overture live smoke + combined live smoke

Important limitation: public Instagram/Facebook profile URLs are not arbitrary messaging recipient IDs. Meta message sending only works where the connected business account and target conversation are API-eligible.
