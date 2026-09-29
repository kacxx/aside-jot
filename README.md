# aside

A side channel for thoughts while you work with coding agents.

Type `>> some thought` into Claude Code, Codex or Cursor and it is stored locally with
its git context (repo, branch, commit), and the prompt is **blocked**, so the
model never sees it. Your flow isn't interrupted and the agent's context stays clean.

```
>> token cache TTL looks too long, check with infra before shipping
✓ Jotted #12
```

Later you can review your notes from the terminal (`aside inbox`), or let an agent read
them through the read-only MCP server.

![Demo: an ordinary prompt reaches Claude, a ">>" prompt is blocked and saved with its git context, Claude reads the inbox over MCP, and "jot done" clears it](docs/demo.gif)

*Recorded with the real Claude Code CLI; Claude's replies are live, so their wording varies between runs. The recording predates the rename of the command from `jot` to `aside`.*

## Install

Requires Go 1.25+. No cgo: SQLite is pure Go (`modernc.org/sqlite`).

```sh
go install github.com/kacxx/aside-jot/cmd/aside@latest
```

or from a checkout:

```sh
go install ./cmd/aside
```

This installs `aside` into `$(go env GOPATH)/bin` (usually `~/go/bin`). Check
where it will store data and where the binary is:

```sh
"$(go env GOPATH)/bin/aside" paths
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

> The command used to be called `jot`, which on macOS and the BSDs is shadowed
> by the system's `/usr/bin/jot` (a number-sequence tool) and made the hooks
> run the wrong program. If you installed it under that name, see
> [Upgrading from `jot`](#upgrading-from-jot).

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

Then run `/hooks` in Codex, review the hook and **trust** it.

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

Not yet verified against a real Codex session: whether a blocked prompt still
appears in Codex's session history, and whether anything sees the prompt before
the hook runs (as Claude Code's session-title model does, above). The
`commandWindows` path is also untested.

## Cursor hook

Add to `~/.cursor/hooks.json` (or `<project>/.cursor/hooks.json`):

```json
{
  "version": 1,
  "hooks": {
    "beforeSubmitPrompt": [
      { "command": "/Users/you/go/bin/aside hook cursor" }
    ]
  }
}
```

On a jot the hook returns `{"continue":false,"user_message":"✓ Jotted #N"}`.
Every other prompt gets `{"continue":true}`. The conversation id is stored as
the session. With a single workspace root, that root is used as the working
directory. With several roots, aside doesn't guess: it records all of them in
the entry's metadata and leaves the git fields empty.

## MCP server (read-only)

```sh
claude mcp add --scope user aside -- /Users/you/go/bin/aside mcp   # Claude Code
codex mcp add aside -- /Users/you/go/bin/aside mcp                 # Codex
codex mcp list
```

For Codex, the same entry in `~/.codex/config.toml`:

```toml
[mcp_servers.aside]
command = "/Users/you/go/bin/aside"
args = ["mcp"]
```

To have Codex check your notes on its own, add a line to your `AGENTS.md`,
for example: "My side notes are in the `aside` MCP server; check `inbox` when
starting work on this repo."

Tools: `inbox`, `show`, `search`. There is intentionally **no capture tool**:
only you write jots, never the model.

## Capture rules

A prompt is a jot only if it **starts** with `>>`, then exactly one space, then
a non-space character (`^>> \S`). Everything else passes through untouched:

| Prompt          | Jot? |
| --------------- | ---- |
| `>> some thought` | yes  |
| `>>x`           | no   |
| ` >> x`         | no (leading space) |
| `\>> x`         | no (escaped) |
| `a >> b`        | no (not at start) |
| `> quote`       | no   |

Need to send a prompt that starts with `>> ` to the model? Prefix it with a
space or a backslash.

### Failure behaviour

- **Ordinary prompts and malformed hook payloads fail open**: the prompt goes
  through as if aside weren't installed. The prefix is checked *before* SQLite is
  opened or git is run, so a normal prompt costs one small process start.
- **A recognised jot fails safe**: it is always blocked, even if saving failed.
  On failure the message says `✗ Jot NOT saved (<reason>)` and includes your
  original text so you can copy it.
- A payload too large to parse in memory (over 16 MiB) is still checked: its
  prompt is found by streaming, and a jot in it is blocked and reported as
  NOT saved.
- The hook process always exits 0.

## CLI

```
aside add <text...>        capture from the terminal (reads stdin if no text)
aside inbox [-n N]         newest inbox jots (default 20, 0 = all)
aside show <id>            one jot with its context
aside search <query...>    substring search over all jots
aside done <id>            mark done (drops out of the inbox)
aside backup <path>        consistent copy via VACUUM INTO; never overwrites
aside paths                where the data and binary live; warns about PATH
aside hook claude|codex|cursor
                           hook entry points
aside mcp                  read-only MCP server on stdio
```

## Storage

One SQLite file with a single `entries` table: `id, text, created_at, status`
(`inbox`/`done`), `source, session_id, cwd, repo_root, repo_name, branch,
commit_sha, metadata` (JSON).

- Path: `$JOT_DB`, else `$XDG_DATA_HOME/jot/jot.db`, else the OS per-user data
  directory (`~/.local/share/jot`, `~/Library/Application Support/jot`,
  `%LOCALAPPDATA%\jot`).
  The data directory and the `JOT_*` variables kept their names when the
  command was renamed, so an existing database is picked up unchanged.
- The database, its `-wal`/`-shm` files and backups are owner-only (`0600`)
  on Unix, whatever the umask, including when `$JOT_DB` points somewhere
  shared like `/tmp`. An existing database with wider permissions is tightened
  when aside opens it.
- WAL mode and `busy_timeout` are set on every connection.
  `$JOT_BUSY_TIMEOUT_MS` (default 2000, capped at 10000) is how long each
  database step of a capture (opening, then the insert) waits on a locked
  database before the jot is reported "NOT saved". The cap keeps a capture well
  inside Claude Code's 30 s hook timeout (Codex allows 600 s): a hook the host
  kills can't block the jot. If you set a `timeout` on the hook entry, keep it
  above 25 s.
- Git context is best-effort, with a ~750ms budget. A detached HEAD records the
  commit but no branch. Outside a repo, the git fields are empty.

## Upgrading from `jot`

The command was renamed from `jot` to `aside` because macOS and the BSDs ship
an unrelated `/usr/bin/jot` that usually comes first on `PATH`
([#6](https://github.com/kacxx/aside-jot/issues/6)). Your notes stay where they
are; only the commands change.

1. Install `aside` as above and run `"$(go env GOPATH)/bin/aside" paths`.
   The `db:` line should show your existing database.
2. In `~/.claude/settings.json`, `~/.cursor/hooks.json` and your MCP config,
   replace the `jot` command with the absolute `binary:` path, for example
   `/Users/you/go/bin/aside hook claude`. For the MCP server:
   `claude mcp remove --scope user jot`, then add it again as shown above.
3. Remove the old binary: `rm "$(go env GOPATH)/bin/jot"` (on macOS this
   leaves the system `/usr/bin/jot` alone).

## Not in v1 (on purpose)

- **Triage**: no priorities, tags, snoozing or workflows beyond `inbox`/`done`.
- **Promote**: no turning jots into issues, TODOs or tasks.
- **Classification**: no automatic categorising or summarising, and no LLM in
  the loop.
- **Sync**: local SQLite file only; use `aside backup` to copy it.
- **UI**: CLI and MCP only.

The MVP proves one thing: capturing is instant, reliable and invisible to the
model. Everything else can come later.

## Development

```sh
go test -race ./...
golangci-lint run   # v2.14.0, config in .golangci.yml
```

See [docs/SMOKE_TEST.md](docs/SMOKE_TEST.md) for an end-to-end check against
the built binary, and [docs/mvp-validation.md](docs/mvp-validation.md) for the
recorded results (tests, end-to-end run and hook latency) of the initial version.
