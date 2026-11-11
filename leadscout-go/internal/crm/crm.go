// Package crm manages the sales pipeline, engagement state and activity timeline.
package crm

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ali-ulu/lead/leadscout-go/internal/db"
)

// Engagement statuses.
var Engagement = map[string]bool{
	"not_contacted": true, "drafted": true, "sent": true, "delivered": true,
	"replied": true, "rejected": true, "bounced": true, "no_response": true,
}

// ActivityKinds are the valid activity types.
var ActivityKinds = map[string]bool{
	"note": true, "message": true, "reply": true, "status": true, "follow_up": true,
	"oauth": true, "agent": true, "audit": true, "enrichment": true, "verification": true,
	"reputation": true, "ai_presence": true,
}

// Activity is a stored timeline entry.
type Activity struct {
	ID         int64          `json:"id"`
	LeadID     int64          `json:"lead_id"`
	Kind       string         `json:"kind"`
	Channel    *string        `json:"channel"`
	Status     *string        `json:"status"`
	Direction  *string        `json:"direction"`
	Body       *string        `json:"body"`
	ExternalID *string        `json:"external_id"`
	Metadata   map[string]any `json:"metadata"`
	CreatedAt  string         `json:"created_at"`
}

func utcnow() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// AddActivity records a timeline entry and updates related lead fields.
func AddActivity(leadID int64, kind, channel, status, direction, body, externalID string, metadata map[string]any) (Activity, error) {
	if !ActivityKinds[kind] {
		return Activity{}, fmt.Errorf("Invalid activity kind: %s", kind)
	}
	if _, err := db.GetLead(leadID); err != nil {
		return Activity{}, errors.New("Lead not found.")
	}
	conn, err := db.Conn()
	if err != nil {
		return Activity{}, err
	}
	defer conn.Close()
	if metadata == nil {
		metadata = map[string]any{}
	}
	metaJSON, _ := json.Marshal(metadata)
	res, err := conn.Exec(
		"INSERT INTO lead_activities(lead_id,kind,channel,status,direction,body,external_id,metadata) VALUES (?,?,?,?,?,?,?,?)",
		leadID, kind, nullStr(channel), nullStr(status), nullStr(direction), nullStr(body), nullStr(externalID), string(metaJSON),
	)
	if err != nil {
		return Activity{}, err
	}
	activityID, _ := res.LastInsertId()

	if kind == "message" && direction == "out" {
		fields := map[string]any{"last_contacted_at": utcnow()}
		if Engagement[status] {
			fields["engagement_status"] = status
		}
		_, _ = db.UpdateLead(leadID, fields)
	}
	if kind == "reply" || (kind == "message" && direction == "in") {
		_, _ = db.UpdateLead(leadID, map[string]any{
			"last_reply_at": utcnow(), "engagement_status": "replied", "pipeline_status": "replied",
		})
	}
	return getActivity(conn, activityID)
}

func getActivity(conn *sql.DB, id int64) (Activity, error) {
	row := conn.QueryRow("SELECT id,lead_id,kind,channel,status,direction,body,external_id,metadata,created_at FROM lead_activities WHERE id=?", id)
	return scanActivity(row)
}

func scanActivity(row interface{ Scan(...any) error }) (Activity, error) {
	var a Activity
	var channel, status, direction, body, externalID, metadata sql.NullString
	if err := row.Scan(&a.ID, &a.LeadID, &a.Kind, &channel, &status, &direction, &body, &externalID, &metadata, &a.CreatedAt); err != nil {
		return Activity{}, err
	}
	a.Channel = nullStrPtr(channel)
	a.Status = nullStrPtr(status)
	a.Direction = nullStrPtr(direction)
	a.Body = nullStrPtr(body)
	a.ExternalID = nullStrPtr(externalID)
	a.Metadata = map[string]any{}
	if metadata.Valid {
		_ = json.Unmarshal([]byte(metadata.String), &a.Metadata)
	}
	return a, nil
}

func nullStrPtr(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	v := ns.String
	return &v
}

// ListActivities returns a lead's timeline, newest first.
func ListActivities(leadID int64) ([]Activity, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	rows, err := conn.Query("SELECT id,lead_id,kind,channel,status,direction,body,external_id,metadata,created_at FROM lead_activities WHERE lead_id=? ORDER BY created_at DESC,id DESC", leadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Activity
	for rows.Next() {
		a, err := scanActivity(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// SetEngagement updates the engagement status.
func SetEngagement(leadID int64, status, note string) (any, error) {
	if !Engagement[status] {
		return nil, fmt.Errorf("Invalid engagement status: %s", status)
	}
	fields := map[string]any{"engagement_status": status}
	if status == "replied" {
		fields["last_reply_at"] = utcnow()
	}
	if status == "sent" || status == "delivered" {
		fields["last_contacted_at"] = utcnow()
	}
	lead, err := db.UpdateLead(leadID, fields)
	if err != nil {
		return nil, errors.New("Lead not found.")
	}
	if _, err := AddActivity(leadID, "status", "", status, "", note, "", nil); err != nil {
		return nil, err
	}
	return lead, nil
}

// SetFollowUp sets or clears the follow-up date.
func SetFollowUp(leadID int64, when *string, note string) (any, error) {
	lead, err := db.UpdateLead(leadID, map[string]any{"follow_up_at": when})
	if err != nil {
		return nil, errors.New("Lead not found.")
	}
	status := "cleared"
	if when != nil {
		status = "scheduled"
	}
	meta := map[string]any{"follow_up_at": when}
	if _, err := AddActivity(leadID, "follow_up", "", status, "", note, "", meta); err != nil {
		return nil, err
	}
	return lead, nil
}

// AddNote appends a note and records an activity.
func AddNote(leadID int64, note string) (Activity, error) {
	if note == "" {
		return Activity{}, errors.New("Note cannot be empty.")
	}
	lead, err := db.GetLead(leadID)
	if err != nil {
		return Activity{}, errors.New("Lead not found.")
	}
	existing := ""
	if lead.Notes != nil {
		existing = *lead.Notes
	}
	combined := existing + "\n" + note
	if existing == "" {
		combined = note
	}
	if _, err := db.UpdateLead(leadID, map[string]any{"notes": combined}); err != nil {
		return Activity{}, err
	}
	return AddActivity(leadID, "note", "", "", "", note, "", nil)
}

// UpdateDeliveryStatus applies a delivery/reply event to a message activity.
func UpdateDeliveryStatus(externalID, status string) (bool, error) {
	if externalID == "" {
		return false, nil
	}
	conn, err := db.Conn()
	if err != nil {
		return false, err
	}
	defer conn.Close()
	var activityID, leadID int64
	err = conn.QueryRow("SELECT id,lead_id FROM lead_activities WHERE external_id=? ORDER BY id DESC LIMIT 1", externalID).Scan(&activityID, &leadID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if _, err := conn.Exec("UPDATE lead_activities SET status=? WHERE id=?", status, activityID); err != nil {
		return false, err
	}
	if status == "delivered" || status == "replied" {
		fields := map[string]any{"engagement_status": status}
		if status == "delivered" {
			fields["last_contacted_at"] = utcnow()
		} else {
			fields["last_reply_at"] = utcnow()
		}
		_, _ = db.UpdateLead(leadID, fields)
	}
	return true, nil
}

// MarkStaleNoResponse flags sent leads with no reply after days.
func MarkStaleNoResponse(days int) (map[string]any, error) {
	if days < 1 {
		days = 1
	}
	if days > 365 {
		days = 365
	}
	cutoff := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour).Format(time.RFC3339)
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	rows, err := conn.Query(
		`SELECT id FROM leads WHERE do_not_contact=0 AND engagement_status IN ('sent','delivered')
		 AND last_contacted_at IS NOT NULL AND last_contacted_at < ? AND last_reply_at IS NULL`, cutoff)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	var updated []int64
	for _, leadID := range ids {
		if _, err := db.UpdateLead(leadID, map[string]any{"engagement_status": "no_response"}); err != nil {
			continue
		}
		_, _ = AddActivity(leadID, "status", "", "no_response", "", "", "", map[string]any{
			"rule": "stale_no_response", "days": days, "cutoff": cutoff,
		})
		updated = append(updated, leadID)
	}
	return map[string]any{"updated_count": len(updated), "lead_ids": updated, "days": days, "cutoff": cutoff}, nil
}
