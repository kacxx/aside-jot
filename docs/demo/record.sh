#!/usr/bin/env bash
# Re-records docs/demo.gif with the real Claude Code CLI.
#
# Needs: claude (logged in), asciinema 3, agg, go, python3. Builds aside from
# this checkout and runs everything in /tmp/aside-demo with its own database,
# so no real jots, paths or settings appear. Claude runs with only a demo
# settings file (--setting-sources local), so warnings about your own settings
# stay out of the recording. Each run makes a few real model calls.
set -euo pipefail

repo=$(cd "$(dirname "$0")/../.." && pwd)
work=/tmp/aside-demo
here="$repo/docs/demo"

rm -rf "$work/api" "$work/bin" "$work"/demo.*
mkdir -p "$work/api" "$work/bin"
(cd "$repo" && go build -o "$work/bin/aside" ./cmd/aside)

cd "$work/api"
git init -q -b main
printf '# api\n\nToken service: issues short-lived auth tokens and caches them per tenant.\n' > README.md
printf 'package api\n\nimport "time"\n\n// tokenTTL is how long an issued token stays in the cache.\nconst tokenTTL = 24 * time.Hour\n' > cache.go
git add .
git -c user.name=demo -c user.email=demo@example.com commit -qm "token cache"

# Demo-only Claude config: the aside hook and MCP server from $work/bin, plus
# the env and model from your user settings (proxy certificates, for example).
python3 - "$work" <<'EOF'
import json, os, sys
work = sys.argv[1]
user = {}
try:
    user = json.load(open(os.path.expanduser("~/.claude/settings.json")))
except OSError:
    pass
settings = {
    "env": user.get("env", {}),
    "permissions": {"allow": ["mcp__aside"]},
    "hooks": {"UserPromptSubmit": [{"hooks": [
        {"type": "command", "command": f"{work}/bin/aside hook claude"}]}]},
}
if "model" in user:
    settings["model"] = user["model"]
json.dump(settings, open(f"{work}/settings.json", "w"), indent=2)
json.dump({"mcpServers": {"aside": {"command": f"{work}/bin/aside", "args": ["mcp"]}}},
          open(f"{work}/mcp.json", "w"), indent=2)
EOF

if [ ! -x "$work/venv/bin/python" ]; then
  python3 -m venv "$work/venv"
  "$work/venv/bin/pip" -q install pexpect
fi

export JOT_DB="$work/demo.db"
export DEMO_WORK="$work"
"$work/venv/bin/python" "$here/drive.py" --trust
asciinema rec --overwrite --quiet --window-size 100x30 \
  -c "$work/venv/bin/python $here/drive.py" "$work/demo.cast" > /dev/null
agg --theme monokai --speed 1.6 --idle-time-limit 1.5 --last-frame-duration 5 \
  "$work/demo.cast" "$repo/docs/demo.gif" > /dev/null 2>&1

"$work/bin/aside" search token
ls -l "$repo/docs/demo.gif"
