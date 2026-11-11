package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ali-ulu/lead/leadscout-go/internal/model"
	"github.com/ali-ulu/lead/leadscout-go/internal/scoring"
)

// insertColumns are the columns written on discovery upsert.
var insertColumns = []string{
	"source", "source_id", "source_refs", "name", "country", "city", "category", "latitude", "longitude",
	"website", "phone", "email", "social_url", "social_links", "messaging_ids", "website_status",
	"verification_status", "verification_notes", "performance_score", "seo_score", "aeo_score", "geo_score",
	"ai_visibility_score", "opportunity_gap_score", "visibility_reasons", "visibility_signals",
	"score_confidence", "ai_presence", "response_ms", "ttfb_ms", "accessibility_score",
	"mobile_ok", "has_cta", "has_booking", "has_https", "audit_engine", "rating", "review_count",
	"data_confidence", "contactability_score", "commercial_score", "intelligence_reasons",
	"lead_score", "score_reasons", "pipeline_status", "engagement_status", "follow_up_at", "notes",
}

// protected columns are never overwritten by a discovery upsert.
var protected = map[string]bool{
	"source": true, "source_id": true, "pipeline_status": true, "engagement_status": true,
	"follow_up_at": true, "notes": true,
	"performance_score": true, "seo_score": true, "aeo_score": true, "geo_score": true,
	"ai_visibility_score": true, "opportunity_gap_score": true,
	"accessibility_score": true, "mobile_ok": true, "has_cta": true, "has_booking": true,
	"has_https": true, "audit_engine": true, "visibility_reasons": true, "visibility_signals": true,
	"score_confidence": true, "ai_presence": true, "response_ms": true, "ttfb_ms": true,
	"verification_status": true, "verification_notes": true, "rating": true, "review_count": true,
	"social_url": true, "social_links": true, "messaging_ids": true, "source_refs": true,
	"data_confidence": true, "contactability_score": true, "commercial_score": true,
	"intelligence_reasons": true, "lead_score": true, "score_reasons": true,
}

var discoveryGapFill = map[string]bool{
	"website": true, "phone": true, "email": true, "country": true, "city": true,
	"latitude": true, "longitude": true,
}

var discoveryClearable = map[string]bool{"category": true}

// UpsertLeads inserts discovery rows, preserving measurement/CRM columns.
func UpsertLeads(rows []*model.Lead) ([]int64, error) {
	conn, err := Open()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	var ids []int64
	for _, raw := range rows {
		lead := *raw
		score, reasons := scoring.Calculate(&lead)
		lead.LeadScore = score
		lead.ScoreReasons = reasons
		if lead.PipelineStatus == "" {
			lead.PipelineStatus = "new"
		}
		if lead.EngagementStatus == "" {
			lead.EngagementStatus = "not_contacted"
		}
		if lead.WebsiteStatus == "" {
			lead.WebsiteStatus = "unknown"
		}
		if lead.VerificationStatus == "" {
			lead.VerificationStatus = "unverified"
		}
		if lead.DataConfidence == "" {
			lead.DataConfidence = "unknown"
		}
		if lead.SourceRefs == nil {
			lead.SourceRefs = map[string]string{lead.Source: lead.SourceID}
		}
		if lead.SocialLinks == nil {
			lead.SocialLinks = map[string]string{}
		}
		if lead.MessagingIDs == nil {
			lead.MessagingIDs = map[string]string{}
		}
		if lead.VerificationNotes == nil {
			lead.VerificationNotes = []string{}
		}
		if lead.IntelligenceReasons == nil {
			lead.IntelligenceReasons = []string{}
		}
		if lead.VisibilityReasons == nil {
			lead.VisibilityReasons = map[string][]string{}
		}
		if lead.VisibilitySignals == nil {
			lead.VisibilitySignals = map[string]any{}
		}
		if lead.ScoreConfidence == nil {
			lead.ScoreConfidence = map[string]string{}
		}
		if lead.AIPresence == nil {
			lead.AIPresence = map[string]any{}
		}

		values := leadValues(&lead)

		var assignments []string
		for _, col := range insertColumns {
			switch {
			case protected[col]:
				continue
			case discoveryGapFill[col]:
				assignments = append(assignments, fmt.Sprintf("%s=CASE WHEN excluded.%s IS NULL THEN leads.%s ELSE excluded.%s END", col, col, col, col))
			case discoveryClearable[col]:
				assignments = append(assignments, fmt.Sprintf("%s=excluded.%s", col, col))
			case col == "website_status":
				assignments = append(assignments,
					"website_status=CASE "+
						"WHEN leads.website_status IS NULL THEN excluded.website_status "+
						"WHEN leads.website_status IN ('dead','weak','missing') THEN leads.website_status "+
						"WHEN excluded.website_status IS NULL OR excluded.website_status='unknown' THEN leads.website_status "+
						"ELSE excluded.website_status END")
			default:
				assignments = append(assignments, fmt.Sprintf("%s=excluded.%s", col, col))
			}
		}
		assignments = append(assignments, "updated_at=CURRENT_TIMESTAMP")

		placeholders := make([]string, len(insertColumns))
		for i := range placeholders {
			placeholders[i] = "?"
		}
		sqlText := fmt.Sprintf(
			"INSERT INTO leads (%s) VALUES (%s) ON CONFLICT(source,source_id) DO UPDATE SET %s",
			strings.Join(insertColumns, ","), strings.Join(placeholders, ","), strings.Join(assignments, ","),
		)
		if _, err := conn.Exec(sqlText, values...); err != nil {
			return ids, err
		}
		var id int64
		if err := conn.QueryRow("SELECT id FROM leads WHERE source=? AND source_id=?", lead.Source, lead.SourceID).Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func leadValues(l *model.Lead) []any {
	return []any{
		l.Source, l.SourceID, jsonMarshal(l.SourceRefs), l.Name, l.Country, l.City, l.Category,
		floatOr(l.Latitude), floatOr(l.Longitude),
		strPtr(l.Website), strPtr(l.Phone), strPtr(l.Email), strPtr(l.SocialURL),
		jsonMarshal(l.SocialLinks), jsonMarshal(l.MessagingIDs), l.WebsiteStatus,
		l.VerificationStatus, jsonMarshal(l.VerificationNotes),
		intOr(l.PerformanceScore), intOr(l.SEOScore), intOr(l.AEOScore), intOr(l.GEOScore),
		intOr(l.AIVisibilityScore), intOr(l.OpportunityGapScore),
		jsonMarshal(l.VisibilityReasons), jsonMarshal(l.VisibilitySignals),
		jsonMarshal(l.ScoreConfidence), jsonMarshal(l.AIPresence),
		intOr(l.ResponseMS), intOr(l.TTFBMS), intOr(l.AccessibilityScore),
		boolOr(l.MobileOK), boolOr(l.HasCTA), boolOr(l.HasBooking), boolOr(l.HasHTTPS),
		strPtr(l.AuditEngine), floatOr(l.Rating), intOr(l.ReviewCount),
		l.DataConfidence, l.ContactabilityScore, l.CommercialScore,
		jsonMarshal(l.IntelligenceReasons), l.LeadScore, jsonMarshal(l.ScoreReasons),
		l.PipelineStatus, l.EngagementStatus, strPtr(l.FollowUpAt), strPtr(l.Notes),
	}
}

func strPtr(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

// updateAllowed are the fields update_lead accepts.
var updateAllowed = map[string]bool{
	"website": true, "phone": true, "email": true, "social_url": true, "social_links": true,
	"messaging_ids": true, "source_refs": true, "website_status": true,
	"verification_status": true, "verification_notes": true, "performance_score": true,
	"seo_score": true, "aeo_score": true, "geo_score": true, "ai_visibility_score": true,
	"opportunity_gap_score": true, "visibility_reasons": true, "visibility_signals": true,
	"score_confidence": true, "ai_presence": true, "response_ms": true, "ttfb_ms": true,
	"accessibility_score": true, "mobile_ok": true, "has_cta": true, "has_booking": true,
	"has_https": true, "audit_engine": true, "rating": true, "review_count": true,
	"data_confidence": true, "contactability_score": true, "commercial_score": true,
	"intelligence_reasons": true, "pipeline_status": true, "engagement_status": true,
	"last_contacted_at": true, "last_reply_at": true, "follow_up_at": true, "notes": true,
	"do_not_contact": true,
}

// UpdateLead applies a partial update and recomputes the lead score.
func UpdateLead(id int64, fields map[string]any) (*model.Lead, error) {
	conn, err := Open()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	var setCols []string
	var args []any
	for k, v := range fields {
		if !updateAllowed[k] {
			continue
		}
		setCols = append(setCols, k+"=?")
		args = append(args, encodeField(k, v))
	}
	if len(setCols) == 0 {
		return GetLead(id)
	}
	args = append(args, id)
	q := fmt.Sprintf("UPDATE leads SET %s,updated_at=CURRENT_TIMESTAMP WHERE id=?", strings.Join(setCols, ","))
	if _, err := conn.Exec(q, args...); err != nil {
		return nil, err
	}
	row := conn.QueryRow("SELECT "+leadColumns+" FROM leads WHERE id=?", id)
	lead, err := scanLead(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	score, reasons := scoring.Calculate(lead)
	if _, err := conn.Exec("UPDATE leads SET lead_score=?,score_reasons=?,updated_at=CURRENT_TIMESTAMP WHERE id=?",
		score, jsonMarshal(reasons), id); err != nil {
		return nil, err
	}
	return GetLead(id)
}

func encodeField(key string, v any) any {
	switch key {
	case "social_links", "messaging_ids", "source_refs", "visibility_reasons", "visibility_signals", "score_confidence", "ai_presence":
		return jsonMarshal(v)
	case "verification_notes", "intelligence_reasons":
		return jsonMarshal(v)
	case "mobile_ok", "has_cta", "has_booking", "has_https", "do_not_contact":
		if b, ok := v.(bool); ok {
			if b {
				return 1
			}
			return 0
		}
	}
	return v
}

var _ = json.Marshal
