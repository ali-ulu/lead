package db

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/ali-ulu/lead/leadscout-go/internal/model"
)

// ListLeads returns filtered leads ordered by score.
func ListLeads(filters map[string]string) ([]*model.Lead, error) {
	conn, err := Open()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	clauses := []string{"do_not_contact = 0"}
	var args []any
	for _, field := range []string{"country", "city", "category", "pipeline_status", "engagement_status"} {
		if v := strings.TrimSpace(filters[field]); v != "" {
			clauses = append(clauses, "LOWER("+field+") LIKE LOWER(?)")
			args = append(args, "%"+v+"%")
		}
	}
	if sid := strings.TrimSpace(filters["search_id"]); sid != "" {
		clauses = append(clauses, "id IN (SELECT lead_id FROM search_run_leads WHERE search_id=?)")
		args = append(args, sid)
	}
	if idsRaw := strings.TrimSpace(filters["ids"]); idsRaw != "" {
		var ids []any
		for _, p := range strings.Split(idsRaw, ",") {
			if n, err := strconv.Atoi(strings.TrimSpace(p)); err == nil {
				ids = append(ids, n)
			}
		}
		if len(ids) > 0 {
			clauses = append(clauses, "id IN ("+placeholders(len(ids))+")")
			args = append(args, ids...)
		} else {
			clauses = append(clauses, "1=0")
		}
	}
	if status := strings.TrimSpace(filters["website_status"]); status != "" {
		clauses = append(clauses, "website_status=?")
		args = append(args, status)
	}
	if filters["has_social"] == "1" {
		clauses = append(clauses, "social_links != '{}' AND social_links != ''")
	}
	if filters["contactable"] == "1" {
		clauses = append(clauses, "(phone IS NOT NULL OR email IS NOT NULL OR social_links != '{}')")
	}
	if ms := strings.TrimSpace(filters["min_score"]); ms != "" {
		if n, err := strconv.Atoi(ms); err == nil {
			clauses = append(clauses, "lead_score>=?")
			args = append(args, n)
		}
	}
	for _, field := range []string{"seo_score", "aeo_score", "geo_score", "ai_visibility_score", "opportunity_gap_score"} {
		if v := strings.TrimSpace(filters["min_"+field]); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				clauses = append(clauses, field+">=?")
				args = append(args, n)
			}
		}
	}
	for _, field := range []string{"seo_score", "aeo_score", "geo_score", "ai_visibility_score"} {
		if v := strings.TrimSpace(filters["max_"+field]); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				clauses = append(clauses, field+" IS NOT NULL AND "+field+"<=?")
				args = append(args, n)
			}
		}
	}
	q := "SELECT " + leadColumns + " FROM leads WHERE " + strings.Join(clauses, " AND ") +
		" ORDER BY lead_score DESC, commercial_score DESC, updated_at DESC, name ASC"
	rows, err := conn.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.Lead{}
	for rows.Next() {
		lead, err := scanLead(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, lead)
	}
	return out, rows.Err()
}

func placeholders(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = "?"
	}
	return strings.Join(parts, ",")
}

// RecordSearchRun stores a search run and per-lead snapshots.
func RecordSearchRun(country, city, category string, radiusKM int, leadIDs []int64,
	providerSummary map[string]any, partial bool, warnings []string) (string, error) {
	conn, err := Open()
	if err != nil {
		return "", err
	}
	defer conn.Close()
	searchID := randomHex(16)
	if providerSummary == nil {
		providerSummary = map[string]any{}
	}
	if warnings == nil {
		warnings = []string{}
	}
	if _, err := conn.Exec(
		"INSERT INTO search_runs(id,country,city,category,radius_km,result_count,provider_summary,partial,warnings) VALUES (?,?,?,?,?,?,?,?,?)",
		searchID, country, city, category, radiusKM, len(leadIDs),
		jsonMarshal(providerSummary), boolToInt(partial), jsonMarshal(warnings),
	); err != nil {
		return "", err
	}

	type snap struct {
		websiteStatus string
		leadScore     any
		aio           any
		gap           any
	}
	snaps := map[int64]snap{}
	if len(leadIDs) > 0 {
		args := make([]any, len(leadIDs))
		for i, id := range leadIDs {
			args[i] = id
		}
		rows, err := conn.Query(
			"SELECT id, website_status, lead_score, ai_visibility_score, opportunity_gap_score FROM leads WHERE id IN ("+placeholders(len(leadIDs))+")",
			args...)
		if err == nil {
			for rows.Next() {
				var id int64
				var ws any
				var ls, aio, gap any
				if err := rows.Scan(&id, &ws, &ls, &aio, &gap); err == nil {
					snaps[id] = snap{websiteStatus: anyStr(ws), leadScore: ls, aio: aio, gap: gap}
				}
			}
			rows.Close()
		}
	}
	for _, id := range leadIDs {
		s := snaps[id]
		if _, err := conn.Exec(
			"INSERT OR IGNORE INTO search_run_leads(search_id,lead_id,website_status,lead_score,ai_visibility_score,opportunity_gap_score) VALUES (?,?,?,?,?,?)",
			searchID, id, s.websiteStatus, s.leadScore, s.aio, s.gap,
		); err != nil {
			return "", err
		}
	}
	return searchID, nil
}

func anyStr(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return ""
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// SearchRun is a stored discovery run.
type SearchRun struct {
	ID              string         `json:"id"`
	Country         string         `json:"country"`
	City            string         `json:"city"`
	Category        string         `json:"category"`
	RadiusKM        int            `json:"radius_km"`
	ResultCount     int            `json:"result_count"`
	ProviderSummary map[string]any `json:"provider_summary"`
	Partial         bool           `json:"partial"`
	Warnings        []string       `json:"warnings"`
	CreatedAt       string         `json:"created_at"`
}

// GetSearchRun returns a run by id, or ErrNotFound.
func GetSearchRun(id string) (*SearchRun, error) {
	conn, err := Open()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	row := conn.QueryRow("SELECT id,country,city,category,radius_km,result_count,provider_summary,partial,warnings,created_at FROM search_runs WHERE id=?", id)
	return scanSearchRun(row)
}

func scanSearchRun(row interface{ Scan(...any) error }) (*SearchRun, error) {
	var r SearchRun
	var summary, warnings string
	var partial int
	if err := row.Scan(&r.ID, &r.Country, &r.City, &r.Category, &r.RadiusKM, &r.ResultCount, &summary, &partial, &warnings, &r.CreatedAt); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(summary), &r.ProviderSummary)
	_ = json.Unmarshal([]byte(warnings), &r.Warnings)
	if r.ProviderSummary == nil {
		r.ProviderSummary = map[string]any{}
	}
	if r.Warnings == nil {
		r.Warnings = []string{}
	}
	r.Partial = partial != 0
	return &r, nil
}

// LatestSearchRun returns the most recent run for an exact market, excluding id.
func LatestSearchRun(city, country, category, excludeID string) (*SearchRun, error) {
	conn, err := Open()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	row := conn.QueryRow(
		"SELECT id,country,city,category,radius_km,result_count,provider_summary,partial,warnings,created_at "+
			"FROM search_runs WHERE LOWER(city)=LOWER(?) AND LOWER(country)=LOWER(?) AND LOWER(category)=LOWER(?) AND id != ? "+
			"ORDER BY created_at DESC, id DESC LIMIT 1",
		city, country, category, excludeID)
	r, err := scanSearchRun(row)
	if err != nil {
		return nil, ErrNotFound
	}
	return r, nil
}

// LeadIDsForSearch returns the lead ids recorded for a run.
func LeadIDsForSearch(searchID string) ([]int64, error) {
	conn, err := Open()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	rows, err := conn.Query("SELECT lead_id FROM search_run_leads WHERE search_id=?", searchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	return ids, rows.Err()
}

// Snapshot is the lead state captured at scan time.
type Snapshot struct {
	WebsiteStatus       string `json:"website_status"`
	LeadScore           *int   `json:"lead_score"`
	AIVisibilityScore   *int   `json:"ai_visibility_score"`
	OpportunityGapScore *int   `json:"opportunity_gap_score"`
}

// SnapshotsForSearch returns snapshots keyed by lead id.
func SnapshotsForSearch(searchID string) (map[int64]Snapshot, error) {
	conn, err := Open()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	rows, err := conn.Query(
		"SELECT lead_id, website_status, lead_score, ai_visibility_score, opportunity_gap_score FROM search_run_leads WHERE search_id=?",
		searchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]Snapshot{}
	for rows.Next() {
		var id int64
		var ws any
		var ls, aio, gap *int
		if err := rows.Scan(&id, &ws, &ls, &aio, &gap); err == nil {
			out[id] = Snapshot{WebsiteStatus: anyStr(ws), LeadScore: ls, AIVisibilityScore: aio, OpportunityGapScore: gap}
		}
	}
	return out, rows.Err()
}

// LeadsByIDs returns leads for the given ids.
func LeadsByIDs(ids []int64) ([]*model.Lead, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	conn, err := Open()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := conn.Query("SELECT "+leadColumns+" FROM leads WHERE id IN ("+placeholders(len(ids))+")", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.Lead
	for rows.Next() {
		lead, err := scanLead(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, lead)
	}
	return out, rows.Err()
}

// ClearAll wipes leads and runs (destructive; permission-gated upstream).
func ClearAll() error {
	conn, err := Open()
	if err != nil {
		return err
	}
	defer conn.Close()
	for _, q := range []string{
		"DELETE FROM lead_activities", "DELETE FROM search_run_leads",
		"DELETE FROM search_runs", "DELETE FROM agent_jobs", "DELETE FROM leads",
	} {
		if _, err := conn.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

// DeleteLead hard-deletes a lead and its activities (GDPR/KVKK erasure).
func DeleteLead(id int64) (bool, error) {
	conn, err := Open()
	if err != nil {
		return false, err
	}
	defer conn.Close()
	if _, err := conn.Exec("DELETE FROM lead_activities WHERE lead_id=?", id); err != nil {
		return false, err
	}
	if _, err := conn.Exec("DELETE FROM search_run_leads WHERE lead_id=?", id); err != nil {
		return false, err
	}
	res, err := conn.Exec("DELETE FROM leads WHERE id=?", id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// PurgeStaleLeads deletes leads not updated within days; keeps engaged/DNC rows.
func PurgeStaleLeads(days int, keepEngaged bool) (int, error) {
	if days < 0 {
		return 0, &InvalidRetentionError{}
	}
	conn, err := Open()
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	cutoff := "-" + strconv.Itoa(days) + " days"
	clauses := []string{"updated_at < datetime('now', ?)"}
	if keepEngaged {
		clauses = append(clauses, "engagement_status = 'not_contacted'", "do_not_contact = 0")
	}
	where := strings.Join(clauses, " AND ")
	rows, err := conn.Query("SELECT id FROM leads WHERE "+where, cutoff)
	if err != nil {
		return 0, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		_, _ = conn.Exec("DELETE FROM lead_activities WHERE lead_id=?", id)
		_, _ = conn.Exec("DELETE FROM search_run_leads WHERE lead_id=?", id)
	}
	if len(ids) > 0 {
		args := make([]any, len(ids))
		for i, id := range ids {
			args[i] = id
		}
		if _, err := conn.Exec("DELETE FROM leads WHERE id IN ("+placeholders(len(ids))+")", args...); err != nil {
			return 0, err
		}
	}
	return len(ids), nil
}

// InvalidRetentionError signals a negative retention period.
type InvalidRetentionError struct{}

func (e *InvalidRetentionError) Error() string { return "retention days must not be negative" }
