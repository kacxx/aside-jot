// Package gitctx collects best-effort git context for a directory.
//
// It never returns an error: a missing git binary, a non-repository, an unborn
// branch or a slow filesystem all degrade to empty fields.
package gitctx

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// Timeout bounds the total time spent running git.
const Timeout = 750 * time.Millisecond

// Info is the git context of a directory. Empty fields mean "unknown".
type Info struct {
	Root   string // absolute repository root
	Name   string // base name of Root
	Branch string // empty on detached HEAD
	Commit string // short SHA; empty before the first commit
}

// Detect returns the git context for dir. It never fails.
func Detect(ctx context.Context, dir string) Info {
	var info Info
	if dir == "" {
		return info
	}
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()

	root, ok := run(ctx, dir, "rev-parse", "--show-toplevel")
	if !ok || root == "" {
		return info
	}
	info.Root = root
	info.Name = filepath.Base(root)
	// symbolic-ref fails on detached HEAD, but works on an unborn branch.
	info.Branch, _ = run(ctx, dir, "symbolic-ref", "--quiet", "--short", "HEAD")
	// rev-parse fails before the first commit.
	info.Commit, _ = run(ctx, dir, "rev-parse", "--verify", "--quiet", "--short", "HEAD")
	return info
}

func run(ctx context.Context, dir string, args ...string) (string, bool) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	// Never take optional locks: a jot must not interfere with the user's git.
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", false
	}
	return string(bytes.TrimSpace(out.Bytes())), true
}
