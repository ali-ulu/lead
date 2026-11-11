// Package scoring computes the opportunity lead score and its human-readable
// reasons.
package scoring

import (
	"fmt"
	"math"

	"github.com/ali-ulu/lead/leadscout-go/internal/config"
	"github.com/ali-ulu/lead/leadscout-go/internal/model"
)

var BookingRelevant = map[string]bool{
	"dentist": true, "beauty": true, "hairdresser": true, "hotel": true,
	"restaurant": true, "clinic": true, "spa": true, "barber": true,
	"physiotherapy": true,
}

func fmtG(f float64) string {
	if f == math.Trunc(f) {
		return fmt.Sprintf("%d", int(f))
	}
	return fmt.Sprintf("%g", f)
}

// Calculate returns the 0-100 opportunity score and the reasons behind it.
func Calculate(lead *model.Lead) (int, []string) {
	w := config.ScoreWeights()
	score := 0.0
	var reasons []string

	website := ""
	if lead.Website != nil {
		website = *lead.Website
	}
	status := lead.WebsiteStatus
	if website == "" || status == "missing" {
		score += w["no_website"]
		reasons = append(reasons, fmt.Sprintf("No known independent website (+%s)", fmtG(w["no_website"])))
	} else {
		if lead.PerformanceScore != nil && float64(*lead.PerformanceScore) < config.PerformanceWeak {
			score += w["low_performance"]
			reasons = append(reasons, fmt.Sprintf("Very low website performance (+%s)", fmtG(w["low_performance"])))
		}
		if lead.MobileOK != nil && !*lead.MobileOK {
			score += w["mobile_problem"]
			reasons = append(reasons, fmt.Sprintf("Mobile usability problem (+%s)", fmtG(w["mobile_problem"])))
		}
		if lead.HasCTA != nil && !*lead.HasCTA {
			score += w["no_cta"]
			reasons = append(reasons, fmt.Sprintf("No clear conversion CTA (+%s)", fmtG(w["no_cta"])))
		}
		if BookingRelevant[lead.Category] && lead.HasBooking != nil && !*lead.HasBooking {
			score += w["no_booking"]
			reasons = append(reasons, fmt.Sprintf("No visible booking/reservation path (+%s)", fmtG(w["no_booking"])))
		}
		if lead.SEOScore != nil && float64(*lead.SEOScore) < config.SEOScoreWeak {
			score += w["weak_seo"]
			reasons = append(reasons, fmt.Sprintf("Weak SEO audit score (+%s)", fmtG(w["weak_seo"])))
		}
		if lead.HasHTTPS != nil && !*lead.HasHTTPS {
			score += w["no_https"]
			reasons = append(reasons, fmt.Sprintf("HTTPS missing (+%s)", fmtG(w["no_https"])))
		}
	}
	if lead.Phone != nil && *lead.Phone != "" {
		score += w["phone"]
		reasons = append(reasons, fmt.Sprintf("Phone contact available (+%s)", fmtG(w["phone"])))
	}
	if lead.Email != nil && *lead.Email != "" {
		score += w["email"]
		reasons = append(reasons, fmt.Sprintf("Email contact available (+%s)", fmtG(w["email"])))
	}
	if lead.SocialURL != nil && *lead.SocialURL != "" {
		score += w["social"]
		reasons = append(reasons, fmt.Sprintf("Social presence available (+%s)", fmtG(w["social"])))
	}
	switch lead.DataConfidence {
	case "high":
		score += w["confidence_high"]
		reasons = append(reasons, fmt.Sprintf("High source confidence (+%s)", fmtG(w["confidence_high"])))
	case "medium":
		score += w["confidence_medium"]
		reasons = append(reasons, fmt.Sprintf("Medium source confidence (+%s)", fmtG(w["confidence_medium"])))
	}
	return int(math.Min(100, math.Round(score))), reasons
}
