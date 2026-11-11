// Package model holds the shared lead record used across discovery, audit,
// scoring, storage and export.
package model

// Lead is the shared record. JSON tags match the Python REST/MCP field names so
// the existing UI and agent clients keep working unchanged.
type Lead struct {
	ID         int64             `json:"id"`
	Source     string            `json:"source"`
	SourceID   string            `json:"source_id"`
	SourceRefs map[string]string `json:"source_refs"`

	Name      string   `json:"name"`
	Country   string   `json:"country"`
	City      string   `json:"city"`
	Category  string   `json:"category"`
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`

	Website      *string           `json:"website"`
	Phone        *string           `json:"phone"`
	Email        *string           `json:"email"`
	SocialURL    *string           `json:"social_url"`
	SocialLinks  map[string]string `json:"social_links"`
	MessagingIDs map[string]string `json:"messaging_ids"`

	WebsiteStatus      string   `json:"website_status"`
	VerificationStatus string   `json:"verification_status"`
	VerificationNotes  []string `json:"verification_notes"`

	PerformanceScore    *int                `json:"performance_score"`
	SEOScore            *int                `json:"seo_score"`
	AEOScore            *int                `json:"aeo_score"`
	GEOScore            *int                `json:"geo_score"`
	AIVisibilityScore   *int                `json:"ai_visibility_score"`
	OpportunityGapScore *int                `json:"opportunity_gap_score"`
	VisibilityReasons   map[string][]string `json:"visibility_reasons"`
	VisibilitySignals   map[string]any      `json:"visibility_signals"`
	ScoreConfidence     map[string]string   `json:"score_confidence"`
	AIPresence          map[string]any      `json:"ai_presence"`

	ResponseMS         *int     `json:"response_ms"`
	TTFBMS             *int     `json:"ttfb_ms"`
	AccessibilityScore *int     `json:"accessibility_score"`
	MobileOK           *bool    `json:"mobile_ok"`
	HasCTA             *bool    `json:"has_cta"`
	HasBooking         *bool    `json:"has_booking"`
	HasHTTPS           *bool    `json:"has_https"`
	AuditEngine        *string  `json:"audit_engine"`
	Rating             *float64 `json:"rating"`
	ReviewCount        *int     `json:"review_count"`

	DataConfidence      string   `json:"data_confidence"`
	ContactabilityScore int      `json:"contactability_score"`
	CommercialScore     int      `json:"commercial_score"`
	IntelligenceReasons []string `json:"intelligence_reasons"`

	LeadScore    int      `json:"lead_score"`
	ScoreReasons []string `json:"score_reasons"`

	PipelineStatus   string  `json:"pipeline_status"`
	EngagementStatus string  `json:"engagement_status"`
	LastContactedAt  *string `json:"last_contacted_at"`
	LastReplyAt      *string `json:"last_reply_at"`
	FollowUpAt       *string `json:"follow_up_at"`
	Notes            *string `json:"notes"`
	DoNotContact     bool    `json:"do_not_contact"`

	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`

	// Derived, not stored: market-relative AI Visibility percentile.
	AIVisibilityScorePercentile *int `json:"ai_visibility_score_percentile,omitempty"`
}

// Area is a geocoded search area.
type Area struct {
	DisplayName string  `json:"display_name"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	BBox        BBox    `json:"bbox"`
	Country     string  `json:"country"`
	CountryCode string  `json:"country_code"`
	City        string  `json:"city"`
}

// BBox is a geographic bounding box.
type BBox struct {
	South float64 `json:"south"`
	North float64 `json:"north"`
	West  float64 `json:"west"`
	East  float64 `json:"east"`
}

// Str returns a pointer to s (helper for optional string fields).
func Str(s string) *string { return &s }

// Int returns a pointer to i.
func Int(i int) *int { return &i }

// Float returns a pointer to f.
func Float(f float64) *float64 { return &f }

// Bool returns a pointer to b.
func Bool(b bool) *bool { return &b }
