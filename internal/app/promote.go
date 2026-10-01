package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os/exec"
	"strings"
	"time"

	"github.com/kacxx/aside-jot/internal/store"
)

// Runner runs an external command. Promote runs git and gh through it, so
// tests can fake both.
type Runner interface {
	// Run runs name with args, feeding it stdin (nil for none), and returns
	// its stdout. A failed command's error includes its trimmed stderr.
	Run(ctx context.Context, stdin io.Reader, name string, args ...string) ([]byte, error)
}

// ExecRunner runs real commands.
type ExecRunner struct{}

// Run implements Runner.
func (ExecRunner) Run(ctx context.Context, stdin io.Reader, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = stdin
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(errb.String()); msg != "" {
			return out.Bytes(), fmt.Errorf("%w: %s", err, msg)
		}
		return out.Bytes(), err
	}
	return out.Bytes(), nil
}

// PromoteRequest asks for a jot to become a GitHub issue.
type PromoteRequest struct {
	ID     int64
	Repo   string // [HOST/]OWNER/NAME; empty means the jot's origin remote
	DryRun bool
}

// Promotion describes the issue a jot became, or would become on a dry run.
type Promotion struct {
	Repo  string
	Title string
	Body  string
	URL   string // empty on a dry run
}

// AlreadyPromotedError is returned for a jot that already has an issue.
type AlreadyPromotedError struct {
	ID  int64
	URL string
}

func (e *AlreadyPromotedError) Error() string {
	return fmt.Sprintf("jot #%d was already promoted to %s", e.ID, e.URL)
}

// Promote creates a GitHub issue from a jot with the gh CLI, then records the
// issue URL in the jot's metadata and marks it done, in one transaction. A dry
// run only builds the issue: it never runs gh and changes nothing.
func (s *Service) Promote(ctx context.Context, r Runner, req PromoteRequest) (Promotion, error) {
	e, err := s.store.Get(ctx, req.ID)
	if err != nil {
		return Promotion{}, err
	}
	if e.IssueURL != "" {
		return Promotion{}, &AlreadyPromotedError{ID: e.ID, URL: e.IssueURL}
	}
	p := Promotion{Title: IssueTitle(e), Body: IssueBody(e)}

	if req.Repo != "" {
		if !validRepo(req.Repo) {
			return Promotion{}, fmt.Errorf("invalid --repo %q (want owner/name)", req.Repo)
		}
		p.Repo = req.Repo
	} else {
		if e.RepoRoot == "" {
			return Promotion{}, fmt.Errorf("jot #%d was not captured in a git repository; pass --repo owner/name", e.ID)
		}
		// Reading the remote is local, so a dry run does it too and shows the
		// real target.
		out, err := r.Run(ctx, nil, "git", "-C", e.RepoRoot, "remote", "get-url", "origin")
		if err != nil {
			return Promotion{}, fmt.Errorf("jot #%d: no origin remote in %s (%w); pass --repo owner/name",
				e.ID, e.RepoRoot, commandError("git", err))
		}
		if p.Repo, err = ParseRemote(strings.TrimSpace(string(out))); err != nil {
			return Promotion{}, fmt.Errorf("jot #%d: %w; pass --repo owner/name", e.ID, err)
		}
	}
	if req.DryRun {
		return p, nil
	}

	host := "github.com"
	if parts := strings.Split(p.Repo, "/"); len(parts) == 3 {
		host = parts[0]
	}
	if _, err := r.Run(ctx, nil, "gh", "auth", "status", "--hostname", host); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return Promotion{}, errGHMissing
		}
		return Promotion{}, fmt.Errorf("gh is not logged in to %s; run 'gh auth login' (%w)", host, err)
	}
	out, err := r.Run(ctx, strings.NewReader(p.Body), "gh", "issue", "create",
		"--repo="+p.Repo, "--title="+p.Title, "--body-file=-")
	if err != nil {
		return Promotion{}, fmt.Errorf("gh issue create: %w", commandError("gh", err))
	}
	p.URL = issueURL(out)
	if p.URL == "" {
		return Promotion{}, fmt.Errorf("gh issue create printed no issue URL: %q", strings.TrimSpace(string(out)))
	}
	if err := s.store.SetMetadata(ctx, e.ID, store.MetaIssueURL, p.URL, store.StatusDone); err != nil {
		return p, fmt.Errorf("created %s but could not record it on jot #%d: %w", p.URL, e.ID, err)
	}
	return p, nil
}

var errGHMissing = errors.New("gh (GitHub CLI) not found on PATH; install it from https://cli.github.com and run 'gh auth login'")

func commandError(name string, err error) error {
	if errors.Is(err, exec.ErrNotFound) {
		if name == "gh" {
			return errGHMissing
		}
		return fmt.Errorf("%s not found on PATH", name)
	}
	return err
}

// issueURL returns the last line of gh's output if it is a URL.
func issueURL(out []byte) string {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	last := strings.TrimSpace(lines[len(lines)-1])
	if strings.HasPrefix(last, "https://") || strings.HasPrefix(last, "http://") {
		return last
	}
	return ""
}

// IssueTitle is the jot's first line, trimmed.
func IssueTitle(e Entry) string {
	line, _, _ := strings.Cut(strings.TrimSpace(e.Text), "\n")
	return strings.TrimSpace(line)
}

// IssueBody is the jot's full text, then where and when it was captured,
// leaving out whatever is unknown.
func IssueBody(e Entry) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(e.Text))
	b.WriteString("\n\nCaptured ")
	b.WriteString(e.CreatedAt.Local().Format(time.RFC3339))
	if e.Source != "" {
		b.WriteString(" from " + e.Source)
	}
	if e.RepoName != "" {
		b.WriteString(" on " + e.RepoName)
		if e.Branch != "" {
			b.WriteString("@" + e.Branch)
		}
		if e.CommitSHA != "" {
			b.WriteString(" (" + e.CommitSHA + ")")
		}
	}
	b.WriteString("\n")
	return b.String()
}

// ParseRemote turns a git remote URL into the repo form gh's --repo takes:
// OWNER/NAME for github.com, HOST/OWNER/NAME for any other host. It accepts
// https, ssh:// and scp-like (git@host:owner/name.git) URLs.
func ParseRemote(remote string) (string, error) {
	if remote == "" {
		return "", errors.New("origin remote is empty")
	}
	var host, path string
	if u, err := url.Parse(remote); err == nil && u.Scheme != "" && u.Host != "" {
		host, path = u.Hostname(), u.Path
	} else if at, rest, ok := strings.Cut(remote, ":"); ok && !strings.Contains(at, "/") {
		// scp-like: [user@]host:path
		_, host, _ = strings.Cut(at, "@")
		if host == "" {
			host = at
		}
		path = rest
	} else {
		return "", fmt.Errorf("cannot read a GitHub repository from remote %q", redactRemote(remote))
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	repo := path
	if !strings.EqualFold(host, "github.com") {
		repo = host + "/" + path
	}
	if host == "" || strings.Count(path, "/") != 1 || !validRepo(repo) {
		return "", fmt.Errorf("cannot read a GitHub repository from remote %q", redactRemote(remote))
	}
	return repo, nil
}

// redactRemote drops any userinfo (user, password or token) from a URL
// remote so it never reaches an error message. It works on the raw string
// because a remote that fails to parse can still carry credentials.
func redactRemote(remote string) string {
	scheme, rest, ok := strings.Cut(remote, "://")
	if !ok {
		return remote
	}
	authority, path, hasPath := strings.Cut(rest, "/")
	i := strings.LastIndex(authority, "@")
	if i < 0 {
		return remote
	}
	out := scheme + "://" + authority[i+1:]
	if hasPath {
		out += "/" + path
	}
	return out
}

// validRepo reports whether repo is OWNER/NAME or HOST/OWNER/NAME.
func validRepo(repo string) bool {
	parts := strings.Split(repo, "/")
	if len(parts) != 2 && len(parts) != 3 {
		return false
	}
	for _, p := range parts {
		// A leading "-" would read as a flag to gh or git.
		if p == "" || strings.HasPrefix(p, "-") || strings.ContainsAny(p, " \t\r\n\\") {
			return false
		}
	}
	return true
}
