package db

import (
	"testing"
)

func insertLeadAt(t *testing.T, name, updated, engagement string, dnc int) int64 {
	t.Helper()
	conn, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	res, err := conn.Exec(
		`INSERT INTO leads (source,source_id,name,country,city,category,engagement_status,do_not_contact,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?)`,
		"test", name, name, "Turkey", "Afyonkarahisar", "restaurant", engagement, dnc, updated,
	)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

func TestDeleteLeadRemovesRowAndActivities(t *testing.T) {
	useTempDB(t)
	id := insertLeadAt(t, "Gone", "2026-10-05 00:00:00", "not_contacted", 0)
	conn, _ := Open()
	conn.Exec("INSERT INTO lead_activities (lead_id,kind) VALUES (?,?)", id, "note")
	conn.Close()

	deleted, err := DeleteLead(id)
	if err != nil || !deleted {
		t.Fatalf("delete: %v %v", deleted, err)
	}
	if _, err := GetLead(id); err == nil {
		t.Errorf("lead still present")
	}
	conn, _ = Open()
	defer conn.Close()
	var remaining int
	conn.QueryRow("SELECT COUNT(*) FROM lead_activities WHERE lead_id=?", id).Scan(&remaining)
	if remaining != 0 {
		t.Errorf("activities remain: %d", remaining)
	}
}

func TestPurgeKeepsRecentAndEngaged(t *testing.T) {
	useTempDB(t)
	old := insertLeadAt(t, "Old", "2020-01-01 00:00:00", "not_contacted", 0)
	insertLeadAt(t, "OldEngaged", "2020-01-01 00:00:00", "contacted", 0)
	insertLeadAt(t, "OldDnc", "2020-01-01 00:00:00", "not_contacted", 1)
	fresh := insertLeadAt(t, "Fresh", "2026-11-10 00:00:00", "not_contacted", 0)

	purged, err := PurgeStaleLeads(30, true)
	if err != nil {
		t.Fatal(err)
	}
	if purged != 1 {
		t.Errorf("purged = %d, want 1", purged)
	}
	if _, err := GetLead(old); err == nil {
		t.Errorf("old lead survived")
	}
	if _, err := GetLead(fresh); err != nil {
		t.Errorf("fresh lead was removed")
	}
	conn, _ := Open()
	defer conn.Close()
	rows, _ := conn.Query("SELECT name FROM leads")
	got := map[string]bool{}
	for rows.Next() {
		var n string
		rows.Scan(&n)
		got[n] = true
	}
	rows.Close()
	for _, want := range []string{"OldEngaged", "OldDnc", "Fresh"} {
		if !got[want] {
			t.Errorf("missing %s", want)
		}
	}
}

func TestPurgeRejectsNegativeDays(t *testing.T) {
	useTempDB(t)
	insertLeadAt(t, "Old", "2020-01-01 00:00:00", "not_contacted", 0)
	if _, err := PurgeStaleLeads(-1, true); err == nil {
		t.Errorf("expected error for negative days")
	}
}
