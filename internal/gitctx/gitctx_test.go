package gitctx

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitRepo creates a repository in a temp dir with an isolated config.
func gitRepo(t *testing.T, commit bool) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	Git(t, dir, "init", "-q", "-b", "main")
	if commit {
		if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		Git(t, dir, "add", ".")
		Git(t, dir, "commit", "-q", "-m", "init")
	}
	return dir
}

// Git runs git in dir with an isolated identity and config.
func Git(t *testing.T, dir string, args ...string) string {
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

func realpath(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestBranch(t *testing.T) {
	dir := gitRepo(t, true)
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	got := Detect(context.Background(), sub)
	want := Info{
		Root:   realpath(t, dir),
		Name:   filepath.Base(dir),
		Branch: "main",
		Commit: Git(t, dir, "rev-parse", "--short", "HEAD"),
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestDetachedHEAD(t *testing.T) {
	dir := gitRepo(t, true)
	Git(t, dir, "checkout", "-q", "--detach")
	got := Detect(context.Background(), dir)
	if got.Branch != "" {
		t.Errorf("branch = %q, want empty on detached HEAD", got.Branch)
	}
	if got.Commit == "" || got.Commit != Git(t, dir, "rev-parse", "--short", "HEAD") {
		t.Errorf("commit = %q, want short HEAD", got.Commit)
	}
	if got.Root == "" {
		t.Error("root should be set")
	}
}

func TestNoCommits(t *testing.T) {
	dir := gitRepo(t, false)
	got := Detect(context.Background(), dir)
	if got.Root != realpath(t, dir) || got.Branch != "main" || got.Commit != "" {
		t.Fatalf("got %+v, want root, branch main, no commit", got)
	}
}

func TestNonRepo(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
	if got := Detect(context.Background(), dir); got != (Info{}) {
		t.Fatalf("got %+v, want empty", got)
	}
}

func TestMissingDirAndEmpty(t *testing.T) {
	if got := Detect(context.Background(), filepath.Join(t.TempDir(), "nope")); got != (Info{}) {
		t.Fatalf("missing dir: got %+v", got)
	}
	if got := Detect(context.Background(), ""); got != (Info{}) {
		t.Fatalf("empty dir: got %+v", got)
	}
}
