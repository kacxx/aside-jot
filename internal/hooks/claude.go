package hooks

import (
	"context"
	"io"

	"github.com/kacxx/aside-jot/internal/app"
)

// ClaudeBlock is the UserPromptSubmit output that blocks and erases a prompt.
type ClaudeBlock struct {
	Decision               string `json:"decision"`
	Reason                 string `json:"reason"`
	SuppressOriginalPrompt bool   `json:"suppressOriginalPrompt"`
}

// Claude handles a Claude Code UserPromptSubmit hook. Pass-through writes
// nothing (any stdout would be added to the model's context).
//
// Cursor imports Claude Code hooks and runs them on beforeSubmitPrompt. A
// silent reply there makes Cursor submit the prompt even when `aside hook
// cursor` blocked it, so that event is handled exactly as `aside hook cursor`
// would handle it.
func Claude(ctx context.Context, r io.Reader, w io.Writer, open app.Opener) error {
	return promptHook{
		event: "UserPromptSubmit",
		alt:   &cursorHook,
		block: func(reason string) any {
			return ClaudeBlock{Decision: "block", Reason: reason, SuppressOriginalPrompt: true}
		},
		parse: parsePrompt("claude", func(in PromptInput) map[string]any {
			if in.TranscriptPath == "" {
				return nil
			}
			return map[string]any{"transcript_path": in.TranscriptPath}
		}),
	}.run(ctx, r, w, open)
}
