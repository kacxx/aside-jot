package hooks

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

// big returns a payload over maxPayload: fields are written in order, with
// padding (a string or a nested object) placed before or after the prompt.
func big(t *testing.T, fields ...string) []byte {
	t.Helper()
	b := []byte("{" + strings.Join(fields, ",") + "}")
	if len(b) <= maxPayload {
		t.Fatalf("payload is only %d bytes", len(b))
	}
	return b
}

func str(s string) string { b, _ := json.Marshal(s); return string(b) }

// smallLimit lowers maxPayload for the test so the oversized path runs on
// small inputs; TestOversizedAtRealLimit covers the real 16 MiB limit.
func smallLimit(t *testing.T) {
	old := maxPayload
	maxPayload = 4 << 10
	t.Cleanup(func() { maxPayload = old })
}

func padString() string { return `"attachments_text":` + str(strings.Repeat("x", maxPayload)) }
func padNested() string {
	return `"meta":{"a":[1,2,{"b":` + str(strings.Repeat("y", maxPayload)) + `}],"c":null}`
}

// TestOversizedAtRealLimit is the reported case: a valid jot plus more than
// 16 MiB of other content must be blocked, not passed through as malformed.
func TestOversizedAtRealLimit(t *testing.T) {
	if maxPayload != 16<<20 {
		t.Fatalf("maxPayload = %d", maxPayload)
	}
	payload := big(t, `"session_id":"s"`, `"prompt":">> secret"`, padString())
	_, b := runClaude(t, payload, neverOpen(t))
	if b == nil || b.Decision != "block" || !strings.Contains(b.Reason, "larger than 16 MiB") {
		t.Fatalf("got %+v", b)
	}
}

func TestOversizedJotIsBlocked(t *testing.T) {
	smallLimit(t)
	padString, padNested := padString(), padNested()
	cases := map[string][]byte{
		"prompt first":   big(t, `"prompt":">> secret"`, padString),
		"prompt last":    big(t, padString, `"session_id":"s"`, `"prompt":">> secret"`),
		"nested padding": big(t, padNested, `"prompt":">> secret"`),
		"key case":       big(t, padString, `"Prompt":">> secret"`),
		"last key wins":  big(t, `"prompt":"hello"`, padString, `"prompt":">> secret"`),
	}
	for name, payload := range cases {
		t.Run(name+"/claude", func(t *testing.T) {
			_, b := runClaude(t, payload, neverOpen(t))
			if b == nil || b.Decision != "block" {
				t.Fatalf("oversized jot must be blocked, got %+v", b)
			}
			if !strings.Contains(b.Reason, "NOT saved") || !strings.Contains(b.Reason, "larger than 4 KiB") ||
				!strings.Contains(b.Reason, ">> secret") {
				t.Fatalf("reason: %q", b.Reason)
			}
		})
		t.Run(name+"/cursor", func(t *testing.T) {
			_, o := runCursor(t, payload, neverOpen(t))
			if o.Continue || !strings.Contains(o.UserMessage, "NOT saved") || !strings.Contains(o.UserMessage, ">> secret") {
				t.Fatalf("oversized jot must be blocked, got %+v", o)
			}
		})
	}
}

func TestOversizedOrdinaryPromptPassesThrough(t *testing.T) {
	smallLimit(t)
	padString, padNested := padString(), padNested()
	cases := map[string][]byte{
		"ordinary prompt": big(t, `"prompt":"fix the bug"`, padString),
		"no prompt":       big(t, padNested),
		"prompt not str":  big(t, `"prompt":42`, padString),
		"malformed tail":  []byte(`{"prompt":">> secret",` + padString + `,`),
		"not an object":   []byte(`[` + str(strings.Repeat("z", maxPayload)) + `]`),
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			if out, _ := runClaude(t, payload, neverOpen(t)); out != "" {
				t.Fatalf("claude: %q", out)
			}
			if _, o := runCursor(t, payload, neverOpen(t)); !o.Continue {
				t.Fatalf("cursor: %+v", o)
			}
		})
	}
}

func TestOversizedStdinIsDrained(t *testing.T) {
	smallLimit(t)
	padString := padString()
	r := bytes.NewReader(big(t, `"prompt":"x"`, padString, `"after":`+str(strings.Repeat("w", 1000))))
	readPayload(r)
	if r.Len() != 0 {
		t.Fatalf("%d bytes left unread", r.Len())
	}
}

func TestTooLargeClipsEcho(t *testing.T) {
	smallLimit(t)
	msg := tooLarge(">> " + strings.Repeat("é", maxEcho))
	if !utf8.ValidString(msg) || !strings.HasSuffix(msg, "…[truncated]") || len(msg) > maxEcho+200 {
		t.Fatalf("len=%d valid=%v suffix=%q", len(msg), utf8.ValidString(msg), msg[len(msg)-20:])
	}
	if short := tooLarge(">> short"); !strings.HasSuffix(short, ">> short") {
		t.Fatalf("short prompt must be echoed whole: %q", short)
	}
}
