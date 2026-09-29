// Command aside is a side channel for thoughts while working with coding agents.
//
// Typing ">> some thought" in Claude Code or Cursor stores the thought with
// git context and blocks the prompt, so the model never sees it.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/kacxx/aside-jot/internal/app"
	"github.com/kacxx/aside-jot/internal/hooks"
	"github.com/kacxx/aside-jot/internal/mcp"
)

var version = "" // set with -ldflags "-X main.version=..."

const usage = `aside — a side channel for thoughts while working with coding agents

Usage:
  aside add <text...>        capture a jot from the terminal (reads stdin if no text)
  aside inbox [-n N]         list the newest inbox jots (default 20, 0 = all)
  aside show <id>            show one jot with its context
  aside search <query...>    search all jots (substring, case-insensitive)
  aside done <id>            mark a jot as done
  aside backup <path>        write a consistent copy of the database (never overwrites)
  aside paths                print data paths and check which aside is on PATH
  aside hook claude|cursor   run as a prompt hook (reads the payload on stdin)
  aside mcp                  run the read-only MCP server on stdio
  aside version

Environment:
  JOT_DB                     database path (default: $XDG_DATA_HOME/jot/jot.db,
                             else the OS per-user data directory)
  JOT_BUSY_TIMEOUT_MS        how long each database step waits on a lock
                             (default 2000, capped at 10000)
`

func main() {
	// If a reader closes its end, a write to stdout must fail with EPIPE rather
	// than kill the process with SIGPIPE: hooks always exit 0, and aside mcp and
	// the CLI shut down through their normal error paths.
	signal.Ignore(syscall.SIGPIPE)
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "hook" {
		os.Exit(runHook(args[1:], os.Stdin, os.Stdout, os.Stderr))
	}
	if err := run(args, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "aside:", err)
		os.Exit(1)
	}
}

// runHook always returns 0: a hook must never break the host editor.
func runHook(args []string, stdin io.Reader, stdout, stderr io.Writer) (code int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintln(stderr, "aside hook: internal error:", r)
		}
	}()
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: aside hook claude|cursor")
		return 0
	}
	var h func(context.Context, io.Reader, io.Writer, app.Opener) error
	switch args[0] {
	case "claude":
		h = hooks.Claude
	case "cursor":
		h = hooks.Cursor
	default:
		fmt.Fprintf(stderr, "aside hook: unknown agent %q (want claude or cursor)\n", args[0])
		return 0
	}
	if err := h(context.Background(), stdin, stdout, app.OpenDefault); err != nil {
		fmt.Fprintln(stderr, "aside hook:", err)
	}
	return 0
}

func run(args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 0 {
		fmt.Fprint(stdout, usage)
		return nil
	}
	cmd, args := args[0], args[1:]
	ctx := context.Background()

	switch cmd {
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return nil
	case "version", "--version":
		fmt.Fprintln(stdout, "aside", buildVersion())
		return nil
	case "paths":
		return cmdPaths(stdout)
	}

	svc, err := app.OpenDefault()
	if err != nil {
		return err
	}
	defer svc.Close()

	switch cmd {
	case "add":
		text := strings.Join(args, " ")
		if strings.TrimSpace(text) == "" {
			b, err := io.ReadAll(stdin)
			if err != nil {
				return err
			}
			text = string(b)
		}
		cwd, _ := os.Getwd()
		e, err := svc.Capture(ctx, app.CaptureRequest{Text: text, Source: "cli", Cwd: cwd})
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "✓ Jotted #%d\n", e.ID)
	case "inbox":
		fs := flag.NewFlagSet("inbox", flag.ContinueOnError)
		n := fs.Int("n", 20, "number of jots to show (0 = all)")
		if err := fs.Parse(args); err != nil {
			return err
		}
		es, err := svc.Inbox(ctx, *n)
		if err != nil {
			return err
		}
		if len(es) == 0 {
			fmt.Fprintln(stdout, "Inbox is empty.")
		}
		printList(stdout, es)
	case "show":
		id, err := parseID(args)
		if err != nil {
			return err
		}
		e, err := svc.Show(ctx, id)
		if errors.Is(err, app.ErrNotFound) {
			return fmt.Errorf("no jot #%d", id)
		}
		if err != nil {
			return err
		}
		printEntry(stdout, e)
	case "search":
		q := strings.Join(args, " ")
		es, err := svc.Search(ctx, q, 0)
		if err != nil {
			return err
		}
		if len(es) == 0 {
			fmt.Fprintf(stdout, "No jots match %q.\n", q)
		}
		printList(stdout, es)
	case "done":
		id, err := parseID(args)
		if err != nil {
			return err
		}
		if err := svc.Done(ctx, id); errors.Is(err, app.ErrNotFound) {
			return fmt.Errorf("no jot #%d", id)
		} else if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "✓ #%d done\n", id)
	case "backup":
		if len(args) != 1 {
			return errors.New("usage: aside backup <path>")
		}
		if err := svc.Backup(ctx, args[0]); err != nil {
			return err
		}
		// Report the absolute path Backup wrote to, not the argument as typed.
		dst, err := filepath.Abs(args[0])
		if err != nil {
			dst = args[0]
		}
		fmt.Fprintln(stdout, "✓ Backed up to", dst)
	case "mcp":
		return mcp.NewServer(svc, buildVersion()).Serve(ctx, stdin, stdout)
	default:
		return fmt.Errorf("unknown command %q (run 'aside help')", cmd)
	}
	return nil
}

func cmdPaths(w io.Writer) error {
	db, err := app.DBPath()
	if err != nil {
		return err
	}
	src := "default"
	switch {
	case os.Getenv("JOT_DB") != "":
		src = "$JOT_DB"
	case os.Getenv("XDG_DATA_HOME") != "":
		src = "$XDG_DATA_HOME"
	}
	fmt.Fprintf(w, "db:           %s (%s)\n", db, src)
	fmt.Fprintf(w, "busy timeout: %s\n", app.BusyTimeout())
	exe, err := os.Executable()
	if err != nil {
		return nil
	}
	fmt.Fprintf(w, "binary:       %s\n", exe)
	if warn := pathWarning(exe, exec.LookPath); warn != "" {
		fmt.Fprintln(w, "warning:     ", warn)
	}
	return nil
}

// pathWarning reports when a bare "aside" would not run exe: nothing named
// aside is on PATH, or another program is found first. Hook and MCP configs
// should then use exe's absolute path.
func pathWarning(exe string, lookPath func(string) (string, error)) string {
	found, err := lookPath("aside")
	if err != nil {
		return fmt.Sprintf("'aside' is not on PATH; use %s in hook and MCP configs", exe)
	}
	fi, err1 := os.Stat(found)
	fe, err2 := os.Stat(exe)
	if err1 != nil || err2 != nil || !os.SameFile(fi, fe) {
		return fmt.Sprintf("'aside' on PATH is %s, not this binary; use %s in hook and MCP configs", found, exe)
	}
	return ""
}

func parseID(args []string) (int64, error) {
	if len(args) != 1 {
		return 0, errors.New("expected exactly one jot id")
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(args[0], "#"), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid id %q", args[0])
	}
	return id, nil
}

func printList(w io.Writer, es []app.Entry) {
	for _, e := range es {
		line, _, _ := strings.Cut(e.Text, "\n")
		if r := []rune(line); len(r) > 80 {
			line = string(r[:79]) + "…"
		}
		where := location(e)
		if where != "" {
			where = "  [" + where + "]"
		}
		status := ""
		if e.Status != "inbox" {
			status = "  (" + e.Status + ")"
		}
		fmt.Fprintf(w, "#%-4d %s  %s%s%s\n", e.ID, e.CreatedAt.Local().Format("2006-01-02 15:04"), line, where, status)
	}
}

func location(e app.Entry) string {
	switch {
	case e.RepoName != "" && e.Branch != "":
		return e.RepoName + "@" + e.Branch
	case e.RepoName != "" && e.CommitSHA != "":
		return e.RepoName + "@" + e.CommitSHA
	default:
		return e.RepoName
	}
}

func printEntry(w io.Writer, e app.Entry) {
	fmt.Fprintf(w, "#%d  (%s)\n\n%s\n\n", e.ID, e.Status, e.Text)
	field := func(k, v string) {
		if v != "" {
			fmt.Fprintf(w, "%-10s %s\n", k+":", v)
		}
	}
	field("created", e.CreatedAt.Local().Format(time.RFC3339))
	field("source", e.Source)
	field("session", e.SessionID)
	field("cwd", e.Cwd)
	field("repo", e.RepoName)
	field("root", e.RepoRoot)
	field("branch", e.Branch)
	field("commit", e.CommitSHA)
	if len(e.Metadata) > 0 {
		field("metadata", string(e.Metadata))
	}
}

func buildVersion() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" {
		return bi.Main.Version
	}
	return "dev"
}
