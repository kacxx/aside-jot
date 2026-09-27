package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kacxx/aside-jot/internal/app"
)

func TestSession(t *testing.T) {
	ctx := context.Background()
	svc, err := app.Open(filepath.Join(t.TempDir(), "jot.db"), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	for _, txt := range []string{"first idea", "second idea", "unrelated"} {
		if _, err := svc.Capture(ctx, app.CaptureRequest{Text: txt, Source: "cli"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.Done(ctx, 3); err != nil {
		t.Fatal(err)
	}

	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"inbox","arguments":{"limit":10}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"show","arguments":{"id":1}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"search","arguments":{"query":"idea"}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"show","arguments":{"id":99}}}`,
		`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"capture","arguments":{"text":"x"}}}`,
		`{"jsonrpc":"2.0","id":8,"method":"resources/list"}`,
		``,
		`{not json`,
		`{"jsonrpc":"2.0","id":"s","method":"ping"}`,
		`{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"search","arguments":{"query":"unrelated"}}}`,
	}, "\n") // no trailing newline: the last line must still be served

	var out bytes.Buffer
	if err := NewServer(svc, "test").Serve(ctx, strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}

	type resp struct {
		ID     json.RawMessage `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  *rpcError       `json:"error"`
	}
	var rs []resp
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var r resp
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("bad response line %q: %v", line, err)
		}
		rs = append(rs, r)
	}
	// 12 requests with ids + 1 parse error; the notification gets no reply.
	wantIDs := []string{"1", "2", "3", "4", "5", "6", "7", "8", "null", `"s"`, "9"}
	if len(rs) != len(wantIDs) {
		t.Fatalf("got %d responses, want %d:\n%s", len(rs), len(wantIDs), out.String())
	}
	for i, id := range wantIDs {
		if string(rs[i].ID) != id {
			t.Errorf("response %d id = %s, want %s", i, rs[i].ID, id)
		}
	}

	var init struct {
		ProtocolVersion string `json:"protocolVersion"`
		ServerInfo      struct{ Name string }
		Capabilities    map[string]any
	}
	json.Unmarshal(rs[0].Result, &init)
	if init.ProtocolVersion != "2025-06-18" || init.ServerInfo.Name != "jot" || init.Capabilities["tools"] == nil {
		t.Errorf("initialize: %s", rs[0].Result)
	}

	var list struct{ Tools []struct{ Name string } }
	json.Unmarshal(rs[1].Result, &list)
	var names []string
	for _, tl := range list.Tools {
		names = append(names, tl.Name)
	}
	if strings.Join(names, ",") != "inbox,show,search" {
		t.Errorf("tools = %v; must be read-only inbox, show, search", names)
	}

	type toolResult struct {
		IsError bool `json:"isError"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		StructuredContent struct {
			Entries []app.Entry `json:"entries"`
			Text    string      `json:"text"`
		} `json:"structuredContent"`
	}
	tr := func(i int) toolResult {
		var r toolResult
		if err := json.Unmarshal(rs[i].Result, &r); err != nil {
			t.Fatalf("result %d: %v", i, err)
		}
		return r
	}

	if r := tr(2); r.IsError || len(r.StructuredContent.Entries) != 2 || r.StructuredContent.Entries[0].Text != "second idea" {
		t.Errorf("inbox: %s", rs[2].Result)
	}
	if r := tr(3); r.IsError || r.StructuredContent.Text != "first idea" || !strings.Contains(r.Content[0].Text, `"first idea"`) {
		t.Errorf("show: %s", rs[3].Result)
	}
	if r := tr(4); r.IsError || len(r.StructuredContent.Entries) != 2 {
		t.Errorf("search: %s", rs[4].Result)
	}
	if r := tr(5); !r.IsError || !strings.Contains(r.Content[0].Text, "#99") {
		t.Errorf("show missing: %s", rs[5].Result)
	}
	if rs[6].Error == nil || rs[6].Error.Code != codeInvalidParams {
		t.Errorf("capture tool must not exist: %+v", rs[6])
	}
	if rs[7].Error == nil || rs[7].Error.Code != codeMethodNotFound {
		t.Errorf("unknown method: %+v", rs[7])
	}
	if rs[8].Error == nil || rs[8].Error.Code != codeParse {
		t.Errorf("parse error: %+v", rs[8])
	}
	if r := tr(10); len(r.StructuredContent.Entries) != 1 || r.StructuredContent.Entries[0].Status != "done" {
		t.Errorf("search includes done: %s", rs[10].Result)
	}
}

func TestUnknownProtocolVersionFallsBack(t *testing.T) {
	var out bytes.Buffer
	in := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"1999-01-01"}}` + "\n"
	if err := NewServer(nil, "t").Serve(context.Background(), strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), LatestProtocolVersion) {
		t.Fatal(out.String())
	}
}
