# Routed jots (#30): design proposal

Status: proposal for review. No code until approved.

Built from issue #30, its 4 Oct review comment and docs/decisions.md.
Where this proposal follows that comment, it says so.

## Summary

A jot that starts with a registered route (`jira: …`, `slack: …`) is tagged
with that route at capture. A worker lists open items for its route and closes
them with `aside done <id> <note>`. aside never calls Jira or Slack, has no
claiming, and has no LLM. Every outward action is confirmed by the user, always.

## Decisions

**1. Routes live in a config file next to the database** (`routes` file, one
route name per line). Not an environment variable: the hook runs in whatever
environment the editor gives it, the same problem that ruled out a tag allowlist
in "Ticket keys are matched at query time". If the file is missing, nothing is
routed and everything behaves as today. This answers open question 2.

**2. The route is fixed at capture.** It is stored in the existing `metadata`
JSON (`route`), so there is no schema change and no migration. Registering
`todo` next month doesn't queue old `todo:` jots, and removing a route doesn't
orphan queued ones. Matching: the first token, case-insensitive, must be
`name:` followed by a space and more text, and `name` must be in the file.
Anything else, such as `todo:`, a typo or a bare `jira:`, is ordinary text.
The stored text is unchanged (the `jira:` prefix stays).

**3. Provenance is recorded as a signal, not a security boundary.** Both options
in the issue are forgeable by an agent with a shell, as the review noted
(`env -u CLAUDECODE aside add …`, or piping JSON into `aside hook claude`). So:
- `aside add` never routes. It is the one path that can't be the user typing in
  an agent, so it is the cheap, honest exclusion. Hook captures route.
- The real control is confirmation (decision 5). The docs will say the source
  check is not a boundary. This answers open question 1: hook captures only,
  no environment sniffing.

**4. Worker API stays minimal.**
- `aside inbox --route jira` (and `--route any`, `--route none`), open items
  only by default. Same filter on the MCP `inbox` tool, which stays read-only.
- Closing is the existing `aside done <id> [note]` (#43). A worker passes the
  result link as the note. No new write command.
- No claiming, lease or timeout. Add one only if an item is processed twice.
- No `abandoned` state. `done` with no note already means it (per the review). Add
  one only if abandoned items need reporting.

**5. Confirmation is mandatory, and it is the worker's contract, not code in
aside.** The docs state: a worker may only draft; the user confirms every Slack
post, Jira ticket or other outward action, every time. Jot text often holds
pasted tickets or logs, so a worker with write access acting unconfirmed is the
prompt-injection path. aside can't enforce this, so the docs won't claim it does.

**6. Routed jots still show in `aside inbox`**, marked with their route, e.g.
`#41 [jira] SUP-4821 needs a backend ticket`. They stay in the user's own view, and
`--route none` shows what isn't routed.

**7. `gh` route.** `aside promote` is the `gh` worker in spirit but stays a
command you run. It already always asks `[y/N]` (#45). Nothing to change; the
`gh` route is just a registered name like the others.

## Out of scope

Jira and Slack workers (they live outside the repo, as examples), aside calling
any service, and claiming.

## Rough size

One PR, about: route parsing in `internal/capture` (pure function, table tests
like `Match`), config reading in `internal/app`, `--route` on `inbox` plus the
MCP tool, the `[route]` marker in output, docs and a decisions.md entry. The
hook gating, `done` and the schema are untouched.

## Open questions

1. Is "`aside add` never routes" acceptable? It means a user can't route from their
   own terminal. The alternative (route `cli` jots too) makes the agent-shell
   case route, with only confirmation protecting you.
2. Route file format: plain names, one per line, or something richer like
   `jira = SUP-` later? I'd start plain.
3. Timing: the issue comment says to revisit in late October after a few weeks of
   using `find` and `sessions`. Approve the design now and build then, or wait?
