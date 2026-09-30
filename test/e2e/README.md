# End-to-end harness

A black-box check of the documented install/use flow against a freshly built
`aside` binary. It complements the unit tests (`go test ./...`) and the manual
[docs/SMOKE_TEST.md](../../docs/SMOKE_TEST.md) by exercising the real entry
points end to end:

1. `install` — builds the binary (from this checkout, or `@latest` outside it);
   pure-Go SQLite, no cgo.
2. `paths` / PATH — prints data + binary locations and whether `aside` is on `PATH`.
3. empty inbox on first run.
4. CLI round-trip — `add` / `inbox` / `search` / `show` / `done`.
5. hooks — a `>>` prompt is captured and a normal prompt passes through, for
   `claude`, `codex` and `cursor`, printing each agent's exact response schema.
6. MCP — a JSON-RPC `initialize` handshake.

The database is redirected to a throwaway path (`JOT_DB` under `mktemp -d`), so
running it **never touches your real inbox**.

## Run it directly

```sh
bash test/e2e/run.sh
```

Run from the repo root, it installs from the checkout, so it tests the code you
have. This is what CI runs.

## Run it in a clean room

For a first-time-user reproduction (empty `HOME`, `@latest`, unprivileged user):

```sh
docker build -t aside-e2e -f test/e2e/Dockerfile .
docker run --rm aside-e2e
```

Behind a TLS-intercepting corporate proxy, see the CA note in
[`Dockerfile`](Dockerfile).
