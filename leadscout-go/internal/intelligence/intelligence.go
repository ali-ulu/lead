// Package intelligence computes contactability and commercial scores.
package intelligence

import (
	"github.com/ali-ulu/lead/leadscout-go/internal/model"
)

var HighValue = map[string]bool{
	"dentist": true, "clinic": true, "lawyer": true, "real_estate": true,
	"accountant": true, "hotel": true, "beauty": true, "spa": true,
	"physiotherapy": true, "car_dealer": true,
}

var BookingValue = map[string]bool{
	"dentist": true, "clinic": true, "physiotherapy": true, "beauty": true,
	"hairdresser": true, "barber": true, "spa": true, "hotel": true, "restaurant": true,
}

// Calculate returns (contactability, commercial, reasons).
func Calculate(lead *model.Lead) (int, int, []string) {
	contact := 0
	commercial := 0
	var reasons []string

	if lead.Phone != nil && *lead.Phone != "" {
		contact += 25
		reasons = append(reasons, "Phone available")
	}
	if lead.Email != nil && *lead.Email != "" {
		contact += 30
		reasons = append(reasons, "Email available")
	}
	if n := len(lead.SocialLinks); n > 0 {
		add := 10 + 5*n
		if add > 30 {
			add = 30
		}
		contact += add
		reasons = append(reasons, plural(n)+" social channel(s)")
	}
	if lead.Website != nil && *lead.Website != "" {
		contact += 10
	}
	if lead.VerificationStatus == "cross_source" || lead.VerificationStatus == "verified" {
		contact += 5
		reasons = append(reasons, "Cross-source verified")
	}
	if contact > 100 {
		contact = 100
	}

	category := lead.Category
	if HighValue[category] {
		commercial += 25
		reasons = append(reasons, "Higher-value service category")
	}
	switch lead.WebsiteStatus {
	case "missing":
		commercial += 30
		reasons = append(reasons, "No site found in verified sources")
	case "weak":
		commercial += 25
		reasons = append(reasons, "Weak website opportunity")
	}
	if lead.MobileOK != nil && !*lead.MobileOK {
		commercial += 10
	}
	if lead.HasCTA != nil && !*lead.HasCTA {
		commercial += 10
	}
	if BookingValue[category] && lead.HasBooking != nil && !*lead.HasBooking {
		commercial += 8
	}
	if lead.Rating != nil && *lead.Rating >= 4 {
		commercial += 7
		reasons = append(reasons, "Strong public rating ("+format1(*lead.Rating)+")")
	}
	if lead.ReviewCount != nil && *lead.ReviewCount >= 20 {
		commercial += 5
		reasons = append(reasons, "Established review volume ("+itoa(*lead.ReviewCount)+")")
	}
	if len(lead.SocialLinks) > 0 {
		commercial += 5
	}
	if commercial > 100 {
		commercial = 100
	}
	return contact, commercial, reasons
}
