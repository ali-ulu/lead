// Package services is the application layer: discovery, verification, audit,
// enrichment, reputation, outreach drafts and lead queries.
package services

import (
	"errors"
	"fmt"
	"time"

	"github.com/ali-ulu/lead/leadscout-go/internal/aipresence"
	"github.com/ali-ulu/lead/leadscout-go/internal/audit"
	"github.com/ali-ulu/lead/leadscout-go/internal/crm"
	"github.com/ali-ulu/lead/leadscout-go/internal/db"
	"github.com/ali-ulu/lead/leadscout-go/internal/enrichment"
	"github.com/ali-ulu/lead/leadscout-go/internal/intelligence"
	"github.com/ali-ulu/lead/leadscout-go/internal/merge"
	"github.com/ali-ulu/lead/leadscout-go/internal/model"
	"github.com/ali-ulu/lead/leadscout-go/internal/normalize"
	"github.com/ali-ulu/lead/leadscout-go/internal/outreach"
	"github.com/ali-ulu/lead/leadscout-go/internal/providers/emailenrich"
	"github.com/ali-ulu/lead/leadscout-go/internal/providers/emailverify"
	"github.com/ali-ulu/lead/leadscout-go/internal/providers/googleplaces"
	"github.com/ali-ulu/lead/leadscout-go/internal/providers/nominatim"
	"github.com/ali-ulu/lead/leadscout-go/internal/providers/osm"
	"github.com/ali-ulu/lead/leadscout-go/internal/providers/photon"
	"github.com/ali-ulu/lead/leadscout-go/internal/providers/websearch"
	"github.com/ali-ulu/lead/leadscout-go/internal/visibility"
)

// PipelineStages are the sales pipeline states.
var PipelineStages = map[string]bool{
	"new": true, "reviewed": true, "contacted": true, "replied": true,
	"proposal": true, "won": true, "lost": true,
}

// MessageLanguages are the supported outreach languages.
var MessageLanguages = map[string]bool{"en": true, "tr": true, "ur": true, "sd": true, "de": true}

func intelligent(rows []*model.Lead) []*model.Lead {
	for _, lead := range rows {
		if len(lead.SourceRefs) >= 2 {
			if lead.Website == nil || *lead.Website == "" {
				lead.VerificationStatus = "verified_no_site"
			} else {
				lead.VerificationStatus = "cross_source"
			}
			notes := append([]string{}, lead.VerificationNotes...)
			notes = append(notes, "Business matched across independent place sources.")
			lead.VerificationNotes = dedupe(notes)
		}
		contact, commercial, reasons := intelligence.Calculate(lead)
		lead.ContactabilityScore = contact
		lead.CommercialScore = commercial
		lead.IntelligenceReasons = reasons
	}
	return rows
}

func dedupe(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range values {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

// DiscoverResult is the discovery response.
type DiscoverResult struct {
	OK             bool              `json:"ok"`
	Count          int               `json:"count"`
	SearchID       string            `json:"search_id"`
	IDs            []int64           `json:"ids"`
	IDsTruncated   bool              `json:"ids_truncated"`
	Area           *model.Area       `json:"area"`
	Category       string            `json:"category"`
	RadiusKM       int               `json:"radius_km"`
	ResultCap      *int              `json:"result_cap"`
	Providers      map[string]any    `json:"providers"`
	Partial        bool              `json:"partial"`
	Degraded       bool              `json:"degraded"`
	ProviderErrors map[string]string `json:"provider_errors"`
	Warnings       []string          `json:"warnings"`
}

// geocodeAreaWithFallback resolves a city, falling back to Photon.
//
// Nominatim is the primary geocoder but is rate-limited and occasionally
// unavailable. Photon is OSM-backed and keyless, so it keeps discovery
// working when Nominatim fails. A hard failure from both is returned.
func geocodeAreaWithFallback(city, country string) (*model.Area, []string, error) {
	area, err := nominatim.GeocodeArea(city, country, 20*time.Second)
	if err == nil {
		return area, nil, nil
	}
	fallback, photonErr := photon.GeocodeArea(city, country, 20*time.Second)
	if photonErr != nil {
		return nil, nil, err
	}
	return fallback, []string{fmt.Sprintf("Nominatim geocoding failed (%v); used Photon fallback.", err)}, nil
}

// DiscoverBusinesses runs multi-source discovery, dedupe, scoring and storage.
func DiscoverBusinesses(city, category, country string, radiusKM int, maxResults *int, providers []string) (*DiscoverResult, error) {
	if city == "" || category == "" {
		return nil, errors.New("City/area and industry are required.")
	}
	if _, ok := osm.CategoryFilters[category]; !ok {
		return nil, fmt.Errorf("Unsupported industry: %s", category)
	}
	if radiusKM < 1 {
		radiusKM = 1
	}
	if radiusKM > 100 {
		radiusKM = 100
	}
	if maxResults != nil && *maxResults <= 0 {
		maxResults = nil
	}

	wanted := map[string]bool{}
	if len(providers) == 0 {
		wanted["osm"] = true
		wanted["overture"] = true
	} else {
		for _, p := range providers {
			wanted[p] = true
		}
	}
	var unknown []string
	for p := range wanted {
		if p != "osm" && p != "overture" {
			unknown = append(unknown, p)
		}
	}

	area, geocodeWarnings, err := geocodeAreaWithFallback(city, country)
	if err != nil {
		return nil, err
	}

	var providerRows [][]*model.Lead
	summary := map[string]any{}
	warnings := append([]string{}, geocodeWarnings...)
	partial := false
	providerErrors := map[string]string{}

	if wanted["osm"] {
		limit := 0
		if maxResults != nil {
			limit = *maxResults
		}
		result, err := osm.SearchAroundDetailed(area.Lat, area.Lon, radiusKM, category, area.City, area.Country, 35, limit)
		if err != nil {
			return nil, err
		}
		rows, _ := result["rows"].([]*model.Lead)
		providerRows = append(providerRows, rows)
		summary["osm"] = map[string]any{"count": len(rows), "partial": result["partial"]}
		if p, _ := result["partial"].(bool); p {
			partial = true
		}
		if w, ok := result["warnings"].([]string); ok {
			warnings = append(warnings, w...)
		}
	}

	if wanted["overture"] {
		// The Go build has no DuckDB, so Overture is always unavailable. It is
		// reported explicitly instead of silently returning fewer results.
		partial = true
		msg := "Overture discovery requires DuckDB, which the Go build does not bundle; install the Python build or enable the OSM provider."
		providerErrors["overture"] = msg
		warnings = append(warnings, "Overture unavailable: "+msg)
		summary["overture"] = map[string]any{"count": 0, "partial": true, "error": msg}
	}

	for _, p := range unknown {
		warnings = append(warnings, "Unknown discovery providers ignored: "+p+".")
	}

	if len(providerRows) == 0 {
		return nil, errors.New("No discovery provider is enabled.")
	}

	rows := intelligent(merge.MergeLeads(providerRows...))
	if maxResults != nil && len(rows) > *maxResults {
		rows = rows[:*maxResults]
	}

	degraded := partial || len(unknown) > 0
	if degraded && len(rows) == 0 {
		warnings = append(warnings, "No businesses returned while one or more discovery providers were unavailable; this may be missing data rather than an empty area.")
	}
	ids, err := db.UpsertLeads(rows)
	if err != nil {
		return nil, err
	}
	searchID, err := db.RecordSearchRun(area.Country, area.City, category, radiusKM, ids, summary, partial, warnings)
	if err != nil {
		return nil, err
	}
	truncated := false
	if len(ids) > 500 {
		ids = ids[:500]
		truncated = true
	}
	return &DiscoverResult{
		OK: true, Count: len(rows), SearchID: searchID, IDs: ids, IDsTruncated: truncated,
		Area: area, Category: category, RadiusKM: radiusKM, ResultCap: maxResults,
		Providers: summary, Partial: partial, Degraded: degraded,
		ProviderErrors: providerErrors, Warnings: warnings,
	}, nil
}

func updateIntelligence(leadID int64, lead *model.Lead) (*model.Lead, error) {
	contact, commercial, reasons := intelligence.Calculate(lead)
	updates := map[string]any{
		"contactability_score": contact,
		"commercial_score":     commercial,
		"intelligence_reasons": reasons,
	}
	if lead.Website != nil && *lead.Website != "" && len(lead.VisibilitySignals) > 0 {
		vis := visibility.Calculate(evidenceFromSignals(lead.VisibilitySignals), commercial)
		updates["seo_score"] = vis.SEOScore
		updates["aeo_score"] = vis.AEOScore
		updates["geo_score"] = vis.GEOScore
		updates["ai_visibility_score"] = vis.AIVisibilityScore
		updates["opportunity_gap_score"] = vis.OpportunityGapScore
		updates["score_confidence"] = vis.ScoreConfidence
		updates["visibility_reasons"] = vis.VisibilityReasons
		updates["visibility_signals"] = vis.VisibilitySignals
	} else if lead.Website == nil || *lead.Website == "" {
		vis := visibility.MissingWebsite(commercial, lead.VerificationStatus == "verified_no_site")
		updates["seo_score"] = vis.SEOScore
		updates["aeo_score"] = vis.AEOScore
		updates["geo_score"] = vis.GEOScore
		updates["ai_visibility_score"] = vis.AIVisibilityScore
		updates["opportunity_gap_score"] = vis.OpportunityGapScore
		updates["score_confidence"] = vis.ScoreConfidence
		updates["visibility_reasons"] = vis.VisibilityReasons
		updates["visibility_signals"] = vis.VisibilitySignals
	}
	updated, err := db.UpdateLead(leadID, updates)
	if err != nil {
		return lead, nil
	}
	return updated, nil
}

func evidenceFromSignals(signals map[string]any) visibility.Evidence {
	getBool := func(k string) bool { b, _ := signals[k].(bool); return b }
	getInt := func(k string) int {
		f, ok := signals[k].(float64)
		if ok {
			return int(f)
		}
		i, ok := signals[k].(int)
		if ok {
			return i
		}
		return 0
	}
	getStr := func(k string) string { s, _ := signals[k].(string); return s }
	getFloatPtr := func(k string) *float64 {
		f, ok := signals[k].(float64)
		if !ok {
			return nil
		}
		return &f
	}
	getStrList := func(k string) []string {
		raw, ok := signals[k].([]any)
		if !ok {
			return nil
		}
		var out []string
		for _, v := range raw {
			if s, ok := v.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return visibility.Evidence{
		Indexable: getBool("indexable"), RobotsNoindex: getBool("robots_noindex"),
		HeaderNoindex: getBool("header_noindex"), Title: getStr("title"),
		MetaDescription: getBool("meta_description"), H1Count: getInt("h1_count"),
		H2H3Count: getInt("h2_h3_count"), Canonical: getStr("canonical"),
		HasHTTPS: getBool("has_https"), MobileOK: getBool("mobile_ok"),
		PerformanceScore: getFloatPtr("performance_score"), LighthouseSEOScore: getFloatPtr("lighthouse_seo_score"),
		AccessibilityScore: getFloatPtr("accessibility_score"), BestPracticesScore: getFloatPtr("best_practices_score"),
		WordCount: getInt("word_count"), QuestionHeadings: getInt("question_headings"),
		BusinessSchema: getBool("business_schema"), OrganizationSchema: getBool("organization_schema"),
		SchemaTypes: getStrList("schema_types"), SchemaFields: getStrList("schema_fields"),
		ExternalLinks: getInt("external_links"), FactSignals: getInt("fact_signals"),
		AuthorSignal: getBool("author_signal"), FreshSignal: getBool("fresh_signal"),
		SemanticMain: getBool("semantic_main"), SocialCount: getInt("social_count"),
		ContactSignal: getBool("contact_signal"), TTFBMS: getInt("ttfb_ms"),
	}
}

// EnrichLead crawls the lead's website for contacts and social links.
func EnrichLead(leadID int64) (map[string]any, error) {
	lead, err := db.GetLead(leadID)
	if err != nil {
		return nil, errors.New("Lead not found.")
	}
	if lead.Website == nil || *lead.Website == "" {
		return map[string]any{"lead": lead, "enrichment": map[string]any{"complete": false, "reason": "no_website"}}, nil
	}
	result, err := enrichment.EnrichWebsite(*lead.Website, 4)
	if err != nil {
		return nil, err
	}
	socials := map[string]string{}
	for k, v := range lead.SocialLinks {
		socials[k] = v
	}
	for k, v := range result.SocialLinks {
		socials[k] = v
	}
	region := lead.Country
	if region == "" {
		region = "TR"
	}
	updates := map[string]any{"social_links": socials}
	var email *string
	emailSource := ""
	if lead.Email != nil && *lead.Email != "" {
		email = lead.Email
		emailSource = "lead"
	} else if len(result.Emails) > 0 {
		email = &result.Emails[0]
		emailSource = "website"
		updates["email"] = *email
	} else if domain := normalize.NormalizeDomain(*lead.Website); domain != "" {
		// The site published no address; try the hosted waterfall.
		if waterfall, err := emailenrich.EnrichDomain(domain, 20*time.Second); err == nil {
			result.EmailEnrichment = waterfall
			if waterfall.Found {
				email = &waterfall.Email
				emailSource = waterfall.Source
				updates["email"] = waterfall.Email
			}
		}
	}
	if email != nil {
		result.EmailVerification = emailverify.VerifyEmail(*email, true, 5*time.Second)
	}
	if lead.Phone == nil && len(result.Phones) > 0 {
		updates["phone"] = normalize.NormalizePhone(result.Phones[0], region)
	}
	if lead.SocialURL == nil && len(socials) > 0 {
		for _, v := range socials {
			updates["social_url"] = v
			break
		}
	}
	if result.Complete {
		updates["verification_status"] = "verified"
	}
	updated, err := db.UpdateLead(leadID, updates)
	if err != nil {
		updated = lead
	}
	updated, _ = updateIntelligence(leadID, updated)
	_, _ = crm.AddActivity(leadID, "enrichment", "", statusFor(result.Complete), "", "", "", map[string]any{
		"pages_scanned": result.PagesScanned, "booking_links": result.BookingLinks,
		"email_source": emailSource, "email_status": emailStatus(result.EmailVerification),
	})
	return map[string]any{"lead": updated, "enrichment": result}, nil
}

func statusFor(complete bool) string {
	if complete {
		return "complete"
	}
	return "partial"
}

// emailStatus extracts the verification status for activity metadata.
func emailStatus(verification any) string {
	if v, ok := verification.(emailverify.Result); ok {
		return v.Status
	}
	return ""
}

// VerifyLead cross-checks a lead and, when a website is missing, a web search.
func VerifyLead(leadID int64) (map[string]any, error) {
	lead, err := db.GetLead(leadID)
	if err != nil {
		return nil, errors.New("Lead not found.")
	}
	var checks []map[string]any
	updated := lead

	// Cross-check against Overture when coordinates exist. The Go build has no
	// DuckDB, so this check reports the provider as unavailable rather than
	// pretending the business was verified.
	if lead.Latitude != nil && lead.Longitude != nil {
		checks = append(checks, map[string]any{
			"provider": "overture", "matched": false,
			"error": "Overture cross-check requires DuckDB, unavailable in the Go build",
		})
	}

	var webCheck *websearch.VerifyResult
	if updated.Website == nil || *updated.Website == "" {
		result := websearch.VerifyBusinessWeb(updated.Name, updated.City, updated.Country, "", 10)
		webCheck = &result
		if result.Verified && result.Website != nil {
			website := normalize.NormalizeWebsite(*result.Website)
			if website == "" {
				website = *result.Website
			}
			notes := append([]string{}, updated.VerificationNotes...)
			notes = append(notes, fmt.Sprintf("Official-site candidate verified via %s: %s", result.Provider, website))
			updated, _ = db.UpdateLead(leadID, map[string]any{
				"website": website, "website_status": "unknown",
				"verification_status": "web_verified", "verification_notes": dedupe(notes),
			})
		} else if result.Configured {
			notes := append([]string{}, updated.VerificationNotes...)
			notes = append(notes, fmt.Sprintf("No high-confidence official website found via %s.", result.Provider))
			updated, _ = db.UpdateLead(leadID, map[string]any{
				"verification_status": "verified_no_site", "verification_notes": dedupe(notes),
			})
		}
		if updated == nil {
			updated = lead
		}
		checks = append(checks, map[string]any{
			"provider": result.Provider, "matched": result.Verified, "configured": result.Configured,
		})
	}

	updated, _ = updateIntelligence(leadID, updated)
	if updated == nil {
		updated = lead
	}
	webMeta := map[string]any{}
	if webCheck != nil {
		webMeta = map[string]any{"provider": webCheck.Provider, "verified": webCheck.Verified, "configured": webCheck.Configured}
	}
	_, _ = crm.AddActivity(leadID, "verification", "", updated.VerificationStatus, "", "", "", map[string]any{
		"checks": checks, "web": webMeta,
	})
	verified := map[string]bool{"cross_source": true, "web_verified": true, "verified_no_site": true, "verified": true}
	var webOut any
	if webCheck != nil {
		webOut = webCheck
	}
	return map[string]any{
		"lead": updated, "verified": verified[updated.VerificationStatus],
		"verification_status": updated.VerificationStatus, "checks": checks, "web_verification": webOut,
	}, nil
}

// EnrichReputation adds Google Places rating/reviews/website/phone.
func EnrichReputation(leadID int64) (map[string]any, error) {
	lead, err := db.GetLead(leadID)
	if err != nil {
		return nil, errors.New("Lead not found.")
	}
	result, err := googleplaces.EnrichReputation(lead.Name, lead.City, lead.Country, lead.Latitude, lead.Longitude)
	if err != nil {
		return nil, err
	}
	if !result.Matched {
		_, _ = crm.AddActivity(leadID, "reputation", "", "not_matched", "", "", "", map[string]any{"reason": result.Reason})
		return map[string]any{"lead": lead, "reputation": result}, nil
	}
	sourceRefs := map[string]string{}
	for k, v := range lead.SourceRefs {
		sourceRefs[k] = v
	}
	if result.PlaceID != "" {
		sourceRefs["google_places"] = result.PlaceID
	}
	notes := append([]string{}, lead.VerificationNotes...)
	notes = append(notes, fmt.Sprintf("Reputation matched via Google Places (%.2f confidence).", result.MatchConfidence))
	updates := map[string]any{
		"rating": result.Rating, "review_count": result.ReviewCount,
		"source_refs": sourceRefs, "verification_notes": dedupe(notes),
	}
	if (lead.Website == nil || *lead.Website == "") && result.Website != "" {
		website := normalize.NormalizeWebsite(result.Website)
		if website == "" {
			website = result.Website
		}
		updates["website"] = website
		updates["website_status"] = "unknown"
		updates["verification_status"] = "reputation_verified"
	}
	if (lead.Phone == nil || *lead.Phone == "") && result.Phone != "" {
		updates["phone"] = normalize.NormalizePhone(result.Phone, orDefault(lead.Country, "TR"))
	}
	updated, err := db.UpdateLead(leadID, updates)
	if err != nil {
		updated = lead
	}
	updated, _ = updateIntelligence(leadID, updated)
	_, _ = crm.AddActivity(leadID, "reputation", "", "matched", "", "", "", map[string]any{
		"source": "google_places", "rating": result.Rating, "review_count": result.ReviewCount, "place_id": result.PlaceID,
	})
	return map[string]any{"lead": updated, "reputation": result}, nil
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// AuditLead runs a deep website audit and stores the result.
func AuditLead(leadID int64, enrich bool) (map[string]any, error) {
	lead, err := db.GetLead(leadID)
	if err != nil {
		return nil, errors.New("Lead not found.")
	}
	if lead.Website == nil || *lead.Website == "" {
		return nil, errors.New("This lead has no website to audit.")
	}
	result, err := audit.AuditURL(*lead.Website, 12*time.Second)
	if err != nil {
		return nil, err
	}
	socials := map[string]string{}
	for k, v := range lead.SocialLinks {
		socials[k] = v
	}
	for k, v := range result.SocialLinks {
		socials[k] = v
	}
	updates := map[string]any{
		"website_status":    result.WebsiteStatus,
		"performance_score": result.PerformanceScore, "seo_score": result.SEOScore,
		"aeo_score": result.AEOScore, "geo_score": result.GEOScore,
		"ai_visibility_score": result.AIVisibilityScore, "opportunity_gap_score": result.OpportunityGapScore,
		"score_confidence": result.ScoreConfidence, "response_ms": result.ResponseMS, "ttfb_ms": result.TTFBMS,
		"visibility_reasons": result.VisibilityReasons, "visibility_signals": result.VisibilitySignals,
		"accessibility_score": result.AccessibilityScore, "mobile_ok": result.MobileOK,
		"has_cta": result.HasCTA, "has_booking": result.HasBooking, "has_https": result.HasHTTPS,
		"audit_engine": result.AuditEngine, "social_links": socials,
	}
	updated, err := db.UpdateLead(leadID, updates)
	if err != nil {
		updated = lead
	}
	_, _ = crm.AddActivity(leadID, "audit", "", result.WebsiteStatus, "", "", "", map[string]any{
		"engine": result.AuditEngine, "quality_flags": result.QualityFlags,
	})
	return map[string]any{"lead": updated, "audit": result}, nil
}

// DraftOutreach returns a draft message for a lead.
func DraftOutreach(leadID int64, lang string) (map[string]any, error) {
	if !MessageLanguages[lang] {
		lang = "en"
	}
	lead, err := db.GetLead(leadID)
	if err != nil {
		return nil, errors.New("Lead not found.")
	}
	return map[string]any{"lead_id": leadID, "lang": lang, "message": outreach.BuildMessage(lead, lang)}, nil
}

// SetPipelineStage updates the sales pipeline stage.
func SetPipelineStage(leadID int64, status string) (any, error) {
	if !PipelineStages[status] {
		return nil, fmt.Errorf("Invalid pipeline stage: %s", status)
	}
	lead, err := db.UpdateLead(leadID, map[string]any{"pipeline_status": status})
	if err != nil {
		return nil, errors.New("Lead not found.")
	}
	return lead, nil
}

// MarkDoNotContact flags a lead as do-not-contact.
func MarkDoNotContact(leadID int64) (any, error) {
	return db.UpdateLead(leadID, map[string]any{"do_not_contact": true})
}

// MeasureAIPresenceForLead runs the opt-in AI presence measurement.
func MeasureAIPresenceForLead(leadID int64) (map[string]any, error) {
	lead, err := db.GetLead(leadID)
	if err != nil {
		return nil, errors.New("Lead not found.")
	}
	website := ""
	if lead.Website != nil {
		website = *lead.Website
	}
	result := aipresence.Measure(lead.Name, lead.Category, lead.City, lead.Country, website, 10)
	if _, err := db.UpdateLead(leadID, map[string]any{"ai_presence": result}); err != nil {
		return nil, err
	}
	_, _ = crm.AddActivity(leadID, "ai_presence", "", "measured", "", "", "", map[string]any{
		"cited_count": result["cited_count"], "queries": result["queries"],
	})
	return map[string]any{"lead_id": leadID, "ai_presence": result}, nil
}
