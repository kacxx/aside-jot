# Setup in detail

The [README](../README.md) has the short version. This page has the rest:
Windows, project-level configs, Codex trust, and what each agent was verified
to do.

## Install

Requires Go 1.25+. No cgo: SQLite is pure Go (`modernc.org/sqlite`).

```sh
go install github.com/kacxx/aside-jot/cmd/aside@latest
```

or from a checkout:

```sh
go install ./cmd/aside
```

This installs `aside` into `$(go env GOPATH)/bin` (usually `~/go/bin`; on
Windows, usually `$env:USERPROFILE\go\bin`). Don't alias it to `jot`, its old
name: macOS and the BSDs ship `/usr/bin/jot`, a number-sequence tool, which
can run instead. Check where it will store data and
where the binary is.

macOS, Linux and the BSDs:

```sh
"$(go env GOPATH)/bin/aside" paths
```

Windows PowerShell:

```powershell
& "$(go env GOPATH)\bin\aside.exe" paths
```

```
db:           /Users/you/Library/Application Support/jot/jot.db (default)
busy timeout: 2s
binary:       /Users/you/go/bin/aside
```

**Use the absolute `binary:` path in the hook and MCP configs below**, shown
here as `/Users/you/go/bin/aside`. GUI editors often run hooks with a minimal
`PATH`, and a bare command name runs whatever is found first on that `PATH`.
`aside paths` prints a warning if `aside` on your `PATH` is missing or is a
different program.

On Windows, JSON strings must either use forward slashes:

```json
"command": "C:/Users/you/go/bin/aside.exe hook claude"
```

or escape every backslash:

```json
"command": "C:\\Users\\you\\go\\bin\\aside.exe hook claude"
```

## Claude Code hook

Add to `~/.claude/settings.json` (or a project's `.claude/settings.json`):

```json
{
  "hooks": {
    "UserPromptSubmit": [
      {
        "hooks": [
          { "type": "command", "command": "/Users/you/go/bin/aside hook claude" }
        ]
      }
    ]
  }
}
```

On a jot, the hook prints
`{"decision":"block","reason":"✓ Jotted #N","suppressOriginalPrompt":true}`:
Claude Code erases the prompt from context and shows you the reason. Every
other prompt produces no output at all. Output from `UserPromptSubmit` would be
added to Claude's context, so the hook stays silent.

Cursor also imports this hook, but aside does not work with Cursor: see
[docs/cursor.md](cursor.md).

Known Claude Code behaviour, outside aside's control (seen with v2.1.283):

- `suppressOriginalPrompt` is ignored, so the block message repeats your jot
  as "Original prompt: >> …". Only you see it; the model does not.
- In web and mobile sessions, Claude Code may send the first prompt to a small
  model to generate the session title while the hook is still running, so the
  title can be derived from a jot. The conversation model never receives it.
  In the terminal CLI no such request was observed.

## Codex hook

Codex runs local `UserPromptSubmit` hooks. Add to `~/.codex/hooks.json` (or a
trusted project's `.codex/hooks.json`):

```json
{
  "hooks": {
    "UserPromptSubmit": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "/Users/you/go/bin/aside hook codex",
            "commandWindows": "C:/Users/you/go/bin/aside.exe hook codex"
          }
        ]
      }
    ]
  }
}
```

Then run `/hooks` in the **Codex CLI**, review the hook and **trust** it. The
Codex desktop app has no `/hooks` command (it sends `/hooks` to the model as an
ordinary message), so use the CLI to review hooks.

> **Until the hook is trusted, it does not run, and `>>` prompts go to the
> model.** Codex only runs a hook whose exact definition you have trusted, so
> changing the command (for example, a new binary path) needs trusting again in
> `/hooks`. Hooks can also be switched off with `[features] hooks = false` in
> `~/.codex/config.toml`. After any change, type `>> test` and check you get
> `✓ Jotted #N` rather than a model reply.

On a jot, the hook prints `{"decision":"block","reason":"✓ Jotted #N"}` and
Codex shows you the reason. Every other prompt produces no output at all: Codex
adds a hook's plain stdout to the model's context. Captures are stored with
`source: codex`, the Codex session id, and the turn id and model in metadata.

Hooks and local MCP servers apply to local Codex sessions. Codex
cloud tasks and ordinary ChatGPT conversations don't run them.

Verified with Codex CLI v0.159.3 on Windows, using the `commandWindows`
forward-slash path above:

- A `>>` prompt shows "Blocked by hook ✓ Jotted #N" and the model does not
  reply. Codex sends `hook_event_name: "UserPromptSubmit"`, as documented.
- Ordinary prompts reach the model as normal.
- The jot is stored with `source: codex`, the Codex session id, `cwd`, and the
  turn id and model in metadata.
- Codex can read jots through the `aside` MCP server's `inbox` tool.

With Codex CLI v0.159.2 on macOS, a blocked jot (#18) was not added to the
session's conversation history: the session file, which records the history
Codex replays on resume, has no trace of it. The model's reply to a follow-up
question was not checked.

Not yet verified: whether anything sees the prompt before the hook runs (as
Claude Code's session-title model does, above). Codex reports a few seconds of
"Worked for" time on a blocked prompt, though no model reply appears.

## MCP server (read-only)

```sh
claude mcp add --scope user aside -- /Users/you/go/bin/aside mcp   # Claude Code
codex mcp add aside -- /Users/you/go/bin/aside mcp                 # Codex
codex mcp list
```

On Windows PowerShell, quote the absolute executable path:

```powershell
claude mcp add --scope user aside -- "$(go env GOPATH)\bin\aside.exe" mcp
codex mcp add aside -- "$(go env GOPATH)\bin\aside.exe" mcp
```

For Codex, the same entry in `~/.codex/config.toml`:

```toml
[mcp_servers.aside]
command = "/Users/you/go/bin/aside"
args = ["mcp"]
```

For Cursor, merge this into the `mcpServers` object in `~/.cursor/mcp.json`:

```json
"aside": {
  "command": "/Users/you/go/bin/aside",
  "args": ["mcp"]
}
```

Cursor reloads `mcp.json` on save. A server added mid-session connected over
stdio on 3.22.12 but did not appear in that chat's tool list. After a restart
onto 3.23.12, the same conversation could call `show`, and it returned jot 9.

To have Codex check your notes on its own, add a line to your `AGENTS.md`,
for example: "My side notes are in the `aside` MCP server; check `inbox` when
starting work on this repo."

Tools: `inbox`, `show`, `search`, `find` (the same lookup as `aside find`,
with the resume command to show you, not run). `inbox`, `search` and `find`
take a `limit` (default 20, at most 200). `find` also returns the totals
before the cut, and lists each session's 5 newest matching jots with a
`match_count`. There is intentionally **no capture tool**:
only you write jots, never the model. `inbox` entries carry `age_days`
(calendar days since the jot, local time; 0 = today). Likewise there is no promote tool: only
you turn jots into issues. `show` returns a promoted jot's `issue_url`.

`inbox`, `show`, `search` and `find` also return `open_url`, a link to the chat a jot
came from, for the agent to show you as a clickable link (the tools open
nothing). It is set for Codex app threads (`codex://threads/<id>`) and for
Claude Desktop sessions (`claude://code/continue?session=local_<id>`, which
Desktop doesn't document). It is empty for everything else, including archived
or deleted Desktop sessions, sessions started in the terminal, and
`claude-desktop-3p`; use `find`'s `resume_command` there. The Desktop link is
built on macOS only, from Desktop's session files; elsewhere it is empty.
`aside show` prints the same link as `open:`. The Codex link is untested for
threads started in the Codex CLI.
