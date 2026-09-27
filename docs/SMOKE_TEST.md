# Smoke test

End-to-end check of the built binary against the hook fixtures. It uses a
throwaway database and never touches your real one.

```sh
go build -o bin/jot ./cmd/jot
export JOT_DB="$(mktemp -d)/jot.db"
```

## Claude Code hook

Ordinary prompts, mid-prompt `>>` and malformed payloads must print **nothing**:

```sh
for f in normal mid_prompt malformed; do
  printf '%s: [' $f; bin/jot hook claude < testdata/hooks/claude/$f.json; echo "] exit=$?"
done
# normal: [] exit=0
# mid_prompt: [] exit=0
# malformed: [] exit=0
```

A jot is blocked and saved. Point `cwd` at a real repo to get git context:

```sh
jq --arg cwd "$PWD" '.cwd=$cwd' testdata/hooks/claude/capture.json | bin/jot hook claude
# {"decision":"block","reason":"✓ Jotted #1","suppressOriginalPrompt":true}
```

## Cursor hook

```sh
bin/jot hook cursor < testdata/hooks/cursor/normal.json      # {"continue":true}
bin/jot hook cursor < testdata/hooks/cursor/malformed.json   # {"continue":true}
jq --arg r "$PWD" '.workspace_roots=[$r]' testdata/hooks/cursor/capture_single_root.json | bin/jot hook cursor
# {"continue":false,"user_message":"✓ Jotted #2"}
bin/jot hook cursor < testdata/hooks/cursor/capture_multi_root.json
# {"continue":false,"user_message":"✓ Jotted #3"}   (roots in metadata, no git fields)
```

## Failure is safe

```sh
JOT_DB=/dev/null/nope.db bin/jot hook claude < testdata/hooks/claude/capture.json; echo "exit=$?"
# {"decision":"block","reason":"✗ Jot NOT saved (...). Your text:\n\n>> token cache ...","suppressOriginalPrompt":true}
# exit=0
```

## Read back

```sh
bin/jot inbox
bin/jot show 1
bin/jot search cursor
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"inbox","arguments":{}}}' \
  | bin/jot mcp
```

## Latency of the non-match path

```sh
time (for i in $(seq 200); do bin/jot hook claude < testdata/hooks/claude/normal.json; done)
```

Divide by 200. The non-match path never opens SQLite or runs git, so this is
essentially process start-up.

## In the real tools

1. Install the hooks as described in the README.
2. In Claude Code, type `>> hello from claude`. You should see `✓ Jotted #N`,
   and the model should not respond.
3. In Cursor, type `>> hello from cursor`. You should see the same.
4. Type `what does >> do in bash?`. It must go to the model as normal.
5. `jot inbox` shows both jots with repo and branch.
