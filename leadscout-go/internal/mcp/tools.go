package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/ali-ulu/lead/leadscout-go/internal/agent"
	"github.com/ali-ulu/lead/leadscout-go/internal/crm"
	"github.com/ali-ulu/lead/leadscout-go/internal/db"
	"github.com/ali-ulu/lead/leadscout-go/internal/exporters"
	"github.com/ali-ulu/lead/leadscout-go/internal/oauthmeta"
	"github.com/ali-ulu/lead/leadscout-go/internal/providers/osm"
	"github.com/ali-ulu/lead/leadscout-go/internal/security"
	"github.com/ali-ulu/lead/leadscout-go/internal/services"
)

func obj(props map[string]any, required ...string) map[string]any {
	if props == nil {
		props = map[string]any{}
	}
	if required == nil {
		required = []string{}
	}
	return map[string]any{"type": "object", "properties": props, "required": required}
}

func strProp(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}
func intProp(desc string) map[string]any {
	return map[string]any{"type": "integer", "description": desc}
}
func boolProp(desc string) map[string]any {
	return map[string]any{"type": "boolean", "description": desc}
}
func arrProp(desc string) map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "description": desc}
}
func strArrProp(desc string) map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": desc}
}

// arg helpers coerce the loosely typed JSON arguments MCP clients send.
func strArg(args map[string]any, key string) string {
	if v, ok := args[key]; ok && v != nil {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

func intArg(args map[string]any, key string) int {
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	case string:
		var n int
		_, _ = fmt.Sscanf(v, "%d", &n)
		return n
	}
	return 0
}

func intPtrArg(args map[string]any, key string) *int {
	if v, ok := args[key]; ok && v != nil {
		n := intArg(args, key)
		return &n
	}
	return nil
}

func boolArg(args map[string]any, key string) bool {
	if v, ok := args[key].(bool); ok {
		return v
	}
	return false
}

func idsArg(args map[string]any, key string) []int64 {
	raw, ok := args[key].([]any)
	if !ok {
		return nil
	}
	var out []int64
	for _, item := range raw {
		if f, ok := item.(float64); ok {
			out = append(out, int64(f))
		}
	}
	return out
}

func strListArg(args map[string]any, key string) []string {
	raw, ok := args[key].([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, item := range raw {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func filtersFrom(args map[string]any) services.QueryFilters {
	return services.QueryFilters{
		SearchID:          strArg(args, "search_id"),
		IDs:               idsArg(args, "ids"),
		Country:           strArg(args, "country"),
		City:              strArg(args, "city"),
		Category:          strArg(args, "category"),
		WebsiteStatus:     strArg(args, "website_status"),
		PipelineStatus:    strArg(args, "pipeline_status"),
		EngagementStatus:  strArg(args, "engagement_status"),
		MinScore:          intArg(args, "min_score"),
		MinSEOScore:       intArg(args, "min_seo_score"),
		MinAEOScore:       intArg(args, "min_aeo_score"),
		MinGEOScore:       intArg(args, "min_geo_score"),
		MinAIVisibility:   intArg(args, "min_ai_visibility_score"),
		MinOpportunityGap: intArg(args, "min_opportunity_gap_score"),
		MaxSEOScore:       intArg(args, "max_seo_score"),
		MaxAEOScore:       intArg(args, "max_aeo_score"),
		MaxGEOScore:       intArg(args, "max_geo_score"),
		MaxAIVisibility:   intArg(args, "max_ai_visibility_score"),
		HasSocial:         boolArg(args, "has_social"),
		Contactable:       boolArg(args, "contactable"),
	}
}

func categories() []string {
	out := make([]string, 0, len(osm.CategoryFilters))
	for key := range osm.CategoryFilters {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func leadscoutTools() []Tool {
	return []Tool{
		{
			Name: "capabilities", Description: "Describe LeadScout capabilities.",
			InputSchema: obj(nil),
			Handler: func(_ map[string]any) (any, error) {
				if err := db.Initialize(); err != nil {
					return nil, err
				}
				return map[string]any{
					"name": "LeadScout", "version": "7.0.0", "global_search": true,
					"providers":  []string{"osm", "overture", "web_search_optional", "google_places_optional"},
					"result_cap": nil,
					"radius_km":  map[string]any{"min": 1, "max": 100},
					"categories": categories(),
					"languages":  []string{"en", "tr", "ur", "sd", "de"},
					"exports":    []string{"csv", "xlsx"},
					"crm":        true, "website_verification": true, "contact_enrichment": true, "reputation_enrichment": true,
					"visibility_scores":         []string{"seo", "aeo", "geo", "ai_visibility", "opportunity_gap"},
					"lighthouse_when_installed": true, "meta_oauth": true, "autonomous_agent": true,
					"mcp_permissions":     map[string]any{"write_default": true, "send_default": false, "clear_default": false},
					"pipeline_stages":     sortedKeys(services.PipelineStages),
					"engagement_statuses": sortedKeys(crm.Engagement),
				}, nil
			},
		},
		{
			Name: "search_businesses", Description: "Search OSM + Overture and merge/dedupe leads.",
			InputSchema: obj(map[string]any{
				"city": strProp("City or area"), "category": strProp("Industry"),
				"country": strProp("Country"), "radius_km": intProp("Search radius in km"),
				"max_results": intProp("Result cap"), "providers": strArrProp("Discovery providers"),
			}, "city", "category"),
			Handler: func(args map[string]any) (any, error) {
				if err := db.Initialize(); err != nil {
					return nil, err
				}
				return services.DiscoverBusinesses(strArg(args, "city"), strArg(args, "category"),
					strArg(args, "country"), orDefaultInt(intArg(args, "radius_km"), 20), intPtrArg(args, "max_results"),
					strListArg(args, "providers"))
			},
		},
		{
			Name: "list_leads", Description: "List/filter leads with pagination.",
			InputSchema: obj(map[string]any{
				"search_id": strProp("Search id"), "ids": arrProp("Lead ids"),
				"country": strProp("Country"), "city": strProp("City"), "category": strProp("Category"),
				"website_status": strProp("Website status"), "pipeline_status": strProp("Pipeline status"),
				"engagement_status": strProp("Engagement status"), "min_score": intProp("Minimum lead score"),
				"min_seo_score": intProp("Minimum SEO score"), "min_aeo_score": intProp("Minimum AEO score"),
				"min_geo_score": intProp("Minimum GEO score"), "min_ai_visibility_score": intProp("Minimum AI visibility"),
				"min_opportunity_gap_score": intProp("Minimum opportunity gap"),
				"max_seo_score":             intProp("Maximum SEO score"), "max_aeo_score": intProp("Maximum AEO score"),
				"max_geo_score": intProp("Maximum GEO score"), "max_ai_visibility_score": intProp("Maximum AI visibility"),
				"has_social": boolProp("Only leads with social links"), "contactable": boolProp("Only contactable leads"),
				"offset": intProp("Pagination offset"), "page_size": intProp("Page size"),
			}),
			Handler: func(args map[string]any) (any, error) {
				if err := db.Initialize(); err != nil {
					return nil, err
				}
				rows, err := services.QueryLeads(filtersFrom(args))
				if err != nil {
					return nil, err
				}
				offset := intArg(args, "offset")
				if offset < 0 {
					offset = 0
				}
				pageSize := intArg(args, "page_size")
				if pageSize <= 0 {
					pageSize = 100
				}
				if pageSize > 500 {
					pageSize = 500
				}
				if offset > len(rows) {
					offset = len(rows)
				}
				end := offset + pageSize
				if end > len(rows) {
					end = len(rows)
				}
				page := rows[offset:end]
				var next any
				if end < len(rows) {
					next = end
				}
				return map[string]any{"total": len(rows), "offset": offset, "page_size": pageSize, "next_offset": next, "items": page}, nil
			},
		},
		{
			Name: "get_lead_detail", Description: "Return a single lead.",
			InputSchema: obj(map[string]any{"lead_id": intProp("Lead id")}, "lead_id"),
			Handler: func(args map[string]any) (any, error) {
				if err := db.Initialize(); err != nil {
					return nil, err
				}
				lead, err := db.GetLead(int64(intArg(args, "lead_id")))
				if err != nil || lead == nil {
					return nil, fmt.Errorf("Lead not found.")
				}
				return lead, nil
			},
		},
		{
			Name: "verify_business", Description: "Cross-check a lead against Overture, especially before treating 'no site found' as verified.",
			InputSchema: obj(map[string]any{"lead_id": intProp("Lead id")}, "lead_id"),
			Handler:     wrapLead(services.VerifyLead),
		},
		{
			Name: "enrich_contacts", Description: "Crawl a public business website for email, phone, booking and social links.",
			InputSchema: obj(map[string]any{"lead_id": intProp("Lead id")}, "lead_id"),
			Handler:     wrapLead(services.EnrichLead),
		},
		{
			Name: "enrich_reputation_data", Description: "Optionally enrich rating/review count and verified business fields through Google Places.",
			InputSchema: obj(map[string]any{"lead_id": intProp("Lead id")}, "lead_id"),
			Handler:     wrapLead(services.EnrichReputation),
		},
		{
			Name: "audit_website", Description: "Run deep website audit; uses Lighthouse when installed and heuristic checks otherwise.",
			InputSchema: obj(map[string]any{"lead_id": intProp("Lead id")}, "lead_id"),
			Handler: func(args map[string]any) (any, error) {
				if err := db.Initialize(); err != nil {
					return nil, err
				}
				return services.AuditLead(int64(intArg(args, "lead_id")), true)
			},
		},
		{
			Name: "measure_ai_presence", Description: "Measure whether the business appears in answer-style search results (opt-in, spends search quota).",
			InputSchema: obj(map[string]any{"lead_id": intProp("Lead id")}, "lead_id"),
			Handler: func(args map[string]any) (any, error) {
				if err := db.Initialize(); err != nil {
					return nil, err
				}
				leadID := int64(intArg(args, "lead_id"))
				_ = security.AuditAgentAction("measure_ai_presence", fmt.Sprint(leadID), "ok", nil, "")
				return services.MeasureAIPresenceForLead(leadID)
			},
		},
		{
			Name: "draft_outreach_message", Description: "Draft an outreach message for a lead.",
			InputSchema: obj(map[string]any{"lead_id": intProp("Lead id"), "lang": strProp("Language")}, "lead_id"),
			Handler: func(args map[string]any) (any, error) {
				if err := db.Initialize(); err != nil {
					return nil, err
				}
				return services.DraftOutreach(int64(intArg(args, "lead_id")), orDefaultStr(strArg(args, "lang"), "en"))
			},
		},
		{
			Name: "update_pipeline_stage", Description: "Update the sales pipeline stage.",
			InputSchema: obj(map[string]any{"lead_id": intProp("Lead id"), "status": strProp("Pipeline stage")}, "lead_id", "status"),
			Handler: func(args map[string]any) (any, error) {
				if err := db.Initialize(); err != nil {
					return nil, err
				}
				if err := security.RequirePermission("LEADSCOUT_MCP_ALLOW_WRITE", "MCP CRM writes", true); err != nil {
					return nil, err
				}
				leadID := int64(intArg(args, "lead_id"))
				result, err := services.SetPipelineStage(leadID, strArg(args, "status"))
				if err != nil {
					return nil, err
				}
				_ = security.AuditAgentAction("update_pipeline_stage", fmt.Sprint(leadID), "ok", map[string]any{"status": strArg(args, "status")}, "")
				return result, nil
			},
		},
		{
			Name: "update_engagement_status", Description: "Record whether outreach was sent, replied, rejected, bounced or had no response.",
			InputSchema: obj(map[string]any{"lead_id": intProp("Lead id"), "status": strProp("Engagement status"), "note": strProp("Note")}, "lead_id", "status"),
			Handler: func(args map[string]any) (any, error) {
				if err := db.Initialize(); err != nil {
					return nil, err
				}
				if err := security.RequirePermission("LEADSCOUT_MCP_ALLOW_WRITE", "MCP CRM writes", true); err != nil {
					return nil, err
				}
				leadID := int64(intArg(args, "lead_id"))
				lead, err := crm.SetEngagement(leadID, strArg(args, "status"), strArg(args, "note"))
				if err != nil {
					return nil, err
				}
				_ = security.AuditAgentAction("update_engagement_status", fmt.Sprint(leadID), "ok", map[string]any{"status": strArg(args, "status")}, "")
				return map[string]any{"lead": lead}, nil
			},
		},
		{
			Name: "schedule_follow_up", Description: "Schedule or clear a follow-up.",
			InputSchema: obj(map[string]any{"lead_id": intProp("Lead id"), "when": strProp("ISO timestamp"), "note": strProp("Note")}, "lead_id"),
			Handler: func(args map[string]any) (any, error) {
				if err := db.Initialize(); err != nil {
					return nil, err
				}
				if err := security.RequirePermission("LEADSCOUT_MCP_ALLOW_WRITE", "MCP CRM writes", true); err != nil {
					return nil, err
				}
				leadID := int64(intArg(args, "lead_id"))
				var when *string
				if raw, ok := args["when"]; ok && raw != nil {
					s := fmt.Sprintf("%v", raw)
					when = &s
				}
				lead, err := crm.SetFollowUp(leadID, when, strArg(args, "note"))
				if err != nil {
					return nil, err
				}
				_ = security.AuditAgentAction("schedule_follow_up", fmt.Sprint(leadID), "ok", map[string]any{"when": when}, "")
				return map[string]any{"lead": lead}, nil
			},
		},
		{
			Name: "add_lead_note", Description: "Append a note to a lead.",
			InputSchema: obj(map[string]any{"lead_id": intProp("Lead id"), "note": strProp("Note")}, "lead_id", "note"),
			Handler: func(args map[string]any) (any, error) {
				if err := db.Initialize(); err != nil {
					return nil, err
				}
				if err := security.RequirePermission("LEADSCOUT_MCP_ALLOW_WRITE", "MCP CRM writes", true); err != nil {
					return nil, err
				}
				leadID := int64(intArg(args, "lead_id"))
				activity, err := crm.AddNote(leadID, strArg(args, "note"))
				if err != nil {
					return nil, err
				}
				_ = security.AuditAgentAction("add_lead_note", fmt.Sprint(leadID), "ok", nil, "")
				return map[string]any{"activity": activity}, nil
			},
		},
		{
			Name: "lead_activity_timeline", Description: "Return the activity timeline for a lead.",
			InputSchema: obj(map[string]any{"lead_id": intProp("Lead id")}, "lead_id"),
			Handler: func(args map[string]any) (any, error) {
				if err := db.Initialize(); err != nil {
					return nil, err
				}
				items, err := crm.ListActivities(int64(intArg(args, "lead_id")))
				if err != nil {
					return nil, err
				}
				return map[string]any{"items": items}, nil
			},
		},
		{
			Name: "refresh_no_response_statuses", Description: "Mark sent/delivered leads with no reply after N days as no_response.",
			InputSchema: obj(map[string]any{"days": intProp("Days without reply")}),
			Handler: func(args map[string]any) (any, error) {
				if err := db.Initialize(); err != nil {
					return nil, err
				}
				if err := security.RequirePermission("LEADSCOUT_MCP_ALLOW_WRITE", "MCP CRM writes", true); err != nil {
					return nil, err
				}
				days := orDefaultInt(intArg(args, "days"), 7)
				result, err := crm.MarkStaleNoResponse(days)
				if err != nil {
					return nil, err
				}
				_ = security.AuditAgentAction("refresh_no_response_statuses", "", "ok", map[string]any{"days": days, "updated_count": result["updated_count"]}, "")
				return result, nil
			},
		},
		{
			Name: "meta_connections", Description: "List connected Facebook Page / Instagram Business accounts.",
			InputSchema: obj(nil),
			Handler: func(_ map[string]any) (any, error) {
				if err := db.Initialize(); err != nil {
					return nil, err
				}
				items, err := oauthmeta.ListConnections()
				if err != nil {
					return nil, err
				}
				return map[string]any{"items": items}, nil
			},
		},
		{
			Name: "messaging_eligibility_for_lead", Description: "Check whether LeadScout has an eligible Meta recipient id for this lead.",
			InputSchema: obj(map[string]any{"lead_id": intProp("Lead id"), "provider": strProp("facebook or instagram")}, "lead_id"),
			Handler: func(args map[string]any) (any, error) {
				if err := db.Initialize(); err != nil {
					return nil, err
				}
				return oauthmeta.MessagingEligibility(int64(intArg(args, "lead_id")), orDefaultStr(strArg(args, "provider"), "instagram"))
			},
		},
		{
			Name: "link_messaging_recipient", Description: "Associate a Meta Page/Instagram scoped recipient id with a lead after a real conversation exists.",
			InputSchema: obj(map[string]any{"lead_id": intProp("Lead id"), "provider": strProp("Provider"), "recipient_id": strProp("Recipient id")}, "lead_id", "provider", "recipient_id"),
			Handler: func(args map[string]any) (any, error) {
				if err := db.Initialize(); err != nil {
					return nil, err
				}
				if err := security.RequirePermission("LEADSCOUT_MCP_ALLOW_WRITE", "MCP CRM writes", true); err != nil {
					return nil, err
				}
				leadID := int64(intArg(args, "lead_id"))
				result, err := oauthmeta.LinkRecipient(leadID, strArg(args, "provider"), strArg(args, "recipient_id"))
				if err != nil {
					return nil, err
				}
				_ = security.AuditAgentAction("link_messaging_recipient", fmt.Sprint(leadID), "ok", map[string]any{"provider": strArg(args, "provider")}, "")
				return result, nil
			},
		},
		{
			Name: "send_social_message", Description: "Send through a connected Meta account when an eligible recipient/conversation id exists.",
			InputSchema: obj(map[string]any{"lead_id": intProp("Lead id"), "provider": strProp("Provider"), "text": strProp("Message text"), "recipient_id": strProp("Recipient id"), "connection_id": intProp("Connection id")}, "lead_id", "provider", "text"),
			Handler: func(args map[string]any) (any, error) {
				if err := db.Initialize(); err != nil {
					return nil, err
				}
				if err := security.RequirePermission("LEADSCOUT_MCP_ALLOW_SEND", "MCP message sending", false); err != nil {
					return nil, err
				}
				leadID := int64(intArg(args, "lead_id"))
				var recipient *string
				if r := strArg(args, "recipient_id"); r != "" {
					recipient = &r
				}
				var connectionID *int64
				if c := intPtrArg(args, "connection_id"); c != nil {
					v := int64(*c)
					connectionID = &v
				}
				result, err := oauthmeta.SendMessage(leadID, strArg(args, "provider"), recipient, strArg(args, "text"), connectionID)
				if err != nil {
					return nil, err
				}
				_ = security.AuditAgentAction("send_social_message", fmt.Sprint(leadID), "ok", map[string]any{"provider": strArg(args, "provider"), "connection_id": connectionID}, "")
				return result, nil
			},
		},
		{
			Name: "run_sales_agent", Description: "Run search -> verify -> audit/enrich -> draft -> Excel; optional sending is separately gated.",
			InputSchema: obj(map[string]any{
				"city": strProp("City"), "category": strProp("Industry"), "country": strProp("Country"),
				"radius_km": intProp("Radius km"), "top_n": intProp("Top N"), "min_score": intProp("Minimum score"),
				"min_opportunity_gap": intProp("Minimum opportunity gap"),
				"max_seo_score":       intProp("Max SEO"), "max_aeo_score": intProp("Max AEO"),
				"max_geo_score": intProp("Max GEO"), "max_ai_visibility_score": intProp("Max AI visibility"),
				"lang": strProp("Language"), "verify_missing": boolProp("Verify missing websites"),
				"audit_websites": boolProp("Audit websites"), "reputation_enrichment": boolProp("Enrich reputation"),
				"send": boolProp("Send messages"), "send_provider": strProp("Send provider"), "connection_id": intProp("Connection id"),
			}, "city", "category"),
			Handler: func(args map[string]any) (any, error) {
				if err := db.Initialize(); err != nil {
					return nil, err
				}
				if err := security.RequirePermission("LEADSCOUT_MCP_ALLOW_WRITE", "MCP agent writes", true); err != nil {
					return nil, err
				}
				send := boolArg(args, "send")
				if send {
					if err := security.RequirePermission("LEADSCOUT_MCP_ALLOW_SEND", "MCP message sending", false); err != nil {
						return nil, err
					}
				}
				req := agent.Request{
					City: strArg(args, "city"), Category: strArg(args, "category"), Country: strArg(args, "country"),
					RadiusKM: orDefaultInt(intArg(args, "radius_km"), 20), TopN: orDefaultInt(intArg(args, "top_n"), 50),
					MinScore: orDefaultInt(intArg(args, "min_score"), 40), MinOpportunityGap: intArg(args, "min_opportunity_gap"),
					MaxSEOScore: intArg(args, "max_seo_score"), MaxAEOScore: intArg(args, "max_aeo_score"),
					MaxGEOScore: intArg(args, "max_geo_score"), MaxAIVisibility: intArg(args, "max_ai_visibility_score"),
					Lang: orDefaultStr(strArg(args, "lang"), "en"), VerifyMissing: boolArgDefault(args, "verify_missing", true),
					AuditWebsites: boolArgDefault(args, "audit_websites", true), ReputationEnrichment: boolArg(args, "reputation_enrichment"),
					Send: send, SendProvider: orDefaultStr(strArg(args, "send_provider"), "instagram"), ConnectionID: intPtrArg(args, "connection_id"),
				}
				result, err := agent.RunSalesJob(req)
				if err != nil {
					return nil, err
				}
				_ = security.AuditAgentAction("run_sales_agent", fmt.Sprint(result["job_id"]), "ok",
					map[string]any{"city": req.City, "country": req.Country, "category": req.Category, "send": send}, "")
				return result, nil
			},
		},
		{
			Name: "get_agent_job", Description: "Fetch a stored agent job.",
			InputSchema: obj(map[string]any{"job_id": strProp("Job id")}, "job_id"),
			Handler: func(args map[string]any) (any, error) {
				if err := db.Initialize(); err != nil {
					return nil, err
				}
				item, err := agent.GetJob(strArg(args, "job_id"))
				if err != nil {
					return nil, err
				}
				if item == nil {
					return nil, fmt.Errorf("Job not found.")
				}
				return item, nil
			},
		},
		{
			Name: "do_not_contact", Description: "Flag a lead as do-not-contact.",
			InputSchema: obj(map[string]any{"lead_id": intProp("Lead id")}, "lead_id"),
			Handler: func(args map[string]any) (any, error) {
				if err := db.Initialize(); err != nil {
					return nil, err
				}
				if err := security.RequirePermission("LEADSCOUT_MCP_ALLOW_WRITE", "MCP CRM writes", true); err != nil {
					return nil, err
				}
				leadID := int64(intArg(args, "lead_id"))
				result, err := services.MarkDoNotContact(leadID)
				if err != nil {
					return nil, err
				}
				_ = security.AuditAgentAction("do_not_contact", fmt.Sprint(leadID), "ok", nil, "")
				return result, nil
			},
		},
		{
			Name: "export_leads", Description: "Export filtered leads to CSV or XLSX.",
			InputSchema: obj(map[string]any{
				"format": strProp("csv or xlsx"), "search_id": strProp("Search id"), "ids": arrProp("Lead ids"),
				"country": strProp("Country"), "city": strProp("City"), "category": strProp("Category"),
				"website_status": strProp("Website status"), "pipeline_status": strProp("Pipeline status"),
				"engagement_status": strProp("Engagement status"), "min_score": intProp("Minimum score"),
				"min_opportunity_gap_score": intProp("Minimum opportunity gap"),
				"max_seo_score":             intProp("Max SEO"), "max_aeo_score": intProp("Max AEO"),
				"max_geo_score": intProp("Max GEO"), "max_ai_visibility_score": intProp("Max AI visibility"),
				"has_social": boolProp("Has social"), "contactable": boolProp("Contactable"),
			}),
			Handler: func(args map[string]any) (any, error) {
				if err := db.Initialize(); err != nil {
					return nil, err
				}
				rows, err := services.QueryLeads(filtersFrom(args))
				if err != nil {
					return nil, err
				}
				format := orDefaultStr(strArg(args, "format"), "xlsx")
				if format != "csv" && format != "xlsx" {
					return nil, fmt.Errorf("format must be csv or xlsx")
				}
				outDir := filepath.Join(exportRoot(), "exports")
				if err := os.MkdirAll(outDir, 0o755); err != nil {
					return nil, err
				}
				path := filepath.Join(outDir, "leadscout-leads."+format)
				var payload []byte
				if format == "xlsx" {
					payload = exporters.XLSX(rows)
				} else {
					payload, err = exporters.CSV(rows)
					if err != nil {
						return nil, err
					}
				}
				if err := os.WriteFile(path, payload, 0o644); err != nil {
					return nil, err
				}
				return map[string]any{"format": format, "count": len(rows), "path": path}, nil
			},
		},
		{
			Name: "clear_local_data", Description: "Clear cached leads and CRM history. Explicit confirm=true and permission are required.",
			InputSchema: obj(map[string]any{"confirm": boolProp("Confirm destructive clear")}),
			Handler: func(args map[string]any) (any, error) {
				if err := db.Initialize(); err != nil {
					return nil, err
				}
				if !boolArg(args, "confirm") {
					return nil, fmt.Errorf("confirm=true is required.")
				}
				if err := security.RequirePermission("LEADSCOUT_MCP_ALLOW_CLEAR", "MCP destructive clear", false); err != nil {
					return nil, err
				}
				_ = security.AuditAgentAction("clear_local_data", "", "requested", nil, "")
				if err := db.ClearAll(); err != nil {
					return nil, err
				}
				return map[string]any{"ok": true}, nil
			},
		},
		{
			Name: "agent_audit_log", Description: "Read recent MCP write/send/clear audit events.",
			InputSchema: obj(map[string]any{"limit": intProp("Max entries")}),
			Handler: func(args map[string]any) (any, error) {
				if err := db.Initialize(); err != nil {
					return nil, err
				}
				items, err := security.ListAgentAudit(orDefaultInt(intArg(args, "limit"), 200))
				if err != nil {
					return nil, err
				}
				return map[string]any{"items": items}, nil
			},
		},
	}
}

func wrapLead(fn func(int64) (map[string]any, error)) func(map[string]any) (any, error) {
	return func(args map[string]any) (any, error) {
		if err := db.Initialize(); err != nil {
			return nil, err
		}
		return fn(int64(intArg(args, "lead_id")))
	}
}

func orDefaultInt(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}

func orDefaultStr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func boolArgDefault(args map[string]any, key string, def bool) bool {
	if _, ok := args[key]; !ok || args[key] == nil {
		return def
	}
	return boolArg(args, key)
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func exportRoot() string {
	if env := os.Getenv("LEADSCOUT_ROOT"); env != "" {
		return env
	}
	wd, _ := os.Getwd()
	return wd
}
