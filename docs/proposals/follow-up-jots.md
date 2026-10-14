# Follow-up jots when closing a jot (#95): proposal

Status: **decided**. Ready to merge, then slice into tickets.
Source: #95

Decisions D1 to D9 come from the grill on #95 (2026-10-14), recorded in the issue body.

## Problem

When "update jot N" finds the jot done but leaves work behind, that work ends up only in the `done --note`. The jot then leaves the inbox, and nothing brings the follow-up back.

Example (2026-10-14): a jot asking for a staging test key was done, but the test the key was for was still blocked on access. The agent put the blocker in the suggested `--note` and told the user to type a new `>>` jot if they wanted a reminder. It can't create jots itself, by design.

## Decisions

### D1: When to offer a follow-up

The agent offers a follow-up only when the records show open work that nothing else tracks (no Jira ticket, PR or other open jot covers it). A follow-up that duplicates a ticket is just more inbox noise.

### D2: Partly done jots

When the work is partly done, the agent offers both keeping the jot open and closing it with a follow-up, and the user picks.

### D3: Follow-up wording

The text reads `follow-up to #<id>: <the open work>`, written by the agent in its own words. Naming the source jot keeps the link to the session through `aside show <id>`. Own words keep it short and stop transcript text from being copied into a command.

### D4: Safe command form

The offered command passes the text through stdin with a quoted heredoc, not as an argument:

```
/full/path/to/aside add <<'EOF'
follow-up to #29: <the open work>
EOF
```

Inside `<<'EOF'` nothing expands, so running the command can't execute anything hidden in the text.

### D5: `aside add --from <id>`

Not tracked for now. Naming the jot (D3) is enough until there's evidence the missing session link matters.

### D6: Command order and note

The `aside add` line comes before the `aside done` line, and the done note ends in "; follow-up added as a new jot". Adding first means the follow-up exists before the original leaves the inbox.

### D7: "Update all"

Each follow-up line sits under its jot's done line and doesn't count toward the 10-session limit. The limit is about reading transcripts, and a follow-up costs no extra reads.

### D8: Run from the original repo

The offered command first `cd`s to the source jot's repo root (from `aside show <id>`), so the follow-up is listed under the same repo. It is left out if that folder no longer exists. Otherwise the follow-up records whatever folder the user runs it from.

### D9: Windows command form

On Windows, the agent offers a single-quoted here-string piped into aside instead of a heredoc:

```powershell
@'
follow-up to #29: <the open work>
'@ | & 'C:\path\to\aside.exe' add
```

A single-quoted here-string expands nothing, as with D4. This issue also includes the CLI fix that strips a leading BOM from `aside add`'s stdin, so Windows follow-ups don't start with a stray character.

## Shape

**Skill (`skills/aside/SKILL.md`):**
- "Updating a jot", step 5: when the records show open work and nothing else tracks it (D1), offer the follow-up command before the `aside done` line (D6). The follow-up text is `follow-up to #<id>: <the open work>` in the agent's words (D3), and the done note ends in "; follow-up added as a new jot" (D6).
- For a partly done jot, offer two options: keep it open, or close it with the follow-up (D2).
- "Updating all jots": each follow-up line goes under its jot's done line and doesn't count toward the 10 sessions (D7).
- "Things to leave to the user" is unchanged: the agent never runs `aside add` itself.

**Command form.** The agent picks the form from the full path to aside it already uses: a path ending in `.exe` or starting with a drive letter gets the Windows form.

macOS and Linux (D4, D8):

```
cd '<repo root>' && /full/path/to/aside add <<'EOF'
follow-up to #29: <the open work>
EOF
/full/path/to/aside done 29 --note "<evidence>; follow-up added as a new jot"
```

Windows (D9, D8):

```powershell
Set-Location -LiteralPath '<repo root>'
@'
follow-up to #29: <the open work>
'@ | & 'C:\path\to\aside.exe' add
& 'C:\path\to\aside.exe' done 29 --note "<evidence>; follow-up added as a new jot"
```

Defaults the decisions leave to the implementer:
- The repo root is the `root:` line of `aside show <id>`. If the jot has no repo root, or the folder no longer exists, the `cd` / `Set-Location` line is left out (D8).
- A repo root containing a single quote is also left out, rather than escaped.
- On Windows the follow-up text is plain ASCII (no curly quotes or em dashes): Windows PowerShell 5.1 pipes text to native programs as ASCII by default, so other characters would arrive as `?`.

**CLI (`cmd/aside/main.go`, `add`):** when the text comes from stdin, strip one leading U+FEFF before `Capture` (D9). Test in `cmd/aside/main_test.go`: stdin `"\ufeffbom jot\r\n"` is stored as `bom jot`. Text given as arguments is unchanged.

## Fact-check

| Claim | Result | Source |
|---|---|---|
| `aside add` records the folder it is run from, with its repo root, name and branch | ✅ | `cmd/aside/main.go:165-166`, `internal/app/app.go:87-96`; probe: `aside add` from go-lib stored `cwd`, `repo`, `root`, `branch` |
| `aside add` has no flag to copy another jot's session or metadata | ✅ | `cmd/aside/main.go:156-170` (only text, `Source: "cli"`, `Cwd`) |
| `aside add` reads the text from stdin when given no arguments | ✅ | `cmd/aside/main.go:157-164`; probe with a heredoc |
| Capture trims surrounding whitespace, so a heredoc's trailing newline or `\r\n` isn't stored | ✅ | `internal/app/app.go:83`; probe: `"…jot\r\n"` stored as `…jot` |
| `sessions` leaves out jots with no session | ✅ | `internal/app/sessions.go:211` |
| `find` lists jots with no session under "Not in a session" (CLI) and `not_in_a_session` (MCP) | ✅. This corrects the issue's fact list, which said `find` leaves them out | `internal/app/sessions.go:126`, `internal/mcp/server.go:449-461`; probe: `aside find follow-up` printed "Not in a session: #1" |
| `done --note` text is searchable | ✅ | `internal/store/store.go:376-383` (`json_extract(metadata, '$.done_note') LIKE ?`) |
| `aside show <id>` prints the jot's session and repo root | ✅ | probe: `show` printed `session:` and `root:` lines |
| The skill already says never to run `aside add` and to offer the command instead | ✅ | `skills/aside/SKILL.md:95-98` |
| The skill's "Updating a jot" step 5 offers `aside done`, and "Updating all jots" has a 10-session limit | ✅ | `skills/aside/SKILL.md:56`, `:65` onward |
| A quoted heredoc (`<<'EOF'`) expands nothing | ✅ | probe in zsh and bash: `$(echo PWNED)`, `` `id` `` and `"quotes"` were stored literally and nothing ran |
| A single-quoted PowerShell here-string expands nothing | ✅ (docs; not probed, no PowerShell on the test machine) | [about_Quoting_Rules](https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_quoting_rules) (5.1 and 7): "In single-quoted here-strings, variables aren't replaced by their values" |
| A leading BOM on `aside add`'s stdin is stored today | ✅ | probe: stdin `EF BB BF "bom jot"` stored with the BOM |
| A BOM is stripped only when reading config files | ✅ | `internal/setup/config.go:43`; no other BOM handling in `*.go` |
| `"text" \| aside add` in Windows PowerShell 5.1 stored U+FEFF | ✅ | owner, Windows results on #77 ("BOM from a PowerShell pipe is stored") |
| #80 left the PowerShell-pipe BOM for later | ✅ | #80 description ("Left for later: the BOM from a PowerShell pipe into `aside add`") |
| Windows PowerShell 5.1 pipes to native programs in `$OutputEncoding`, which defaults to ASCII | ✅ | [about_Preference_Variables (5.1)](https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_preference_variables?view=powershell-5.1#outputencoding): "Default: ASCIIEncoding object" |

## Out of scope

- `aside add --from <id>` (D5).
- Any MCP server change: the agent offers commands; it gets no write tool.
- Changing how the existing `aside done --note "<evidence>"` line is quoted (see Risks).
- A hook payload that starts with a BOM, reported on #77 as passing through uncaptured. Hosts don't normally send one.

## Risks

- **The existing `--note` is double-quoted.** The skill already offers `aside done <id> --note "<the evidence>"`, and the evidence is drawn from the transcript, so a `$(…)` in it would run when the user runs the command. D4 protects the new `add` line; the `done` line has the same exposure today. Worth its own issue.
- **Terminator lines.** A follow-up line that is exactly `EOF` would end the heredoc early, and on Windows a line starting with `'@` (or a curly `’@`, since PowerShell treats curly quotes as quotes) would end the here-string. A one-line follow-up in the agent's words makes this unlikely.
- **Non-ASCII on Windows.** Under Windows PowerShell 5.1's default ASCII encoding, any non-ASCII character becomes `?`. Shape keeps the Windows text ASCII. PowerShell 7 defaults to UTF-8 and is unaffected.
- **"Tracked elsewhere" is the agent's call (D1).** It can miss a ticket and offer a duplicate follow-up. The user approves every `add`, so the cost is one extra jot.
