package hooks

import (
	"context"
	"io"

	"github.com/kacxx/aside-jot/internal/app"
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

// Cursor handles a Cursor beforeSubmitPrompt hook. Unlike Claude Code and
// Codex, Cursor expects a reply on every prompt, so pass-through answers
// {"continue":true}.
func Cursor(ctx context.Context, r io.Reader, w io.Writer, open app.Opener) error {
	return promptHook{
		event: "beforeSubmitPrompt",
		pass:  CursorOutput{Continue: true},
		block: func(reason string) any {
			return CursorOutput{Continue: false, UserMessage: reason}
		},
		parse: parseCursor,
	}.run(ctx, r, w, open)
}

func parseCursor(data []byte) (event, prompt string, build func() app.CaptureRequest, err error) {
	var in CursorInput
	if err := decodePayload(data, &in); err != nil {
		return "", "", nil, err
	}
	build = func() app.CaptureRequest {
		meta := map[string]any{}
		if in.GenerationID != "" {
			meta["generation_id"] = in.GenerationID
		}
		// A wrong-typed element decodes as ""; don't record roots that weren't sent.
		roots := in.WorkspaceRoots[:0:0]
		for _, r := range in.WorkspaceRoots {
			if r != "" {
				roots = append(roots, r)
			}
		}
		var cwd string
		switch len(roots) {
		case 0:
		case 1:
			cwd = roots[0]
		default:
			// Several roots: record them all rather than guess which one is meant.
			meta["workspace_roots"] = roots
		}
		return app.CaptureRequest{
			Source:    "cursor",
			SessionID: in.ConversationID,
			Cwd:       cwd,
			Metadata:  meta,
		}
	}
	return in.HookEventName, in.Prompt, build, nil
}
