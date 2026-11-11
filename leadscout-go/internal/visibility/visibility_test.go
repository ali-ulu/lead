package visibility

import (
	"os"
	"testing"

	"github.com/ali-ulu/lead/leadscout-go/internal/config"
)

func strongEvidence() Evidence {
	return Evidence{
		Indexable: true, Title: "Example Dental Karachi", MetaDescription: true,
		H1Count: 1, H2H3Count: 6, Canonical: "https://example.com/", HasHTTPS: true,
		MobileOK: true, PerformanceScore: fptr(88), LighthouseSEOScore: fptr(95),
		WordCount: 950, QuestionHeadings: 4, BusinessSchema: true, OrganizationSchema: false,
		SchemaTypes:   []string{"Dentist", "LocalBusiness"},
		SchemaFields:  []string{"name", "address", "telephone", "url", "logo", "sameAs"},
		ExternalLinks: 4, FactSignals: 12, AuthorSignal: true, FreshSignal: true,
		SemanticMain: true, SocialCount: 3, ContactSignal: true,
	}
}

func fptr(f float64) *float64 { return &f }

func TestStrongPageScoresHigh(t *testing.T) {
	r := Calculate(strongEvidence(), 80)
	if *r.SEOScore < 85 {
		t.Errorf("seo = %d, want >= 85", *r.SEOScore)
	}
	if *r.AEOScore < 80 {
		t.Errorf("aeo = %d, want >= 80", *r.AEOScore)
	}
	if *r.GEOScore < 80 {
		t.Errorf("geo = %d, want >= 80", *r.GEOScore)
	}
	if *r.AIVisibilityScore < 80 {
		t.Errorf("aio = %d, want >= 80", *r.AIVisibilityScore)
	}
	if *r.OpportunityGapScore >= 50 {
		t.Errorf("gap = %d, want < 50", *r.OpportunityGapScore)
	}
}

func TestNoindexPenalizesAIVisibility(t *testing.T) {
	e := strongEvidence()
	e.Indexable = false
	r := Calculate(e, 0)
	if *r.AIVisibilityScore >= 60 {
		t.Errorf("aio = %d, want < 60", *r.AIVisibilityScore)
	}
}

func TestVerifiedMissingSiteHasMaxGap(t *testing.T) {
	r := MissingWebsite(80, true)
	if r.SEOScore == nil || *r.SEOScore != 0 {
		t.Errorf("seo != 0")
	}
	if r.AIVisibilityScore == nil || *r.AIVisibilityScore != 0 {
		t.Errorf("aio != 0")
	}
	if *r.OpportunityGapScore < 90 {
		t.Errorf("gap = %d, want >= 90", *r.OpportunityGapScore)
	}
}

func TestUnverifiedMissingHasNoScores(t *testing.T) {
	r := MissingWebsite(50, false)
	if r.SEOScore != nil || r.AIVisibilityScore != nil {
		t.Errorf("unverified missing site must stay unscored")
	}
}

func TestPercentileRankIsShareAtOrBelow(t *testing.T) {
	pop := []*int{ip(10), ip(20), ip(30), ip(40), ip(50)}
	if got := *PercentileRank(ip(30), pop); got != 60 {
		t.Errorf("rank(30) = %d, want 60", got)
	}
	if got := *PercentileRank(ip(50), pop); got != 100 {
		t.Errorf("rank(50) = %d, want 100", got)
	}
	if got := *PercentileRank(ip(5), pop); got != 0 {
		t.Errorf("rank(5) = %d, want 0", got)
	}
	if PercentileRank(nil, pop) != nil {
		t.Errorf("nil should stay nil")
	}
}

func ip(i int) *int { return &i }

func TestScoreConfidenceLabels(t *testing.T) {
	r := Calculate(Evidence{Indexable: true}, 0)
	if r.ScoreConfidence["seo"] != "heuristic" {
		t.Errorf("seo confidence = %q", r.ScoreConfidence["seo"])
	}
	if r.ScoreConfidence["ai_visibility"] != "composite" {
		t.Errorf("aio confidence = %q", r.ScoreConfidence["ai_visibility"])
	}
	r2 := Calculate(Evidence{Indexable: true, LighthouseSEOScore: fptr(90), AccessibilityScore: fptr(95), BestPracticesScore: fptr(92)}, 0)
	if r2.ScoreConfidence["seo"] != "lighthouse" || r2.ScoreConfidence["aeo"] != "lighthouse" || r2.ScoreConfidence["geo"] != "lighthouse" {
		t.Errorf("lighthouse labels wrong: %v", r2.ScoreConfidence)
	}
}

func TestEnvOverrideChangesScore(t *testing.T) {
	base := Calculate(Evidence{Indexable: true, Title: "T", HasHTTPS: true}, 0)
	os.Setenv("LEADSCOUT_VISIBILITY_WEIGHTS", `{"seo": {"indexable": 100}}`)
	defer os.Unsetenv("LEADSCOUT_VISIBILITY_WEIGHTS")
	boosted := Calculate(Evidence{Indexable: true, Title: "T", HasHTTPS: true}, 0)
	if *boosted.SEOScore < *base.SEOScore {
		t.Errorf("override should not lower seo: base=%d boosted=%d", *base.SEOScore, *boosted.SEOScore)
	}
}

func TestInvalidEnvIsIgnored(t *testing.T) {
	os.Setenv("LEADSCOUT_VISIBILITY_WEIGHTS", "{not json")
	defer os.Unsetenv("LEADSCOUT_VISIBILITY_WEIGHTS")
	got := config.VisibilityWeights()
	if got["seo"]["indexable"] != config.DefaultVisibilityWeights()["seo"]["indexable"] {
		t.Errorf("invalid JSON should fall back to defaults")
	}
}
