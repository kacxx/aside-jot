// Package hooks adapts Claude Code and Cursor prompt hooks to jot.
//
// Contract:
//   - The prefix check runs before SQLite is opened or git is run; the
//     Service is opened lazily through an app.Opener.
//   - Ordinary prompts and malformed payloads fail open: the prompt goes
//     through as if the hook were not installed.
//   - A recognised jot fails safe: it is always blocked, whether or not it was
//     saved, so the model never sees it. On failure the message says it was
//     NOT saved and repeats the original text so nothing is lost.
//   - A payload too large to parse in memory is still checked: its top-level
//     prompt is found by streaming, and a jot is blocked as NOT saved.
//   - Handlers never make the process exit non-zero; callers exit 0.
package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/kacxx/aside-jot/internal/app"
)

// maxPayload caps how much of a hook payload is parsed in memory. A variable
// so tests can exercise the oversized path without 16 MiB inputs.
var maxPayload = 16 << 20

// maxEcho caps how much of the original prompt a failure message repeats when
// the payload was oversized.
const maxEcho = 64 << 10

// payload is a hook's stdin.
type payload struct {
	data []byte // the whole payload; nil if oversized

	// For an oversized payload: its top-level "prompt" string, or "" if it
	// has none or the payload is malformed.
	oversized bool
	prompt    string
}

// readPayload reads a hook payload. One larger than maxPayload is not parsed
// in full: the rest is streamed to find its prompt, so a jot hidden behind
// large metadata is still recognised instead of passing through as malformed.
func readPayload(r io.Reader) payload {
	data, _ := io.ReadAll(io.LimitReader(r, int64(maxPayload)+1))
	if len(data) <= maxPayload {
		return payload{data: data}
	}
	prompt := scanPrompt(io.MultiReader(bytes.NewReader(data), r))
	// Drain stdin so the host never sees a broken pipe.
	_, _ = io.Copy(io.Discard, r)
	return payload{oversized: true, prompt: prompt}
}

// scanPrompt streams a JSON object and returns its top-level "prompt" string,
// matching json.Unmarshal: keys compare case-insensitively and the last one
// wins. Other values are skipped token by token, so memory is bounded by the
// largest single token, not the payload. Malformed input yields "", like a
// malformed payload that fits in memory.
func scanPrompt(r io.Reader) string {
	dec := json.NewDecoder(r)
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return ""
	}
	var prompt string
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return ""
		}
		if key, _ := t.(string); strings.EqualFold(key, "prompt") {
			if err := dec.Decode(&prompt); err != nil {
				return ""
			}
			continue
		}
		if err := skipValue(dec); err != nil {
			return ""
		}
	}
	if t, err := dec.Token(); err != nil || t != json.Delim('}') {
		return ""
	}
	return prompt
}

// skipValue consumes one JSON value from dec.
func skipValue(dec *json.Decoder) error {
	depth := 0
	for {
		t, err := dec.Token()
		if err != nil {
			return err
		}
		switch t {
		case json.Delim('{'), json.Delim('['):
			depth++
		case json.Delim('}'), json.Delim(']'):
			depth--
		}
		if depth == 0 {
			return nil
		}
	}
}

// tooLarge is the failure message for a jot in an oversized payload.
func tooLarge(original string) string {
	if len(original) > maxEcho {
		cut := maxEcho
		for cut > 0 && !utf8.RuneStart(original[cut]) {
			cut--
		}
		original = original[:cut] + "\n…[truncated]"
	}
	return notSaved(fmt.Errorf("hook payload is larger than %s", sizeLabel(maxPayload)), original)
}

func sizeLabel(n int) string {
	if n%(1<<20) == 0 {
		return fmt.Sprintf("%d MiB", n>>20)
	}
	return fmt.Sprintf("%d KiB", n>>10)
}

// save opens the service and captures req. It always returns a message for
// the user and never panics.
func save(ctx context.Context, open app.Opener, req app.CaptureRequest, original string) (msg string) {
	defer func() {
		if r := recover(); r != nil {
			msg = notSaved(fmt.Errorf("internal error: %v", r), original)
		}
	}()
	svc, err := open()
	if err != nil {
		return notSaved(err, original)
	}
	defer svc.Close()
	e, err := svc.Capture(ctx, req)
	if err != nil {
		return notSaved(err, original)
	}
	return fmt.Sprintf("✓ Jotted #%d", e.ID)
}

func notSaved(err error, original string) string {
	return fmt.Sprintf("✗ Jot NOT saved (%v). Your text:\n\n%s", err, original)
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}
