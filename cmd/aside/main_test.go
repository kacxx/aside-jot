package main

import (
	"bytes"
	"os"
	"os/exec"
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
			t.Fatalf("aside %v: %v", args, err)
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

	for _, args := range [][]string{{}, {"nope"}, {"claude"}, {"codex"}, {"cursor"}} {
		var out, errb bytes.Buffer
		if code := runHook(args, strings.NewReader(`{"prompt":">> x"}`), &out, &errb); code != 0 {
			t.Errorf("hook %v exited %d", args, code)
		}
		if len(args) == 1 && args[0] != "nope" && !strings.Contains(out.String(), "NOT saved") {
			t.Errorf("hook %v: %q", args, out.String())
		}
	}
}

// The backup message reports the absolute path written, not the argument.
func TestBackupReportsAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("JOT_DB", filepath.Join(dir, "jot.db"))
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(sub)
	var out bytes.Buffer
	if err := run([]string{"backup", filepath.Join("..", "b.db")}, nil, &out); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "b.db")
	if got := strings.TrimSpace(out.String()); got != "✓ Backed up to "+want {
		t.Fatalf("got %q, want path %q", got, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatal(err)
	}
}

// paths warns when a bare "aside" would not run this binary, as on macOS
// where an unrelated tool could shadow it (issue #6).
func TestPathWarning(t *testing.T) {
	dir := t.TempDir()
	self := filepath.Join(dir, "self")
	other := filepath.Join(dir, "other")
	for _, p := range []string{self, other} {
		if err := os.WriteFile(p, nil, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	look := func(found string, err error) func(string) (string, error) {
		return func(name string) (string, error) {
			if name != "aside" {
				t.Errorf("looked up %q", name)
			}
			return found, err
		}
	}

	if got := pathWarning(self, look(self, nil)); got != "" {
		t.Errorf("same binary: %q", got)
	}
	if got := pathWarning(self, look(other, nil)); !strings.Contains(got, other) || !strings.Contains(got, "use "+self) {
		t.Errorf("shadowed: %q", got)
	}
	if got := pathWarning(self, look("", exec.ErrNotFound)); !strings.Contains(got, "not on PATH") || !strings.Contains(got, self) {
		t.Errorf("missing: %q", got)
	}
	// A file that can't be stat'ed is reported as unchecked, not as a
	// different binary.
	gone := filepath.Join(dir, "gone")
	if got := pathWarning(self, look(gone, nil)); !strings.Contains(got, "could not check 'aside' on PATH") || !strings.Contains(got, "use "+self) {
		t.Errorf("PATH entry vanished: %q", got)
	}
	if got := pathWarning(gone, look(self, nil)); !strings.Contains(got, "could not check this binary") || !strings.Contains(got, "use "+gone) {
		t.Errorf("binary vanished: %q", got)
	}
}
