// Package mcp is a minimal Model Context Protocol server over stdio
// (newline-delimited JSON-RPC 2.0): initialize, ping, tools/list, tools/call.
package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

// Annotations tell MCP hosts how a tool behaves. All four hints are always sent.
type Annotations struct {
	Title           string `json:"title,omitempty"`
	ReadOnlyHint    bool   `json:"readOnlyHint"`
	DestructiveHint bool   `json:"destructiveHint"`
	IdempotentHint  bool   `json:"idempotentHint"`
	OpenWorldHint   bool   `json:"openWorldHint"`
}

// Tool is one callable tool. Handler returns text for the model, or an error shown as a tool error.
type Tool struct {
	Name        string                                     `json:"name"`
	Description string                                     `json:"description"`
	InputSchema map[string]any                             `json:"inputSchema"`
	Annotations Annotations                                `json:"annotations"`
	Handler     func(args json.RawMessage) (string, error) `json:"-"`
}

// Server dispatches JSON-RPC requests to tools.
type Server struct {
	Name, Version, Instructions string
	tools                       []Tool
	byName                      map[string]int
}

// NewServer registers tools. Names must be unique.
func NewServer(name, version, instructions string, tools []Tool) *Server {
	s := &Server{Name: name, Version: version, Instructions: instructions, tools: tools, byName: map[string]int{}}
	for i, t := range tools {
		s.byName[t.Name] = i
	}
	return s
}

// Tools returns the registered tools (for tests and docs).
func (s *Server) Tools() []Tool { return s.tools }

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// Serve reads requests line by line until EOF.
func (s *Server) Serve(in io.Reader, out io.Writer) error {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	enc := json.NewEncoder(out)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		if resp := s.Handle(line); resp != nil {
			if err := enc.Encode(resp); err != nil {
				return err
			}
		}
	}
	return sc.Err()
}

// Handle processes one raw JSON-RPC message; it returns nil for notifications.
func (s *Server) Handle(raw []byte) any {
	var req request
	if err := json.Unmarshal(raw, &req); err != nil {
		return response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{-32700, "parse error"}}
	}
	notification := len(req.ID) == 0 || string(req.ID) == "null"
	result, rerr := s.dispatch(req)
	if notification {
		return nil
	}
	return response{JSONRPC: "2.0", ID: req.ID, Result: result, Error: rerr}
}

func (s *Server) dispatch(req request) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		v := p.ProtocolVersion
		if v == "" {
			v = "2025-06-18"
		}
		return map[string]any{
			"protocolVersion": v,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": s.Name, "version": s.Version},
			"instructions":    s.Instructions,
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": s.tools}, nil
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, &rpcError{-32602, "invalid params"}
		}
		i, ok := s.byName[p.Name]
		if !ok {
			return nil, &rpcError{-32602, fmt.Sprintf("unknown tool %q", p.Name)}
		}
		if len(p.Arguments) == 0 || string(p.Arguments) == "null" {
			p.Arguments = json.RawMessage("{}")
		}
		text, err := s.tools[i].Handler(p.Arguments)
		if err != nil {
			return toolResult(err.Error(), true), nil
		}
		return toolResult(text, false), nil
	default:
		if len(req.Method) > 14 && req.Method[:14] == "notifications/" {
			return nil, nil
		}
		return nil, &rpcError{-32601, fmt.Sprintf("method not found: %s", req.Method)}
	}
}

func toolResult(text string, isError bool) map[string]any {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
		"isError": isError,
	}
}
