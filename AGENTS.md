# AGENTS.md

Context for coding agents working on aside. Read this first, then the README
for user-facing behaviour. Why things are the way they are is in
[docs/decisions.md](docs/decisions.md); check it before reopening a settled
question.

## What aside is

A Go CLI that captures notes ("jots") typed as `>> …` in an AI coding agent.
An agent hook sees the prompt, saves it to a local SQLite database with its git
context, and blocks it so the model never sees it. Jots are read back with the
CLI or a read-only MCP server, and can be promoted to GitHub issues.

Supported: Claude Code and Codex. Cursor is not supported (see below). The
command was renamed from `jot` to `aside`; the data directory and the `JOT_*`
environment variables kept the old name on purpose.

## Code map

- `cmd/aside/` — CLI entry point, flag parsing and all terminal output
  (`printList`, `printSession`, `age`).
- `internal/hooks/` — one file per agent (`claude.go`, `codex.go`,
  `cursor.go`) decoding its payload and shaping its replies; `prompt.go` is
  the gating sequence they share.
- `internal/capture/` — the `>>` rule, deliberately narrow: `>>`, one space,
  then a non-space character.
- `internal/app/` — the service layer: capture, inbox, done, `promote.go`
  (jot to GitHub issue via `gh`), `sessions.go` (`find` and `sessions`).
- `internal/store/` — SQLite (pure Go, no cgo), one `entries` table, owner-only
  file permissions.
- `internal/gitctx/` — best-effort git context with a time budget.
- `internal/mcp/` — the read-only MCP server (`inbox`, `show`, `search`, `find`).
- `testdata/hooks/<agent>/` — recorded hook payloads used by tests and the
  smoke test.
- `test/e2e/run.sh` — black-box install and hook flow, run in CI.

## Commands

```sh
gofmt -l .                 # must print nothing
go vet ./...
go test -race ./...
golangci-lint run          # v2.14.0, config in .golangci.yml
bash test/e2e/run.sh       # end-to-end, throwaway database
```

CI runs all of these on Linux. On Windows it runs `go vet` and `go test`
(without `-race`, which needs cgo).

`go install ./cmd/aside` installs to `$(go env GOPATH)/bin`, which is
`~/go/bin` unless `GOPATH` is set.

Use a throwaway database (`JOT_DB="$(mktemp -d)/jot.db"`) for any manual
run; never write test jots to the real one.
[docs/SMOKE_TEST.md](docs/SMOKE_TEST.md) has the manual checks, including
`find` and `sessions`.

## Facts that are easy to get wrong

- **Pass-through must be silent.** For an ordinary prompt, the Claude Code and
  Codex hooks print nothing: both add hook stdout to the model's context.
  Cursor's hook prints `{"continue":true}`.
- **A failed save still blocks.** If the database is unwritable, a `>>` prompt
  is blocked with "NOT saved" and the original text, and the hook exits 0.
- **Cursor runs Claude Code hooks too.** It imports user hooks from
  `~/.claude/settings.json` and runs them on `beforeSubmitPrompt` next to its
  own, then merges the replies and joins their messages. So one Cursor prompt
  can reach aside twice; captures are deduplicated by Cursor's
  `generation_id` (`CaptureRequest.OnceKey`), and only the reply that saved
  the jot carries a message.
- **Cursor sends blocked prompts anyway.** It keeps a blocked prompt in the
  chat and sends it with the next message. This is a Cursor bug
  ([#25](https://github.com/kacxx/aside-jot/issues/25)) that aside can't work
  around, which is why Cursor is unsupported. Don't test jots in a Cursor chat
  you care about.
- **Resume directories differ by agent.** Claude Code stores a session under
  the directory it started in, which can differ from where a jot was captured
  if the agent changed directory. `sessions.go` reads the first `cwd` in the
  Claude transcript (`transcript_path` in the jot's metadata). Codex resumes
  from any directory, so its resume command has no `cd`.
- **The MCP server is read-only by design.** Agents can read jots but not
  create, edit or close them.
- **Hook paths are absolute.** Hook and MCP configs use the full path to the
  binary; `aside paths` warns when a bare `aside` would run something else.

## Working agreements

- Changes go through a PR on a branch, with tests. CI must be green.
- After a feature merges, test it by hand: hook payloads into a throwaway
  database, then the real database read-only. Check printed commands against
  the files they point at.
- Ask before posting to GitHub (comments, issues, reviews) and before
  replacing the installed binary or editing hook configs. Back up anything
  replaced.
- README examples must match real output. Docs claim only what was verified,
  and say which version it was verified on.

## Current work

Open issues are the roadmap and the status: `gh issue list -R kacxx/aside-jot`.
Don't keep status in this file.
