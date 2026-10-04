# Decisions

Why aside is the way it is. Each entry says what was decided, why, and what
would make it worth revisiting. Add an entry when a choice is likely to be
questioned again; don't rewrite old ones, add a new entry that replaces them.

## Not in v1, on purpose

**Decided:** v1 leaves these out:

- **Triage**: no priorities, tags, snoozing or workflows beyond `inbox`/`done`.
- **Promote beyond GitHub**: `aside promote` creates GitHub issues only, on
  request; no TODOs, Jira tickets or automatic promotion.
- **Classification**: no automatic categorising or summarising, and no LLM in
  the loop.
- **Sync**: local SQLite file only; use `aside backup` to copy it.
- **UI**: CLI and MCP only.
- **Cursor**: not supported until Cursor stops sending blocked prompts to the
  model; see [cursor.md](cursor.md).

**Why:** the MVP proves one thing: capturing is instant, reliable and
invisible to the model. Everything else can come later.

**Revisit when:** real use shows one of these is missing often enough to be
worth its weight.

## One tool for every agent, not one per agent

**Decided:** a single binary with a small adapter per agent
(`internal/hooks/<agent>.go`), not separate Claude Code, Codex and Cursor
projects.

**Why:** the capture rule, storage, git context, CLI and MCP server are the
same for every agent; only the hook payload and reply format differ, and those
are a few dozen lines each. Separate projects would triplicate the parts that
matter and drift apart. Agent-specific behaviour, such as Cursor running Claude
Code hooks, is handled where the adapters meet, which separate tools couldn't
see.

**Revisit if:** an agent's hook model diverges so far that its adapter
dominates the codebase.

## Cursor is parked until Cursor fixes its bug

**Decided:** Cursor is documented as not supported. `aside hook cursor` stays in
the binary and its tests stay green, but nothing new is built for Cursor.

**Why:** Cursor keeps a blocked prompt in the chat and sends it to the model
with the next message ([#25](https://github.com/kacxx/aside-jot/issues/25)).
Cursor has reproduced it and is tracking it, with no date for a fix. A hook
only decides whether a prompt is sent now, so aside can't work around it, and
"the model never sees a jot" is the product's main promise. Cursor's own
workaround (edit the blocked message and resend) was verified on Cursor
3.23.12, but it's manual and easy to forget.

**Revisit when:** Cursor ships a fix. The hook replays in
[SMOKE_TEST.md](SMOKE_TEST.md) only check the hook's JSON and stay green
while the bug exists, so they can't confirm a fix. Confirm it in Cursor on
that version, the way the edit-and-resend check in [cursor.md](cursor.md) was done on 3.23.12:
in one chat, jot `>> ` with a word that appears nowhere else, send an ordinary
message, then ask the model to quote every earlier message. Cursor is fixed
only if the word never reaches the model.

## The MCP server is read-only

**Decided:** the MCP server offers `inbox`, `show`, `search` and `find` only. Agents
can't create, edit, close or delete jots.

**Why:** jots are the user's notes. An agent that tidies them silently can
lose something the user meant to keep, and a read-only server can't be
prompt-injected into doing that. Capture already has a reliable path (the
hook), and closing a jot is one command.

**Cost:** an agent triaging the inbox can only tell the user which jots to
close. If that friction matters, prefer making the CLI faster to use (for
example, `aside done` taking several ids) over giving the server write access.

**Revisit if:** there's a write that's safe to do without asking, or a
confirmation step the agent can't bypass.

## Find and sessions shipped as a thin slice

**Decided:** `aside find` and `aside sessions` (PR #32) group existing jots by
the session id hooks already record. No new capture fields, tags or indexes.
The larger session-index design stays in
[#20](https://github.com/kacxx/aside-jot/issues/20) and
[#30](https://github.com/kacxx/aside-jot/issues/30).

**Why:** what's useful is easier to learn from daily use than to design up
front. The slice needed no schema change, so it's cheap to change or drop.

**Revisit after:** a few weeks of real use. Let what's missing in practice
decide what comes next from #20.

## Resume commands use where the session started

**Decided:** for Claude Code, the printed resume command `cd`s to the first
`cwd` in the session's transcript, falling back to the jot's `cwd` if the
transcript is gone. For Codex, there's no `cd`.

**Why:** Claude Code files a session under the directory it started in. A
jot's `cwd` is wherever the agent was at the time, which can be a worktree or
repo it moved into later; resuming from there can't find the session. Codex
looks sessions up globally, so a `cd` would only make the command fail if that
directory was deleted.

**Revisit if:** either agent changes how it stores or finds sessions.

## Promote only creates GitHub issues, on request

**Decided:** `aside promote <id>` creates one GitHub issue through `gh`, only
when asked, with `--dry-run` to preview it. No other trackers and no automatic
promotion.

**Why:** it keeps the "no LLM in the loop, nothing automatic" promise and
reuses the user's existing `gh` authentication instead of storing tokens.
Including the agent's reply in the issue
([#33](https://github.com/kacxx/aside-jot/issues/33)) is opt-in for the same
reason: transcripts can contain secrets and the target repo may be public.

## Ticket keys are matched at query time, not tagged

**Decided:** `aside find SUP-4821` runs the substring search, then keeps the
jots where the key appears as a whole word. There is no `jot_tags` table, no
prefix allowlist and no backfill. The MCP server gets a read-only `find` with
the same lookup.

**Why:** the problem to solve was `SUP-4821` also finding `SUP-48210`. Tags
written at capture time depend on the environment of whichever process captured
the jot (hooks in GUI editors run with a minimal one), so a jot captured
without the allowlist would be untagged and `find` would silently miss it,
which substring search never did. A query-time filter has nothing to drift out
of sync, needs no migration, and the data is small enough that an index buys
nothing noticeable. A plain `[A-Z]+-\d+` pattern would also match `UTF-8`, which
only matters when picking keys out of text; a user asking for `UTF-8` gets
whole-word matches of it.

**Revisit if:** `find` needs to answer questions the text can't, such as "all
ticket keys in this session" or listing tickets by count. Then add `jot_tags`
with the allowlist in a config file next to the database, so every process
sees the same one.

