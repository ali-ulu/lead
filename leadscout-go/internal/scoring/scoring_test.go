package scoring

import (
	"strings"
	"testing"

	"github.com/ali-ulu/lead/leadscout-go/internal/model"
)

func TestMissingWebsiteWithContacts(t *testing.T) {
	score, reasons := Calculate(&model.Lead{
		WebsiteStatus: "missing", Phone: model.Str("x"), Email: model.Str("x"),
		SocialURL: model.Str("x"), DataConfidence: "high",
	})
	if score != 60 {
		t.Errorf("score = %d, want 60", score)
	}
	found := false
	for _, r := range reasons {
		if strings.Contains(r, "No known") {
			found = true
		}
	}
	if !found {
		t.Errorf("missing 'No known' reason: %v", reasons)
	}
}

func TestWeakDentist(t *testing.T) {
	score, reasons := Calculate(&model.Lead{
		Website: model.Str("https://example.test"), Category: "dentist",
		PerformanceScore: model.Int(30), SEOScore: model.Int(40),
		MobileOK: model.Bool(false), HasCTA: model.Bool(false), HasBooking: model.Bool(false),
		HasHTTPS: model.Bool(true), Phone: model.Str("x"), DataConfidence: "medium",
	})
	if score != 60 {
		t.Errorf("score = %d, want 60", score)
	}
	if len(reasons) < 6 {
		t.Errorf("reasons = %d, want >= 6", len(reasons))
	}
}

func TestCap(t *testing.T) {
	score, _ := Calculate(&model.Lead{
		Website: model.Str("http://x"), Category: "dentist",
		PerformanceScore: model.Int(1), SEOScore: model.Int(1),
		MobileOK: model.Bool(false), HasCTA: model.Bool(false), HasBooking: model.Bool(false),
		HasHTTPS: model.Bool(false), Phone: model.Str("x"), Email: model.Str("x"),
		SocialURL: model.Str("x"), DataConfidence: "high",
	})
	if score > 100 {
		t.Errorf("score = %d, want <= 100", score)
	}
}
