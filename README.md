# aside — `jot`

A side channel for thoughts while you work with coding agents.

Type `>> some thought` into Claude Code or Cursor and it is stored locally with
its git context (repo, branch, commit), and the prompt is **blocked**, so the
model never sees it. Your flow isn't interrupted and the agent's context stays clean.

```
>> token cache TTL looks too long, check with infra before shipping
✓ Jotted #12
```

Later you can review your notes from the terminal (`jot inbox`), or let an agent read
them through the read-only MCP server.

## Install

Requires Go 1.25+. No cgo: SQLite is pure Go (`modernc.org/sqlite`).

```sh
go install github.com/kacxx/aside-jot/cmd/jot@latest
```

or from a checkout:

```sh
go install ./cmd/jot
```

Check where it will store data:

```sh
jot paths
```

GUI editors often run hooks with a minimal `PATH`. If `jot` lives in `~/go/bin`,
use the absolute path (`which jot`) in the configs below.

## Claude Code hook

Add to `~/.claude/settings.json` (or a project's `.claude/settings.json`):

```json
{
  "hooks": {
    "UserPromptSubmit": [
      {
        "hooks": [
          { "type": "command", "command": "jot hook claude" }
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

## Cursor hook

Add to `~/.cursor/hooks.json` (or `<project>/.cursor/hooks.json`):

```json
{
  "version": 1,
  "hooks": {
    "beforeSubmitPrompt": [
      { "command": "jot hook cursor" }
    ]
  }
}
```

On a jot the hook returns `{"continue":false,"user_message":"✓ Jotted #N"}`.
Every other prompt gets `{"continue":true}`. The conversation id is stored as
the session. With a single workspace root, that root is used as the working
directory. With several roots, jot doesn't guess: it records all of them in
the entry's metadata and leaves the git fields empty.

## MCP server (read-only)

```sh
claude mcp add --scope user jot -- jot mcp
```

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
  through as if jot weren't installed. The prefix is checked *before* SQLite is
  opened or git is run, so a normal prompt costs one small process start.
- **A recognised jot fails safe**: it is always blocked, even if saving failed.
  On failure the message says `✗ Jot NOT saved (<reason>)` and includes your
  original text so you can copy it.
- The hook process always exits 0.

## CLI

```
jot add <text...>        capture from the terminal (reads stdin if no text)
jot inbox [-n N]         newest inbox jots (default 20, 0 = all)
jot show <id>            one jot with its context
jot search <query...>    substring search over all jots
jot done <id>            mark done (drops out of the inbox)
jot backup <path>        consistent copy via VACUUM INTO; never overwrites
jot paths                where the data lives
jot hook claude|cursor   hook entry points
jot mcp                  read-only MCP server on stdio
```

## Storage

One SQLite file with a single `entries` table: `id, text, created_at, status`
(`inbox`/`done`), `source, session_id, cwd, repo_root, repo_name, branch,
commit_sha, metadata` (JSON).

- Path: `$JOT_DB`, else `$XDG_DATA_HOME/jot/jot.db`, else the OS per-user data
  directory (`~/.local/share/jot`, `~/Library/Application Support/jot`,
  `%LOCALAPPDATA%\jot`).
- WAL mode and `busy_timeout` are set on every connection.
  `$JOT_BUSY_TIMEOUT_MS` (default 2000) is how long each database step of a
  capture (opening, then the insert) waits on a locked database before the jot
  is reported "NOT saved".
- Git context is best-effort, with a ~750ms budget. A detached HEAD records the
  commit but no branch. Outside a repo, the git fields are empty.

## Not in v1 (on purpose)

- **Triage**: no priorities, tags, snoozing or workflows beyond `inbox`/`done`.
- **Promote**: no turning jots into issues, TODOs or tasks.
- **Classification**: no automatic categorising or summarising, and no LLM in
  the loop.
- **Sync**: local SQLite file only; use `jot backup` to copy it.
- **UI**: CLI and MCP only.

The MVP proves one thing: capturing is instant, reliable and invisible to the
model. Everything else can come later.

## Development

```sh
go test -race ./...
```

See [docs/SMOKE_TEST.md](docs/SMOKE_TEST.md) for an end-to-end check against
the built binary, and [docs/mvp-validation.md](docs/mvp-validation.md) for the
recorded results (tests, end-to-end run and hook latency) of the initial version.
