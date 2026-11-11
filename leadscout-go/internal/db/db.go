// Package db is the SQLite persistence layer. It reuses the shipped schema.sql
// and reproduces the discovery-upsert merge rules so a re-scan never erases
// audit, verification or enrichment measurements.
package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"

	"github.com/ali-ulu/lead/leadscout-go/internal/model"
)

// ErrNotFound is returned when a lead/run does not exist.
var ErrNotFound = errors.New("not found")

// RootDir returns the project root (two levels above internal/db).
func RootDir() string {
	if env := os.Getenv("LEADSCOUT_ROOT"); env != "" {
		return env
	}
	wd, _ := os.Getwd()
	return wd
}

// DBPath returns the SQLite database path.
func DBPath() string {
	if env := os.Getenv("LEADSCOUT_DB"); env != "" {
		return env
	}
	return filepath.Join(RootDir(), "data", "lead_hunter.db")
}

func schemaPath() string {
	if env := os.Getenv("LEADSCOUT_SCHEMA"); env != "" {
		return env
	}
	// schema.sql is embedded next to this package at build time via the
	// generated path; fall back to the repo sql/ directory.
	candidates := []string{
		filepath.Join(RootDir(), "sql", "schema.sql"),
		filepath.Join(RootDir(), "internal", "db", "schema.sql"),
		filepath.Join(RootDir(), "leadscout-go", "internal", "db", "schema.sql"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return candidates[0]
}

// Open opens the SQLite database.
func Open() (*sql.DB, error) {
	p := DBPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return nil, err
	}
	conn, err := sql.Open("sqlite", p)
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(1)
	if _, err := conn.Exec("PRAGMA busy_timeout=5000"); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

var leadMigrations = []struct{ name, ddl string }{
	{"source_refs", "TEXT NOT NULL DEFAULT '{}'"},
	{"messaging_ids", "TEXT NOT NULL DEFAULT '{}'"},
	{"verification_status", "TEXT NOT NULL DEFAULT 'unverified'"},
	{"verification_notes", "TEXT NOT NULL DEFAULT '[]'"},
	{"audit_engine", "TEXT"},
	{"rating", "REAL"},
	{"review_count", "INTEGER"},
	{"aeo_score", "INTEGER"},
	{"geo_score", "INTEGER"},
	{"ai_visibility_score", "INTEGER"},
	{"opportunity_gap_score", "INTEGER"},
	{"visibility_reasons", "TEXT NOT NULL DEFAULT '{}'"},
	{"visibility_signals", "TEXT NOT NULL DEFAULT '{}'"},
	{"score_confidence", "TEXT NOT NULL DEFAULT '{}'"},
	{"ai_presence", "TEXT NOT NULL DEFAULT '{}'"},
	{"response_ms", "INTEGER"},
	{"ttfb_ms", "INTEGER"},
	{"contactability_score", "INTEGER NOT NULL DEFAULT 0"},
	{"commercial_score", "INTEGER NOT NULL DEFAULT 0"},
	{"intelligence_reasons", "TEXT NOT NULL DEFAULT '[]'"},
	{"engagement_status", "TEXT NOT NULL DEFAULT 'not_contacted'"},
	{"last_contacted_at", "TEXT"},
	{"last_reply_at", "TEXT"},
	{"follow_up_at", "TEXT"},
	{"notes", "TEXT"},
}

// Initialize creates tables and applies additive migrations.
func Initialize() error {
	conn, err := Open()
	if err != nil {
		return err
	}
	defer conn.Close()
	schema, err := os.ReadFile(schemaPath())
	if err != nil {
		return err
	}
	if _, err := conn.Exec(string(schema)); err != nil {
		return err
	}
	if err := addMissingColumns(conn, "leads", leadMigrations); err != nil {
		return err
	}
	if err := addMissingColumns(conn, "search_runs", []struct{ name, ddl string }{
		{"provider_summary", "TEXT NOT NULL DEFAULT '{}'"},
		{"partial", "INTEGER NOT NULL DEFAULT 0"},
		{"warnings", "TEXT NOT NULL DEFAULT '[]'"},
	}); err != nil {
		return err
	}
	return addMissingColumns(conn, "search_run_leads", []struct{ name, ddl string }{
		{"website_status", "TEXT"},
		{"lead_score", "INTEGER"},
		{"ai_visibility_score", "INTEGER"},
		{"opportunity_gap_score", "INTEGER"},
	})
}

func addMissingColumns(conn *sql.DB, table string, cols []struct{ name, ddl string }) error {
	rows, err := conn.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return err
	}
	existing := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			rows.Close()
			return err
		}
		existing[name] = true
	}
	rows.Close()
	for _, c := range cols {
		if existing[c.name] {
			continue
		}
		if _, err := conn.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, c.name, c.ddl)); err != nil {
			return err
		}
	}
	return nil
}

// --- JSON helpers ---------------------------------------------------------

func jsonMarshal(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func decodeRefs(s string) map[string]string {
	out := map[string]string{}
	if s != "" {
		_ = json.Unmarshal([]byte(s), &out)
	}
	return out
}

func decodeSocials(s string) map[string]string {
	out := map[string]string{}
	if s != "" {
		_ = json.Unmarshal([]byte(s), &out)
	}
	return out
}

func decodeStringList(s string) []string {
	out := []string{}
	if s != "" {
		_ = json.Unmarshal([]byte(s), &out)
	}
	return out
}

func decodeStringMap(s string) map[string]string {
	out := map[string]string{}
	if s != "" {
		_ = json.Unmarshal([]byte(s), &out)
	}
	return out
}

func decodeReasons(s string) map[string][]string {
	out := map[string][]string{}
	if s != "" {
		_ = json.Unmarshal([]byte(s), &out)
	}
	return out
}

func decodeAnyMap(s string) map[string]any {
	out := map[string]any{}
	if s != "" {
		_ = json.Unmarshal([]byte(s), &out)
	}
	return out
}

func nullStr(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	v := ns.String
	return &v
}

func nullInt(ni sql.NullInt64) *int {
	if !ni.Valid {
		return nil
	}
	v := int(ni.Int64)
	return &v
}

func nullFloat(nf sql.NullFloat64) *float64 {
	if !nf.Valid {
		return nil
	}
	v := nf.Float64
	return &v
}

func nullBool(ni sql.NullInt64) *bool {
	if !ni.Valid {
		return nil
	}
	v := ni.Int64 != 0
	return &v
}

func strOr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func intOr(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

func floatOr(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

func boolOr(p *bool) any {
	if p == nil {
		return nil
	}
	if *p {
		return 1
	}
	return 0
}

const leadColumns = `id,source,source_id,source_refs,name,country,city,category,latitude,longitude,
website,phone,email,social_url,social_links,messaging_ids,website_status,verification_status,
verification_notes,performance_score,seo_score,aeo_score,geo_score,ai_visibility_score,
opportunity_gap_score,visibility_reasons,visibility_signals,score_confidence,ai_presence,
response_ms,ttfb_ms,accessibility_score,mobile_ok,has_cta,has_booking,has_https,audit_engine,
rating,review_count,data_confidence,contactability_score,commercial_score,intelligence_reasons,
lead_score,score_reasons,pipeline_status,engagement_status,last_contacted_at,last_reply_at,
follow_up_at,notes,do_not_contact,created_at,updated_at`

func scanLead(row interface{ Scan(...any) error }) (*model.Lead, error) {
	var (
		l                                                           model.Lead
		sourceRefs, socialLinks, msgIDs, visReasons, visSignals     sql.NullString
		scoreConf, aiPresence, verNotes, intelReasons, scoreReasons sql.NullString
		lat, lon                                                    sql.NullFloat64
		website, phone, email, socialURL                            sql.NullString
		perf, seo, aeo, geo, aio, gap                               sql.NullInt64
		responseMS, ttfbMS, accessibility                           sql.NullInt64
		mobileOK, hasCTA, hasBooking, hasHTTPS, dnc                 sql.NullInt64
		auditEngine, lastContact, lastReply, followUp, notes        sql.NullString
		rating                                                      sql.NullFloat64
		reviewCount                                                 sql.NullInt64
	)
	err := row.Scan(
		&l.ID, &l.Source, &l.SourceID, &sourceRefs, &l.Name, &l.Country, &l.City, &l.Category,
		&lat, &lon, &website, &phone, &email, &socialURL, &socialLinks, &msgIDs,
		&l.WebsiteStatus, &l.VerificationStatus, &verNotes,
		&perf, &seo, &aeo, &geo, &aio, &gap, &visReasons, &visSignals, &scoreConf, &aiPresence,
		&responseMS, &ttfbMS, &accessibility, &mobileOK, &hasCTA, &hasBooking, &hasHTTPS,
		&auditEngine, &rating, &reviewCount, &l.DataConfidence, &l.ContactabilityScore,
		&l.CommercialScore, &intelReasons, &l.LeadScore, &scoreReasons, &l.PipelineStatus,
		&l.EngagementStatus, &lastContact, &lastReply, &followUp, &notes, &dnc, &l.CreatedAt, &l.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	l.Latitude = nullFloat(lat)
	l.Longitude = nullFloat(lon)
	l.Website = nullStr(website)
	l.Phone = nullStr(phone)
	l.Email = nullStr(email)
	l.SocialURL = nullStr(socialURL)
	l.SocialLinks = decodeSocials(socialLinks.String)
	l.MessagingIDs = decodeStringMap(msgIDs.String)
	l.SourceRefs = decodeRefs(sourceRefs.String)
	l.VerificationNotes = decodeStringList(verNotes.String)
	l.PerformanceScore = nullInt(perf)
	l.SEOScore = nullInt(seo)
	l.AEOScore = nullInt(aeo)
	l.GEOScore = nullInt(geo)
	l.AIVisibilityScore = nullInt(aio)
	l.OpportunityGapScore = nullInt(gap)
	l.VisibilityReasons = decodeReasons(visReasons.String)
	l.VisibilitySignals = decodeAnyMap(visSignals.String)
	l.ScoreConfidence = decodeStringMap(scoreConf.String)
	l.AIPresence = decodeAnyMap(aiPresence.String)
	l.ResponseMS = nullInt(responseMS)
	l.TTFBMS = nullInt(ttfbMS)
	l.AccessibilityScore = nullInt(accessibility)
	l.MobileOK = nullBool(mobileOK)
	l.HasCTA = nullBool(hasCTA)
	l.HasBooking = nullBool(hasBooking)
	l.HasHTTPS = nullBool(hasHTTPS)
	l.AuditEngine = nullStr(auditEngine)
	l.Rating = nullFloat(rating)
	l.ReviewCount = nullInt(reviewCount)
	l.IntelligenceReasons = decodeStringList(intelReasons.String)
	l.ScoreReasons = decodeStringList(scoreReasons.String)
	l.LastContactedAt = nullStr(lastContact)
	l.LastReplyAt = nullStr(lastReply)
	l.FollowUpAt = nullStr(followUp)
	l.Notes = nullStr(notes)
	l.DoNotContact = dnc.Valid && dnc.Int64 != 0
	return &l, nil
}

// GetLead returns a lead by id, or ErrNotFound.
func GetLead(id int64) (*model.Lead, error) {
	conn, err := Open()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	row := conn.QueryRow("SELECT "+leadColumns+" FROM leads WHERE id=?", id)
	lead, err := scanLead(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return lead, err
}

// leadExists reports whether a lead id exists.
func leadExists(conn *sql.DB, id int64) bool {
	var n int
	_ = conn.QueryRow("SELECT COUNT(*) FROM leads WHERE id=?", id).Scan(&n)
	return n > 0
}

// Conn exposes a raw connection for crm/security/oauth/agent packages.
func Conn() (*sql.DB, error) { return Open() }

var _ = strconv.Itoa
var _ = strings.TrimSpace
