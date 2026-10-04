# aside-jot

A side channel for thoughts while you work with coding agents.

Type `>> some thought` into Claude Code or Codex. aside saves it with its git
context (repo, branch, commit) and **blocks the prompt**, so the model never
sees it. Your flow isn't interrupted and the agent's context stays clean.

```
>> token cache TTL looks too long, check with infra before shipping
✓ Jotted #12
```

Later, review your notes with `aside inbox`, find the chat where you worked on
something with `aside find`, or let an agent read them through the read-only
MCP server.

![Demo: an ordinary prompt reaches Claude, a ">>" prompt is blocked and saved with its git context, Claude reads the inbox over MCP, and "jot done" clears it](docs/demo.gif)

*Recorded with the real Claude Code CLI before the command was renamed from
`jot` to `aside`.*

> **aside does not work with Cursor.** Cursor sends a blocked prompt to the
> model with your next message, so a jot there is not private. See
> [docs/cursor.md](docs/cursor.md).

## Install

Requires Go 1.25+.

```sh
go install github.com/kacxx/aside-jot/cmd/aside@latest
aside paths    # or "$(go env GOPATH)/bin/aside" paths if it isn't on PATH
```

`aside paths` prints where your notes are stored and the binary's full path.
**Use that full `binary:` path in the configs below**: editors often run hooks
with a minimal `PATH`.

### Claude Code

Add to `~/.claude/settings.json`:

```json
{
  "hooks": {
    "UserPromptSubmit": [
      { "hooks": [{ "type": "command", "command": "/Users/you/go/bin/aside hook claude" }] }
    ]
  }
}
```

### Codex

Add to `~/.codex/hooks.json`, then run `/hooks` in the **Codex CLI** and
**trust** the hook. Until it's trusted, `>>` prompts go to the model.

```json
{
  "hooks": {
    "UserPromptSubmit": [
      { "hooks": [{ "type": "command", "command": "/Users/you/go/bin/aside hook codex" }] }
    ]
  }
}
```

### Check it works

Type `>> test` in the agent. You should see `✓ Jotted #N` and no reply from
the model. Then run `aside inbox` in a terminal.

Windows paths, project-level configs, Codex trust details and known agent
behaviour: [docs/setup.md](docs/setup.md).

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

Not in a session:
#14   2026-10-04 18:03  SUP-4821 needs a backend ticket  [api@main]
```

```sh
aside find SUP-4821         # chats that mention it (ticket keys match as whole words)
aside sessions              # your recent chats and their labels
```

Only chats with a jot in them can be found, so start a ticket's chat with
`>> session: SUP-4821 …`.

### Turn a jot into a GitHub issue

```sh
aside promote 12 --dry-run      # preview the repo, title and body; creates nothing
aside promote 12                # create the issue with gh and mark the jot done
aside promote 12 --with-reply   # include the agent's reply (Claude Code jots); asks before posting
```

The issue goes to the repo the jot was captured in, or `--repo owner/name`. It
uses your existing `gh` login.

### Let your agent read your notes

```sh
claude mcp add --scope user aside -- /Users/you/go/bin/aside mcp   # Claude Code
codex mcp add aside -- /Users/you/go/bin/aside mcp                 # Codex
```

Then ask the agent things like "what's in my inbox?" or "where did I work on
SUP-4821?". The server is **read-only**: agents can read and search your jots,
but can't add, change or close them.

## All commands

| Command | What it does |
| --- | --- |
| `aside add <text>` | Save a jot from the terminal (reads stdin if no text) |
| `aside inbox [-n N] [--older D]` | Open jots, newest first (default 20, `-n 0` for all) |
| `aside show <id>` | One jot with its context |
| `aside search <query>` | Search all jots |
| `aside find <query>` | Chats with a matching jot, and how to resume them |
| `aside sessions [-n N]` | Recent chats with their labels (default 10) |
| `aside done <id>` | Mark a jot done |
| `aside promote <id> [--repo owner/name] [--with-reply [--yes]] [--dry-run]` | Turn a jot into a GitHub issue |
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
- [docs/decisions.md](docs/decisions.md): why aside works the way it does.

### Not in v1 (on purpose)

No priorities, tags or workflows beyond inbox and done; no trackers other than
GitHub issues, and nothing created automatically; no LLM inside aside; no sync
(use `aside backup`); no UI beyond the CLI and MCP. Capturing has to be instant,
reliable and invisible to the model. Everything else can come later.

## Development

```sh
go test -race ./...
golangci-lint run   # v2.14.0, config in .golangci.yml
```

See [docs/SMOKE_TEST.md](docs/SMOKE_TEST.md) for an end-to-end check against
the built binary, and [docs/mvp-validation.md](docs/mvp-validation.md) for the
recorded results of the initial version. [AGENTS.md](AGENTS.md) is the starting
point for coding agents.
