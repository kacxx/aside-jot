#!/usr/bin/env bash
# End-to-end check of the documented install/use flow against a freshly built
# binary: install -> paths -> empty inbox -> CLI round-trip -> per-agent hook
# capture/passthrough -> MCP initialize. Portable: runs on a CI runner as-is and
# inside the clean-room container (test/e2e/Dockerfile).
#
# The database is redirected to a throwaway location, so running this never
# touches your real inbox.
set -u

TMP="$(mktemp -d)"
export JOT_DB="$TMP/e2e.db"
trap 'rm -rf "$TMP"' EXIT

PASS=0; FAIL=0; OBSERVE=0
ok()   { echo "  PASS: $1"; PASS=$((PASS+1)); }
bad()  { echo "  FAIL: $1"; FAIL=$((FAIL+1)); }
note() { echo "  OBSERVE: $1"; OBSERVE=$((OBSERVE+1)); }
hr()   { echo; echo "=== $1 ==="; }
captured() { echo "$1" | grep -q "Jotted"; }

# Install from this checkout when run inside the repo (so CI exercises the code
# under review); otherwise install the published @latest (clean-room default).
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." 2>/dev/null && pwd || true)"

hr "1. install (no cgo; pure-Go sqlite)"
if [ -n "$REPO_ROOT" ] && grep -q '^module github.com/kacxx/aside-jot$' "$REPO_ROOT/go.mod" 2>/dev/null; then
  echo "  installing from checkout: $REPO_ROOT"
  ( cd "$REPO_ROOT" && go install ./cmd/aside ) && ok "go install ./cmd/aside" || bad "go install from checkout"
else
  echo "  installing published @latest"
  go install github.com/kacxx/aside-jot/cmd/aside@latest && ok "go install @latest" || bad "go install @latest"
fi
BIN="$(go env GOPATH)/bin/aside"
[ -x "$BIN" ] && ok "binary present at \$(go env GOPATH)/bin/aside" || bad "binary missing at $BIN"

hr "2. paths / PATH check"
"$BIN" paths
if command -v aside >/dev/null 2>&1; then ON_PATH=yes; else ON_PATH=no; fi
if [ "${ASIDE_E2E_EXPECT_NOT_ON_PATH:-0}" = "1" ]; then
  # The clean-room sets GOPATH under HOME with ~/go/bin off PATH, so this asserts
  # the real new-user case the README warns about: use the absolute binary path.
  [ "$ON_PATH" = no ] && ok "'aside' is NOT on bare PATH (new-user case reproduced)" \
                       || bad "'aside' unexpectedly on PATH (fidelity lost)"
else
  note "'aside' on bare PATH: $ON_PATH (environment-dependent; use the absolute binary path in configs)"
fi

hr "3. first run: empty inbox"
OUT="$("$BIN" inbox 2>&1)"; echo "$OUT"
echo "$OUT" | grep -qE '^#' && bad "inbox not empty on a fresh DB" || ok "inbox empty on first run"

hr "4. CLI round-trip (add / inbox / search / show / done)"
"$BIN" add "e2e note" >/dev/null && ok "add" || bad "add"
"$BIN" inbox | grep -q "e2e note" && ok "inbox lists it" || bad "inbox missing it"
"$BIN" search "e2e" | grep -q "e2e note" && ok "search finds it" || bad "search"
ID="$("$BIN" inbox | grep -oE '#[0-9]+' | head -1 | tr -d '#')"
if [ -n "$ID" ]; then
  "$BIN" show "$ID" | grep -q "e2e note" && ok "show #$ID" || bad "show"
  "$BIN" done "$ID" >/dev/null && ok "done #$ID" || bad "done"
else
  bad "could not parse a jot id from inbox"
fi

hr "5. hooks: >> captured, normal passes through (per-agent schemas printed)"
for agent in claude codex cursor; do
  CAP="$(printf '{"prompt":">> note via %s","cwd":"%s"}' "$agent" "$TMP" | "$BIN" hook "$agent" 2>&1)"
  echo "  $agent >>     : ${CAP:-<empty>}"
  captured "$CAP" && ok "hook $agent captures a >> prompt" || bad "hook $agent did NOT capture a >> prompt"
  THRU="$(printf '{"prompt":"just a normal question","cwd":"%s"}' "$TMP" | "$BIN" hook "$agent" 2>&1)"
  echo "  $agent normal : ${THRU:-<empty>}"
  captured "$THRU" && bad "hook $agent captured a normal prompt" || ok "hook $agent passes a normal prompt through"
done

hr "6. MCP server: JSON-RPC initialize"
INIT='{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"e2e","version":"0"}}}'
# Portable bounded wait (no GNU `timeout`, which macOS lacks): feed one request,
# close stdin, and hard-stop the server after a few seconds if it lingers.
"$BIN" mcp >"$TMP/mcp.out" 2>&1 <<<"$INIT" &
MCP_PID=$!
for _ in 1 2 3 4 5; do kill -0 "$MCP_PID" 2>/dev/null || break; sleep 1; done
kill "$MCP_PID" 2>/dev/null; wait "$MCP_PID" 2>/dev/null
MCP_OUT="$(head -c 600 "$TMP/mcp.out" 2>/dev/null)"
echo "  ${MCP_OUT:-<no output>}"
echo "$MCP_OUT" | grep -q '"result"' && ok "MCP responded to initialize" || bad "no MCP initialize response"

hr "SUMMARY"
echo "PASS=$PASS  FAIL=$FAIL  OBSERVE=$OBSERVE"
[ "$FAIL" -eq 0 ]
