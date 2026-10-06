package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/kacxx/aside-jot/internal/setup"
)

const cursorDocs = "https://github.com/kacxx/aside-jot/blob/main/docs/cursor.md"

// setupBinary says which aside to write into the config; tests replace it.
var setupBinary = func() setup.Binary { return setup.Binary{Arg0: os.Args[0]} }

// setupGOOS is the platform the hook is written for; tests replace it.
var setupGOOS = runtime.GOOS

func cmdSetup(args []string, w io.Writer) error {
	const usage = "usage: aside setup claude|codex [--dry-run] [--mcp]"
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dry := fs.Bool("dry-run", false, "print the planned changes and the resulting file, write nothing")
	mcp := fs.Bool("mcp", false, "also print the command that registers the MCP server (not run)")
	// Flags may come before or after the agent.
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return fmt.Errorf("setup: %w (%s)", err, usage)
		}
		if fs.NArg() == 0 {
			break
		}
		pos = append(pos, fs.Arg(0))
		args = fs.Args()[1:]
	}
	if len(pos) != 1 {
		return errors.New(usage)
	}
	agent := pos[0]
	switch agent {
	case "claude", "codex":
	case "cursor":
		fmt.Fprintf(w, "Cursor isn't supported, so aside setup cursor writes nothing.\nCursor keeps a blocked prompt in the chat and sends it with your next message,\nwhich defeats the point of a jot. See %s\nTo jot from a terminal instead, use `aside add`.\n", cursorDocs)
		return nil
	default:
		return fmt.Errorf("unknown agent %q (%s)", agent, usage)
	}

	bin, err := setupBinary().Resolve()
	if err != nil {
		return err
	}
	path, err := setup.ConfigPath(agent)
	if err != nil {
		return err
	}
	windows := setupGOOS == "windows"
	quoted := setup.Quote(bin, windows)
	h := setup.Hook{Agent: agent, Command: quoted + " hook " + agent, Windows: windows}
	res, err := setup.Install(path, h, *dry, now())
	if err != nil {
		return err
	}
	printSetup(w, agent, h, res, *dry)
	if *mcp {
		add := "claude mcp add --scope user aside -- "
		if agent == "codex" {
			add = "codex mcp add aside -- "
		}
		fmt.Fprintf(w, "\nTo give %s read-only access to your jots, run (not run for you):\n  %s%s mcp\n", agentName(agent), add, quoted)
	}
	return nil
}

func agentName(agent string) string {
	if agent == "codex" {
		return "Codex"
	}
	return "Claude Code"
}

func printSetup(w io.Writer, agent string, h setup.Hook, res setup.Result, dry bool) {
	ch := res.Change
	name := agentName(agent)
	switch {
	case ch.Kind == "unchanged":
		fmt.Fprintf(w, "✓ %s already has this hook; nothing changed.\n  command: %s\n", res.Path, h.Command)
	case dry:
		fmt.Fprintf(w, "Dry run: nothing written.\nWould %s the aside hook in %s\n  command: %s\n", verb(ch.Kind), res.Path, h.Command)
	default:
		fmt.Fprintf(w, "✓ %s the aside hook in %s\n  command: %s\n", done(ch.Kind), res.Path, h.Command)
	}
	if ch.Kind == "updated" {
		for _, old := range ch.Replaced {
			fmt.Fprintf(w, "  replaced: %s\n", old)
		}
		if ch.Removed > 0 {
			fmt.Fprintf(w, "  removed %d duplicate aside entr%s so only one hook runs\n", ch.Removed, plural(ch.Removed, "y", "ies"))
		}
	}
	if ch.Kind != "unchanged" {
		switch {
		case dry:
			fmt.Fprintln(w, "  A backup would be written before the file changes.")
		case res.Backup != "":
			fmt.Fprintf(w, "  backup:  %s\n", res.Backup)
		default:
			fmt.Fprintln(w, "  backup:  none needed, the file did not exist")
		}
		fmt.Fprintln(w, "  Other settings and key order are kept; only whitespace may differ.")
	}
	if dry && ch.Kind != "unchanged" {
		fmt.Fprintf(w, "\nResulting %s:\n%s", res.Path, res.New)
	}

	fmt.Fprintln(w)
	switch agent {
	case "codex":
		switch ch.Kind {
		case "added":
			fmt.Fprintln(w, "Codex only runs a hook you have trusted: run /hooks in the Codex CLI (the desktop")
			fmt.Fprintln(w, "app has no /hooks) and trust it. Until then, >> prompts go to the model.")
		case "updated":
			fmt.Fprintln(w, "The hook's definition changed, and Codex won't run a changed hook until you trust it")
			fmt.Fprintln(w, "again: run /hooks in the Codex CLI and trust it. Until then, >> prompts go to the model.")
		}
		fmt.Fprintln(w, "Then type `>> test` in Codex: you should see ✓ Jotted #N and no model reply.")
	default:
		fmt.Fprintf(w, "Next: type `>> test` in %s: you should see ✓ Jotted #N and no model reply.\n", name)
		fmt.Fprintf(w, "Cursor imports hooks from ~/.claude/settings.json, so this also turns aside on in\nCursor, which isn't supported (see %s).\n", cursorDocs)
	}
}

func verb(kind string) string {
	if kind == "updated" {
		return "update"
	}
	return "add"
}

func done(kind string) string {
	if kind == "updated" {
		return "Updated"
	}
	return "Added"
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
