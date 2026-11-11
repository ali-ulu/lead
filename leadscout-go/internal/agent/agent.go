// Package agent runs the autonomous sales workflow: search -> verify ->
// audit/enrich -> draft -> Excel.
package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ali-ulu/lead/leadscout-go/internal/crm"
	"github.com/ali-ulu/lead/leadscout-go/internal/db"
	"github.com/ali-ulu/lead/leadscout-go/internal/exporters"
	"github.com/ali-ulu/lead/leadscout-go/internal/model"
	"github.com/ali-ulu/lead/leadscout-go/internal/oauthmeta"
	"github.com/ali-ulu/lead/leadscout-go/internal/services"
)

// Request configures a sales job.
type Request struct {
	City                 string `json:"city"`
	Category             string `json:"category"`
	Country              string `json:"country"`
	RadiusKM             int    `json:"radius_km"`
	TopN                 int    `json:"top_n"`
	MinScore             int    `json:"min_score"`
	MinOpportunityGap    int    `json:"min_opportunity_gap"`
	MaxSEOScore          int    `json:"max_seo_score"`
	MaxAEOScore          int    `json:"max_aeo_score"`
	MaxGEOScore          int    `json:"max_geo_score"`
	MaxAIVisibility      int    `json:"max_ai_visibility_score"`
	Lang                 string `json:"lang"`
	VerifyMissing        bool   `json:"verify_missing"`
	AuditWebsites        bool   `json:"audit_websites"`
	ReputationEnrichment bool   `json:"reputation_enrichment"`
	Send                 bool   `json:"send"`
	SendProvider         string `json:"send_provider"`
	ConnectionID         *int   `json:"connection_id"`
}

func jobWriteState(jobID, status string, request map[string]any, result map[string]any, errMsg string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	var resultJSON any
	if result != nil {
		b, _ := json.Marshal(result)
		resultJSON = string(b)
	}
	var errVal any
	if errMsg != "" {
		errVal = errMsg
	}
	if request != nil {
		reqJSON, _ := json.Marshal(request)
		_, err = conn.Exec(
			"INSERT INTO agent_jobs(id,status,request_json,result_json,error) VALUES (?,?,?,?,?) "+
				"ON CONFLICT(id) DO UPDATE SET status=excluded.status,request_json=excluded.request_json,"+
				"result_json=excluded.result_json,error=excluded.error,updated_at=CURRENT_TIMESTAMP",
			jobID, status, string(reqJSON), resultJSON, errVal,
		)
		return err
	}
	_, err = conn.Exec(
		"UPDATE agent_jobs SET status=?,result_json=?,error=?,updated_at=CURRENT_TIMESTAMP WHERE id=?",
		status, resultJSON, errVal, jobID,
	)
	return err
}

// GetJob returns a stored job.
func GetJob(jobID string) (map[string]any, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var id, status, requestJSON, resultJSON, errMsg *string
	var createdAt, updatedAt string
	row := conn.QueryRow("SELECT id,status,request_json,result_json,error,created_at,updated_at FROM agent_jobs WHERE id=?", jobID)
	if err := row.Scan(&id, &status, &requestJSON, &resultJSON, &errMsg, &createdAt, &updatedAt); err != nil {
		return nil, nil
	}
	out := map[string]any{
		"id": deref(id), "status": deref(status), "created_at": createdAt, "updated_at": updatedAt,
	}
	req := map[string]any{}
	if requestJSON != nil {
		_ = json.Unmarshal([]byte(*requestJSON), &req)
	}
	res := map[string]any{}
	if resultJSON != nil {
		_ = json.Unmarshal([]byte(*resultJSON), &res)
	}
	out["request"] = req
	out["result"] = res
	if errMsg != nil {
		out["error"] = *errMsg
	}
	return out, nil
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// RunSalesJob executes the autonomous sales workflow.
func RunSalesJob(req Request) (map[string]any, error) {
	if req.TopN == 0 {
		req.TopN = 50
	}
	if req.MinScore == 0 {
		req.MinScore = 40
	}
	if req.RadiusKM == 0 {
		req.RadiusKM = 20
	}
	if req.Lang == "" {
		req.Lang = "en"
	}
	if req.SendProvider == "" {
		req.SendProvider = "instagram"
	}
	request := map[string]any{
		"city": req.City, "category": req.Category, "country": req.Country, "radius_km": req.RadiusKM,
		"top_n": req.TopN, "min_score": req.MinScore, "min_opportunity_gap": req.MinOpportunityGap,
		"max_seo_score": req.MaxSEOScore, "max_aeo_score": req.MaxAEOScore, "max_geo_score": req.MaxGEOScore,
		"max_ai_visibility_score": req.MaxAIVisibility, "lang": req.Lang, "verify_missing": req.VerifyMissing,
		"audit_websites": req.AuditWebsites, "reputation_enrichment": req.ReputationEnrichment,
		"send": req.Send, "send_provider": req.SendProvider, "connection_id": req.ConnectionID,
	}
	jobID := randomHex(16)
	if err := jobWriteState(jobID, "running", request, nil, ""); err != nil {
		return nil, err
	}

	search, err := services.DiscoverBusinesses(req.City, req.Category, req.Country, req.RadiusKM, nil, nil)
	if err != nil {
		_ = jobWriteState(jobID, "failed", nil, nil, err.Error())
		return nil, err
	}

	rows, err := services.QueryLeads(services.QueryFilters{SearchID: search.SearchID, MinScore: req.MinScore})
	if err != nil {
		_ = jobWriteState(jobID, "failed", nil, nil, err.Error())
		return nil, err
	}
	if req.TopN > 0 && len(rows) > req.TopN {
		rows = rows[:req.TopN]
	}

	var processed []map[string]any
	sent := 0
	for _, lead := range rows {
		leadID := lead.ID
		if req.VerifyMissing && lead.WebsiteStatus == "missing" {
			_, _ = services.VerifyLead(leadID)
		}
		current, _ := db.GetLead(leadID)
		if current == nil {
			current = lead
		}
		if req.AuditWebsites && current.Website != nil && *current.Website != "" {
			_, _ = services.AuditLead(leadID, true)
		}
		current, _ = db.GetLead(leadID)
		if current == nil {
			current = lead
		}
		var reputationResult any
		if req.ReputationEnrichment {
			if res, err := services.EnrichReputation(leadID); err == nil {
				reputationResult = res["reputation"]
			} else {
				reputationResult = map[string]any{"matched": false, "error": err.Error()}
			}
		}
		current, _ = db.GetLead(leadID)
		if current == nil {
			current = lead
		}

		if req.MinOpportunityGap > 0 && (current.OpportunityGapScore == nil || *current.OpportunityGapScore < req.MinOpportunityGap) {
			processed = append(processed, map[string]any{
				"lead_id": leadID, "name": current.Name, "skipped": true,
				"reason": "opportunity_gap_below_threshold", "opportunity_gap_score": current.OpportunityGapScore,
			})
			continue
		}

		maxVisibility := map[string]int{
			"seo_score": req.MaxSEOScore, "aeo_score": req.MaxAEOScore,
			"geo_score": req.MaxGEOScore, "ai_visibility_score": req.MaxAIVisibility,
		}
		exceededField := ""
		var exceededValue int
		missingField := ""
		for field, threshold := range maxVisibility {
			value := visibilityValue(current, field)
			if threshold == 0 {
				continue
			}
			if value == nil {
				if missingField == "" {
					missingField = field
				}
				continue
			}
			if *value > threshold && exceededField == "" {
				exceededField = field
				exceededValue = *value
			}
		}
		if exceededField != "" || missingField != "" {
			entry := map[string]any{"lead_id": leadID, "name": current.Name, "skipped": true}
			if exceededField != "" {
				entry["reason"] = "visibility_above_target"
				entry["field"] = exceededField
				entry["value"] = exceededValue
				entry["threshold"] = maxVisibility[exceededField]
			} else {
				entry["reason"] = "visibility_not_scored"
				entry["field"] = missingField
				entry["value"] = nil
				entry["threshold"] = maxVisibility[missingField]
			}
			processed = append(processed, entry)
			continue
		}

		draft, err := services.DraftOutreach(leadID, req.Lang)
		if err != nil {
			continue
		}
		action := map[string]any{
			"lead_id": leadID, "name": current.Name, "score": current.LeadScore,
			"commercial_score": current.CommercialScore, "seo_score": current.SEOScore,
			"aeo_score": current.AEOScore, "geo_score": current.GEOScore,
			"ai_visibility_score": current.AIVisibilityScore, "opportunity_gap_score": current.OpportunityGapScore,
			"rating": current.Rating, "review_count": current.ReviewCount, "reputation": reputationResult,
			"engagement_status": current.EngagementStatus, "message": draft["message"], "sent": false,
		}
		if req.Send {
			if os.Getenv("LEADSCOUT_AGENT_SEND") != "1" {
				action["send_error"] = "LEADSCOUT_AGENT_SEND=1 is required for autonomous sending."
			} else {
				var connectionID *int64
				if req.ConnectionID != nil {
					cid := int64(*req.ConnectionID)
					connectionID = &cid
				}
				message, _ := draft["message"].(string)
				sendResult, err := oauthmeta.SendMessage(leadID, req.SendProvider, nil, message, connectionID)
				if err != nil {
					action["send_error"] = err.Error()
				} else {
					action["sent"] = true
					action["send_result"] = sendResult
					sent++
				}
			}
		}
		processed = append(processed, action)
		_, _ = crm.AddActivity(leadID, "agent", "", "processed", "", "", "", map[string]any{"job_id": jobID})
	}

	exportRows, _ := services.QueryLeads(services.QueryFilters{
		SearchID: search.SearchID, MinScore: req.MinScore, MinOpportunityGap: req.MinOpportunityGap,
		MaxSEOScore: req.MaxSEOScore, MaxAEOScore: req.MaxAEOScore, MaxGEOScore: req.MaxGEOScore,
		MaxAIVisibility: req.MaxAIVisibility,
	})
	outDir := filepath.Join(rootDir(), "exports")
	_ = os.MkdirAll(outDir, 0o755)
	exportPath := filepath.Join(outDir, fmt.Sprintf("leadscout-agent-%s.xlsx", jobID[:8]))
	_ = os.WriteFile(exportPath, exporters.XLSX(exportRows), 0o644)

	result := map[string]any{
		"job_id": jobID, "search": search, "candidate_count": len(rows), "processed": processed,
		"sent_count": sent, "export_path": exportPath, "partial": search.Partial,
		"warnings": search.Warnings,
	}
	_ = jobWriteState(jobID, "completed", nil, result, "")
	return result, nil
}

func visibilityValue(lead *model.Lead, field string) *int {
	switch field {
	case "seo_score":
		return lead.SEOScore
	case "aeo_score":
		return lead.AEOScore
	case "geo_score":
		return lead.GEOScore
	case "ai_visibility_score":
		return lead.AIVisibilityScore
	}
	return nil
}

func rootDir() string {
	if env := os.Getenv("LEADSCOUT_ROOT"); env != "" {
		return env
	}
	wd, _ := os.Getwd()
	return wd
}

func randomHex(n int) string {
	const hexdigits = "0123456789abcdef"
	b := make([]byte, n)
	t := time.Now().UnixNano()
	for i := range b {
		t = t*6364136223846793005 + 1442695040888963407
		b[i] = hexdigits[(t>>33)&0xf]
	}
	return string(b)
}

var _ = errors.New
