// Package server exposes the REST API and static UI. It mirrors the Python
// server's routes, auth model and static-file safety checks.
package server

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ali-ulu/lead/leadscout-go/internal/agent"
	"github.com/ali-ulu/lead/leadscout-go/internal/crm"
	"github.com/ali-ulu/lead/leadscout-go/internal/db"
	"github.com/ali-ulu/lead/leadscout-go/internal/doctor"
	"github.com/ali-ulu/lead/leadscout-go/internal/exporters"
	"github.com/ali-ulu/lead/leadscout-go/internal/oauthmeta"
	"github.com/ali-ulu/lead/leadscout-go/internal/providers/osm"
	"github.com/ali-ulu/lead/leadscout-go/internal/services"
)

// Config holds the server settings.
type Config struct {
	Host           string
	Port           int
	StaticDir      string
	RootDir        string
	APIToken       string
	MaxBody        int64
	RatePerMin     int
	AllowedOrigins map[string]bool
}

// Server is the HTTP application.
type Server struct {
	cfg      Config
	rateMu   sync.Mutex
	rateHits map[string][]float64
}

// New builds a server from the environment.
func New(cfg Config) *Server {
	return &Server{cfg: cfg, rateHits: map[string][]float64{}}
}

func (s *Server) rateLimited(key string) bool {
	if s.cfg.RatePerMin <= 0 {
		return false
	}
	now := float64(time.Now().UnixNano()) / 1e9
	s.rateMu.Lock()
	defer s.rateMu.Unlock()
	var kept []float64
	for _, t := range s.rateHits[key] {
		if now-t < 60 {
			kept = append(kept, t)
		}
	}
	if len(kept) >= s.cfg.RatePerMin {
		s.rateHits[key] = kept
		return true
	}
	kept = append(kept, now)
	s.rateHits[key] = kept
	return false
}

func (s *Server) authorized(path string) bool {
	for _, prefix := range []string{"/api/v1/oauth/meta/", "/api/v1/webhooks/meta"} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	if path == "/api/health" || path == "/api/v1/health" {
		return true
	}
	if !strings.HasPrefix(path, "/api/") {
		return true
	}
	if s.cfg.APIToken != "" {
		return true // checked per request against the header
	}
	return isLoopback(s.cfg.Host)
}

func isLoopback(host string) bool {
	return host == "127.0.0.1" || host == "::1" || host == "localhost"
}

func (s *Server) tokenOK(r *http.Request) bool {
	if s.cfg.APIToken == "" {
		return true
	}
	return r.Header.Get("Authorization") == "Bearer "+s.cfg.APIToken
}

// originAllowed rejects cross-origin browser requests that are not on the
// localhost allow-list. A missing Origin header is always allowed.
func (s *Server) originAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	if s.cfg.AllowedOrigins[origin] {
		return true
	}
	port := strconv.Itoa(s.cfg.Port)
	for _, allowed := range []string{
		"http://127.0.0.1:8787", "http://localhost:8787",
		"http://" + s.cfg.Host + ":" + port,
		"http://127.0.0.1:" + port, "http://localhost:" + port,
	} {
		if origin == allowed {
			return true
		}
	}
	return false
}

func (s *Server) corsOrigin(r *http.Request) string {
	if s.originAllowed(r) {
		if origin := r.Header.Get("Origin"); origin != "" {
			return origin
		}
	}
	return "http://127.0.0.1"
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	body, _ := json.Marshal(payload)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func readJSON(r *http.Request, maxBody int64) (map[string]any, error) {
	limited := io.LimitReader(r.Body, maxBody+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return map[string]any{}, err
	}
	if int64(len(raw)) > maxBody {
		return nil, errors.New("request body too large")
	}
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]any{}, nil
	}
	return out, nil
}

// Handler returns the root HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.route)
	return mux
}

func (s *Server) route(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if s.rateLimited(clientIP(r)) {
		writeError(w, 429, "rate limited")
		return
	}
	if !s.originAllowed(r) {
		writeError(w, 403, "origin not allowed")
		return
	}
	if !s.authorized(path) {
		writeError(w, 401, "unauthorized")
		return
	}
	if !s.tokenOK(r) {
		writeError(w, 401, "unauthorized")
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleGet(w, r, path)
	case http.MethodPost:
		if path == "/api/v1/webhooks/meta" {
			s.handleMetaWebhook(w, r)
			return
		}
		s.handlePost(w, r, path)
	case http.MethodDelete:
		s.handleDelete(w, r, path)
	case http.MethodOptions:
		w.Header().Set("Access-Control-Allow-Origin", s.corsOrigin(r))
		w.Header().Set("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.WriteHeader(http.StatusNoContent)
	default:
		writeError(w, 404, "not found")
	}
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request, path string) {
	switch {
	case path == "/api/health" || path == "/api/v1/health":
		checks := doctor.RunChecks()
		problems, _ := checks["problems"].([]string)
		writeJSON(w, 200, map[string]any{
			"ok": true, "name": "LeadScout", "version": "7.0.0-go",
			"providers":       []string{"OpenStreetMap/Overpass", "DuckDuckGo/Google/Brave/SearXNG web verification", "Google Places reputation (optional)"},
			"provider_status": checks["providers"], "degraded": !checks["ok"].(bool),
			"problems": problems, "languages": []string{"en", "tr", "ur", "sd", "de"},
			"agent_api": "/api/v1", "openapi": "/api/v1/openapi.json", "mcp": "run with -mcp (stdio or streamable-http)",
		})
	case path == "/api/categories" || path == "/api/v1/categories":
		items := make([]string, 0, len(osm.CategoryFilters))
		for k := range osm.CategoryFilters {
			items = append(items, k)
		}
		writeJSON(w, 200, map[string]any{"items": items})
	case path == "/api/v1/openapi.json":
		spec := filepath.Join(s.cfg.RootDir, "docs", "openapi.json")
		if data, err := os.ReadFile(spec); err == nil {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_, _ = w.Write(data)
			return
		}
		writeError(w, 404, "not found")
	case path == "/api/leads" || path == "/api/v1/leads":
		rows, err := services.ListLeadsRelative(queryFilters(r))
		if err != nil {
			writeError(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"items": rows})
	case path == "/api/export.csv" || path == "/api/v1/export.csv":
		rows, _ := services.ListLeadsRelative(queryFilters(r))
		body, _ := exporters.CSV(rows)
		writeAttachment(w, body, "text/csv; charset=utf-8", "leadscout-leads.csv")
	case path == "/api/export.xlsx" || path == "/api/v1/export.xlsx":
		rows, _ := services.ListLeadsRelative(queryFilters(r))
		writeAttachment(w, exporters.XLSX(rows),
			"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "leadscout-leads.xlsx")
	case path == "/api/message":
		id, _ := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
		lang := r.URL.Query().Get("lang")
		if lang == "" {
			lang = "en"
		}
		result, err := services.DraftOutreach(id, lang)
		if err != nil {
			writeError(w, 404, err.Error())
			return
		}
		writeJSON(w, 200, result)
	case path == "/api/v1/oauth/connections":
		items, err := oauthmeta.ListConnections()
		if err != nil {
			writeError(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"items": items})
	case path == "/api/v1/oauth/meta/start" || path == "/api/v1/oauth/meta/facebook/start":
		target, err := oauthmeta.FacebookOAuthStartURL()
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		http.Redirect(w, r, target, http.StatusFound)
	case path == "/api/v1/oauth/meta/instagram/start":
		target, err := oauthmeta.InstagramOAuthStartURL()
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		http.Redirect(w, r, target, http.StatusFound)
	case path == "/api/v1/oauth/meta/callback" || path == "/api/v1/oauth/meta/facebook/callback":
		q := r.URL.Query()
		if q.Get("error") != "" {
			http.Redirect(w, r, "/?meta=facebook_error", http.StatusFound)
			return
		}
		if _, err := oauthmeta.HandleFacebookCallback(q.Get("code"), q.Get("state")); err != nil {
			http.Redirect(w, r, "/?meta=facebook_error", http.StatusFound)
			return
		}
		http.Redirect(w, r, "/?meta=facebook_connected", http.StatusFound)
	case path == "/api/v1/oauth/meta/instagram/callback":
		q := r.URL.Query()
		if q.Get("error") != "" {
			http.Redirect(w, r, "/?meta=instagram_error", http.StatusFound)
			return
		}
		if _, err := oauthmeta.HandleInstagramCallback(q.Get("code"), q.Get("state")); err != nil {
			http.Redirect(w, r, "/?meta=instagram_error", http.StatusFound)
			return
		}
		http.Redirect(w, r, "/?meta=instagram_connected", http.StatusFound)
	case path == "/api/v1/webhooks/meta":
		q := r.URL.Query()
		verify := strings.TrimSpace(os.Getenv("META_WEBHOOK_VERIFY_TOKEN"))
		if q.Get("hub.mode") == "subscribe" && verify != "" && q.Get("hub.verify_token") == verify {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write([]byte(q.Get("hub.challenge")))
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("forbidden"))
	case strings.HasPrefix(path, "/api/v1/leads/") && strings.HasSuffix(path, "/messaging-eligibility"):
		provider := r.URL.Query().Get("provider")
		if provider == "" {
			provider = "instagram"
		}
		result, err := oauthmeta.MessagingEligibility(leadID(path, "/messaging-eligibility"), provider)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, result)
	case strings.HasPrefix(path, "/api/v1/searches/"):
		id := path[strings.LastIndex(path, "/")+1:]
		run, err := db.GetSearchRun(id)
		if err != nil {
			writeError(w, 404, "not found")
			return
		}
		writeJSON(w, 200, run)
	case strings.HasPrefix(path, "/api/v1/agent/jobs/"):
		id := path[strings.LastIndex(path, "/")+1:]
		job, _ := agent.GetJob(id)
		if job == nil {
			writeError(w, 404, "not found")
			return
		}
		writeJSON(w, 200, job)
	case strings.HasPrefix(path, "/api/v1/leads/") && strings.HasSuffix(path, "/activities"):
		id := leadID(path, "/activities")
		items, err := crm.ListActivities(id)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"items": items})
	case strings.HasPrefix(path, "/api/v1/leads/") && strings.HasSuffix(path, "/message"):
		id := leadID(path, "/message")
		lang := r.URL.Query().Get("lang")
		if lang == "" {
			lang = "en"
		}
		result, err := services.DraftOutreach(id, lang)
		if err != nil {
			writeError(w, 404, err.Error())
			return
		}
		writeJSON(w, 200, result)
	case strings.HasPrefix(path, "/api/leads/") || strings.HasPrefix(path, "/api/v1/leads/"):
		id := leadID(path, "")
		lead, err := db.GetLead(id)
		if err != nil {
			writeError(w, 404, "not found")
			return
		}
		writeJSON(w, 200, services.AnnotateLeadRelative(lead))
	default:
		s.serveStatic(w, r, path)
	}
}

func (s *Server) handlePost(w http.ResponseWriter, r *http.Request, path string) {
	payload, err := readJSON(r, s.cfg.MaxBody)
	if err != nil {
		writeError(w, 413, err.Error())
		return
	}

	if path == "/api/discover" || path == "/api/v1/search" {
		city, _ := payload["city"].(string)
		category, _ := payload["category"].(string)
		country, _ := payload["country"].(string)
		radius := int(asFloat(payload["radius_km"]))
		var maxResults *int
		if v := asFloat(payload["max_results"]); v > 0 {
			m := int(v)
			maxResults = &m
		}
		result, err := services.DiscoverBusinesses(city, category, country, radius, maxResults, nil)
		if err != nil {
			writeError(w, 502, err.Error())
			return
		}
		writeJSON(w, 200, result)
		return
	}

	suffixes := []struct {
		suffix string
		fn     func(int64) (map[string]any, error)
	}{
		{"/audit", func(id int64) (map[string]any, error) { return services.AuditLead(id, true) }},
		{"/enrich", services.EnrichLead},
		{"/verify", services.VerifyLead},
		{"/reputation", services.EnrichReputation},
		{"/ai-presence", services.MeasureAIPresenceForLead},
	}
	for _, item := range suffixes {
		if (strings.HasPrefix(path, "/api/leads/") || strings.HasPrefix(path, "/api/v1/leads/")) && strings.HasSuffix(path, item.suffix) {
			id := leadID(path, item.suffix)
			result, err := item.fn(id)
			if err != nil {
				writeError(w, 404, err.Error())
				return
			}
			writeJSON(w, 200, result)
			return
		}
	}

	if (strings.HasPrefix(path, "/api/leads/") || strings.HasPrefix(path, "/api/v1/leads/")) && strings.HasSuffix(path, "/status") {
		status, _ := payload["status"].(string)
		result, err := services.SetPipelineStage(leadID(path, "/status"), strings.TrimSpace(status))
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, result)
		return
	}
	if strings.HasPrefix(path, "/api/v1/leads/") && strings.HasSuffix(path, "/engagement") {
		status, _ := payload["status"].(string)
		note, _ := payload["note"].(string)
		lead, err := crm.SetEngagement(leadID(path, "/engagement"), status, note)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"lead": lead})
		return
	}
	if strings.HasPrefix(path, "/api/v1/leads/") && strings.HasSuffix(path, "/follow-up") {
		note, _ := payload["note"].(string)
		var when *string
		if wv, ok := payload["when"].(string); ok && wv != "" {
			when = &wv
		}
		lead, err := crm.SetFollowUp(leadID(path, "/follow-up"), when, note)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"lead": lead})
		return
	}
	if strings.HasPrefix(path, "/api/v1/leads/") && strings.HasSuffix(path, "/notes") {
		note, _ := payload["note"].(string)
		activity, err := crm.AddNote(leadID(path, "/notes"), note)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"activity": activity})
		return
	}
	if (strings.HasPrefix(path, "/api/leads/") || strings.HasPrefix(path, "/api/v1/leads/")) && strings.HasSuffix(path, "/dnc") {
		result, err := services.MarkDoNotContact(leadID(path, "/dnc"))
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, result)
		return
	}
	if strings.HasPrefix(path, "/api/v1/leads/") && strings.HasSuffix(path, "/messaging-recipient") {
		result, err := oauthmeta.LinkRecipient(leadID(path, "/messaging-recipient"), str(payload["provider"]), str(payload["recipient_id"]))
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, result)
		return
	}
	if strings.HasPrefix(path, "/api/v1/leads/") && strings.HasSuffix(path, "/send") {
		var recipient *string
		if v, ok := payload["recipient_id"].(string); ok && v != "" {
			recipient = &v
		}
		var connectionID *int64
		if v := asFloat(payload["connection_id"]); v > 0 {
			cid := int64(v)
			connectionID = &cid
		}
		result, err := oauthmeta.SendMessage(leadID(path, "/send"), str(payload["provider"]), recipient, str(payload["text"]), connectionID)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, result)
		return
	}
	if strings.HasPrefix(path, "/api/v1/oauth/connections/") && strings.HasSuffix(path, "/disconnect") {
		parts := strings.Split(strings.Trim(path, "/"), "/")
		cid, err := strconv.ParseInt(parts[len(parts)-2], 10, 64)
		if err != nil {
			writeError(w, 400, "invalid connection id")
			return
		}
		if err := oauthmeta.Disconnect(cid); err != nil {
			writeError(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
		return
	}
	if path == "/api/v1/crm/refresh-no-response" {
		days := int(asFloat(payload["days"]))
		if days == 0 {
			days = 7
		}
		result, err := crm.MarkStaleNoResponse(days)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, result)
		return
	}
	if path == "/api/v1/agent/run" {
		req := agent.Request{
			City: str(payload["city"]), Category: str(payload["category"]), Country: str(payload["country"]),
			RadiusKM: int(asFloat(payload["radius_km"])), TopN: int(asFloat(payload["top_n"])),
			MinScore: int(asFloat(payload["min_score"])), MinOpportunityGap: int(asFloat(payload["min_opportunity_gap"])),
			MaxSEOScore: int(asFloat(payload["max_seo_score"])), MaxAEOScore: int(asFloat(payload["max_aeo_score"])),
			MaxGEOScore: int(asFloat(payload["max_geo_score"])), MaxAIVisibility: int(asFloat(payload["max_ai_visibility_score"])),
			Lang: str(payload["lang"]), VerifyMissing: boolOr(payload["verify_missing"], true),
			AuditWebsites: boolOr(payload["audit_websites"], true), ReputationEnrichment: boolOr(payload["reputation_enrichment"], false),
			Send: boolOr(payload["send"], false), SendProvider: str(payload["send_provider"]),
		}
		result, err := agent.RunSalesJob(req)
		if err != nil {
			writeError(w, 502, err.Error())
			return
		}
		writeJSON(w, 200, result)
		return
	}
	if path == "/api/clear" || path == "/api/v1/clear" {
		if path == "/api/v1/clear" {
			if confirm, ok := payload["confirm"].(bool); !ok || !confirm {
				writeError(w, 400, "confirm=true is required")
				return
			}
		}
		if err := db.ClearAll(); err != nil {
			writeError(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
		return
	}
	writeError(w, 404, "not found")
}

func (s *Server) handleMetaWebhook(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, s.cfg.MaxBody+1))
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if int64(len(raw)) > s.cfg.MaxBody {
		writeError(w, 413, "request body too large")
		return
	}
	provider := r.URL.Query().Get("provider")
	if provider == "" {
		provider = "facebook"
	}
	allowUnsigned := os.Getenv("LEADSCOUT_META_ALLOW_UNSIGNED_WEBHOOK") == "1"
	if !allowUnsigned && !oauthmeta.VerifyWebhookSignature(raw, r.Header.Get("X-Hub-Signature-256"), provider) {
		writeError(w, 401, "invalid webhook signature")
		return
	}
	payload := map[string]any{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &payload)
	}
	writeJSON(w, 200, oauthmeta.HandleWebhookPayload(provider, payload))
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request, path string) {
	if strings.HasPrefix(path, "/api/leads/") || strings.HasPrefix(path, "/api/v1/leads/") {
		deleted, err := db.DeleteLead(leadID(path, ""))
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"deleted": deleted})
		return
	}
	if path == "/api/v1/retention/purge" || path == "/api/retention/purge" {
		raw := r.URL.Query().Get("days")
		days := 365
		if raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed < 0 {
				writeError(w, 400, "days must be a non-negative integer")
				return
			}
			days = parsed
		}
		purged, err := db.PurgeStaleLeads(days, true)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"purged": purged})
		return
	}
	writeError(w, 404, "not found")
}

func (s *Server) serveStatic(w http.ResponseWriter, r *http.Request, path string) {
	if path == "/" {
		path = "/index.html"
	}
	clean := filepath.Clean("/" + strings.TrimPrefix(path, "/"))
	candidate := filepath.Join(s.cfg.StaticDir, clean)
	absStatic, _ := filepath.Abs(s.cfg.StaticDir)
	absCandidate, _ := filepath.Abs(candidate)
	if !strings.HasPrefix(absCandidate, absStatic) {
		writeError(w, 403, "forbidden")
		return
	}
	data, err := os.ReadFile(absCandidate)
	if err != nil {
		writeError(w, 404, "not found")
		return
	}
	ctype := mime.TypeByExtension(filepath.Ext(absCandidate))
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(data)
}

func writeAttachment(w http.ResponseWriter, body []byte, contentType, filename string) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(body)
}

func leadID(path, suffix string) int64 {
	clean := path
	if suffix != "" && strings.HasSuffix(clean, suffix) {
		clean = clean[:len(clean)-len(suffix)]
	}
	clean = strings.TrimRight(clean, "/")
	idx := strings.LastIndex(clean, "/")
	id, _ := strconv.ParseInt(clean[idx+1:], 10, 64)
	return id
}

func queryFilters(r *http.Request) map[string]string {
	out := map[string]string{}
	for k, v := range r.URL.Query() {
		if len(v) > 0 {
			out[k] = v[0]
		}
	}
	return out
}

func asFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case string:
		f, _ := strconv.ParseFloat(n, 64)
		return f
	}
	return 0
}

func str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func boolOr(v any, def bool) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return def
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 15 * time.Second,
	}
	return srv.ListenAndServe()
}
