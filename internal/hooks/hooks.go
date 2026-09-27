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
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/dustin/go-humanize"

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

	// For an oversized payload: its top-level "prompt" and "hook_event_name"
	// strings, or "" if absent or the payload is malformed.
	oversized bool
	prompt    string
	event     string
}

// readPayload reads a hook payload. One larger than maxPayload is not parsed
// in full: the rest is streamed to find its prompt, so a jot hidden behind
// large metadata is still recognised instead of passing through as malformed.
func readPayload(r io.Reader) payload {
	data, _ := io.ReadAll(io.LimitReader(r, int64(maxPayload)+1))
	if len(data) <= maxPayload {
		return payload{data: data}
	}
	prompt, event := scanPrompt(io.MultiReader(bytes.NewReader(data), r))
	// Drain stdin so the host never sees a broken pipe.
	_, _ = io.Copy(io.Discard, r)
	return payload{oversized: true, prompt: prompt, event: event}
}

// scanPrompt streams a JSON object and returns its top-level "prompt" and
// "hook_event_name" strings, matching json.Unmarshal: keys compare
// case-insensitively and the last one wins. Other values are skipped token by
// token, so memory is bounded by the largest single token, not the payload.
// Malformed input yields "", "", like a malformed payload that fits in memory.
func scanPrompt(r io.Reader) (prompt, event string) {
	dec := json.NewDecoder(r)
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return "", ""
	}
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return "", ""
		}
		key, _ := t.(string)
		switch {
		case strings.EqualFold(key, "prompt"):
			err = dec.Decode(&prompt)
		case strings.EqualFold(key, "hook_event_name"):
			err = dec.Decode(&event)
		default:
			err = skipValue(dec)
		}
		if err != nil {
			return "", ""
		}
	}
	if t, err := dec.Token(); err != nil || t != json.Delim('}') {
		return "", ""
	}
	return prompt, event
}

// skipValue consumes one JSON value from dec. It walks tokens rather than
// calling dec.Decode into a json.RawMessage, which would buffer the whole value
// (possibly most of an oversized payload) in memory.
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

// decodePayload decodes a hook payload that fits in memory. A field of the
// wrong JSON type is not fatal: json.Unmarshal skips it and still fills every
// other field, so a jot is recognised (and blocked) even when, say, session_id
// arrives as a number. Only a syntactically malformed payload is an error,
// which callers treat as pass-through.
func decodePayload(data []byte, v any) error {
	err := json.Unmarshal(data, v)
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		return nil
	}
	return err
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
	return notSaved(fmt.Errorf("hook payload is larger than %s", humanize.IBytes(uint64(maxPayload))), original)
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
