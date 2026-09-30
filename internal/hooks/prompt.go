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

// promptHook adapts one agent's prompt-submit hook. run owns the gating
// sequence shared by every agent; the adapter only decodes its payload and
// shapes its replies.
type promptHook struct {
	event string                  // the hook_event_name this hook handles
	pass  any                     // pass-through reply; nil writes nothing
	block func(reason string) any // the agent's blocking reply

	// parse decodes a payload that fits in memory into its event and prompt,
	// plus a build func that constructs the request to save (Text is filled in
	// by run). build is called only once the prompt is a confirmed jot, so its
	// metadata allocation stays off the path ordinary prompts take. An error
	// means a malformed payload.
	parse func(data []byte) (event, prompt string, build func() app.CaptureRequest, err error)
}

// eventMatches reports whether a payload's hook_event_name is one this hook
// should act on. An empty event (the field wasn't sent) is accepted leniently.
func (h promptHook) eventMatches(event string) bool {
	return event == "" || event == h.event
}

func (h promptHook) run(ctx context.Context, r io.Reader, w io.Writer, open app.Opener) error {
	p := readPayload(r)
	if p.oversized {
		if !h.eventMatches(p.event) {
			return h.passThrough(w)
		}
		if _, ok := capture.Match(p.prompt); ok {
			return writeJSON(w, h.block(tooLarge(p.prompt)))
		}
		return h.passThrough(w)
	}
	event, prompt, build, err := h.parse(p.data)
	if err != nil {
		return h.passThrough(w)
	}
	if !h.eventMatches(event) {
		return h.passThrough(w)
	}
	text, ok := capture.Match(prompt)
	if !ok {
		return h.passThrough(w)
	}
	// Build the request (and its metadata) only now that the prompt is a
	// confirmed jot; an ordinary prompt pays no request or metadata allocation.
	req := build()
	req.Text = text
	return writeJSON(w, h.block(save(ctx, open, req, prompt)))
}

func (h promptHook) passThrough(w io.Writer) error {
	if h.pass == nil {
		return nil
	}
	return writeJSON(w, h.pass)
}

// parsePrompt returns a promptHook parse function for the UserPromptSubmit
// payload shared by Claude Code and Codex.
func parsePrompt(source string, meta func(in PromptInput) map[string]any) func([]byte) (string, string, func() app.CaptureRequest, error) {
	return func(data []byte) (string, string, func() app.CaptureRequest, error) {
		var in PromptInput
		if err := decodePayload(data, &in); err != nil {
			return "", "", nil, err
		}
		build := func() app.CaptureRequest {
			return app.CaptureRequest{
				Source:    source,
				SessionID: in.SessionID,
				Cwd:       in.Cwd,
				Metadata:  meta(in),
			}
		}
		return in.HookEventName, in.Prompt, build, nil
	}
}
