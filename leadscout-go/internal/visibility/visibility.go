// Package visibility computes inspectable SEO/AEO/GEO readiness scores, the
// composite AI Visibility score and the sales Opportunity Gap.
package visibility

import (
	"fmt"
	"math"
	"sort"

	"github.com/ali-ulu/lead/leadscout-go/internal/config"
)

// Evidence holds the markup and performance signals an audit extracts.
type Evidence struct {
	Indexable          bool     `json:"indexable"`
	RobotsNoindex      bool     `json:"robots_noindex"`
	HeaderNoindex      bool     `json:"header_noindex"`
	Title              string   `json:"title"`
	MetaDescription    bool     `json:"meta_description"`
	H1Count            int      `json:"h1_count"`
	H2H3Count          int      `json:"h2_h3_count"`
	Canonical          string   `json:"canonical"`
	HasHTTPS           bool     `json:"has_https"`
	MobileOK           bool     `json:"mobile_ok"`
	PerformanceScore   *float64 `json:"performance_score"`
	LighthouseSEOScore *float64 `json:"lighthouse_seo_score"`
	AccessibilityScore *float64 `json:"accessibility_score"`
	BestPracticesScore *float64 `json:"best_practices_score"`
	WordCount          int      `json:"word_count"`
	QuestionHeadings   int      `json:"question_headings"`
	BusinessSchema     bool     `json:"business_schema"`
	OrganizationSchema bool     `json:"organization_schema"`
	SchemaTypes        []string `json:"schema_types"`
	SchemaFields       []string `json:"schema_fields"`
	ExternalLinks      int      `json:"external_links"`
	FactSignals        int      `json:"fact_signals"`
	AuthorSignal       bool     `json:"author_signal"`
	FreshSignal        bool     `json:"fresh_signal"`
	SemanticMain       bool     `json:"semantic_main"`
	SocialCount        int      `json:"social_count"`
	ContactSignal      bool     `json:"contact_signal"`
	TTFBMS             int      `json:"ttfb_ms"`
}

// Result is the visibility calculation output.
type Result struct {
	SEOScore            *int                `json:"seo_score"`
	AEOScore            *int                `json:"aeo_score"`
	GEOScore            *int                `json:"geo_score"`
	AIVisibilityScore   *int                `json:"ai_visibility_score"`
	OpportunityGapScore *int                `json:"opportunity_gap_score"`
	ScoreConfidence     map[string]string   `json:"score_confidence"`
	VisibilityReasons   map[string][]string `json:"visibility_reasons"`
	VisibilitySignals   map[string]any      `json:"visibility_signals"`
	ScoreNote           string              `json:"score_note,omitempty"`
}

func clamp(v float64) int {
	return int(math.Max(0, math.Min(100, math.Round(v))))
}

func add(reasons *[]string, condition bool, points float64, positive, negative string) float64 {
	if condition {
		*reasons = append(*reasons, fmt.Sprintf("+%s %s", fmtG(points), positive))
		return points
	}
	if negative != "" {
		*reasons = append(*reasons, fmt.Sprintf("0/%s %s", fmtG(points), negative))
	}
	return 0
}

func fmtG(f float64) string {
	if f == math.Trunc(f) {
		return fmt.Sprintf("%d", int(f))
	}
	return fmt.Sprintf("%g", f)
}

func present(v *float64) bool { return v != nil }

// Calculate produces readiness scores. These are readiness/quality heuristics,
// not ranking or citation predictions.
func Calculate(e Evidence, commercialScore int) Result {
	w := config.VisibilityWeights()
	seoW, aeoW, geoW := w["seo"], w["aeo"], w["geo"]
	aioW, gapW := w["ai_visibility"], w["opportunity_gap"]

	indexable := e.Indexable
	title := e.Title != ""
	meta := e.MetaDescription
	h1 := e.H1Count
	h2h3 := e.H2H3Count
	canonical := e.Canonical != ""
	https := e.HasHTTPS
	mobile := e.MobileOK
	wordCount := e.WordCount
	questionHeadings := e.QuestionHeadings
	businessSchema := e.BusinessSchema
	organizationSchema := e.OrganizationSchema
	schemaFieldSet := map[string]bool{}
	for _, f := range e.SchemaFields {
		schemaFieldSet[f] = true
	}
	externalLinks := e.ExternalLinks
	factSignals := e.FactSignals
	authorSignal := e.AuthorSignal
	freshSignal := e.FreshSignal
	semanticMain := e.SemanticMain
	socialCount := e.SocialCount
	contactSignal := e.ContactSignal

	reasons := map[string][]string{"seo": {}, "aeo": {}, "geo": {}, "ai_visibility": {}, "opportunity_gap": {}}

	seoReasons := reasons["seo"]
	seoHeuristic := 0.0
	seoHeuristic += add(&seoReasons, indexable, seoW["indexable"], "Page is indexable", "Page has a noindex signal")
	seoHeuristic += add(&seoReasons, https, seoW["https"], "HTTPS", "HTTPS missing")
	seoHeuristic += add(&seoReasons, mobile, seoW["mobile"], "Mobile viewport present", "Mobile viewport missing")
	seoHeuristic += add(&seoReasons, title, seoW["title"], "Page title present", "Page title missing")
	seoHeuristic += add(&seoReasons, meta, seoW["meta"], "Meta description present", "Meta description missing")
	seoHeuristic += add(&seoReasons, h1 == 1, seoW["h1"], "Single H1", "H1 structure is missing or ambiguous")
	seoHeuristic += add(&seoReasons, h2h3 >= 2, seoW["headings"], "Supporting heading structure", "Thin heading structure")
	seoHeuristic += add(&seoReasons, canonical, seoW["canonical"], "Canonical URL declared", "Canonical URL not detected")
	seoHeuristic += add(&seoReasons, businessSchema || organizationSchema, seoW["schema"], "Business/entity structured data", "Business/entity structured data not detected")
	seoHeuristic += add(&seoReasons, present(e.PerformanceScore) && *e.PerformanceScore >= config.PerformanceGood, seoW["performance"], "Usable performance signal", "Performance signal is weak or unavailable")
	var seo int
	if present(e.LighthouseSEOScore) {
		share := seoW["lighthouse_share"]
		seo = clamp(*e.LighthouseSEOScore*share + seoHeuristic*(1-share))
		seoReasons = append(seoReasons, fmt.Sprintf("Lighthouse SEO contributes %d%% (%d/100)", int(math.Round(share*100)), int(math.Round(*e.LighthouseSEOScore))))
	} else {
		seo = clamp(seoHeuristic)
	}
	reasons["seo"] = seoReasons

	aeoReasons := reasons["aeo"]
	aeo := 0.0
	aeo += add(&aeoReasons, indexable, aeoW["indexable"], "Content can be indexed", "Noindex blocks answer discovery")
	aeo += add(&aeoReasons, title && h1 >= 1, aeoW["topic"], "Clear page topic", "Page topic is weakly defined")
	aeo += add(&aeoReasons, h2h3 >= 3, aeoW["hierarchy"], "Scannable answer hierarchy", "More descriptive subheadings would help")
	aeo += add(&aeoReasons, questionHeadings >= 1, aeoW["questions"], "Question-oriented headings", "No question-oriented headings detected")
	aeo += add(&aeoReasons, wordCount >= 350, aeoW["depth"], "Enough explanatory content", "Content is thin")
	aeo += add(&aeoReasons, businessSchema || organizationSchema, aeoW["schema"], "Entity structured data supports explicit answers", "Entity structured data not detected")
	aeo += add(&aeoReasons, contactSignal, aeoW["contact"], "Concrete contact/action facts", "Contact/action facts are weak")
	aeo += add(&aeoReasons, semanticMain, aeoW["semantic"], "Semantic main/article structure", "Semantic main/article structure not detected")
	aeo += add(&aeoReasons, meta, aeoW["summary"], "Concise page summary available", "Meta summary missing")
	if present(e.AccessibilityScore) {
		aeo += add(&aeoReasons, *e.AccessibilityScore >= 90, aeoW["accessibility"], "Accessible markup supports extraction", "Accessibility score is low")
	}
	aeoScore := clamp(aeo)
	reasons["aeo"] = aeoReasons

	geoReasons := reasons["geo"]
	geo := 0.0
	geo += add(&geoReasons, indexable, geoW["indexable"], "Content is crawlable/indexable", "Noindex reduces discoverability")
	geo += add(&geoReasons, businessSchema || organizationSchema, geoW["schema"], "Explicit business/entity schema", "Entity schema not detected")
	fieldsHit := 0
	for _, f := range []string{"name", "address", "telephone", "url", "logo"} {
		if schemaFieldSet[f] {
			fieldsHit++
		}
	}
	geo += add(&geoReasons, fieldsHit >= 3, geoW["fields"], "Structured entity has multiple concrete fields", "Structured entity facts are sparse")
	geo += add(&geoReasons, schemaFieldSet["sameAs"] || socialCount >= 2, geoW["identity"], "Entity identity is connected across profiles", "Cross-profile entity links are weak")
	geo += add(&geoReasons, externalLinks >= 2, geoW["external"], "Outbound source/citation links detected", "Few external source links detected")
	geo += add(&geoReasons, factSignals >= 3, geoW["facts"], "Concrete factual/numeric signals", "Few concrete factual signals")
	geo += add(&geoReasons, wordCount >= 500, geoW["depth"], "Substantive crawlable content", "Content depth is limited")
	geo += add(&geoReasons, authorSignal, geoW["author"], "Authorship signal detected", "Authorship signal not detected")
	geo += add(&geoReasons, freshSignal, geoW["fresh"], "Freshness/date signal detected", "Freshness signal is weak")
	if present(e.BestPracticesScore) {
		geo += add(&geoReasons, *e.BestPracticesScore >= 90, geoW["best_practices"], "Clean technical baseline for machine consumption", "Technical best-practices score is low")
	}
	geoScore := clamp(geo)
	reasons["geo"] = geoReasons

	aiVisibility := clamp(float64(seo)*aioW["seo"] + float64(aeoScore)*aioW["aeo"] + float64(geoScore)*aioW["geo"])
	reasons["ai_visibility"] = append(reasons["ai_visibility"], fmt.Sprintf("%d%% SEO (%d) + %d%% AEO (%d) + %d%% GEO (%d)",
		int(math.Round(aioW["seo"]*100)), seo, int(math.Round(aioW["aeo"]*100)), aeoScore, int(math.Round(aioW["geo"]*100)), geoScore))
	if !indexable {
		aiVisibility = clamp(float64(aiVisibility) * aioW["noindex_penalty"])
		reasons["ai_visibility"] = append(reasons["ai_visibility"], "Noindex penalty applied")
	}

	commercial := int(math.Max(0, math.Min(100, float64(commercialScore))))
	opportunityGap := clamp(float64(100-aiVisibility)*gapW["visibility"] + float64(commercial)*gapW["commercial"])
	reasons["opportunity_gap"] = append(reasons["opportunity_gap"], fmt.Sprintf("%d%% digital visibility gap (%d) + %d%% commercial signal (%d)",
		int(math.Round(gapW["visibility"]*100)), 100-aiVisibility, int(math.Round(gapW["commercial"]*100)), commercial))

	conf := map[string]string{
		"seo": "heuristic", "aeo": "heuristic", "geo": "heuristic", "ai_visibility": "composite",
	}
	if present(e.LighthouseSEOScore) {
		conf["seo"] = "lighthouse"
	}
	if present(e.AccessibilityScore) {
		conf["aeo"] = "lighthouse"
	}
	if present(e.BestPracticesScore) {
		conf["geo"] = "lighthouse"
	}

	signals := map[string]any{
		"indexable": indexable, "robots_noindex": e.RobotsNoindex, "header_noindex": e.HeaderNoindex,
		"title": e.Title, "meta_description": e.MetaDescription, "h1_count": h1, "h2_h3_count": h2h3,
		"canonical": e.Canonical, "has_https": https, "mobile_ok": mobile,
		"performance_score": e.PerformanceScore, "lighthouse_seo_score": e.LighthouseSEOScore,
		"accessibility_score": e.AccessibilityScore, "best_practices_score": e.BestPracticesScore,
		"word_count": wordCount, "question_headings": questionHeadings, "business_schema": businessSchema,
		"organization_schema": organizationSchema, "schema_types": e.SchemaTypes, "schema_fields": e.SchemaFields,
		"external_links": externalLinks, "fact_signals": factSignals, "author_signal": authorSignal,
		"fresh_signal": freshSignal, "semantic_main": semanticMain, "social_count": socialCount,
		"contact_signal": contactSignal, "ttfb_ms": e.TTFBMS,
	}

	return Result{
		SEOScore:            &seo,
		AEOScore:            &aeoScore,
		GEOScore:            &geoScore,
		AIVisibilityScore:   &aiVisibility,
		OpportunityGapScore: &opportunityGap,
		ScoreConfidence:     conf,
		VisibilityReasons:   reasons,
		VisibilitySignals:   signals,
		ScoreNote:           "Readiness heuristics, not search ranking or AI citation predictions.",
	}
}

// MissingWebsite returns the scorecard for a lead without a website.
func MissingWebsite(commercialScore int, verified bool) Result {
	if !verified {
		reasons := map[string][]string{
			"seo":             {"Website not yet verified as absent."},
			"aeo":             {"Website not yet verified as absent."},
			"geo":             {"Website not yet verified as absent."},
			"ai_visibility":   {"Website not yet verified as absent."},
			"opportunity_gap": {"Verify website absence before assigning a full digital gap."},
		}
		return Result{
			ScoreConfidence: map[string]string{
				"seo": "unverified", "aeo": "unverified", "geo": "unverified", "ai_visibility": "unverified",
			},
			VisibilityReasons: reasons,
			VisibilitySignals: map[string]any{"website_missing": true, "verified": false},
		}
	}
	commercial := int(math.Max(0, math.Min(100, float64(commercialScore))))
	gap := clamp(75 + float64(commercial)*0.25)
	zero := 0
	reasons := map[string][]string{
		"seo":             {"Verified: no independent website available to optimize."},
		"aeo":             {"Verified: no independent website available to answer from."},
		"geo":             {"Verified: no independent website entity/content surface found."},
		"ai_visibility":   {"Verified website absence creates a maximum owned-web visibility gap."},
		"opportunity_gap": {fmt.Sprintf("Verified website absence + commercial signal (%d).", commercialScore)},
	}
	return Result{
		SEOScore: &zero, AEOScore: &zero, GEOScore: &zero, AIVisibilityScore: &zero,
		OpportunityGapScore: &gap,
		ScoreConfidence: map[string]string{
			"seo": "verified_missing", "aeo": "verified_missing",
			"geo": "verified_missing", "ai_visibility": "verified_missing",
		},
		VisibilityReasons: reasons,
		VisibilitySignals: map[string]any{"website_missing": true, "verified": true},
	}
}

// PercentileRank is the share of the sample at or below value, as 0-100.
func PercentileRank(value *int, population []*int) *int {
	if value == nil {
		return nil
	}
	var sample []int
	for _, p := range population {
		if p != nil {
			sample = append(sample, *p)
		}
	}
	if len(sample) == 0 {
		return nil
	}
	sort.Ints(sample)
	below := sort.SearchInts(sample, *value+1)
	out := int(math.Round(float64(below) / float64(len(sample)) * 100))
	return &out
}
