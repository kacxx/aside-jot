package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Record shapes below are copied from real Claude Code transcripts: prompts
// and slash commands are string content, interrupts are a text block, and a
// skill's expanded prompt is an isMeta record.
func TestReplyBeforeTurnBoundaries(t *testing.T) {
	const (
		prompt    = `{"type":"user","timestamp":"2026-09-30T11:00:00Z","message":{"role":"user","content":"fix the cache"}}`
		old       = `{"type":"assistant","timestamp":"2026-09-30T11:00:01Z","message":{"id":"a0","role":"assistant","content":[{"type":"text","text":"Old answer."}]}}`
		found     = `{"type":"assistant","timestamp":"2026-09-30T11:00:02Z","message":{"id":"a1","role":"assistant","content":[{"type":"text","text":"Found the bug."}]}}`
		toolUse   = `{"type":"assistant","timestamp":"2026-09-30T11:00:03Z","message":{"id":"a1","role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Bash"}]}}`
		interrupt = `{"type":"user","timestamp":"2026-09-30T11:00:04Z","message":{"role":"user","content":[{"type":"text","text":"[Request interrupted by user for tool use]"}]}}`
		notify    = `{"type":"user","timestamp":"2026-09-30T11:00:04Z","message":{"role":"user","content":"<task-notification>\n<task-id>x</task-id>\n</task-notification>"}}`
		model     = `{"type":"user","timestamp":"2026-09-30T11:00:04Z","message":{"role":"user","content":"<command-name>/model</command-name>\n            <command-message>model</command-message>"}}`
		modelOut  = `{"type":"user","timestamp":"2026-09-30T11:00:05Z","message":{"role":"user","content":"<local-command-stdout>Set model to x</local-command-stdout>"}}`
		skill     = `{"type":"user","timestamp":"2026-09-30T11:00:04Z","message":{"role":"user","content":"<command-message>code-review</command-message>\n<command-name>/code-review</command-name>"}}`
		expanded  = `{"type":"user","timestamp":"2026-09-30T11:00:05Z","isMeta":true,"message":{"role":"user","content":[{"type":"text","text":"Review the code."}]}}`
		later     = `{"type":"assistant","timestamp":"2026-09-30T11:00:06Z","message":{"id":"a2","role":"assistant","content":[{"type":"text","text":"Later answer."}]}}`
		notJot    = `{"type":"user","timestamp":"2026-09-30T11:00:04Z","message":{"role":"user","content":" >> leading space is a prompt"}}`
		jot       = `{"type":"user","timestamp":"2026-09-30T11:00:04Z","message":{"role":"user","content":">> a jot"}}`
		bash      = `{"type":"user","timestamp":"2026-09-30T11:00:04Z","message":{"role":"user","content":"<bash-input>git status</bash-input><bash-stdout>clean</bash-stdout><bash-stderr></bash-stderr>"}}`
		compact   = `{"type":"user","timestamp":"2026-09-30T11:00:04Z","message":{"role":"user","content":"<command-name>/compact</command-name>\n            <command-message>compact</command-message>"}}`
		summary   = `{"type":"user","timestamp":"2026-09-30T11:00:05Z","isCompactSummary":true,"isVisibleInTranscriptOnly":true,"message":{"role":"user","content":"This session is being continued from a previous conversation that ran out of context."}}`
		compacted = `{"type":"user","timestamp":"2026-09-30T11:00:05Z","message":{"role":"user","content":"<local-command-stdout>Compacted</local-command-stdout>"}}`
		reminded  = `{"type":"user","timestamp":"2026-09-30T11:00:04Z","message":{"role":"user","content":[{"type":"text","text":"<system-reminder>\nYou are operating in a git worktree.\n</system-reminder>"},{"type":"text","text":"now fix the tests"}]}}`
		reminder  = `{"type":"user","timestamp":"2026-09-30T11:00:04Z","message":{"role":"user","content":"<system-reminder>\nThe user started a background task.\n</system-reminder>"}}`
		image     = `{"type":"user","timestamp":"2026-09-30T11:00:04Z","message":{"role":"user","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AA=="}}]}}`
	)
	cases := []struct {
		name    string
		records []string
		want    string
	}{
		{"interrupt keeps the interrupted turn", []string{prompt, found, toolUse, interrupt}, "Found the bug."},
		{"task notification is mid-turn", []string{prompt, found, notify, later}, "Found the bug.\n\nLater answer."},
		{"local command gets no reply", []string{prompt, found, model, modelOut}, "Found the bug."},
		{"answered skill starts a turn", []string{prompt, old, skill, expanded, later}, "Later answer."},
		{"prompt that only looks like a jot", []string{prompt, old, notJot, later}, "Later answer."},
		{"jot is not a prompt", []string{prompt, found, jot}, "Found the bug."},
		{"unanswered ! command", []string{prompt, found, bash}, "Found the bug."},
		{"answered ! command starts a turn", []string{prompt, old, bash, later}, "Later answer."},
		{"compaction summary is not a prompt", []string{prompt, found, summary, later}, "Found the bug.\n\nLater answer."},
		{"jot right after /compact", []string{prompt, found, compact, summary, compacted}, "Found the bug."},
		{"system reminder before a prompt", []string{prompt, old, reminded, later}, "Later answer."},
		{"system reminder on its own is mid-turn", []string{prompt, found, reminder, later}, "Found the bug.\n\nLater answer."},
		{"image-only prompt starts a turn", []string{prompt, old, image, later}, "Later answer."},
		{"local command output ends the pending command", []string{prompt, found, model, modelOut, notify, later}, "Found the bug.\n\nLater answer."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "t.jsonl")
			if err := os.WriteFile(path, []byte(strings.Join(c.records, "\n")+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := replyBefore(path, time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC))
			if err != nil || got != c.want {
				t.Fatalf("got %q, %v; want %q", got, err, c.want)
			}
		})
	}
}

func TestTruncateReplyClosesCodeBlock(t *testing.T) {
	for _, fence := range []string{"```", "~~~~"} {
		long := "Here:\n\n" + fence + "go\n" + strings.Repeat("x", maxReplyRunes)
		got := truncateReply(long)
		before, _, ok := strings.Cut(got, "_[reply cut")
		if !ok || !strings.HasSuffix(strings.TrimSpace(before), "\n"+fence) {
			t.Fatalf("fence %q not closed before the note: %.40q", fence, got[len(got)-120:])
		}
	}
	closed := "```\ncode\n```  \n" + strings.Repeat("y", maxReplyRunes)
	if got := truncateReply(closed); strings.Count(got, "```") != 2 {
		t.Fatalf("closed block got another fence")
	}
}

func TestOpenFence(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"inline code line opens nothing", "```x``` is inline\nmore", ""},
		{"backtick fence with info string", "```go\ncode", "```"},
		{"tilde info string may hold backticks", "~~~ `x`\ncode", "~~~"},
		{"list item fence keeps its indent", "- step\n\n  ```sh\n  run", "  ```"},
		{"four spaces is an indented code block", "    ```\ncode", ""},
		{"four-space line does not close", "```\ncode\n    ```\nmore", "```"},
	}
	for _, c := range cases {
		if got := openFence(c.in); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestPromoteConfirm(t *testing.T) {
	ctx := context.Background()
	newRunner := func() *fakeRunner {
		return &fakeRunner{remote: "https://github.com/o/n.git", url: "https://github.com/o/n/issues/1"}
	}

	svc := promoteService(t, repoInfo)
	f := newRunner()
	_, err := svc.Promote(ctx, f, PromoteRequest{ID: 1, Confirm: func(Promotion) error { return ErrNotConfirmed }})
	if !errors.Is(err, ErrNotConfirmed) {
		t.Fatalf("declined: err = %v", err)
	}
	for _, c := range f.calls {
		if strings.HasPrefix(c, "gh issue") {
			t.Fatalf("declined promote created an issue: %q", c)
		}
	}
	if e, _ := svc.store.Get(ctx, 1); e.IssueURL != "" {
		t.Fatalf("declined promote recorded %q", e.IssueURL)
	}

	var shown Promotion
	f = newRunner()
	p, err := svc.Promote(ctx, f, PromoteRequest{ID: 1, Confirm: func(p Promotion) error { shown = p; return nil }})
	if err != nil {
		t.Fatal(err)
	}
	if shown.Body == "" || f.stdin[len(f.stdin)-1] != shown.Body || p.URL == "" {
		t.Fatalf("posted %q, confirmed %q", f.stdin[len(f.stdin)-1], shown.Body)
	}

	called := false
	if _, err := promoteService(t, repoInfo).Promote(ctx, newRunner(), PromoteRequest{ID: 1, DryRun: true,
		Confirm: func(Promotion) error { called = true; return nil }}); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("Confirm called on a dry run")
	}
}
