package app

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kacxx/aside-jot/internal/gitctx"
)

// fakeRunner answers git and gh calls without running anything.
type fakeRunner struct {
	calls  []string // "name arg arg ..."
	stdin  []string // what each call was fed
	remote string   // git remote get-url output; empty fails
	auth   error    // gh auth status result
	create error    // gh issue create result
	url    string   // gh issue create output
}

func (f *fakeRunner) Run(_ context.Context, stdin io.Reader, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, strings.Join(append([]string{name}, args...), " "))
	in := ""
	if stdin != nil {
		b, _ := io.ReadAll(stdin)
		in = string(b)
	}
	f.stdin = append(f.stdin, in)
	switch {
	case name == "git":
		if f.remote == "" {
			return nil, errors.New("exit status 2: error: No such remote 'origin'")
		}
		return []byte(f.remote + "\n"), nil
	case name == "gh" && args[0] == "auth":
		return nil, f.auth
	case name == "gh" && args[0] == "issue":
		if f.create != nil {
			return nil, f.create
		}
		return []byte(f.url + "\n"), nil
	}
	return nil, errors.New("unexpected command " + name)
}

func promoteService(t *testing.T, gi gitctx.Info) *Service {
	t.Helper()
	svc, err := Open(filepath.Join(t.TempDir(), "jot.db"), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { svc.Close() })
	svc.git = func(context.Context, string) gitctx.Info { return gi }
	svc.now = func() time.Time { return time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC) }
	if _, err := svc.Capture(context.Background(), CaptureRequest{
		Text: "  fix the cache TTL  \nsee https://example.com/x", Source: "claude", Cwd: "/r",
		Metadata: map[string]any{"transcript_path": "/t.jsonl"},
	}); err != nil {
		t.Fatal(err)
	}
	return svc
}

var repoInfo = gitctx.Info{Root: "/r", Name: "r", Branch: "main", Commit: "abc1234"}

func TestParseRemote(t *testing.T) {
	for in, want := range map[string]string{
		"https://github.com/kacxx/aside-jot.git":          "kacxx/aside-jot",
		"https://github.com/kacxx/aside-jot":              "kacxx/aside-jot",
		"https://github.com/kacxx/aside-jot/":             "kacxx/aside-jot",
		"https://user:tok@github.com/kacxx/aside-jot.git": "kacxx/aside-jot",
		"http://GitHub.com/kacxx/aside-jot":               "kacxx/aside-jot",
		"git@github.com:kacxx/aside-jot.git":              "kacxx/aside-jot",
		"github.com:kacxx/aside-jot":                      "kacxx/aside-jot",
		"ssh://git@github.com/kacxx/aside-jot.git":        "kacxx/aside-jot",
		"ssh://git@github.com:22/kacxx/aside-jot":         "kacxx/aside-jot",
		"https://ghe.example.com/team/proj.git":           "ghe.example.com/team/proj",
		"git@ghe.example.com:team/proj.git":               "ghe.example.com/team/proj",
	} {
		if got, err := ParseRemote(in); err != nil || got != want {
			t.Errorf("ParseRemote(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{
		"", "/srv/git/proj.git", `C:\src\proj`, "file:///srv/git/proj.git",
		"https://github.com/kacxx", "https://github.com/a/b/c", "git@github.com:", "https://github.com/",
	} {
		if got, err := ParseRemote(in); err == nil {
			t.Errorf("ParseRemote(%q) = %q; want an error", in, got)
		}
	}
}

func TestIssueTitleAndBody(t *testing.T) {
	e := Entry{
		Text: "  first line  \nsecond line", CreatedAt: time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC),
		Source: "codex", RepoName: "r", Branch: "main", CommitSHA: "abc1234",
	}
	if got := IssueTitle(e); got != "first line" {
		t.Errorf("title: %q", got)
	}
	ts := e.CreatedAt.Local().Format(time.RFC3339)
	if got, want := IssueBody(e), "first line  \nsecond line\n\nCaptured "+ts+" from codex on r@main (abc1234)\n"; got != want {
		t.Errorf("body:\n%q\nwant\n%q", got, want)
	}

	for _, c := range []struct {
		e    Entry
		want string
	}{
		{Entry{Text: "x", CreatedAt: e.CreatedAt}, "Captured " + ts + "\n"},
		{Entry{Text: "x", CreatedAt: e.CreatedAt, Source: "cli", RepoName: "r", CommitSHA: "abc"}, "Captured " + ts + " from cli on r (abc)\n"},
		{Entry{Text: "x", CreatedAt: e.CreatedAt, RepoName: "r", Branch: "b"}, "Captured " + ts + " on r@b\n"},
	} {
		if got := IssueBody(c.e); got != "x\n\n"+c.want {
			t.Errorf("body for %+v: %q", c.e, got)
		}
	}
}

func TestPromoteDryRun(t *testing.T) {
	ctx := context.Background()
	svc := promoteService(t, repoInfo)
	before, _ := svc.Show(ctx, 1)
	f := &fakeRunner{remote: "git@github.com:kacxx/aside-jot.git"}

	p, err := svc.Promote(ctx, f, PromoteRequest{ID: 1, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if p.Repo != "kacxx/aside-jot" || p.Title != "fix the cache TTL" || p.URL != "" ||
		!strings.HasPrefix(p.Body, "fix the cache TTL  \nsee https://example.com/x\n\nCaptured ") {
		t.Fatalf("promotion: %+v", p)
	}
	for _, c := range f.calls {
		if strings.HasPrefix(c, "gh") {
			t.Errorf("dry run ran %q", c)
		}
	}

	// With --repo, not even git runs.
	f = &fakeRunner{}
	if p, err := svc.Promote(ctx, f, PromoteRequest{ID: 1, Repo: "o/n", DryRun: true}); err != nil || p.Repo != "o/n" {
		t.Fatalf("dry run --repo: %+v, %v", p, err)
	}
	if len(f.calls) != 0 {
		t.Errorf("dry run --repo ran %v", f.calls)
	}
	if after, _ := svc.Show(ctx, 1); after.Status != before.Status || string(after.Metadata) != string(before.Metadata) {
		t.Fatalf("dry run changed the jot: %+v", after)
	}
}

func TestPromoteSuccessThenRefused(t *testing.T) {
	ctx := context.Background()
	svc := promoteService(t, repoInfo)
	const url = "https://github.com/kacxx/aside-jot/issues/42"
	f := &fakeRunner{remote: "https://github.com/kacxx/aside-jot.git", url: url}

	p, err := svc.Promote(ctx, f, PromoteRequest{ID: 1})
	if err != nil {
		t.Fatal(err)
	}
	if p.URL != url {
		t.Fatalf("url: %q", p.URL)
	}
	want := []string{
		"git -C /r remote get-url origin",
		"gh auth status --hostname github.com",
		"gh issue create --repo=kacxx/aside-jot --title=fix the cache TTL --body-file=-",
	}
	if strings.Join(f.calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("calls:\n%s", strings.Join(f.calls, "\n"))
	}
	if f.stdin[2] != p.Body {
		t.Errorf("issue body on stdin: %q", f.stdin[2])
	}

	e, _ := svc.Show(ctx, 1)
	if e.Status != "done" || e.IssueURL != url || !strings.Contains(string(e.Metadata), `"transcript_path":"/t.jsonl"`) {
		t.Fatalf("after promote: %+v (%s)", e, e.Metadata)
	}

	f = &fakeRunner{remote: "https://github.com/kacxx/aside-jot.git", url: "https://other"}
	_, err = svc.Promote(ctx, f, PromoteRequest{ID: 1})
	var ap *AlreadyPromotedError
	if !errors.As(err, &ap) || ap.URL != url || !strings.Contains(err.Error(), url) {
		t.Fatalf("second promote: %v", err)
	}
	if len(f.calls) != 0 {
		t.Errorf("second promote ran %v", f.calls)
	}
}

func TestPromoteOtherHost(t *testing.T) {
	svc := promoteService(t, repoInfo)
	f := &fakeRunner{remote: "git@ghe.example.com:team/proj.git", url: "https://ghe.example.com/team/proj/issues/1"}
	if _, err := svc.Promote(context.Background(), f, PromoteRequest{ID: 1}); err != nil {
		t.Fatal(err)
	}
	if f.calls[1] != "gh auth status --hostname ghe.example.com" || !strings.Contains(f.calls[2], "--repo=ghe.example.com/team/proj") {
		t.Fatalf("calls: %v", f.calls)
	}
}

func TestPromoteFailuresLeaveJotUnchanged(t *testing.T) {
	ctx := context.Background()
	notFound := &exec.Error{Name: "gh", Err: exec.ErrNotFound}
	for _, c := range []struct {
		name string
		gi   gitctx.Info
		req  PromoteRequest
		f    *fakeRunner
		want string
	}{
		{"no repo", gitctx.Info{}, PromoteRequest{ID: 1}, &fakeRunner{}, "not captured in a git repository; pass --repo"},
		{"no origin", repoInfo, PromoteRequest{ID: 1}, &fakeRunner{}, "no origin remote in /r"},
		{"bad origin", repoInfo, PromoteRequest{ID: 1}, &fakeRunner{remote: "/srv/proj.git"}, "pass --repo"},
		{"bad --repo", repoInfo, PromoteRequest{ID: 1, Repo: "nope"}, &fakeRunner{}, `invalid --repo "nope"`},
		{"gh missing", repoInfo, PromoteRequest{ID: 1, Repo: "o/n"}, &fakeRunner{auth: notFound}, "gh (GitHub CLI) not found"},
		{"gh logged out", repoInfo, PromoteRequest{ID: 1, Repo: "o/n"},
			&fakeRunner{auth: errors.New("exit status 1: You are not logged into any GitHub hosts")}, "gh is not logged in to github.com; run 'gh auth login'"},
		{"create fails", repoInfo, PromoteRequest{ID: 1, Repo: "o/n"},
			&fakeRunner{create: errors.New("exit status 1: GraphQL: Could not resolve to a Repository")}, "gh issue create: exit status 1: GraphQL"},
		{"no url", repoInfo, PromoteRequest{ID: 1, Repo: "o/n"}, &fakeRunner{url: "huh"}, "printed no issue URL"},
		{"unknown id", repoInfo, PromoteRequest{ID: 99, Repo: "o/n"}, &fakeRunner{}, ErrNotFound.Error()},
	} {
		t.Run(c.name, func(t *testing.T) {
			svc := promoteService(t, c.gi)
			p, err := svc.Promote(ctx, c.f, c.req)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v; want %q", err, c.want)
			}
			if p.URL != "" {
				t.Errorf("url on failure: %q", p.URL)
			}
			if e, _ := svc.Show(ctx, 1); e.Status != "inbox" || e.IssueURL != "" {
				t.Errorf("jot changed: %+v", e)
			}
		})
	}
	svc := promoteService(t, repoInfo)
	if _, err := svc.Promote(ctx, &fakeRunner{}, PromoteRequest{ID: 99}); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown id: %v", err)
	}
}
