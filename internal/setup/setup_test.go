package setup

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestMain lets the test binary stand in for aside in TestQuotedCommandRuns.
func TestMain(m *testing.M) {
	if os.Getenv("ASIDE_SETUP_HELPER") == "1" {
		os.Stdout.WriteString("helper ran: " + strings.Join(os.Args[1:], " "))
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func merge(t *testing.T, in string, h Hook) (string, Change) {
	t.Helper()
	out, ch, err := Merge([]byte(in), h)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	return string(out), ch
}

var claude = Hook{Agent: "claude", Command: "/bin/aside hook claude"}

func TestMergeEmpty(t *testing.T) {
	want := `{
  "hooks": {
    "UserPromptSubmit": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "/bin/aside hook claude"
          }
        ]
      }
    ]
  }
}
`
	for _, in := range []string{"", "  \n", "{}", "\xef\xbb\xbf{}"} {
		got, ch := merge(t, in, claude)
		if got != want || ch.Kind != "added" {
			t.Errorf("Merge(%q) = %q, %+v", in, got, ch)
		}
	}
}

func TestMergeKeepsOtherContentAndOrder(t *testing.T) {
	in := `{
  "z": 1.50,
  "permissions": {"allow": ["Bash(ls)"]},
  "hooks": {
    "Stop": [{"hooks": [{"type": "command", "command": "notify"}]}],
    "UserPromptSubmit": [
      {"matcher": "", "hooks": [{"type": "command", "command": "other-tool"}]}
    ],
    "A": []
  },
  "a": "<&>"
}`
	got, ch := merge(t, in, claude)
	if ch.Kind != "added" {
		t.Fatalf("change = %+v", ch)
	}
	// Existing keys keep their order, and the other hook is untouched.
	order := []string{`"z": 1.50`, `"permissions"`, `"hooks"`, `"Stop"`, `"UserPromptSubmit"`, `"other-tool"`, `/bin/aside hook claude`, `"A"`, `"a": "<&>"`}
	last := -1
	for _, s := range order {
		i := strings.Index(got, s)
		if i < 0 || i < last {
			t.Fatalf("%q missing or out of order in:\n%s", s, got)
		}
		last = i
	}
	if strings.Count(got, "aside hook") != 1 {
		t.Errorf("want one aside hook:\n%s", got)
	}
}

func TestMergeUpdatesInPlace(t *testing.T) {
	in := `{"hooks":{"UserPromptSubmit":[{"hooks":[{"type":"command","command":"/old/path/aside hook claude","timeout":5}]}]}}`
	got, ch := merge(t, in, claude)
	if ch.Kind != "updated" || len(ch.Replaced) != 1 || ch.Replaced[0] != "/old/path/aside hook claude" {
		t.Fatalf("change = %+v", ch)
	}
	if strings.Count(got, "aside hook") != 1 || strings.Contains(got, "/old/path") || !strings.Contains(got, `"timeout": 5`) {
		t.Errorf("got:\n%s", got)
	}
}

func TestMergeUnchanged(t *testing.T) {
	in := `{"hooks":{"UserPromptSubmit":[{"hooks":[{"type":"command","command":"/bin/aside hook claude"}]}]}}`
	_, ch := merge(t, in, claude)
	if ch.Kind != "unchanged" {
		t.Errorf("change = %+v", ch)
	}
}

func TestMergeReplacesOldJotEntries(t *testing.T) {
	for _, agent := range []string{"claude", "codex"} {
		for _, old := range []string{
			"jot hook " + agent,
			"/Users/x/go/bin/jot hook " + agent,
			`"C:\Users\John Smith\go\bin\jot.exe" hook ` + agent,
			"'/o'\\''brien/go/bin/aside' hook " + agent,
		} {
			in := `{"hooks":{"UserPromptSubmit":[{"hooks":[{"type":"command","command":` + string(jsonString(old)) + `}]}]}}`
			h := Hook{Agent: agent, Command: "/bin/aside hook " + agent}
			got, ch := merge(t, in, h)
			if ch.Kind != "updated" || strings.Count(got, " hook ") != 1 || !strings.Contains(got, h.Command) {
				t.Errorf("%q: change %+v, got:\n%s", old, ch, got)
			}
		}
	}
}

func TestMergeLeavesOtherAgentsAndPrograms(t *testing.T) {
	// A codex hook is not replaced by claude's, and a program that only
	// looks similar is left alone.
	in := `{"hooks":{"UserPromptSubmit":[{"hooks":[
	  {"type":"command","command":"/bin/aside hook codex"},
	  {"type":"command","command":"/bin/asides hook claude"},
	  {"type":"command","command":"/bin/aside hook claude --extra"}]}]}}`
	got, ch := merge(t, in, claude)
	if ch.Kind != "added" || strings.Count(got, `"command": "`) != 4 {
		t.Errorf("change %+v, got:\n%s", ch, got)
	}
}

func TestMergeRemovesDuplicates(t *testing.T) {
	in := `{"hooks":{"UserPromptSubmit":[
	  {"hooks":[{"type":"command","command":"jot hook claude"}]},
	  {"hooks":[{"type":"command","command":"/x/aside hook claude"},{"type":"command","command":"keep"}]}]}}`
	got, ch := merge(t, in, claude)
	if ch.Kind != "updated" || ch.Removed != 1 {
		t.Fatalf("change %+v", ch)
	}
	if strings.Count(got, "aside hook") != 1 || !strings.Contains(got, `"keep"`) {
		t.Errorf("got:\n%s", got)
	}
	// The first match is updated in place; the later one is removed.
	if strings.Index(got, "/bin/aside hook claude") > strings.Index(got, `"keep"`) {
		t.Errorf("updated entry should stay first:\n%s", got)
	}
	if strings.Contains(got, "jot hook") {
		t.Errorf("old entry kept:\n%s", got)
	}
}

func TestMergeCodexWindows(t *testing.T) {
	h := Hook{Agent: "codex", Command: "C:/bin/aside.exe hook codex", Windows: true}
	got, _ := merge(t, "{}", h)
	if !strings.Contains(got, `"commandWindows": "C:/bin/aside.exe hook codex"`) || !strings.Contains(got, `"command": "C:/bin/aside.exe hook codex"`) {
		t.Errorf("got:\n%s", got)
	}
	// On another platform an existing commandWindows is left alone.
	in := `{"hooks":{"UserPromptSubmit":[{"hooks":[{"type":"command","command":"/old/aside hook codex","commandWindows":"C:/keep/aside.exe hook codex"}]}]}}`
	got, ch := merge(t, in, Hook{Agent: "codex", Command: "/new/aside hook codex"})
	if ch.Kind != "updated" || !strings.Contains(got, "C:/keep/aside.exe") || !strings.Contains(got, "/new/aside hook codex") {
		t.Errorf("change %+v, got:\n%s", ch, got)
	}
}

func TestMergeRejects(t *testing.T) {
	for name, in := range map[string]string{
		"invalid":       `{"hooks": `,
		"comment":       "{} // c",
		"array":         `[]`,
		"hooks string":  `{"hooks": "x"}`,
		"event object":  `{"hooks": {"UserPromptSubmit": {}}}`,
		"duplicate key": `{"a": 1, "a": 2}`,
	} {
		if _, _, err := Merge([]byte(in), claude); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestInstall(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "settings.json")
	now := time.Date(2026, 10, 6, 6, 46, 50, 0, time.UTC)

	// Dry run writes nothing, not even the directory.
	res, err := Install(path, claude, true, now)
	if err != nil || res.Wrote || len(res.New) == 0 {
		t.Fatalf("dry run: %+v, %v", res, err)
	}
	if _, err := os.Stat(filepath.Dir(path)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("dry run created the directory")
	}

	// A new file needs no backup.
	if res, err = Install(path, claude, false, now); err != nil || !res.Wrote || res.Backup != "" {
		t.Fatalf("new file: %+v, %v", res, err)
	}

	// An existing one is backed up first, with its previous contents.
	before, _ := os.ReadFile(path)
	h2 := Hook{Agent: "claude", Command: "/other/aside hook claude"}
	if res, err = Install(path, h2, false, now); err != nil || !res.Wrote {
		t.Fatalf("update: %+v, %v", res, err)
	}
	if !strings.HasSuffix(res.Backup, "settings.json.bak-20261006-064650") {
		t.Errorf("backup = %q", res.Backup)
	}
	if b, _ := os.ReadFile(res.Backup); string(b) != string(before) {
		t.Errorf("backup holds %q, want %q", b, before)
	}
	// A second backup in the same second doesn't overwrite the first.
	h3 := Hook{Agent: "claude", Command: "/third/aside hook claude"}
	res2, err := Install(path, h3, false, now)
	if err != nil || res2.Backup == res.Backup {
		t.Fatalf("second backup: %+v, %v", res2, err)
	}

	// No change: no write, no backup.
	if res, err = Install(path, h3, false, now.Add(time.Hour)); err != nil || res.Wrote || res.Backup != "" || res.Change.Kind != "unchanged" {
		t.Fatalf("unchanged: %+v, %v", res, err)
	}
}

func TestInstallUnparseableLeftUntouched(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	bad := "{ // my settings\n}"
	must(t, os.WriteFile(path, []byte(bad), 0o600))
	_, err := Install(path, claude, false, time.Now())
	if err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("err = %v, want one naming the file", err)
	}
	if b, _ := os.ReadFile(path); string(b) != bad {
		t.Errorf("file changed to %q", b)
	}
	if m, _ := filepath.Glob(path + ".*"); len(m) != 0 {
		t.Errorf("left files behind: %v", m)
	}
}

func TestInstallThroughSymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "dotfiles", "settings.json")
	link := filepath.Join(dir, "settings.json")
	must(t, os.MkdirAll(filepath.Dir(real), 0o755))
	must(t, os.WriteFile(real, []byte(`{"a":1}`), 0o640))
	if err := os.Symlink(real, link); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	res, err := Install(link, claude, false, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Lstat(link); fi.Mode()&os.ModeSymlink == 0 {
		t.Error("the symlink was replaced")
	}
	if b, _ := os.ReadFile(real); !strings.Contains(string(b), "aside hook") {
		t.Errorf("target not updated: %s", b)
	}
	if filepath.Dir(res.Backup) != dir {
		t.Errorf("backup %q is not next to the link", res.Backup)
	}
	if runtime.GOOS != "windows" {
		if fi, _ := os.Stat(real); fi.Mode().Perm() != 0o640 {
			t.Errorf("mode = %v, want 0640", fi.Mode().Perm())
		}
	}
}

func TestConfigPath(t *testing.T) {
	t.Setenv("HOME", "/h")
	t.Setenv("USERPROFILE", "/h")
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CODEX_HOME", "")
	for agent, want := range map[string]string{
		"claude": filepath.Join("/h", ".claude", "settings.json"),
		"codex":  filepath.Join("/h", ".codex", "hooks.json"),
	} {
		if got, err := ConfigPath(agent); err != nil || got != want {
			t.Errorf("%s: %q, %v; want %q", agent, got, err, want)
		}
	}
	t.Setenv("CODEX_HOME", "/c")
	if got, _ := ConfigPath("codex"); got != filepath.Join("/c", "hooks.json") {
		t.Errorf("CODEX_HOME ignored: %q", got)
	}
	if _, err := ConfigPath("cursor"); err == nil {
		t.Error("want an error for cursor")
	}
}

func TestQuote(t *testing.T) {
	for _, c := range []struct {
		path    string
		windows bool
		want    string
	}{
		{"/Users/you/go/bin/aside", false, "/Users/you/go/bin/aside"},
		{"/Users/John Smith/go/bin/aside", false, "'/Users/John Smith/go/bin/aside'"},
		{"/o'brien/aside", false, `'/o'\''brien/aside'`},
		{`C:\Users\you\go\bin\aside.exe`, true, "C:/Users/you/go/bin/aside.exe"},
		{`C:\Users\John Smith\go\bin\aside.exe`, true, `"C:/Users/John Smith/go/bin/aside.exe"`},
	} {
		if got := Quote(c.path, c.windows); got != c.want {
			t.Errorf("Quote(%q, %v) = %s, want %s", c.path, c.windows, got, c.want)
		}
	}
}

// The command written for a path with a space has to run, on Windows too.
func TestQuotedCommandRuns(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "John Smith", "go bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	name := "aside"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	bin := filepath.Join(dir, name)
	data, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, data, 0o755); err != nil {
		t.Fatal(err)
	}
	shell := "sh"
	if runtime.GOOS == "windows" {
		shell = "bash" // Git Bash, which Claude Code uses for hooks there
	}
	sh, err := exec.LookPath(shell)
	if err != nil {
		t.Skipf("no %s on PATH", shell)
	}

	out, _, _ := Merge(nil, Hook{Agent: "claude", Command: Quote(bin, runtime.GOOS == "windows") + " hook claude"})
	cmd := extractCommand(t, string(out))
	if !strings.ContainsAny(cmd, `'"`) {
		t.Fatalf("path with a space was not quoted: %s", cmd)
	}
	c := exec.Command(sh, "-c", cmd)
	c.Env = append(os.Environ(), "ASIDE_SETUP_HELPER=1")
	got, err := c.CombinedOutput()
	if err != nil || string(got) != "helper ran: hook claude" {
		t.Fatalf("running %s: %q, %v", cmd, got, err)
	}
}

func extractCommand(t *testing.T, merged string) string {
	t.Helper()
	top, _ := parseObject([]byte(merged))
	hooks, _ := parseObject(top.vals["hooks"])
	var groups []json.RawMessage
	_ = json.Unmarshal(hooks.vals[event], &groups)
	g, _ := parseObject(groups[0])
	var hs []json.RawMessage
	_ = json.Unmarshal(g.vals["hooks"], &hs)
	e, _ := parseObject(hs[0])
	return stringField(e, "command")
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestMergeRecognisesOwnBinaryUnderAnyName(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "aside53")
	must(t, os.WriteFile(bin, []byte("x"), 0o755))
	h := Hook{Agent: "claude", Command: Quote(bin, false) + " hook claude", Binary: bin}
	cfg := "{}"
	for i := 0; i < 3; i++ {
		got, ch := merge(t, cfg, h)
		want := map[int]string{0: "added", 1: "unchanged", 2: "unchanged"}[i]
		if ch.Kind != want || strings.Count(got, "hook claude") != 1 {
			t.Fatalf("run %d: %+v\n%s", i, ch, got)
		}
		cfg = got
	}
	// Another name for the same file is the same binary, too.
	link := filepath.Join(dir, "aside-link")
	if err := os.Link(bin, link); err != nil {
		t.Skip("hard links unavailable:", err)
	}
	h2 := Hook{Agent: "claude", Command: Quote(link, false) + " hook claude", Binary: link}
	got, ch := merge(t, cfg, h2)
	if ch.Kind != "updated" || strings.Count(got, "hook claude") != 1 {
		t.Fatalf("%+v\n%s", ch, got)
	}
}

func TestMergeKeepsGroupWithOtherKeys(t *testing.T) {
	in := `{"hooks":{"UserPromptSubmit":[{"hooks":[{"type":"command","command":"/x/aside hook claude"},{"type":"command","command":"/y/aside hook claude"}],"note":"mine"}]}}`
	got, ch := merge(t, in, claude)
	if ch.Removed != 1 || !strings.Contains(got, `"note": "mine"`) {
		t.Errorf("%+v\n%s", ch, got)
	}
	// Emptied by removal: the group goes only if nothing else is in it.
	in = `{"hooks":{"UserPromptSubmit":[{"hooks":[{"type":"command","command":"/y/aside hook claude"}],"note":"mine"},{"matcher":"","hooks":[{"type":"command","command":"/x/aside hook claude"}]}]}}`
	got, _ = merge(t, in, claude)
	if !strings.Contains(got, `"note": "mine"`) {
		t.Errorf("group with another key was dropped:\n%s", got)
	}
}

func TestSectionShowsOnlyHooks(t *testing.T) {
	out, _ := merge(t, `{"env":{"API_KEY":"secret"},"hooks":{"Stop":[],"UserPromptSubmit":[]}}`, claude)
	sec, err := Section([]byte(out))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(sec), "secret") || strings.Contains(string(sec), "Stop") || !strings.Contains(string(sec), "aside hook claude") {
		t.Errorf("section:\n%s", sec)
	}
}
