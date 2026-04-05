#!/usr/bin/env bash
# Start weissd for iOS Simulator / local debugging.
# Default port must match pixiv-client HttpProxySessionManager.proxyPort (28492).
#
# Usage:
#   ./scripts/run-dev.sh
#   ./scripts/run-dev.sh 28492
#   ./scripts/run-dev.sh 28492 /path/to/weiss-hosts.json   # one-line JSON file from Xcode WEISS_JSON
#
set -euo pipefail
WEISS_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PORT="${1:-28492}"
JSON_FILE="${2:-}"

if [[ -n "$JSON_FILE" ]]; then
  if [[ ! -f "$JSON_FILE" ]]; then
    echo "JSON file not found: $JSON_FILE" >&2
    exit 1
  fi
  JSON="$(tr -d '\n' < "$JSON_FILE" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"
else
  # Fallback sample — replace with output between WEISS_JSON_BEGIN/END from Xcode.
  JSON='{"i.pximg.net":"210.140.139.131","source.pixiv.net":"210.140.139.152","app-api.pixiv.net":"210.140.139.152","www.pixivision.net":"210.140.131.224","i.pixiv.cat":"104.20.0.127","accounts.pixiv.net":"210.140.139.152","s.pximg.net":"210.140.139.136","doh":"doh.dns.sb","oauth.secure.pixiv.net":"210.140.139.152"}'
  echo "[run-dev] using built-in JSON sample; pass a file as 2nd arg for live map from the app." >&2
fi

cd "$WEISS_ROOT"

if command -v lsof >/dev/null 2>&1; then
  if lsof -nP -iTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
    echo "[run-dev] ERROR: port $PORT is already in use (leftover weissd, embedded Weiss in app, or other process)." >&2
    echo "[run-dev] Who is listening:" >&2
    lsof -nP -iTCP:"$PORT" -sTCP:LISTEN >&2 || true
    echo "[run-dev] Free the port, then retry:" >&2
    echo "    kill \$(lsof -t -iTCP:$PORT -sTCP:LISTEN)" >&2
    echo "[run-dev] Or pick another port and match the app, e.g.:" >&2
    echo "    ./scripts/run-dev.sh 28493 weiss.json" >&2
    echo "    # In app DEBUG: HttpProxySessionManager.cacheProxyPort(28493) once, or change default in HttpProxySessionManager.swift" >&2
    exit 1
  fi
fi

echo "[run-dev] PORT=$PORT (set HttpProxySessionManager / same as app)" >&2
echo "[run-dev] logs use prefix [weiss] / [weiss-ech]" >&2
exec go run ./cmd/weissd "$PORT" "$JSON"
