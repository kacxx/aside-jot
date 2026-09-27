package hooks

import (
	"context"
	"encoding/json"
	"io"

	"github.com/kacxx/aside-jot/internal/app"
	"github.com/kacxx/aside-jot/internal/capture"
)

// CursorInput is the subset of the beforeSubmitPrompt payload jot uses.
type CursorInput struct {
	ConversationID string   `json:"conversation_id"`
	GenerationID   string   `json:"generation_id"`
	HookEventName  string   `json:"hook_event_name"`
	WorkspaceRoots []string `json:"workspace_roots"`
	Prompt         string   `json:"prompt"`
}

// CursorOutput is the beforeSubmitPrompt response.
type CursorOutput struct {
	Continue    bool   `json:"continue"`
	UserMessage string `json:"user_message,omitempty"`
}

// Cursor handles a Cursor beforeSubmitPrompt hook.
func Cursor(ctx context.Context, r io.Reader, w io.Writer, open app.Opener) error {
	p := readPayload(r)
	if p.oversized {
		if p.event != "" && p.event != "beforeSubmitPrompt" {
			return writeJSON(w, CursorOutput{Continue: true})
		}
		if _, ok := capture.Match(p.prompt); ok {
			return writeJSON(w, CursorOutput{Continue: false, UserMessage: tooLarge(p.prompt)})
		}
		return writeJSON(w, CursorOutput{Continue: true})
	}
	var in CursorInput
	if err := json.Unmarshal(p.data, &in); err != nil {
		return writeJSON(w, CursorOutput{Continue: true})
	}
	if in.HookEventName != "" && in.HookEventName != "beforeSubmitPrompt" {
		return writeJSON(w, CursorOutput{Continue: true})
	}
	text, ok := capture.Match(in.Prompt)
	if !ok {
		return writeJSON(w, CursorOutput{Continue: true})
	}

	meta := map[string]any{}
	if in.GenerationID != "" {
		meta["generation_id"] = in.GenerationID
	}
	var cwd string
	switch len(in.WorkspaceRoots) {
	case 0:
	case 1:
		cwd = in.WorkspaceRoots[0]
	default:
		// Several roots: record them all rather than guess which one is meant.
		meta["workspace_roots"] = in.WorkspaceRoots
	}
	msg := save(ctx, open, app.CaptureRequest{
		Text:      text,
		Source:    "cursor",
		SessionID: in.ConversationID,
		Cwd:       cwd,
		Metadata:  meta,
	}, in.Prompt)
	return writeJSON(w, CursorOutput{Continue: false, UserMessage: msg})
}
