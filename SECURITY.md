# Security

## Reporting a problem

Please report security problems privately, not in a public issue: use
**Report a vulnerability** on the repository's
[Security tab](https://github.com/kacxx/aside-jot/security). You'll get a reply
there.

Fixes go into the latest release only, so check the problem is still there in
[the latest release](https://github.com/kacxx/aside-jot/releases/latest).

## What counts

aside sees every prompt you type into a hooked agent, so the things worth
reporting are ones that break what it promises:

- A prompt that doesn't start with `>> ` being blocked, saved or changed.
- A jot reaching the model when the host is one aside supports.
- The MCP server adding, changing or closing jots, or reading outside the
  database and the transcripts recorded with jots.
- `aside promote` posting without asking, or a jot's text or transcript text
  getting run as a command.
- The database or its backups being readable by other users on Unix.

aside makes no network connections of its own: `aside promote` posts through
your `gh` login.
