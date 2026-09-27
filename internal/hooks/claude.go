package hooks

import (
	"context"
	"encoding/json"
	"io"

	"github.com/kacxx/aside-jot/internal/app"
	"github.com/kacxx/aside-jot/internal/capture"
)

// ClaudeInput is the subset of the UserPromptSubmit payload jot uses.
type ClaudeInput struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	Cwd            string `json:"cwd"`
	HookEventName  string `json:"hook_event_name"`
	Prompt         string `json:"prompt"`
}

// ClaudeBlock is the UserPromptSubmit output that blocks and erases a prompt.
type ClaudeBlock struct {
	Decision               string `json:"decision"`
	Reason                 string `json:"reason"`
	SuppressOriginalPrompt bool   `json:"suppressOriginalPrompt"`
}

// Claude handles a Claude Code UserPromptSubmit hook. Pass-through writes
// nothing (any stdout would be added to the model's context).
func Claude(ctx context.Context, r io.Reader, w io.Writer, open app.Opener) error {
	p := readPayload(r)
	if p.oversized {
		if _, ok := capture.Match(p.prompt); ok {
			return writeJSON(w, ClaudeBlock{Decision: "block", Reason: tooLarge(p.prompt), SuppressOriginalPrompt: true})
		}
		return nil
	}
	var in ClaudeInput
	if err := json.Unmarshal(p.data, &in); err != nil {
		return nil
	}
	if in.HookEventName != "" && in.HookEventName != "UserPromptSubmit" {
		return nil
	}
	text, ok := capture.Match(in.Prompt)
	if !ok {
		return nil
	}

	var meta map[string]any
	if in.TranscriptPath != "" {
		meta = map[string]any{"transcript_path": in.TranscriptPath}
	}
	msg := save(ctx, open, app.CaptureRequest{
		Text:      text,
		Source:    "claude",
		SessionID: in.SessionID,
		Cwd:       in.Cwd,
		Metadata:  meta,
	}, in.Prompt)
	return writeJSON(w, ClaudeBlock{Decision: "block", Reason: msg, SuppressOriginalPrompt: true})
}
