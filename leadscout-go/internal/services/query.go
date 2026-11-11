package services

import (
	"strconv"
	"strings"

	"github.com/ali-ulu/lead/leadscout-go/internal/db"
	"github.com/ali-ulu/lead/leadscout-go/internal/model"
	"github.com/ali-ulu/lead/leadscout-go/internal/visibility"
)

// QueryFilters mirrors the query_leads keyword arguments.
type QueryFilters struct {
	SearchID          string
	IDs               []int64
	Country           string
	City              string
	Category          string
	WebsiteStatus     string
	PipelineStatus    string
	EngagementStatus  string
	MinScore          int
	MinSEOScore       int
	MinAEOScore       int
	MinGEOScore       int
	MinAIVisibility   int
	MinOpportunityGap int
	MaxSEOScore       int
	MaxAEOScore       int
	MaxGEOScore       int
	MaxAIVisibility   int
	HasSocial         bool
	Contactable       bool
}

// QueryLeads returns filtered leads with a market-relative AI Visibility percentile.
func QueryLeads(f QueryFilters) ([]*model.Lead, error) {
	filters := map[string]string{}
	if f.SearchID != "" {
		filters["search_id"] = f.SearchID
	}
	if len(f.IDs) > 0 {
		parts := make([]string, len(f.IDs))
		for i, id := range f.IDs {
			parts[i] = strconv.FormatInt(id, 10)
		}
		filters["ids"] = strings.Join(parts, ",")
	}
	for key, value := range map[string]string{
		"country": f.Country, "city": f.City, "category": f.Category,
		"website_status": f.WebsiteStatus, "pipeline_status": f.PipelineStatus,
		"engagement_status": f.EngagementStatus,
	} {
		if value != "" {
			filters[key] = value
		}
	}
	if f.MinScore != 0 {
		filters["min_score"] = strconv.Itoa(f.MinScore)
	}
	for key, value := range map[string]int{
		"min_seo_score": f.MinSEOScore, "min_aeo_score": f.MinAEOScore, "min_geo_score": f.MinGEOScore,
		"min_ai_visibility_score": f.MinAIVisibility, "min_opportunity_gap_score": f.MinOpportunityGap,
		"max_seo_score": f.MaxSEOScore, "max_aeo_score": f.MaxAEOScore, "max_geo_score": f.MaxGEOScore,
		"max_ai_visibility_score": f.MaxAIVisibility,
	} {
		if value != 0 {
			filters[key] = strconv.Itoa(value)
		}
	}
	if f.HasSocial {
		filters["has_social"] = "1"
	}
	if f.Contactable {
		filters["contactable"] = "1"
	}
	rows, err := db.ListLeads(filters)
	if err != nil {
		return nil, err
	}
	return AddRelativeScores(rows, "ai_visibility_score"), nil
}

// ListLeadsRelative returns leads with the market-relative percentile.
func ListLeadsRelative(filters map[string]string) ([]*model.Lead, error) {
	rows, err := db.ListLeads(filters)
	if err != nil {
		return nil, err
	}
	return AddRelativeScores(rows, "ai_visibility_score"), nil
}

// AddRelativeScores annotates each row with a market-relative percentile.
func AddRelativeScores(rows []*model.Lead, field string) []*model.Lead {
	var population []*int
	for _, r := range rows {
		population = append(population, scoreOf(r, field))
	}
	for _, r := range rows {
		value := scoreOf(r, field)
		p := visibility.PercentileRank(value, population)
		switch field {
		case "ai_visibility_score":
			r.AIVisibilityScorePercentile = p
		}
	}
	return rows
}

func scoreOf(r *model.Lead, field string) *int {
	switch field {
	case "ai_visibility_score":
		return r.AIVisibilityScore
	case "seo_score":
		return r.SEOScore
	}
	return nil
}

// AnnotateLeadRelative adds the AI Visibility percentile for a single lead's market.
func AnnotateLeadRelative(lead *model.Lead) *model.Lead {
	if lead == nil {
		return nil
	}
	field := "ai_visibility_score"
	if lead.AIVisibilityScore == nil {
		return lead
	}
	filters := map[string]string{}
	if lead.City != "" {
		filters["city"] = lead.City
	}
	if lead.Category != "" {
		filters["category"] = lead.Category
	}
	if lead.Country != "" {
		filters["country"] = lead.Country
	}
	peers, _ := db.ListLeads(filters)
	var population []*int
	for _, p := range peers {
		population = append(population, scoreOf(p, field))
	}
	lead.AIVisibilityScorePercentile = visibility.PercentileRank(lead.AIVisibilityScore, population)
	return lead
}
