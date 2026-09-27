package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIFlow(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("JOT_DB", filepath.Join(dir, "jot.db"))
	sh := func(stdin string, args ...string) string {
		t.Helper()
		var out bytes.Buffer
		if err := run(args, strings.NewReader(stdin), &out); err != nil {
			t.Fatalf("jot %v: %v", args, err)
		}
		return out.String()
	}

	if got := sh("", "add", "remember", "the", "milk"); got != "✓ Jotted #1\n" {
		t.Fatalf("add: %q", got)
	}
	if got := sh("from stdin\n", "add"); got != "✓ Jotted #2\n" {
		t.Fatalf("add stdin: %q", got)
	}
	if got := sh("", "inbox", "-n", "1"); !strings.Contains(got, "#2") || strings.Contains(got, "#1 ") {
		t.Fatalf("inbox -n 1: %q", got)
	}
	if got := sh("", "show", "#1"); !strings.Contains(got, "remember the milk") || !strings.Contains(got, "source:    cli") {
		t.Fatalf("show: %q", got)
	}
	if got := sh("", "search", "MILK"); !strings.Contains(got, "#1") {
		t.Fatalf("search: %q", got)
	}
	sh("", "done", "1")
	if got := sh("", "inbox"); strings.Contains(got, "milk") {
		t.Fatalf("inbox after done: %q", got)
	}
	bk := filepath.Join(dir, "b.db")
	sh("", "backup", bk)
	if err := run([]string{"backup", bk}, nil, &bytes.Buffer{}); err == nil {
		t.Fatal("backup must refuse to overwrite")
	}
	if err := run([]string{"show", "99"}, nil, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "no jot #99") {
		t.Fatalf("show missing: %v", err)
	}
	if got := sh("", "paths"); !strings.Contains(got, "jot.db ($JOT_DB)") {
		t.Fatalf("paths: %q", got)
	}
}

func TestHookAlwaysExitsZero(t *testing.T) {
	t.Setenv("JOT_DB", filepath.Join(t.TempDir(), "file", "jot.db"))
	// A regular file where the data directory should be makes the DB unopenable.
	if err := os.WriteFile(filepath.Dir(os.Getenv("JOT_DB")), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{{}, {"nope"}, {"claude"}, {"cursor"}} {
		var out, errb bytes.Buffer
		if code := runHook(args, strings.NewReader(`{"prompt":">> x"}`), &out, &errb); code != 0 {
			t.Errorf("hook %v exited %d", args, code)
		}
		if len(args) == 1 && (args[0] == "claude" || args[0] == "cursor") && !strings.Contains(out.String(), "NOT saved") {
			t.Errorf("hook %v: %q", args, out.String())
		}
	}
}
