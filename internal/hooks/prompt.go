package hooks

import (
	"context"
	"io"

	"github.com/kacxx/aside-jot/internal/app"
	"github.com/kacxx/aside-jot/internal/capture"
)

// PromptInput is the subset of a UserPromptSubmit payload jot uses. Claude
// Code and Codex share this shape; TurnID and Model are sent only by Codex.
type PromptInput struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	Cwd            string `json:"cwd"`
	HookEventName  string `json:"hook_event_name"`
	Prompt         string `json:"prompt"`
	TurnID         string `json:"turn_id"`
	Model          string `json:"model"`
}

// promptHook adapts one agent's UserPromptSubmit hook. Pass-through writes
// nothing: both Claude Code and Codex add hook stdout to the model's context.
type promptHook struct {
	source string
	block  func(reason string) any             // the agent's blocking output
	meta   func(in PromptInput) map[string]any // entry metadata; nil for none
}

func (h promptHook) run(ctx context.Context, r io.Reader, w io.Writer, open app.Opener) error {
	p := readPayload(r)
	if p.oversized {
		if p.event != "" && p.event != "UserPromptSubmit" {
			return nil
		}
		if _, ok := capture.Match(p.prompt); ok {
			return writeJSON(w, h.block(tooLarge(p.prompt)))
		}
		return nil
	}
	var in PromptInput
	if err := decodePayload(p.data, &in); err != nil {
		return nil
	}
	if in.HookEventName != "" && in.HookEventName != "UserPromptSubmit" {
		return nil
	}
	text, ok := capture.Match(in.Prompt)
	if !ok {
		return nil
	}

	msg := save(ctx, open, app.CaptureRequest{
		Text:      text,
		Source:    h.source,
		SessionID: in.SessionID,
		Cwd:       in.Cwd,
		Metadata:  h.meta(in),
	}, in.Prompt)
	return writeJSON(w, h.block(msg))
}
