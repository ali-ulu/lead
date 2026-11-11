// Package doctor reports the health of a local LeadScout install.
package doctor

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ali-ulu/lead/leadscout-go/internal/db"
	"github.com/ali-ulu/lead/leadscout-go/internal/providers/osm"
	"github.com/ali-ulu/lead/leadscout-go/internal/providers/websearch"
)

var envKeys = []string{
	"LEADSCOUT_API_TOKEN", "LEADSCOUT_WEB_SEARCH", "LEADSCOUT_AI_PRESENCE",
	"LEADSCOUT_RESPECT_ROBOTS", "GOOGLE_CSE_API_KEY", "GOOGLE_CSE_ID",
	"BRAVE_SEARCH_API_KEY", "SEARXNG_URL", "GOOGLE_PLACES_API_KEY",
}

func envSet(name string) bool { return strings.TrimSpace(os.Getenv(name)) != "" }

// RunChecks returns the install health report.
func RunChecks() map[string]any {
	problems := []string{}

	database := map[string]any{}
	path := db.DBPath()
	database["path"] = path
	if _, err := os.Stat(path); err == nil {
		database["exists"] = true
	} else {
		database["exists"] = false
	}
	writeErr := db.Initialize()
	if writeErr != nil {
		database["writable"] = false
		database["error"] = writeErr.Error()
		problems = append(problems, fmt.Sprintf("Database is not usable at %s: %s", path, writeErr.Error()))
	} else {
		database["writable"] = true
		var tables []string
		if conn, err := db.Conn(); err == nil {
			rows, err := conn.Query("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
			if err == nil {
				for rows.Next() {
					var name string
					if rows.Scan(&name) == nil {
						tables = append(tables, name)
					}
				}
				rows.Close()
			}
			var count int
			_ = conn.QueryRow("SELECT COUNT(*) FROM leads").Scan(&count)
			database["lead_count"] = count
			conn.Close()
		}
		database["tables"] = tables
	}

	web := websearch.ConfiguredProviders()
	providers := map[string]any{
		"osm":           map[string]any{"ok": true, "detail": "public Overpass, no key required"},
		"overture":      map[string]any{"ok": false, "detail": "Overture discovery requires DuckDB and is not available in the Go build"},
		"web_search":    map[string]any{"ok": len(web) > 0, "detail": chainDetail(web)},
		"google_places": map[string]any{"ok": true, "detail": "optional reputation; key " + setState("GOOGLE_PLACES_API_KEY")},
	}
	problems = append(problems, "Overture discovery is disabled in the Go build: it requires DuckDB (the Python build). Discovery falls back to OpenStreetMap only.")

	config := map[string]string{}
	for _, key := range envKeys {
		config[key] = setState(key)
	}
	if !providers["web_search"].(map[string]any)["ok"].(bool) {
		problems = append(problems, "Web verification is disabled by configuration (LEADSCOUT_WEB_SEARCH=off).")
	}

	node := "not found (optional: website audits fall back to heuristics)"
	if _, err := exec.LookPath("node"); err == nil {
		node = "found"
	}

	categories := make([]string, 0, len(osm.CategoryFilters))
	for k := range osm.CategoryFilters {
		categories = append(categories, k)
	}
	sort.Strings(categories)

	return map[string]any{
		"ok":         len(problems) == 0,
		"runtime":    "go",
		"node":       node,
		"database":   database,
		"providers":  providers,
		"config":     config,
		"categories": categories,
		"problems":   problems,
	}
}

func chainDetail(web []string) string {
	if len(web) == 0 {
		return "disabled (LEADSCOUT_WEB_SEARCH=off)"
	}
	return "chain: " + strings.Join(web, ", ")
}

func setState(name string) string {
	if envSet(name) {
		return "set"
	}
	return "not set"
}

// FormatReport renders a human-readable report.
func FormatReport(report map[string]any) string {
	var b strings.Builder
	fmt.Fprintf(&b, "LeadScout doctor — runtime: %v\n", report["runtime"])
	fmt.Fprintf(&b, "node: %v\n", report["node"])
	b.WriteString("\nDatabase:\n")
	dbi := report["database"].(map[string]any)
	mark := func(ok bool) string {
		if ok {
			return "OK "
		}
		return "!! "
	}
	ok, _ := dbi["writable"].(bool)
	fmt.Fprintf(&b, "  [%s] %v\n", mark(ok), dbi["path"])
	if tables, ok := dbi["tables"].([]string); ok {
		fmt.Fprintf(&b, "      tables: %s\n", strings.Join(tables, ", "))
	}
	fmt.Fprintf(&b, "      leads: %v\n", dbi["lead_count"])
	b.WriteString("\nProviders:\n")
	provs := report["providers"].(map[string]any)
	var names []string
	for name := range provs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		info := provs[name].(map[string]any)
		ok, _ := info["ok"].(bool)
		fmt.Fprintf(&b, "  [%s] %-14s %v\n", mark(ok), name, info["detail"])
	}
	b.WriteString("\nConfig:\n")
	cfg := report["config"].(map[string]string)
	var keys []string
	for k := range cfg {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, "  %-24s %s\n", k, cfg[k])
	}
	cats, _ := report["categories"].([]string)
	fmt.Fprintf(&b, "  %-24s %d\n", "categories", len(cats))
	problems, _ := report["problems"].([]string)
	if len(problems) > 0 {
		b.WriteString("\nProblems:\n")
		for _, p := range problems {
			fmt.Fprintf(&b, "  - %s\n", p)
		}
	} else {
		b.WriteString("\nNo problems found.\n")
	}
	return b.String()
}

// Run executes the doctor command; returns the process exit code.
func Run(asJSON bool) int {
	report := RunChecks()
	if asJSON {
		out, _ := json.MarshalIndent(report, "", "  ")
		fmt.Println(string(out))
	} else {
		fmt.Print(FormatReport(report))
	}
	if ok, _ := report["ok"].(bool); ok {
		return 0
	}
	return 1
}

var _ = filepath.Join
