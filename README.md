# aside-jot

A side channel for thoughts while you work with coding agents.

Mid-task, you think of something unrelated: a bug to check, a ticket to raise,
a question for someone. Telling the agent derails it, and switching to a notes
app breaks your flow. With aside, you type it into the agent with `>>` in
front:

```
>> token cache TTL looks too long, check with infra before shipping
✓ Jotted #12
```

aside saves it with its git context (repo, branch, commit) and **blocks the
prompt**, so the model never sees it. The agent's context stays clean, and you
keep working.

Works with **Claude Code** and **Codex**. Not Cursor: it sends a blocked prompt
to the model with your next message ([why](docs/cursor.md)).

![Demo: an ordinary prompt reaches Claude, a ">>" prompt is blocked and saved with its git context, Claude reads the inbox over MCP, and "jot done" clears it](docs/demo.gif)

*Recorded with the real Claude Code CLI before the command was renamed from
`jot` to `aside`.*

## Install

Requires Go 1.25+.

```sh
go install github.com/kacxx/aside-jot/cmd/aside@latest
aside setup claude    # or: aside setup codex
```

`aside setup` adds the `UserPromptSubmit` hook to `~/.claude/settings.json`
(or `~/.codex/hooks.json`) with the binary's absolute path, since editors often
run hooks with a minimal `PATH`. It keeps your other settings and hooks, backs
the file up first (`settings.json.bak-<time>`), and replaces an older aside
entry instead of adding a second. `--dry-run` shows the result and writes
nothing; `--mcp` also prints the command that gives the agent read-only access
to your jots (it doesn't run it). If `aside` isn't on your `PATH` yet, run it as
`"$(go env GOPATH)/bin/aside" setup claude`.

```
$ aside setup claude
✓ Added the aside hook in /Users/you/.claude/settings.json
  command: /Users/you/go/bin/aside hook claude
  backup:  /Users/you/.claude/settings.json.bak-20261006-065359
  …
```

For **Codex**, run `/hooks` in the **Codex CLI** afterwards and **trust** the
hook. Until it's trusted, `>>` prompts go to the model.

Prefer to edit the JSON yourself? See [docs/setup.md](docs/setup.md).

### Check it works

Type `>> test` in the agent. You should see `✓ Jotted #N` and no reply from
the model. Then run `aside inbox` in a terminal. If your shell can't find
`aside`, add `$(go env GOPATH)/bin` to your `PATH`.

Windows paths, project-level configs, Codex trust details and known agent
behaviour: [docs/setup.md](docs/setup.md).

## A day with aside

- **Morning.** Deep in a refactor, you jot three things without stopping:
  `>> SUP-4821 needs a backend ticket` (#21), `>> ask infra about the cache
  TTL` (#22) and `>> flaky test in billing, look later` (#23).
- **Lunch.** `aside inbox` lists them, newest first, with the repo and branch
  each came from. You fix the flaky test and run `aside done 23`.
- **Afternoon.** The ticket note deserves an issue. Since you jotted it in
  Claude Code, `aside promote 21 --with-reply` shows you an issue with your jot
  and what the agent had just said when you jotted it, and creates it in that
  repo when you say yes. (`promote` always asks first.)
- **Friday.** Someone asks about SUP-4821. `aside find SUP-4821` lists the
  chats where you jotted about it, with the command to reopen each one.

## Using aside

### Capture

| You type | What happens |
| --- | --- |
| `>> some thought` (in the agent) | Saved with the repo, branch, commit and chat; the model never sees it |
| `>> session: SUP-4821 token TTL` | Also labels the chat, so `aside find` and `aside sessions` show why it exists |
| `aside add some thought` (in a terminal) | Saved with the git context of the current directory |

Only a prompt that **starts** with `>> ` and then text is a jot. `>>x`,
` >> x` and `a >> b` go to the model as normal. To send a prompt that starts
with `>> `, put a space or a backslash in front.

### Review

```sh
aside inbox                 # your open jots, newest first, with their age
aside inbox --older 7       # only the ones that have waited a week or more
aside show 12               # one jot with its full context
aside search ttl            # search every jot, done ones included
aside done 12               # mark it done; it drops out of the inbox
aside done 3 4 5 --note "pushed to Q4, see https://example.com/wiki/plan"   # several at once, with why
```

```
$ aside inbox
#14   today  SUP-4821 needs a backend ticket  [api@main]
#12   2d     token cache TTL looks too long, check with infra before shipping  [api@main]
#11   2d     session: SUP-4821 token TTL investigation  [api@main]
```

### Get back to a chat

Every jot made in an agent remembers its chat. `aside find` shows the chats
where you jotted about something, and prints the command to reopen each one:

```
$ aside find SUP-4821
SUP-4821 token TTL investigation
  claude · api@main · 2 jots · last 2d (#12)
  #11   2d     session: SUP-4821 token TTL investigation
  cd /Users/you/code/api && claude --resume 0f3c9a…
  aside open 12

Not in a session:
#14   2026-10-04 18:03  SUP-4821 needs a backend ticket  [api@main]
```

```sh
aside find SUP-4821         # chats that mention it (ticket keys match as whole words)
aside sessions              # your recent chats and their labels
```

Only chats with a jot in them can be found, so start a ticket's chat with
`>> session: SUP-4821 …`.

On a Mac, `aside open 12` opens the chat a jot came from, for jots made in
Claude Desktop (checked on Desktop 2.19675.0). When a chat has such a link,
`aside find` prints the command under its resume command, as above. `aside show 12` prints the link as
`open:`, and the MCP tools return it as `open_url` with an `open_command`. Codex
app jots get a link too, but `aside open` hasn't been run for one on a Mac with
the Codex app. A jot with no link, such as one from a terminal `claude` session,
gets its resume command instead.

### Turn a jot into a GitHub issue

```sh
aside promote 12 --dry-run      # preview the repo, title and body; creates nothing
aside promote 12                # show the issue, ask [y/N], then create it with gh and mark the jot done
aside promote 12 --yes          # skip the question (needed when stdin is not a terminal)
aside promote 12 --with-reply   # include the agent's reply (Claude Code jots)
```

The issue goes to the repo the jot was captured in, or `--repo owner/name`. It
uses your existing `gh` login.

### Let your agent read your notes

```sh
claude mcp add --scope user aside -- /Users/you/go/bin/aside mcp   # Claude Code
codex mcp add aside -- /Users/you/go/bin/aside mcp                 # Codex
```

Then ask the agent things like "what's in my inbox?" or "where did I work on
SUP-4821?". The server is **read-only**: through it, agents can read and
search your jots, but not add, change or close them. An agent that can run
shell commands can still run `aside` itself: `aside done` when you ask it to
close jots, or `aside promote`, which posts a jot to GitHub. `promote` asks
first and refuses to run when stdin isn't a terminal, so an agent has to pass
`--yes`, which shows in the command (a pseudo-terminal can still answer it).
Your agent's command approval is what guards that, and in auto-approve modes
little or nothing does.

## All commands

| Command | What it does |
| --- | --- |
| `aside add <text>` | Save a jot from the terminal (reads stdin if no text) |
| `aside inbox [-n N] [--older D]` | Open jots, newest first (default 20, `-n 0` for all) |
| `aside show <id>` | One jot with its context |
| `aside search <query>` | Search all jots |
| `aside find <query>` | Chats with a matching jot, and how to resume them |
| `aside open <id>` | Open the chat a jot came from (macOS; Claude Desktop and Codex app jots) |
| `aside sessions [-n N]` | Recent chats with their labels (default 10) |
| `aside done <id>... [--note "why"]` | Mark jots done; `--note` records why (search finds it) |
| `aside promote <id> [--repo owner/name] [--with-reply] [--yes] [--dry-run]` | Turn a jot into a GitHub issue; asks first unless `--yes` |
| `aside backup <path>` | Copy the database safely; never overwrites |
| `aside paths` | Where the data and binary are |
| `aside version` | Print the version |
| `aside hook claude\|codex\|cursor` | Hook entry point, used by the agent configs |
| `aside mcp` | Read-only MCP server, used by the agent configs |

## More detail

- [docs/setup.md](docs/setup.md): Windows, project-level configs, Codex trust,
  MCP for Codex's config file, and known Claude Code and Codex behaviour.
- [docs/reference.md](docs/reference.md): the capture rule, what happens when
  saving fails, how `find` matches, `promote` in full, and storage.
- [docs/cursor.md](docs/cursor.md): why Cursor isn't supported and what was
  tested.
- [docs/decisions.md](docs/decisions.md): why aside works the way it does, and
  what's left out on purpose.
- [AGENTS.md](AGENTS.md): building, testing and contributing.
