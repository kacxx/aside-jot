# Reference

How aside behaves in detail. For everyday use, see the [README](../README.md).

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

## Failure behaviour

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
- **Ticket keys match whole words.** A query shaped like a ticket key, such as
  `SUP-4821`, only matches that key as a word, so it doesn't list `SUP-48210`
  or `XSUP-4821`. Any other character next to it counts as a separator,
  including `_` and `/`, so `SUP-4821_cache` and `feature/SUP-4821-fix`
  match. Anything else is a substring search, as in `aside search`.
  There is no list of ticket prefixes to configure and no tag table: the
  filter runs on the matching jots at query time.
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
aside promote 12             # show the issue, ask [y/N], then create it and print its URL
aside promote 12 --yes       # create it without asking
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
- **`--with-reply`** (opt-in, Claude Code jots only) adds the agent's reply
  under an `## Agent reply` heading, between the jot and the `Captured` line.
  The reply is all the assistant text since your last prompt in the transcript
  the jot recorded (a multi-step turn is many messages with tool calls between;
  tool output is not included), up to when the jot was captured, cut at 8000
  characters with a note. Interrupting the agent, a background task finishing,
  a local command such as `/model`, a `!` command the agent doesn't answer, or
  compacting the conversation does not start a new turn, so jotting right
  after any of them still picks up the reply before it. Transcripts can
  hold secrets and internal paths, so check the preview. The reply is posted as written, so an
  `@name` in it notifies that GitHub user and a `#123` links to that issue. A
  missing or unreadable transcript, or no reply before the jot, is an error; no
  issue is created without it. A warning for public repos is not implemented
  yet.
- **Confirmation:** without `--dry-run`, `promote` always shows the repo, title
  and full body and asks `[y/N]` before creating anything, and creates exactly
  the body it showed. `--yes` skips the question. If stdin isn't a terminal and
  `--yes` isn't given, it exits non-zero without asking or running `gh`, so
  piping in "y" doesn't work; an agent has to pass `--yes`, which shows in the
  command a person or approval rule sees. `--yes` is still the agent's choice.
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
