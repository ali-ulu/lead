// Package mcp implements the LeadScout Model Context Protocol server. It
// exposes the same tool surface as the Python build over stdio or
// streamable-HTTP JSON-RPC, so existing MCP clients keep working unchanged.
package mcp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ProtocolVersion is the MCP revision this server speaks.
const ProtocolVersion = "2025-06-18"

// Tool is a registered MCP tool.
type Tool struct {
	Name        string
	Description string
	InputSchema map[string]any
	Handler     func(args map[string]any) (any, error)
}

// Server holds the registered tools.
type Server struct {
	name    string
	version string
	tools   []Tool
	byName  map[string]Tool
}

// New returns a server with the full LeadScout tool set.
func New() *Server {
	s := &Server{name: "LeadScout", version: "7.0.0", byName: map[string]Tool{}}
	s.register(leadscoutTools()...)
	return s
}

func (s *Server) register(tools ...Tool) {
	for _, tool := range tools {
		s.tools = append(s.tools, tool)
		s.byName[tool.Name] = tool
	}
}

// Tools lists the registered tools (used by the HTTP surface and tests).
func (s *Server) Tools() []Tool { return s.tools }

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

func (s *Server) handle(req rpcRequest) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		var params struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &params)
		version := params.ProtocolVersion
		if version == "" {
			version = ProtocolVersion
		}
		return map[string]any{
			"protocolVersion": version,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": s.name, "version": s.version},
		}, nil
	case "notifications/initialized", "notifications/cancelled":
		return nil, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		tools := make([]map[string]any, 0, len(s.tools))
		for _, tool := range s.tools {
			tools = append(tools, map[string]any{
				"name": tool.Name, "description": tool.Description, "inputSchema": tool.InputSchema,
			})
		}
		return map[string]any{"tools": tools}, nil
	case "tools/call":
		var params struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, &rpcError{Code: -32602, Message: "invalid params"}
		}
		tool, ok := s.byName[params.Name]
		if !ok {
			return nil, &rpcError{Code: -32602, Message: "unknown tool: " + params.Name}
		}
		if params.Arguments == nil {
			params.Arguments = map[string]any{}
		}
		result, err := tool.Handler(params.Arguments)
		if err != nil {
			// Report tool failures in-band so the model sees the real reason
			// rather than a bare protocol error.
			return map[string]any{
				"content": []map[string]any{{"type": "text", "text": err.Error()}},
				"isError": true,
			}, nil
		}
		return map[string]any{"content": toolContent(result), "isError": false}, nil
	}
	return nil, &rpcError{Code: -32601, Message: "method not found: " + req.Method}
}

// toolContent renders a tool result as a JSON text block.
func toolContent(result any) []map[string]any {
	payload, err := json.Marshal(result)
	if err != nil {
		return []map[string]any{{"type": "text", "text": fmt.Sprint(result)}}
	}
	return []map[string]any{{"type": "text", "text": string(payload)}}
}

// ServeStdio runs the JSON-RPC loop over newline-delimited stdin/stdout.
func (s *Server) ServeStdio(in io.Reader, out io.Writer) error {
	reader := bufio.NewReader(in)
	writer := bufio.NewWriter(out)
	defer writer.Flush()
	for {
		line, err := reader.ReadBytes('\n')
		if len(strings.TrimSpace(string(line))) > 0 {
			s.dispatch(writer, line)
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

func (s *Server) dispatch(w *bufio.Writer, raw []byte) {
	var req rpcRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		writeRPC(w, rpcResponse{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: "parse error"}})
		return
	}
	if len(req.ID) == 0 {
		// Notification: no response is sent.
		_, _ = s.handle(req)
		return
	}
	result, rpcErr := s.handle(req)
	writeRPC(w, rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: result, Error: rpcErr})
}

func writeRPC(w *bufio.Writer, resp rpcResponse) {
	payload, _ := json.Marshal(resp)
	_, _ = w.Write(payload)
	_ = w.WriteByte('\n')
	_ = w.Flush()
}

// Handler serves the streamable-HTTP transport (POST /mcp, JSON responses).
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodDelete {
			// Stateless streamable-HTTP answers with JSON; no SSE stream is opened.
			w.Header().Set("Allow", "POST")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		var req rpcRequest
		if err := json.Unmarshal(body, &req); err != nil {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(rpcResponse{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: "parse error"}})
			return
		}
		if len(req.ID) == 0 {
			_, _ = s.handle(req)
			w.WriteHeader(http.StatusAccepted)
			return
		}
		result, rpcErr := s.handle(req)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: result, Error: rpcErr})
	})
	return mux
}
