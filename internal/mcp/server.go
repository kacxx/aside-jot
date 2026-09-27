// Package mcp is a minimal Model Context Protocol server over stdio.
//
// Messages are newline-delimited JSON-RPC 2.0. The server exposes read-only
// tools (inbox, show, search). There is deliberately no capture tool: jots are
// written by the user, never by the model.
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/kacxx/aside-jot/internal/app"
)

// LatestProtocolVersion is offered when the client asks for one we don't know.
const LatestProtocolVersion = "2025-06-18"

var supportedVersions = map[string]bool{
	"2024-11-05": true,
	"2025-03-26": true,
	"2025-06-18": true,
	"2025-11-25": true,
}

// JSON-RPC error codes.
const (
	codeParse          = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
)

// Reader is the read-only slice of app.Service the server needs.
type Reader interface {
	Inbox(ctx context.Context, n int) ([]app.Entry, error)
	Show(ctx context.Context, id int64) (app.Entry, error)
	Search(ctx context.Context, q string, n int) ([]app.Entry, error)
}

// Server serves MCP requests against a Reader.
type Server struct {
	svc     Reader
	version string
}

// NewServer returns a server backed by svc.
func NewServer(svc Reader, version string) *Server {
	return &Server{svc: svc, version: version}
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Serve reads requests from r and writes responses to w until r is exhausted.
func (s *Server) Serve(ctx context.Context, r io.Reader, w io.Writer) error {
	br := bufio.NewReader(r)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	for {
		line, err := br.ReadBytes('\n')
		if line = bytes.TrimSpace(line); len(line) > 0 {
			if resp := s.handle(ctx, line); resp != nil {
				if werr := enc.Encode(resp); werr != nil {
					return werr
				}
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
}

func (s *Server) handle(ctx context.Context, line []byte) *response {
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		var syntax *json.SyntaxError
		if errors.As(err, &syntax) {
			return errResp(json.RawMessage("null"), codeParse, "parse error")
		}
		return errResp(json.RawMessage("null"), codeInvalidRequest, "invalid request")
	}
	isNotification := len(req.ID) == 0 || string(req.ID) == "null"
	if req.JSONRPC != "2.0" || req.Method == "" {
		if isNotification {
			return nil
		}
		return errResp(req.ID, codeInvalidRequest, "invalid request")
	}

	result, rerr := s.dispatch(ctx, req)
	if isNotification {
		return nil
	}
	if rerr != nil {
		return &response{JSONRPC: "2.0", ID: req.ID, Error: rerr}
	}
	return &response{JSONRPC: "2.0", ID: req.ID, Result: result}
}

func errResp(id json.RawMessage, code int, msg string) *response {
	return &response{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg}}
}

func (s *Server) dispatch(ctx context.Context, req request) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		v := p.ProtocolVersion
		if !supportedVersions[v] {
			v = LatestProtocolVersion
		}
		return map[string]any{
			"protocolVersion": v,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "jot", "version": s.version},
			"instructions": "Read-only access to the user's jot inbox: side-channel notes they " +
				"captured while working. You cannot create jots; only the user can.",
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": tools}, nil
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, &rpcError{Code: codeInvalidParams, Message: "invalid params"}
		}
		return s.callTool(ctx, p.Name, p.Arguments)
	default:
		if strings.HasPrefix(req.Method, "notifications/") {
			return nil, nil
		}
		return nil, &rpcError{Code: codeMethodNotFound, Message: "method not found: " + req.Method}
	}
}

const maxLimit = 200

var tools = []map[string]any{
	{
		"name":        "inbox",
		"description": "List the newest jots still in the user's inbox (not marked done).",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"limit": map[string]any{"type": "integer", "minimum": 1, "maximum": maxLimit, "default": 20},
			},
		},
		"annotations": map[string]any{"readOnlyHint": true},
	},
	{
		"name":        "show",
		"description": "Show one jot by id, including its git context and metadata.",
		"inputSchema": map[string]any{
			"type":       "object",
			"properties": map[string]any{"id": map[string]any{"type": "integer", "minimum": 1}},
			"required":   []string{"id"},
		},
		"annotations": map[string]any{"readOnlyHint": true},
	},
	{
		"name":        "search",
		"description": "Search all jots (inbox and done) for a substring, newest first.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "minLength": 1},
				"limit": map[string]any{"type": "integer", "minimum": 1, "maximum": maxLimit, "default": 20},
			},
			"required": []string{"query"},
		},
		"annotations": map[string]any{"readOnlyHint": true},
	},
}

func (s *Server) callTool(ctx context.Context, name string, raw json.RawMessage) (any, *rpcError) {
	var args struct {
		Limit int    `json:"limit"`
		ID    int64  `json:"id"`
		Query string `json:"query"`
	}
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return toolError("invalid arguments: " + err.Error()), nil
		}
	}
	if args.Limit <= 0 {
		args.Limit = 20
	}
	if args.Limit > maxLimit {
		args.Limit = maxLimit
	}

	var v any
	var err error
	switch name {
	case "inbox":
		var es []app.Entry
		es, err = s.svc.Inbox(ctx, args.Limit)
		v = map[string]any{"entries": nonNil(es)}
	case "show":
		if args.ID <= 0 {
			return toolError("id is required"), nil
		}
		v, err = s.svc.Show(ctx, args.ID)
		if errors.Is(err, app.ErrNotFound) {
			return toolError(fmt.Sprintf("no jot #%d", args.ID)), nil
		}
	case "search":
		var es []app.Entry
		es, err = s.svc.Search(ctx, args.Query, args.Limit)
		v = map[string]any{"entries": nonNil(es)}
	default:
		return nil, &rpcError{Code: codeInvalidParams, Message: "unknown tool: " + name}
	}
	if err != nil {
		return toolError(err.Error()), nil
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return toolError(err.Error()), nil
	}
	return map[string]any{
		"content":           []map[string]any{{"type": "text", "text": string(b)}},
		"structuredContent": v,
	}, nil
}

func toolError(msg string) map[string]any {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": msg}},
		"isError": true,
	}
}

func nonNil(es []app.Entry) []app.Entry {
	if es == nil {
		return []app.Entry{}
	}
	return es
}
