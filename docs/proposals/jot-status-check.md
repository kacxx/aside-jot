# Jot status check (#78): design proposal

Status: proposal for review, nothing built.

## The example this must handle

In a Claude Desktop session the user jotted `>> get code reviews on the new skill PRs, including the retro skill` (#31, with a typo in "retro"). They carried on in that same session, asked for the PR URLs, and the session then worked through the review comments on four PRs. Later, another agent reading the inbox should be able to open #31's session, see that the reviews happened after the jot, and mark #31 done.

Two things follow from the example:
- The evidence is in the **same session's transcript, after the jot's timestamp**. That is the easy and common case, so it is the one to build for.
- The jot text and the later work don't match word for word ("code reviews" vs "address review comments", plus a typo). Keyword matching would miss it; only an agent reading it can judge. That confirms the judging belongs to the agent, not aside.

## What aside already has (checked on main, 0.2.0)
- Each hook jot stores `source`, `session_id`, `cwd`, repo, branch, commit SHA, `created_at`.
- Claude Code and Claude Desktop jots store `transcript_path` in metadata. `internal/app/reply.go` already reads these transcripts (`promote --with-reply`).
- Codex: `transcript_path` is null in the recorded fixture, so Codex jots probably have no transcript. Not verified on a real Codex session.
- `aside done <id> --note "why"` already records a reason. The MCP server is read-only by design; the aside skill (PR #72) tells agents not to run `done` unprompted.

## Proposed shape
1. **New read-only `activity <id>` (MCP tool + CLI).** For a jot with a readable transcript, returns the session's records after `created_at`: user prompts and assistant text (tool calls summarised, size-capped, oldest first), plus how many records were skipped. No judgement. Returns `transcript: unavailable` plus the reason for Codex, deleted files, or `aside add` jots.
2. **Aside skill gets an "update" routine.** "update jot 31" means: call `show`, call `activity`, then say "done because X (evidence)", "no sign yet", or "can't tell", and offer `aside done 31 --note "..."`, with the note naming the evidence (e.g. "review comments on the four PRs addressed later in the same session").
3. **"update all"** is the skill looping over open jots that have a session (oldest first, default maybe 20 at a time to limit reading), giving one verdict per jot and one `done` line per jot.
4. Optional later: if the jot's session later has no more records, say so ("session ended 3 days ago with no mention").

Reuse: generalise the `reply.go` transcript reader from "reply before time t" to "records after time t".

## What I'd not do
- No `aside update` that decides by itself (heuristic or LLM, both wrong for aside).
- No write tools on the MCP server.
- No GitHub/Jira lookups from aside.

## Open questions
1. Agent proposes and you approve each `done` (recommended), or may it close clear cases itself? The second means relaxing the skill's no-`done` rule. Possible middle: it closes only when the evidence is in the transcript and says so in the note.
2. The jot may be actioned in a *different* session (e.g. via a PR from another chat). Out of scope for v1; `find` can already locate other sessions by ticket or PR number.
3. Codex: accept "can't tell" for now?
4. Command name: `activity`, `since` or `transcript`.

## Size
One app function, one MCP tool, one CLI subcommand, tests with a recorded transcript fixture, a skill section. Suggested prefix `feat:`.
