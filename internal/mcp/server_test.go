package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"reflect"
	"strconv"
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
	if err := json.Unmarshal(rs[0].Result, &init); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if init.ProtocolVersion != "2025-06-18" || init.ServerInfo.Name != "aside" || init.Capabilities["tools"] == nil {
		t.Errorf("initialize: %s", rs[0].Result)
	}

	var list struct{ Tools []struct{ Name string } }
	if err := json.Unmarshal(rs[1].Result, &list); err != nil {
		t.Fatalf("tools/list: %v", err)
	}
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
	if !strings.Contains(out.String(), `"protocolVersion":"2025-11-25"`) {
		t.Fatalf("unknown versions must be counter-offered the newest supported one: %s", out.String())
	}
}

// JSON-RPC edge cases: which messages get a reply, and with which id.
func TestRequestIDHandling(t *testing.T) {
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":null,"method":"ping"}`,
		`{"jsonrpc":"2.0","id":{"a":1},"method":"ping"}`,
		`{"jsonrpc":"2.0","id":true,"method":"ping"}`,
		`{"jsonrpc":"2.0","id":16,"method":123}`,
		`{"jsonrpc":"2.0","id":15,"result":{}}`,
		`{"jsonrpc":"2.0","id":14,"error":{"code":1,"message":"x"}}`,
		`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{}}`,
		`{}`,
		`{"jsonrpc":"2.0"}`,
		`{"jsonrpc":"2.0","id":"s1","method":"ping"}`,
		`{"jsonrpc":"2.0","id":-2,"method":"ping"}`,
	}, "\n")
	var out bytes.Buffer
	if err := NewServer(nil, "t").Serve(context.Background(), strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	type resp struct {
		ID    json.RawMessage `json:"id"`
		Error *rpcError       `json:"error"`
	}
	var got []string
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var r resp
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("bad line %q", line)
		}
		code := 0
		if r.Error != nil {
			code = r.Error.Code
		}
		got = append(got, string(r.ID)+":"+strconv.Itoa(code))
	}
	want := []string{
		"null:-32600", // id null
		"null:-32600", // id object
		"null:-32600", // id boolean
		"16:-32600",   // method not a string, id still usable
		// client responses and the notification get no reply
		"null:-32600", // {} has no id and no method: invalid, not a notification
		"null:-32600", // same without a method
		`"s1":0`,
		"-2:0",
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("got  %v\nwant %v", got, want)
	}
}

func TestShowArgumentErrors(t *testing.T) {
	svc, err := app.Open(filepath.Join(t.TempDir(), "jot.db"), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	s := NewServer(svc, "t")
	for args, want := range map[string]string{
		`{"id":0}`:     "id is required and must be a positive integer",
		`{"id":-3}`:    "id is required and must be a positive integer",
		`{}`:           "id is required and must be a positive integer",
		`{"id":"one"}`: "invalid arguments: id must be an integer",
		`{"id":1.5}`:   "invalid arguments: id must be an integer",
		`["x"]`:        "invalid arguments: expected an object",
		`{"query":5}`:  "invalid arguments: query must be a string",
	} {
		res, rerr := s.callTool(context.Background(), "show", json.RawMessage(args))
		if rerr != nil {
			t.Fatalf("%s: rpc error %v", args, rerr)
		}
		m := res.(map[string]any)
		text := m["content"].([]map[string]any)[0]["text"].(string)
		if m["isError"] != true || text != want {
			t.Errorf("%s: got %q (isError=%v), want %q", args, text, m["isError"], want)
		}
		if strings.Contains(text, "Go") || strings.Contains(text, "int64") {
			t.Errorf("%s: leaks Go internals: %q", args, text)
		}
	}
}

func TestJSONTypeName(t *testing.T) {
	for k, want := range map[reflect.Kind]string{
		reflect.Int: "an integer", reflect.Int64: "an integer", reflect.Uint32: "an integer",
		reflect.Float64: "a number", reflect.Bool: "a boolean", reflect.String: "a string",
		reflect.Slice: "an array", reflect.Map: "an object", reflect.Struct: "an object",
	} {
		if got := jsonTypeName(k); got != want {
			t.Errorf("%v: got %q, want %q", k, got, want)
		}
	}
}

// fakeGH creates an issue without running anything.
type fakeGH struct{}

func (fakeGH) Run(_ context.Context, _ io.Reader, name string, args ...string) ([]byte, error) {
	if name == "gh" && args[0] == "issue" {
		return []byte("https://github.com/o/n/issues/3\n"), nil
	}
	return nil, nil
}

func TestShowIssueURL(t *testing.T) {
	ctx := context.Background()
	svc, err := app.Open(filepath.Join(t.TempDir(), "jot.db"), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	if _, err := svc.Capture(ctx, app.CaptureRequest{Text: "promote me", Source: "cli"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Promote(ctx, fakeGH{}, app.PromoteRequest{ID: 1, Repo: "o/n"}); err != nil {
		t.Fatal(err)
	}
	res, rerr := NewServer(svc, "test").callTool(ctx, "show", json.RawMessage(`{"id":1}`))
	if rerr != nil {
		t.Fatal(rerr)
	}
	text := res.(map[string]any)["content"].([]map[string]any)[0]["text"].(string)
	var e struct {
		Status   string `json:"status"`
		IssueURL string `json:"issue_url"`
	}
	if err := json.Unmarshal([]byte(text), &e); err != nil || e.IssueURL != "https://github.com/o/n/issues/3" || e.Status != "done" {
		t.Fatalf("show: %s (%v)", text, err)
	}
}
