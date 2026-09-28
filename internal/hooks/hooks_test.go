package hooks

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kacxx/aside-jot/internal/app"
	"github.com/kacxx/aside-jot/internal/store"
)

// fixture loads testdata/hooks/<tool>/<name>.json, optionally rewriting
// top-level fields (e.g. cwd) so captures point at real temp dirs.
func fixture(t *testing.T, tool, name string, override map[string]any) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "hooks", tool, name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(override) == 0 {
		return b
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for k, v := range override {
		m[k] = v
	}
	b, err = json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// neverOpen fails the test if the hook tries to open the database.
func neverOpen(t *testing.T) app.Opener {
	return func() (*app.Service, error) {
		t.Helper()
		t.Error("opener called for a non-capture prompt")
		return nil, errors.New("must not open")
	}
}

// tempDB returns an opener for a fresh database and its path.
func tempDB(t *testing.T) (app.Opener, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "jot.db")
	return func() (*app.Service, error) { return app.Open(path, time.Second) }, path
}

func entries(t *testing.T, path string) []app.Entry {
	t.Helper()
	svc, err := app.Open(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	es, err := svc.Inbox(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	return es
}

func gitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "feature/jot")
	if err := os.WriteFile(filepath.Join(dir, "a"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "init")
	return dir
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func runClaude(t *testing.T, payload []byte, open app.Opener) (string, *ClaudeBlock) {
	t.Helper()
	var out bytes.Buffer
	if err := Claude(context.Background(), bytes.NewReader(payload), &out, open); err != nil {
		t.Fatal(err)
	}
	if out.Len() == 0 {
		return "", nil
	}
	var b ClaudeBlock
	if err := json.Unmarshal(out.Bytes(), &b); err != nil {
		t.Fatalf("invalid JSON %q: %v", out.String(), err)
	}
	return out.String(), &b
}

func runCursor(t *testing.T, payload []byte, open app.Opener) (string, CursorOutput) {
	t.Helper()
	var out bytes.Buffer
	if err := Cursor(context.Background(), bytes.NewReader(payload), &out, open); err != nil {
		t.Fatal(err)
	}
	var o CursorOutput
	if err := json.Unmarshal(out.Bytes(), &o); err != nil {
		t.Fatalf("invalid JSON %q: %v", out.String(), err)
	}
	return out.String(), o
}

func assertBlockedOK(t *testing.T, b *ClaudeBlock, id int64) {
	t.Helper()
	if b == nil || b.Decision != "block" || !b.SuppressOriginalPrompt {
		t.Fatalf("expected block, got %+v", b)
	}
	if want := "✓ Jotted #" + itoa(id); b.Reason != want {
		t.Fatalf("reason = %q, want %q", b.Reason, want)
	}
}

func assertBlockedFailed(t *testing.T, reason, prompt string) {
	t.Helper()
	if !strings.Contains(reason, "NOT saved") || !strings.Contains(reason, prompt) {
		t.Fatalf("failure message must say NOT saved and include %q; got %q", prompt, reason)
	}
}

func itoa(i int64) string { b, _ := json.Marshal(i); return string(b) }

// --- Claude ---

func TestClaudePassThrough(t *testing.T) {
	for _, name := range []string{"normal", "mid_prompt", "malformed"} {
		t.Run(name, func(t *testing.T) {
			out, _ := runClaude(t, fixture(t, "claude", name, nil), neverOpen(t))
			if out != "" {
				t.Fatalf("pass-through must write nothing, got %q", out)
			}
		})
	}
	t.Run("empty stdin", func(t *testing.T) {
		if out, _ := runClaude(t, nil, neverOpen(t)); out != "" {
			t.Fatalf("got %q", out)
		}
	})
	t.Run("wrong prompt type", func(t *testing.T) {
		if out, _ := runClaude(t, []byte(`{"prompt": 42}`), neverOpen(t)); out != "" {
			t.Fatalf("got %q", out)
		}
	})
}

func TestClaudeCaptureInRepo(t *testing.T) {
	repo := gitRepo(t)
	open, path := tempDB(t)
	_, b := runClaude(t, fixture(t, "claude", "capture", map[string]any{"cwd": repo}), open)
	assertBlockedOK(t, b, 1)

	es := entries(t, path)
	if len(es) != 1 {
		t.Fatalf("entries: %d", len(es))
	}
	e := es[0]
	if e.Text != "token cache TTL looks too long, check with infra before shipping" ||
		e.Source != "claude" || e.SessionID != "3f1c2a9e-7b4d-4e0a-9c55-0d2f8e6b1a47" ||
		e.Cwd != repo || e.RepoName != filepath.Base(repo) || e.RepoRoot == "" ||
		e.Branch != "feature/jot" || e.CommitSHA != git(t, repo, "rev-parse", "--short", "HEAD") {
		t.Fatalf("entry: %+v", e)
	}
	if !strings.Contains(string(e.Metadata), "transcript_path") {
		t.Fatalf("metadata: %s", e.Metadata)
	}

	// A second jot gets the next id.
	_, b = runClaude(t, fixture(t, "claude", "capture", map[string]any{"cwd": repo}), open)
	assertBlockedOK(t, b, 2)
}

func TestClaudeCaptureDetachedHEAD(t *testing.T) {
	repo := gitRepo(t)
	git(t, repo, "checkout", "-q", "--detach")
	open, path := tempDB(t)
	_, b := runClaude(t, fixture(t, "claude", "capture", map[string]any{"cwd": repo}), open)
	assertBlockedOK(t, b, 1)
	e := entries(t, path)[0]
	if e.Branch != "" || e.CommitSHA == "" || e.RepoRoot == "" {
		t.Fatalf("detached: %+v", e)
	}
}

func TestClaudeCaptureNonGitDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
	open, path := tempDB(t)
	_, b := runClaude(t, fixture(t, "claude", "capture", map[string]any{"cwd": dir}), open)
	assertBlockedOK(t, b, 1)
	e := entries(t, path)[0]
	if e.Cwd != dir || e.RepoRoot != "" || e.RepoName != "" || e.Branch != "" || e.CommitSHA != "" {
		t.Fatalf("non-git: %+v", e)
	}
}

func TestClaudeDBOpenFailure(t *testing.T) {
	// A regular file where the data directory should be.
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for name, open := range map[string]app.Opener{
		"opener error": func() (*app.Service, error) { return nil, errors.New("disk on fire") },
		"real open":    func() (*app.Service, error) { return app.Open(filepath.Join(blocker, "jot.db"), time.Second) },
		"panic":        func() (*app.Service, error) { panic("boom") },
	} {
		t.Run(name, func(t *testing.T) {
			payload := fixture(t, "claude", "capture", nil)
			_, b := runClaude(t, payload, open)
			if b == nil || b.Decision != "block" {
				t.Fatalf("a recognised jot must be blocked even on failure: %+v", b)
			}
			assertBlockedFailed(t, b.Reason, ">> token cache TTL looks too long, check with infra before shipping")
		})
	}
}

// lockDB holds an exclusive lock on the database from another connection.
func lockDB(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", store.DSN(path, 0))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(context.Background(), "BEGIN EXCLUSIVE"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		conn.ExecContext(context.Background(), "ROLLBACK")
		conn.Close()
		db.Close()
	})
}

func TestClaudeDBLocked(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jot.db")
	// Create the schema first so the lock is on a real database.
	svc, err := app.Open(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	svc.Close()
	lockDB(t, path)

	open := func() (*app.Service, error) { return app.Open(path, 100*time.Millisecond) }
	start := time.Now()
	_, b := runClaude(t, fixture(t, "claude", "capture", map[string]any{"cwd": t.TempDir()}), open)
	if b == nil || b.Decision != "block" {
		t.Fatalf("locked DB must still block: %+v", b)
	}
	assertBlockedFailed(t, b.Reason, ">> token cache TTL")
	if !strings.Contains(strings.ToLower(b.Reason), "locked") && !strings.Contains(strings.ToLower(b.Reason), "busy") {
		t.Errorf("reason should mention the lock: %q", b.Reason)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("locked capture took %v; busy_timeout not honoured", d)
	}
}

// --- Cursor ---

func TestCursorPassThrough(t *testing.T) {
	for _, name := range []string{"normal", "malformed"} {
		t.Run(name, func(t *testing.T) {
			out, o := runCursor(t, fixture(t, "cursor", name, nil), neverOpen(t))
			if !o.Continue || strings.TrimSpace(out) != `{"continue":true}` {
				t.Fatalf("pass-through must be {\"continue\":true}, got %q", out)
			}
		})
	}
	t.Run("mid-prompt", func(t *testing.T) {
		_, o := runCursor(t, fixture(t, "cursor", "normal", map[string]any{"prompt": "use >> to append"}), neverOpen(t))
		if !o.Continue {
			t.Fatal("should continue")
		}
	})
}

func TestCursorSingleRoot(t *testing.T) {
	repo := gitRepo(t)
	open, path := tempDB(t)
	_, o := runCursor(t, fixture(t, "cursor", "capture_single_root", map[string]any{"workspace_roots": []string{repo}}), open)
	if o.Continue || o.UserMessage != "✓ Jotted #1" {
		t.Fatalf("got %+v", o)
	}
	e := entries(t, path)[0]
	if e.Source != "cursor" || e.SessionID != "e5b0a1c7-3d2f-4c8e-9a14-7f6d2b0c8e31" ||
		e.Cwd != repo || e.Branch != "feature/jot" || e.CommitSHA == "" ||
		e.Text != "orders pagination should probably be cursor-based, not offset" {
		t.Fatalf("entry: %+v", e)
	}
	if strings.Contains(string(e.Metadata), "workspace_roots") {
		t.Fatalf("single root should not be in metadata: %s", e.Metadata)
	}
}

// A wrong-typed element in workspace_roots must not be saved as an empty root.
func TestCursorWrongTypedRootDropped(t *testing.T) {
	repo := gitRepo(t)
	open, path := tempDB(t)
	_, o := runCursor(t, fixture(t, "cursor", "capture_single_root", map[string]any{
		"workspace_roots": []any{repo, 5}}), open)
	if o.Continue {
		t.Fatalf("got %+v", o)
	}
	e := entries(t, path)[0]
	if e.Cwd != repo || strings.Contains(string(e.Metadata), "workspace_roots") {
		t.Fatalf("one real root must be the cwd, with no empty root recorded: cwd=%q meta=%s", e.Cwd, e.Metadata)
	}

	open, path = tempDB(t)
	runCursor(t, fixture(t, "cursor", "capture_single_root", map[string]any{
		"workspace_roots": []any{"/repo", 5, "/other"}}), open)
	var meta struct {
		Roots []string `json:"workspace_roots"`
	}
	e = entries(t, path)[0]
	if err := json.Unmarshal(e.Metadata, &meta); err != nil || len(meta.Roots) != 2 || meta.Roots[0] != "/repo" || meta.Roots[1] != "/other" {
		t.Fatalf("roots = %v (%v), want [/repo /other]", meta.Roots, err)
	}
}

func TestCursorMultiRoot(t *testing.T) {
	a, b := gitRepo(t), gitRepo(t)
	open, path := tempDB(t)
	_, o := runCursor(t, fixture(t, "cursor", "capture_multi_root", map[string]any{"workspace_roots": []string{a, b}}), open)
	if o.Continue || o.UserMessage != "✓ Jotted #1" {
		t.Fatalf("got %+v", o)
	}
	e := entries(t, path)[0]
	if e.Cwd != "" || e.RepoRoot != "" || e.Branch != "" {
		t.Fatalf("multi-root must not guess a cwd: %+v", e)
	}
	var meta struct {
		Roots []string `json:"workspace_roots"`
	}
	if err := json.Unmarshal(e.Metadata, &meta); err != nil || len(meta.Roots) != 2 || meta.Roots[0] != a || meta.Roots[1] != b {
		t.Fatalf("metadata: %s (%v)", e.Metadata, err)
	}
}

func TestCursorFailureBlocks(t *testing.T) {
	_, o := runCursor(t, fixture(t, "cursor", "capture_single_root", nil),
		func() (*app.Service, error) { return nil, errors.New("no db") })
	if o.Continue {
		t.Fatal("a recognised jot must be blocked even on failure")
	}
	assertBlockedFailed(t, o.UserMessage, ">> orders pagination should probably be cursor-based, not offset")
}

// A field of the wrong JSON type must not let a jot through: the payload is
// still well-formed, the prompt is still a jot, and it is saved with the
// metadata that did decode.
func TestWrongTypedFieldsStillBlock(t *testing.T) {
	t.Run("claude", func(t *testing.T) {
		for name, override := range map[string]map[string]any{
			"session_id":      {"session_id": 123},
			"cwd":             {"cwd": []int{1}},
			"transcript_path": {"transcript_path": false},
			"hook_event_name": {"hook_event_name": 5},
		} {
			t.Run(name, func(t *testing.T) {
				open, path := tempDB(t)
				_, b := runClaude(t, fixture(t, "claude", "capture", override), open)
				assertBlockedOK(t, b, 1)
				e := entries(t, path)[0]
				if e.Text != "token cache TTL looks too long, check with infra before shipping" || e.Source != "claude" {
					t.Fatalf("entry: %+v", e)
				}
				if name != "session_id" && e.SessionID != "3f1c2a9e-7b4d-4e0a-9c55-0d2f8e6b1a47" {
					t.Fatalf("well-typed session_id lost: %+v", e)
				}
			})
		}
	})
	t.Run("cursor", func(t *testing.T) {
		for name, override := range map[string]map[string]any{
			"conversation_id":        {"conversation_id": 7},
			"generation_id":          {"generation_id": []string{"x"}},
			"workspace_roots string": {"workspace_roots": "/x"},
			"workspace_roots ints":   {"workspace_roots": []int{1}},
			"hook_event_name":        {"hook_event_name": true},
		} {
			t.Run(name, func(t *testing.T) {
				open, path := tempDB(t)
				_, o := runCursor(t, fixture(t, "cursor", "capture_single_root", override), open)
				if o.Continue || o.UserMessage != "✓ Jotted #1" {
					t.Fatalf("got %+v", o)
				}
				if e := entries(t, path)[0]; e.Source != "cursor" || e.Text != "orders pagination should probably be cursor-based, not offset" {
					t.Fatalf("entry: %+v", e)
				}
			})
		}
	})
	t.Run("prompt itself wrong type passes through", func(t *testing.T) {
		if out, _ := runClaude(t, []byte(`{"session_id":1,"prompt":[">> x"]}`), neverOpen(t)); out != "" {
			t.Fatalf("got %q", out)
		}
		if _, o := runCursor(t, []byte(`{"workspace_roots":"/x","prompt":{"a":">> x"}}`), neverOpen(t)); !o.Continue {
			t.Fatalf("got %+v", o)
		}
	})
}
