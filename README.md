# aside-jot

A side channel for thoughts while you work with coding agents.

Type `>> some thought` into Claude Code or Codex and it is stored locally with
its git context (repo, branch, commit), and the prompt is **blocked**, so the
model never sees it. Your flow isn't interrupted and the agent's context stays clean.

**aside does not work with Cursor.** Cursor sends a blocked prompt to the model
with your next message, so a jot there is not private: see
[Cursor (not supported)](#cursor-not-supported).

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

This installs `aside` into `$(go env GOPATH)/bin` (usually `~/go/bin`; on
Windows, usually `$env:USERPROFILE\go\bin`). Check where it will store data and
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

Cursor also imports this hook, but aside does not work with Cursor: see
[Cursor (not supported)](#cursor-not-supported).

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

## Cursor (not supported)

> **aside does not work with Cursor, and Cursor support is paused.** The hook
> saves the jot and blocks that turn, but Cursor keeps the blocked prompt in the
> chat and sends it to the model with your next message there
> ([#25](https://github.com/kacxx/aside-jot/issues/25)). This is a
> [known Cursor bug](https://forum.cursor.com/t/prompt-blocked-by-a-beforesubmitprompt-hook-is-still-sent-to-the-model-with-the-next-message/173565),
> and aside can't work around it: the hook only decides whether a prompt is
> sent now. Don't rely on `>>` in Cursor; jot from a terminal with `aside add`
> instead. Cursor also runs the Claude Code hook from
> `~/.claude/settings.json`, so the same applies if only that one is installed.
>
> Cursor has reproduced the bug and is tracking it, with no date for a fix.
> Cursor recommends clicking the blocked message, editing it into the prompt you
> want to send, and resending. On Cursor 3.23.12 this kept the jot out of the
> model's context: after editing a blocked jot (#19) into a question, the model
> said it was the first message, and on the next turn quoted only the two
> messages sent after it. Starting a new chat also avoids the leak, since the
> jot is sent with the next message in the same chat.
>
> `aside hook cursor` stays in the binary, and the setup and test notes below
> are kept for when Cursor fixes the bug.

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

Cursor reloads this file on save.

Cursor also loads Claude Code user hooks from `~/.claude/settings.json` and runs
them on `beforeSubmitPrompt` next to its own, sending them
`hook_event_name: "beforeSubmitPrompt"`. On 3.23.12, with both hooks installed
and the Claude hook printing nothing, the prompt reached the model even though
`aside hook cursor` replied `{"continue":false}`. With the Claude hook removed,
the jot was blocked ([#23](https://github.com/kacxx/aside-jot/issues/23)). So on
that event `aside hook claude` now replies exactly as `aside hook cursor` does:
it blocks jots and answers `{"continue":true}` to every other prompt. Either
hook is enough in Cursor, and both together are safe. If both run on one
prompt, the jot is saved once, keyed on Cursor's `generation_id`. Only the hook
that saved it shows `✓ Jotted #N`; the other blocks without a message, so
Cursor doesn't show the confirmation twice. After any change, type `>> test`
and check you get `✓ Jotted #N` rather than a model reply.

On a jot the hook returns `{"continue":false,"user_message":"✓ Jotted #N"}`.
Every other prompt gets `{"continue":true}`. The conversation id is stored as
the session. With a single workspace root, that root is used as the working
directory. With several roots, aside doesn't guess: it records all of them in
the entry's metadata and leaves the git fields empty. A multi-root window is a
normal Cursor layout, and every jot captured there has no repo, branch, or
commit.

Verified with Cursor 3.22.12 and, after a restart, 3.23.12, on macOS. This was
before the fix for #23, while the Claude hook was silent on `beforeSubmitPrompt`:

- An ordinary prompt ran `aside hook cursor` from `~/.cursor/hooks.json` and
  returned `{"continue":true}` in 16ms, and the model received it.
- A `>>` prompt sent while only the Claude hook was installed ran as
  `aside hook claude`, produced no output, and reached the model.
- Replayed with `hook_event_name: "beforeSubmitPrompt"`: a `>>` prompt returns
  `{"continue":false,"user_message":"✓ Jotted #N"}`. One workspace root records
  repo, branch, and commit. Two or more leave the git fields empty and store
  the roots in metadata.
- `>>x`, a leading space, and `>>` mid-sentence pass through.
- Live on 3.23.12 with both hooks installed, `>> hello from cursor` ran
  `aside hook cursor`, which returned
  `{"continue":false,"user_message":"✓ Jotted #9"}`. The imported Claude hook
  produced no output, and the prompt still reached the model. `aside show 9`
  has `source: cursor`, the conversation id as the session, no repo, branch,
  or commit, and the workspace roots in metadata.
- Live on 3.23.12 with the Claude hook removed, `>> test` returned
  `✓ Jotted #10` and the model did not reply.

Verified live on Cursor 3.23.12, macOS, after the fix for #23. A `>>` prompt was
blocked and saved once in each setup:

- Both hooks: jot #13. Both hooks replied `{"continue":false}` and Cursor
  logged "Merged 2 valid response(s)". That build showed `✓ Jotted #13` twice.
- Both hooks again, with the hook that finds the jot already saved replying
  without a message: jot #17, and `✓ Jotted #17` appeared once.
- Only `aside hook claude`, imported by Cursor: jot #14, saved with
  `source: cursor`.
- Only `aside hook cursor`: jot #15.
- After the both-hooks and Claude-only runs, the next message in that chat sent
  the blocked jot to the model with it
  ([#25](https://github.com/kacxx/aside-jot/issues/25)).

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

Tools: `inbox`, `show`, `search`. There is intentionally **no capture tool**:
only you write jots, never the model. Likewise there is no promote tool: only
you turn jots into issues. `show` returns a promoted jot's `issue_url`.

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
aside find <query...>      sessions with a matching jot, and how to resume them
aside sessions [-n N]      recent sessions with their label (default 10, 0 = all)
aside done <id>            mark done (drops out of the inbox)
aside promote <id> [--repo owner/name] [--with-reply] [--dry-run]
                           turn a jot into a GitHub issue (see Promote)
aside backup <path>        consistent copy via VACUUM INTO; never overwrites
aside paths                where the data and binary live; warns about PATH
aside hook claude|codex|cursor
                           hook entry points (cursor: not supported, see Cursor)
aside mcp                  read-only MCP server on stdio
```

## Finding a session

Every jot captured by a hook records the agent's session id. `aside find`
groups the jots that match a query by session, so you can get back to the chat
where you worked on something:

```
$ aside find SUP-4821
SUP-4821 token TTL investigation
  claude · api@main · 3 jots · last 2d (#18)
  #12   5d     session: SUP-4821 token TTL investigation
  #18   2d     SUP-4821 needs a backend ticket
  cd /Users/you/code/api && claude --resume 0f3c9a…
```

`aside sessions` lists recent sessions the same way, without the matching jots.

- **Label a session** by jotting `>> session: <why>` in it, for example
  `>> session: SUP-4821 token TTL investigation`. The newest `session:` jot
  labels the session; without one, its first jot does.
- **Resume commands** are printed, not run. Claude Code stores a session under
  the directory it started in, so the command starts with a `cd` there, read
  from the session's transcript (the jot's directory if the transcript is
  gone). Codex resumes from any directory, so its sessions get
  `codex resume <id>` with no `cd`. Cursor isn't supported, so its sessions
  have no resume command.
- Done jots are listed with `(done)`, as in `aside search`.
- **Only chats with a jot in them are listed.** A chat where you never jotted
  doesn't appear. Starting a ticket's chat with a `session:` jot makes it
  findable.
- Jots from `aside add` have no session. `find` lists matching ones under
  "Not in a session".

## Promote

Some jots deserve more than a note. `aside promote <id>` turns one into a
GitHub issue, as a deliberate step after capture:

```sh
aside promote 12 --dry-run   # print the target repo, title and body; creates nothing
aside promote 12             # create the issue and print its URL
aside promote 12 --repo kacxx/aside-jot
aside promote 12 --with-reply --dry-run   # preview the body with the agent's reply
```

- The issue is created with the [GitHub CLI](https://cli.github.com)
  (`gh issue create`) under your existing `gh` login. aside stores no tokens;
  run `gh auth login` first.
- **Repo:** `--repo owner/name` if given, else the `origin` remote of the repo
  the jot was captured in (https and ssh remotes both work; for a GitHub
  Enterprise host it becomes `host/owner/name`). A jot captured outside a repo,
  or a repo without an `origin` remote, needs `--repo`.
- **Title:** the jot's first line. **Body:** the full jot, then a line such as
  `Captured 2026-09-30T14:02:11+02:00 from claude on aside-jot@main (abc1234)`,
  leaving out anything unknown.
- **`--with-reply`** (opt-in, Claude Code jots only) adds the agent's last reply
  before the jot under an `## Agent reply` heading, between the jot and the
  `Captured` line. It is read from the transcript the jot recorded, picking the
  last assistant message written before the jot was captured, and cut at 8000
  characters with a note. Transcripts can hold secrets and internal paths, so
  check it with `--dry-run` first, especially when the target repo is public.
  A missing or unreadable transcript, or no reply before the jot, is an error;
  no issue is created without it.
- On success the jot is marked `done` and the issue URL is stored in its
  metadata as `issue_url`, which `aside show` prints. Promoting the same jot
  again is refused and prints the existing URL.
- `--dry-run` never runs `gh` and changes nothing (it does read the `origin`
  remote with git, to show the real target).
- Unlike the hooks, `promote` exits non-zero on any error: `gh` missing or
  not logged in, an unknown id, or no target repo.

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


## Not in v1 (on purpose)

- **Triage**: no priorities, tags, snoozing or workflows beyond `inbox`/`done`.
- **Promote beyond GitHub**: `aside promote` creates GitHub issues only, on
  request; no TODOs, Jira tickets or automatic promotion.
- **Classification**: no automatic categorising or summarising, and no LLM in
  the loop.
- **Sync**: local SQLite file only; use `aside backup` to copy it.
- **UI**: CLI and MCP only.
- **Cursor**: not supported until Cursor stops sending blocked prompts to the
  model; see [Cursor (not supported)](#cursor-not-supported).

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
