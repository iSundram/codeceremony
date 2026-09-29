#!/usr/bin/env bash
# Refresh acceptance-report.txt against a real portal, then clean up.
# Everything happens in one foreground invocation: the portal is started, used
# and killed before this script exits, so no process outlives the command.
set -euo pipefail

REPO=/root/dogfood
PORT=55733
WORK=$(mktemp -d /tmp/opencode/acc-XXXXXX)
PORTAL="$WORK/codeceremony"
LOG="$WORK/boot.log"

cleanup() {
  if [[ -n "${PORTAL_PID:-}" ]] && kill -0 "$PORTAL_PID" 2>/dev/null; then
    kill "$PORTAL_PID" 2>/dev/null || true
    wait "$PORTAL_PID" 2>/dev/null || true
  fi
  rm -rf "$WORK"
}
trap cleanup EXIT

cd "$REPO/backend"
go build -o "$PORTAL" ./cmd/codeceremony

mkdir -p "$WORK/data"
DATA_DIR="$WORK/data" \
HTTP_ADDR=":$PORT" \
ALLOWED_ORIGIN="http://localhost:$PORT" \
APP_BASE_URL="http://localhost:$PORT" \
FIXTURES_PATH="$REPO/fixtures.json" \
SESSION_SECRET=acceptance-run-secret \
AUDIT_SECRET=acceptance-run-audit-secret \
  "$PORTAL" >"$LOG" 2>&1 < /dev/null &
PORTAL_PID=$!

for _ in $(seq 1 60); do
  if curl -sf -m 2 "http://localhost:$PORT/healthz" >/dev/null 2>&1; then break; fi
  if ! kill -0 "$PORTAL_PID" 2>/dev/null; then
    echo "the portal exited during boot:"; cat "$LOG"; exit 1
  fi
  sleep 0.5
done

if ! curl -sf -m 2 "http://localhost:$PORT/healthz" >/dev/null 2>&1; then
  echo "the portal never became healthy:"; cat "$LOG"; exit 1
fi

sed "s|base_url = \"[^\"]*\"|base_url = \"http://localhost:$PORT\"|" \
  "$REPO/.dogfood.toml" > "$WORK/dogfood.toml"

cd "$REPO"
python3 run.py "$WORK/dogfood.toml" > acceptance-report.txt
echo "--- acceptance-report.txt ---"
cat acceptance-report.txt
