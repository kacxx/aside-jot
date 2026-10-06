package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kacxx/aside-jot/internal/setup"
)

// setupEnv points the agents' config at a throwaway home and pretends aside
// was invoked as bin.
func setupEnv(t *testing.T) (home, bin string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CODEX_HOME", "")
	bin, _ = filepath.Abs(filepath.Join("testdata-bin", "aside"))
	old, oldOS := setupBinary, setupGOOS
	setupBinary = func() setup.Binary {
		return setup.Binary{Arg0: bin, TempDir: filepath.Join(home, "no-temp")}
	}
	setupGOOS = "linux"
	t.Cleanup(func() { setupBinary, setupGOOS = old, oldOS })
	return home, bin
}

func runSetup(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := run(append([]string{"setup"}, args...), strings.NewReader(""), &out)
	return out.String(), err
}

func TestSetupClaude(t *testing.T) {
	home, bin := setupEnv(t)
	path := filepath.Join(home, ".claude", "settings.json")

	out, err := runSetup(t, "claude", "--dry-run", "--mcp")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("--dry-run wrote the file")
	}
	for _, want := range []string{"Dry run", bin + " hook claude", "docs/cursor.md", "claude mcp add --scope user aside -- " + bin + " mcp", "not run"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry run output lacks %q:\n%s", want, out)
		}
	}

	out, err = runSetup(t, "claude")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), bin+" hook claude") {
		t.Fatalf("settings.json:\n%s", b)
	}
	if !strings.Contains(out, "Added") || !strings.Contains(out, ">> test") || !strings.Contains(out, "Cursor imports hooks") {
		t.Errorf("output:\n%s", out)
	}
	if strings.Contains(out, "claude mcp add") {
		t.Errorf("printed the MCP command without --mcp:\n%s", out)
	}

	// Running again changes nothing and makes no backup.
	out, err = runSetup(t, "claude")
	if err != nil || !strings.Contains(out, "nothing changed") {
		t.Fatalf("second run: %v\n%s", err, out)
	}
	if m, _ := filepath.Glob(path + ".bak-*"); len(m) != 0 {
		t.Errorf("backups after a no-op: %v", m)
	}
}

func TestSetupCodexTrustReminder(t *testing.T) {
	home, bin := setupEnv(t)
	path := filepath.Join(home, ".codex", "hooks.json")

	out, err := runSetup(t, "codex")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "/hooks") || !strings.Contains(out, "trust") {
		t.Errorf("first install doesn't mention trusting the hook:\n%s", out)
	}
	if strings.Contains(out, "Cursor") {
		t.Errorf("codex output mentions Cursor:\n%s", out)
	}

	// The path changes: trusting is needed again.
	old := setupBinary
	other := filepath.Join(filepath.Dir(bin), "moved", "aside")
	setupBinary = func() setup.Binary { b := old(); b.Arg0 = other; return b }
	out, err = runSetup(t, "codex", "--mcp")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Updated", "replaced: " + bin + " hook codex", "trust it", "again", "codex mcp add aside -- " + other + " mcp"} {
		if !strings.Contains(out, want) {
			t.Errorf("update output lacks %q:\n%s", want, out)
		}
	}
	if m, _ := filepath.Glob(path + ".bak-*"); len(m) != 1 {
		t.Errorf("want one backup, got %v", m)
	}
	// The same path again: nothing to trust.
	out, _ = runSetup(t, "codex")
	if strings.Contains(out, "trust it") {
		t.Errorf("no-op run asks to trust again:\n%s", out)
	}
}

func TestSetupQuotesPathWithSpace(t *testing.T) {
	home, _ := setupEnv(t)
	spaced := filepath.Join(home, "John Smith", "go", "bin", "aside")
	setupBinary = func() setup.Binary { return setup.Binary{Arg0: spaced, TempDir: filepath.Join(home, "no-temp")} }
	out, err := runSetup(t, "claude", "--mcp")
	if err != nil {
		t.Fatal(err)
	}
	q := setup.Quote(spaced, false)
	if !strings.HasPrefix(q, "'") || !strings.Contains(out, "command: "+q+" hook claude") || !strings.Contains(out, "-- "+q+" mcp") {
		t.Errorf("path not quoted (want %s):\n%s", q, out)
	}
}

func TestSetupCursorWritesNothing(t *testing.T) {
	home, _ := setupEnv(t)
	out, err := runSetup(t, "cursor")
	if err != nil || !strings.Contains(out, "docs/cursor.md") || !strings.Contains(out, "isn't supported") {
		t.Fatalf("%v\n%s", err, out)
	}
	if m, _ := filepath.Glob(filepath.Join(home, ".*")); len(m) != 0 {
		t.Errorf("wrote %v", m)
	}
}

func TestSetupErrors(t *testing.T) {
	home, _ := setupEnv(t)
	for _, args := range [][]string{{}, {"vim"}, {"claude", "codex"}, {"claude", "--nope"}} {
		if _, err := runSetup(t, args...); err == nil {
			t.Errorf("aside setup %v: want an error", args)
		}
	}
	// A config that doesn't parse is left alone.
	path := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{ nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runSetup(t, "claude"); err == nil || !strings.Contains(err.Error(), "settings.json") {
		t.Errorf("err = %v, want one naming the file", err)
	}
	if b, _ := os.ReadFile(path); string(b) != "{ nope" {
		t.Errorf("file changed: %q", b)
	}
	// A temporary binary is refused.
	setupBinary = func() setup.Binary { return setup.Binary{Arg0: filepath.Join(os.TempDir(), "go-build1", "aside")} }
	if _, err := runSetup(t, "claude"); err == nil || !strings.Contains(err.Error(), "go install") {
		t.Errorf("err = %v, want a go install hint", err)
	}
}
