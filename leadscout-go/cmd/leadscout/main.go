// Command leadscout is the Go rewrite of the LeadScout local-first sales
// opportunity system. It serves the REST API and static UI and provides a
// doctor command.
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ali-ulu/lead/leadscout-go/internal/db"
	"github.com/ali-ulu/lead/leadscout-go/internal/doctor"
	"github.com/ali-ulu/lead/leadscout-go/internal/mcp"
	"github.com/ali-ulu/lead/leadscout-go/internal/server"
)

func envInt(name string, def int) int {
	if raw := os.Getenv(name); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil {
			return v
		}
	}
	return def
}

func envInt64(name string, def int64) int64 {
	if raw := os.Getenv(name); raw != "" {
		if v, err := strconv.ParseInt(raw, 10, 64); err == nil {
			return v
		}
	}
	return def
}

func main() {
	mcpFlag := flag.Bool("mcp", false, "run the MCP server instead of the web server")
	mcpTransport := flag.String("mcp-transport", "stdio", "MCP transport: stdio or streamable-http")
	mcpHost := flag.String("mcp-host", "127.0.0.1", "MCP streamable-http host")
	mcpPort := flag.Int("mcp-port", 8790, "MCP streamable-http port")
	doctorFlag := flag.Bool("doctor", false, "run install health checks and exit")
	asJSON := flag.Bool("json", false, "emit machine-readable JSON (with -doctor)")
	flag.Parse()

	if *doctorFlag {
		os.Exit(doctor.Run(*asJSON))
	}

	if *mcpFlag {
		if err := db.Initialize(); err != nil {
			fmt.Fprintf(os.Stderr, "database init failed: %v\n", err)
			os.Exit(1)
		}
		os.Exit(runMCP(*mcpTransport, *mcpHost, *mcpPort))
	}

	if err := db.Initialize(); err != nil {
		fmt.Fprintf(os.Stderr, "database init failed: %v\n", err)
		os.Exit(1)
	}

	root := os.Getenv("LEADSCOUT_ROOT")
	if root == "" {
		root, _ = os.Getwd()
	}
	host := firstNonEmpty(os.Getenv("LEADSCOUT_HOST"), os.Getenv("LEAD_HUNTER_HOST"), "127.0.0.1")
	port := envInt("LEADSCOUT_PORT", envInt("LEAD_HUNTER_PORT", 8787))

	srv := server.New(server.Config{
		Host:           host,
		Port:           port,
		StaticDir:      filepath.Join(root, "static"),
		RootDir:        root,
		APIToken:       os.Getenv("LEADSCOUT_API_TOKEN"),
		MaxBody:        envInt64("LEADSCOUT_MAX_BODY_BYTES", 2*1024*1024),
		RatePerMin:     envInt("LEADSCOUT_HTTP_RATE_PER_MIN", 600),
		AllowedOrigins: parseOrigins(os.Getenv("LEADSCOUT_ALLOWED_ORIGINS")),
	})

	fmt.Printf("LeadScout 7.0 (go) ready: http://%s:%d\n", host, port)
	if err := srv.ListenAndServe(); err != nil {
		fmt.Fprintf(os.Stderr, "server stopped: %v\n", err)
		os.Exit(1)
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func runMCP(transport, host string, port int) int {
	srv := mcp.New()
	if transport == "streamable-http" {
		addr := fmt.Sprintf("%s:%d", host, port)
		fmt.Printf("LeadScout MCP (streamable-http) listening on http://%s/mcp\n", addr)
		if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
			fmt.Fprintf(os.Stderr, "mcp server stopped: %v\n", err)
			return 1
		}
		return 0
	}
	if err := srv.ServeStdio(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "mcp server stopped: %v\n", err)
		return 1
	}
	return 0
}

func parseOrigins(raw string) map[string]bool {
	out := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out[trimmed] = true
		}
	}
	return out
}
