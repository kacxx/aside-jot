package main

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kacxx/aside-jot/internal/app"
)

func TestOpenCLI(t *testing.T) {
	t.Setenv("JOT_DB", filepath.Join(t.TempDir(), "jot.db"))
	const thread = "11111111-1111-4111-8111-111111111111"
	svc, err := app.OpenDefault()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, req := range []app.CaptureRequest{
		{Text: "codex jot", Source: "codex", SessionID: thread},
		{Text: "claude jot", Source: "claude", SessionID: thread},
		{Text: "terminal jot", Source: "cli"},
	} {
		if _, err := svc.Capture(ctx, req); err != nil {
			t.Fatal(err)
		}
	}
	svc.Close()

	var opened []string
	oldOS, oldOpen := openGOOS, openURL
	t.Cleanup(func() { openGOOS, openURL = oldOS, oldOpen })
	openURL = func(_ context.Context, u string) error { opened = append(opened, u); return nil }
	do := func(args ...string) (string, error) {
		var out bytes.Buffer
		err := run(args, strings.NewReader(""), &out)
		return out.String(), err
	}

	openGOOS = "darwin"
	out, err := do("open", "1")
	if err != nil || out != "opened codex://threads/"+thread+"\n" || len(opened) != 1 || opened[0] != "codex://threads/"+thread {
		t.Fatalf("out=%q err=%v opened=%v", out, err, opened)
	}
	// No link (a Claude jot not from Desktop): nothing opened, resume command shown.
	_, err = do("open", "2")
	if err == nil || !strings.Contains(err.Error(), "no link") || !strings.Contains(err.Error(), "claude --resume "+thread) || len(opened) != 1 {
		t.Fatalf("no link: %v opened=%v", err, opened)
	}
	// No session at all.
	if _, err = do("open", "3"); err == nil || !strings.Contains(err.Error(), "no link") || strings.Contains(err.Error(), "run:") {
		t.Fatalf("no session: %v", err)
	}
	if _, err = do("open", "99"); err == nil || !strings.Contains(err.Error(), "no jot #99") {
		t.Fatalf("missing: %v", err)
	}
	if _, err = do("open"); err == nil {
		t.Fatal("open with no id succeeded")
	}
	// Other platforms say so and show the link.
	openGOOS = "linux"
	if _, err = do("open", "1"); err == nil || !strings.Contains(err.Error(), "only supported on macOS") || len(opened) != 1 {
		t.Fatalf("linux: %v opened=%v", err, opened)
	}
	// A failing opener is reported.
	openGOOS = "darwin"
	openURL = func(context.Context, string) error { return errors.New("boom") }
	if _, err = do("open", "1"); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("open failure: %v", err)
	}
}
