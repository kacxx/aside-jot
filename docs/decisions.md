# Decisions

Why aside is the way it is. Each entry says what was decided, why, and what
would make it worth revisiting. Add an entry when a choice is likely to be
questioned again; don't rewrite old ones, add a new entry that replaces them.

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

**Revisit when:** Cursor ships a fix. Re-run the Cursor checks in the README
and [SMOKE_TEST.md](SMOKE_TEST.md) on that version before claiming support.

## The MCP server is read-only

**Decided:** the MCP server offers `inbox`, `show` and `search` only. Agents
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
