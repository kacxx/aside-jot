# A Claude Code plugin for the aside skill (#110): proposal

Status: **decided**. Ready to merge, then slice into tickets.
Source: #110

Decisions D1 to D7 come from the grill on #110 (2026-10-15), recorded in [the decisions comment](https://github.com/kacxx/aside-jot/issues/110#issuecomment-6187065673). D7 was revised the same day, after this doc's fact-check found its premise wrong. Question 3 in the issue (what happens to `aside setup claude`) no longer applies: under D4 the plugin carries no hook or MCP server.

## Problem

The aside skill (`skills/aside/SKILL.md`) has no way to reach users. Most of 0.4.0 is skill-only (follow-up jots, the Windows form, the "partly done" verdict, safe done notes: #104, #106), but someone who downloads 0.4.0 gets only the binary change (#107). The skill on the owner's machine was copied into `~/.claude/skills/aside/` by hand.

The skill isn't embedded in the binary, the release archive holds the binary and `README.md` only, and until #111 the README didn't mention the skill. 0.4.0 shipped with #111's manual-copy section as the stopgap (D3).

## Decisions

### D1: The channel
A Claude Code plugin, with this repo as its own marketplace (`.claude-plugin/plugin.json` and `marketplace.json`, as dev-flow does it), plus a README section covering a manual copy for anything the plugin doesn't reach. One plugin covers both the terminal and Claude Desktop's Claude Code sessions, and `claude plugin update` keeps it current.

### D2: Hosts in scope now
Now: Claude Code in the terminal and in Claude Desktop, both through the plugin. Codex gets a README line saying to copy or symlink `skills/aside/` into `~/.agents/skills/`, which works because it's the same format. Later, as separate issues if anyone asks: a Codex plugin, a Cursor plugin, and claude.ai account skills for Cowork and the phone. It covers where aside is actually used today, at almost no cost.

### D3: 0.4.0 doesn't wait
Ship 0.4.0 now with a one-line README stopgap ("copy `skills/aside/` into `~/.claude/skills/aside/`"), and the plugin comes in the next release. The plugin is a few tickets of work, and 0.4.0 also carries the byte order mark fix, which needn't wait.

### D4: What the plugin carries
The skill only, for now, while `aside setup` keeps installing the hook and MCP server. Bundling the binary, hook and MCP server into the plugin becomes its own issue: one install for Claude Code, through a launcher that downloads and checks the matching release binary. It gives the low-friction install without putting launcher, Windows and coexistence work into this release.

### D5: Keeping versions in step
release-please bumps `version` in `.claude-plugin/plugin.json` through `extra-files`, in the same release PR as the binary, so the plugin and the binary always share a version number. Every release then updates both, and `claude plugin update` picks it up.

### D6: Names
The plugin is `aside` and the marketplace is `aside-jot`, so it installs with `/plugin marketplace add kacxx/aside-jot` and `/plugin install aside@aside-jot`. They match the skill's name and the repo's.

### D7: Without aside installed (revised 2026-10-15)
The plugin's description and the README say it needs aside installed and `aside setup claude` run, since that hook is what records the jots. Adding the MCP server (the command `aside setup claude --mcp` prints) is recommended: reads go through read-only tools instead of shell commands, and the tool descriptions give the full path to aside for `done` and `add`. Without it, `aside` must be on `PATH`. The skill itself doesn't change.

<details><summary>Superseded (2026-10-15)</summary>

The plugin's description and the README say it needs aside installed and `aside setup claude --mcp` run first. The skill itself doesn't change. The skill does nothing without aside's MCP tools, so saying so up front saves a confusing first run.

Reason: the fact-check found the skill falls back to the CLI without the MCP tools (`SKILL.md:26-28`), and `aside setup claude --mcp` only prints the MCP command, it doesn't run it.
</details>

## Shape

**`.claude-plugin/plugin.json`**: `name: "aside"` (D6), `version` set to the current release when it's added (`0.4.0`) and bumped by release-please from then on (D5), a `description` saying what the skill does and what it needs (wording follows D7), `author`, and `repository`. No `skills` list: the default `skills/` scan finds `skills/aside`. No `license` field, since the repo has no licence file.

**`.claude-plugin/marketplace.json`**: `name: "aside-jot"` (D6), `owner`, a `description` (the validator warns without one), and one plugin entry, `{ "name": "aside", "source": "./" }`, so the repo root is the plugin, as in dev-flow (D1). Installed, the skill shows as `aside:aside`.

**`release-please-config.json`**: the `.` package gets

```json
"extra-files": [
  { "type": "json", "path": ".claude-plugin/plugin.json", "jsonpath": "$.version" }
]
```

so the release PR bumps the plugin with the binary (D5).

**README**: the "Teach your agent the jot routines" section (#111) leads with the plugin for Claude Code, in the terminal and in Desktop:

```text
/plugin marketplace add kacxx/aside-jot
/plugin install aside@aside-jot
```

with `claude plugin update aside@aside-jot` to update, and what it needs: aside installed and `aside setup claude` run, with the MCP server recommended (D7). It keeps the manual copy for Codex (`~/.agents/skills/`) and anything else the plugin doesn't reach (D1, D2), and tells anyone who copied the skill into `~/.claude/skills/aside` by hand to remove that copy when they install the plugin. The sentence "It needs the MCP server above" changes to match D7, and the "planned (#110)" sentence goes.

Nothing in the Go code, the skill text, `aside setup` or the release archive changes (D4, D7).

## Fact-check

Probes ran on 2026-10-15 at `7825248` (the 0.4.0 release commit), with Claude Code's `claude plugin validate` and a headless `claude -p --plugin-dir` session on a scratch copy of the repo. Nothing was installed.

| Claim | Result | Source |
|---|---|---|
| The skill isn't embedded in the binary | ✅ no `go:embed` and no `skills` path in any Go file | `grep -rn 'go:embed'`, `grep -rln '"skills'` over `*.go` |
| The release archive holds the binary and `README.md` only | ✅ | `.goreleaser.yaml:23-24` |
| `aside setup` writes the binary's absolute path into the hook config | ✅ | `README.md:59-62`; `internal/setup/binary.go` `Resolve` |
| A repo can be its own marketplace with `.claude-plugin/plugin.json` and `marketplace.json` (D1) | ✅ the sample manifests in Shape validate as a marketplace and a plugin | probe: `claude plugin validate .` |
| The default `skills/` scan finds `skills/aside` with no `skills` list | ✅ the session lists `aside:aside`; the control run without the plugin lists none | probe: `claude -p … --plugin-dir <copy> --setting-sources project`, and the same without `--plugin-dir` |
| `marketplace.json` wants a `description` | ✅ warning without one, error under `--strict` | probe: `claude plugin validate --strict` |
| The repo's `CLAUDE.md` at the plugin root isn't loaded by the plugin | ✅ a validator warning, so `--strict` fails on this layout | probe: `claude plugin validate --strict .` |
| Installing copies the whole plugin source, not just the skill | ✅ dev-flow's cache holds `docs/`, `evals/`, `scripts/` and `CLAUDE.md`; aside-jot is 90 files, about 1.3 MB | `~/.claude/plugins/cache/dev-flow/dev-flow/0.11.0/`; `git ls-files` |
| dev-flow is built this way and installs with `/plugin marketplace add` and `/plugin install …@…` | ✅ installed at user scope on the owner's machine | `~/.claude/plugins/installed_plugins.json:24-27`; `kacxx/dev-flow` `.claude-plugin/` |
| `claude plugin update <plugin>` updates a plugin (D1, D5) | ✅ | `claude plugin --help` |
| Setting `version` pins the plugin until it changes, so each release's bump is what delivers an update (D5) | ✅ | [plugins reference, `version`](https://code.claude.com/docs/en/plugins-reference) |
| Claude Desktop's local and SSH Claude Code sessions load plugins from configured marketplaces; cloud sessions and WSL don't; Cowork uses the claude.ai account's skills (D1, D2) | ✅ docs, not probed in Desktop | [Desktop docs](https://code.claude.com/docs/en/desktop) |
| Codex reads the same `SKILL.md` format (`name`, `description`) from `~/.agents/skills` (D2) | ✅ docs; the skill's front matter has both fields | [Codex skills](https://developers.openai.com/codex/skills); `skills/aside/SKILL.md:1-3` |
| release-please's `extra-files` bumps a JSON field named by `jsonpath` (D5) | ✅ docs | [customizing.md, extra-files](https://github.com/googleapis/release-please/blob/main/docs/customizing.md) |
| release-please here has no extra files yet | ✅ `release-type: go`, one `.` package | `release-please-config.json` |
| 0.4.0 shipped with the README stopgap (D3) | ✅ #111 merged 08:13Z, v0.4.0 released 08:13Z on 2026-10-15 | `gh pr view 111`; `gh release list` |
| Superseded D7: "The skill does nothing without aside's MCP tools" | ❌ the skill says that without the MCP tools it runs the same commands through the CLI (`aside inbox`, `aside show`, …); D7 revised | `skills/aside/SKILL.md:26-28` |
| Superseded D7: running `aside setup claude --mcp` gives the agent the MCP tools | ❌ `--mcp` prints the command that adds the MCP server; it doesn't run it; D7 revised | `README.md:64-65` |
| D7: `aside setup claude` adds the hook that records the jots | ✅ it adds the `UserPromptSubmit` hook | `README.md:59`; `cmd/aside/main.go:67-68` |
| D7: the MCP server's tools are read-only, and `done` and `add` go through the shell | ✅ | `SKILL.md:14`, `SKILL.md:58-59`; `internal/mcp/server.go:248` |
| D7: the MCP tool descriptions give the full path to aside | ✅ the server fills `{{DONE_COMMAND}}` with the path of its own executable | `internal/mcp/server.go:243-250`, `72`; `SKILL.md:167-169` |
| D7: without the MCP server, `aside` must be on `PATH` | ✅ the CLI fallback names bare `aside …` commands, and the skill's only source for the full path is the MCP tool descriptions | `SKILL.md:26-28`, `SKILL.md:167-169` |
| D4: `aside setup` "keeps installing the hook and MCP server" | ⚠️ partly: it installs the hook and prints the MCP command for the user to run. Nothing depends on the difference: D4 leaves setup as it is either way | `README.md:59-65` |
| The README already says the skill "needs the MCP server above" | ✅ the README edit in Shape changes it to match D7 | `README.md:225` |

## Out of scope

- Bundling the binary, hook and MCP server into the plugin (D4): its own issue, drafted, not yet filed.
- A Codex plugin, a Cursor plugin, and claude.ai account skills for Cowork and the phone (D2): separate issues if anyone asks.
- Any change to the skill text, `aside setup`, or the release archive.
- Running `claude plugin validate` in CI.

## Risks

- **Two copies of the skill.** Someone who copied the skill by hand and then installs the plugin has both `aside` and `aside:aside`, which can drift apart. The README tells them to remove the hand copy.
- **`--strict` validation fails on the repo's `CLAUDE.md`.** It's a warning only, and Claude Code loads the plugin; it matters if strict validation is added to CI later.
- **The whole repo is installed.** About 1.3 MB today; it grows with the repo. A plugin subfolder would avoid it, but it's not worth doing at this size.
- **Skill fixes wait for a release.** Because `version` pins the plugin, a skill change merged to main reaches plugin users only at the next release. That's what D5 asks for, but it means a skill-only fix needs a release to ship.
- **Manual copies don't update.** Codex users and anyone else on the copy path keep the version they copied until they copy it again.
