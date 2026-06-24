#!/usr/bin/env bash
# Local end-to-end acceptance test for the Robot Agent Registry (spec §5).
# Spins up a throwaway Postgres, builds + runs the server, installs the Python
# CLI in a venv, and walks owner → platform → agent → discover. Cleans up after
# itself. Requires: go, python3, and Postgres client tools (initdb/pg_ctl/psql).
set -uo pipefail
cd "$(dirname "$0")/.."

WORK=$(mktemp -d)
HOME_ORIG="$HOME"
PGPORT=${PGPORT:-55434}
APIPORT=${APIPORT:-8090}
SRV="" AG="" PG_STARTED=""

cleanup() {
  [ -n "$AG" ] && kill "$AG" 2>/dev/null
  [ -n "$SRV" ] && kill "$SRV" 2>/dev/null
  [ -n "$PG_STARTED" ] && pg_ctl -D "$WORK/pg" stop >/dev/null 2>&1
  export HOME="$HOME_ORIG"
  rm -rf "$WORK"
}
trap cleanup EXIT

fail() { echo "FAIL: $1"; exit 1; }

echo "== postgres =="
initdb -D "$WORK/pg" -U postgres --auth=trust >/dev/null 2>&1 || fail "initdb"
pg_ctl -D "$WORK/pg" -o "-p $PGPORT -k $WORK/pg" -l "$WORK/pg.log" start >/dev/null 2>&1 || fail "pg start"
PG_STARTED=1
sleep 1
createdb -h 127.0.0.1 -p "$PGPORT" -U postgres rar >/dev/null 2>&1 || fail "createdb"

echo "== build + run server =="
go build -o "$WORK/registry" ./cmd/registry || fail "go build"
DATABASE_URL="postgres://postgres@127.0.0.1:$PGPORT/rar?sslmode=disable" \
  JWT_SIGNING_KEY="localtestkey" PORT="$APIPORT" \
  REGISTRY_BASE_URL="http://localhost:$APIPORT/v1" \
  "$WORK/registry" >"$WORK/server.log" 2>&1 &
SRV=$!
for _ in $(seq 1 20); do curl -sf "http://localhost:$APIPORT/healthz" >/dev/null 2>&1 && break; sleep 0.3; done
curl -sf "http://localhost:$APIPORT/healthz" >/dev/null || fail "server health"

echo "== install python cli =="
python3 -m venv "$WORK/venv" || fail "venv"
"$WORK/venv/bin/pip" install -q "./sdk/python" || fail "pip install"
RAR="$WORK/venv/bin/rar"; RARAGENT="$WORK/venv/bin/rar-agent"
export HOME="$WORK"
export RAR_REGISTRY_URL="http://localhost:$APIPORT/v1"

echo "== owner register + login =="
"$RAR" auth register --name "E2E Tester" --email e2e@example.com || fail "register"
"$RAR" auth login || fail "login"

echo "== platform register =="
"$RAR" platform register --name "E2E RPi" --type raspberry_pi --tags "lidar,outdoor" >"$WORK/plt.txt" || fail "platform"
export RAR_PLATFORM_TOKEN=$(awk '/platform_token/{print $3; exit}' "$WORK/plt.txt")
export RAR_PLATFORM_ID=$(awk '/platform_id/{print $3; exit}' "$WORK/plt.txt")
[ -n "$RAR_PLATFORM_TOKEN" ] && [ -n "$RAR_PLATFORM_ID" ] || fail "missing platform token/id"

echo "== rar-agent start =="
"$RARAGENT" start --preset operations --interval 30 >"$WORK/agent.log" 2>&1 &
AG=$!; sleep 3

echo "== discover =="
OUT=$("$RAR" discover agents --online --platform-type raspberry_pi)
echo "$OUT"
echo "$OUT" | grep -q "operations-agent" || fail "agent not discovered"
echo "$OUT" | grep -q "get_system_status" || fail "capability not discovered"

echo
echo "PASS: end-to-end discovery works."
