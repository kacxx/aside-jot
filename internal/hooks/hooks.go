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
//   - Handlers never make the process exit non-zero; callers exit 0.
package hooks

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/kacxx/aside-jot/internal/app"
)

// maxPayload caps how much stdin a hook reads.
const maxPayload = 16 << 20

func readPayload(r io.Reader) []byte {
	b, _ := io.ReadAll(io.LimitReader(r, maxPayload))
	return b
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
