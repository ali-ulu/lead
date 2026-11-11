// Package diff compares two stored search runs and reports what changed.
package diff

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ali-ulu/lead/leadscout-go/internal/db"
	"github.com/ali-ulu/lead/leadscout-go/internal/model"
)

// OpportunityScore is the minimum score for a high-opportunity alert.
const OpportunityScore = 70

func contactable(lead *model.Lead) bool {
	return strOr(lead.Phone) != "" || strOr(lead.Email) != "" || len(lead.SocialLinks) > 0
}

func isHighOpportunity(lead *model.Lead, minScore int) bool {
	return lead.LeadScore >= minScore && lead.WebsiteStatus == "missing" && strOr(lead.Phone) != ""
}

func brief(lead *model.Lead) map[string]any {
	if lead == nil {
		return map[string]any{}
	}
	return map[string]any{
		"id": lead.ID, "name": lead.Name, "category": lead.Category, "city": lead.City,
		"lead_score": lead.LeadScore, "website_status": lead.WebsiteStatus,
		"phone": strOr(lead.Phone), "email": strOr(lead.Email),
	}
}

func strOr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// CompareRuns compares the previous and current search runs.
func CompareRuns(previousID, currentID string, minScore int) (map[string]any, error) {
	if previousID == "" || currentID == "" {
		return nil, fmt.Errorf("Both previous and current search ids are required.")
	}
	prevIDs, err := db.LeadIDsForSearch(previousID)
	if err != nil {
		return nil, err
	}
	currIDs, err := db.LeadIDsForSearch(currentID)
	if err != nil {
		return nil, err
	}
	prevSnap, err := db.SnapshotsForSearch(previousID)
	if err != nil {
		return nil, err
	}
	currSnap, err := db.SnapshotsForSearch(currentID)
	if err != nil {
		return nil, err
	}
	prevLeads, err := db.LeadsByIDs(prevIDs)
	if err != nil {
		return nil, err
	}
	currLeads, err := db.LeadsByIDs(currIDs)
	if err != nil {
		return nil, err
	}

	prevSet := toSet(prevIDs)
	currSet := toSet(currIDs)
	prevMap := byID(prevLeads)
	currMap := byID(currLeads)

	var newIDs, goneIDs, commonIDs []int64
	for id := range currSet {
		if prevSet[id] {
			commonIDs = append(commonIDs, id)
		} else {
			newIDs = append(newIDs, id)
		}
	}
	for id := range prevSet {
		if !currSet[id] {
			goneIDs = append(goneIDs, id)
		}
	}
	sortIDs(newIDs)
	sortIDs(goneIDs)
	sortIDs(commonIDs)

	var websiteGained, websiteLost []*model.Lead
	for _, id := range commonIDs {
		before := prevSnap[id].WebsiteStatus
		after := currSnap[id].WebsiteStatus
		if (before == "missing" || before == "dead") && (after == "unknown" || after == "healthy" || after == "weak") {
			websiteGained = append(websiteGained, currMap[id])
		} else if (before == "unknown" || before == "healthy" || before == "weak") && (after == "missing" || after == "dead") {
			websiteLost = append(websiteLost, currMap[id])
		}
	}

	var alerts []*model.Lead
	for _, id := range newIDs {
		lead := currMap[id]
		if lead == nil {
			continue
		}
		snap := currSnap[id]
		merged := *lead
		if snap.LeadScore != nil {
			merged.LeadScore = *snap.LeadScore
		}
		if snap.WebsiteStatus != "" {
			merged.WebsiteStatus = snap.WebsiteStatus
		}
		if isHighOpportunity(&merged, minScore) {
			alerts = append(alerts, lead)
		}
	}

	var improved, regressed []map[string]any
	for _, id := range commonIDs {
		before := prevSnap[id].AIVisibilityScore
		after := currSnap[id].AIVisibilityScore
		if before == nil || after == nil {
			continue
		}
		delta := *after - *before
		entry := brief(currMap[id])
		entry["ai_visibility_before"] = *before
		entry["ai_visibility_after"] = *after
		entry["delta"] = delta
		if delta > 0 {
			improved = append(improved, entry)
		} else if delta < 0 {
			regressed = append(regressed, entry)
		}
	}
	sortByDelta(improved, true)
	sortByDelta(regressed, false)

	report := map[string]any{
		"previous_search_id":         previousID,
		"current_search_id":          currentID,
		"previous_count":             len(prevIDs),
		"current_count":              len(currIDs),
		"new_count":                  len(newIDs),
		"gone_count":                 len(goneIDs),
		"website_gained_count":       len(websiteGained),
		"website_lost_count":         len(websiteLost),
		"alert_count":                len(alerts),
		"visibility_improved_count":  len(improved),
		"visibility_regressed_count": len(regressed),
		"new":                        briefs(currMap, newIDs),
		"gone":                       briefs(prevMap, goneIDs),
		"website_gained":             briefList(websiteGained),
		"website_lost":               briefList(websiteLost),
		"alerts":                     briefList(alerts),
		"visibility_improved":        improved,
		"visibility_regressed":       regressed,
	}
	return report, nil
}

func toSet(ids []int64) map[int64]bool {
	out := map[int64]bool{}
	for _, id := range ids {
		out[id] = true
	}
	return out
}

func byID(leads []*model.Lead) map[int64]*model.Lead {
	out := map[int64]*model.Lead{}
	for _, l := range leads {
		out[l.ID] = l
	}
	return out
}

func sortIDs(ids []int64) {
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
}

func briefs(m map[int64]*model.Lead, ids []int64) []map[string]any {
	var out []map[string]any
	for _, id := range ids {
		if l, ok := m[id]; ok {
			out = append(out, brief(l))
		}
	}
	return out
}

func briefList(leads []*model.Lead) []map[string]any {
	var out []map[string]any
	for _, l := range leads {
		out = append(out, brief(l))
	}
	return out
}

func sortByDelta(items []map[string]any, desc bool) {
	sort.Slice(items, func(i, j int) bool {
		a := items[i]["delta"].(int)
		b := items[j]["delta"].(int)
		if desc {
			return a > b
		}
		return a < b
	})
}

// FormatReport renders a human-readable diff summary.
func FormatReport(report map[string]any) string {
	prev := short(report["previous_search_id"])
	curr := short(report["current_search_id"])
	var b strings.Builder
	fmt.Fprintf(&b, "Run diff: %s -> %s\n", prev, curr)
	fmt.Fprintf(&b, "  leads: %v -> %v\n", report["previous_count"], report["current_count"])
	fmt.Fprintf(&b, "  new: %v  gone: %v\n", report["new_count"], report["gone_count"])
	fmt.Fprintf(&b, "  websites gained: %v  lost: %v\n", report["website_gained_count"], report["website_lost_count"])
	fmt.Fprintf(&b, "  high-opportunity alerts: %v\n", report["alert_count"])
	fmt.Fprintf(&b, "  visibility improved: %v  regressed: %v\n", report["visibility_improved_count"], report["visibility_regressed_count"])

	labels := [][2]string{
		{"New businesses", "new"}, {"No longer seen", "gone"}, {"Websites gained", "website_gained"},
		{"Websites lost", "website_lost"}, {"High-opportunity alerts", "alerts"},
		{"Visibility improved", "visibility_improved"}, {"Visibility regressed", "visibility_regressed"},
	}
	for _, pair := range labels {
		items, _ := report[pair[1]].([]map[string]any)
		if len(items) == 0 {
			continue
		}
		fmt.Fprintf(&b, "  %s:\n", pair[0])
		for i, item := range items {
			if i >= 20 {
				fmt.Fprintf(&b, "    ... and %d more\n", len(items)-20)
				break
			}
			suffix := ""
			if d, ok := item["delta"].(int); ok {
				suffix = fmt.Sprintf(" (AIO %v->%v, %+d)", item["ai_visibility_before"], item["ai_visibility_after"], d)
			}
			fmt.Fprintf(&b, "    - %v (%v, %v) score=%v%s\n", item["name"], item["category"], item["city"], item["lead_score"], suffix)
		}
	}
	return b.String()
}

func short(v any) string {
	s, _ := v.(string)
	if len(s) > 8 {
		return s[:8]
	}
	return s
}
