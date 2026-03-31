#!/usr/bin/env bash
# Prelude: clone gostack, build, and start the fake OpenStack server for integration tests.
# Run this before executing any integration test playbook (make integration-test does this).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INTEGRATION_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
GOSTACK_DIR="${INTEGRATION_DIR}/.gostack"
PID_FILE="${PID_FILE:-/tmp/fake_os_server.pid}"
PORT="${GOSTACK_PORT:-5000}"
GOSTACK_REPO="${GOSTACK_REPO:-https://github.com/os-migrate/gostack.git}"
GOSTACK_REF="${GOSTACK_REF:-main}"

log() { echo "[prelude] $*" >&2; }

if [[ -f "$PID_FILE" ]]; then
  PID=$(cat "$PID_FILE" 2>/dev/null || true)
  if [[ -n "$PID" ]] && kill -0 "$PID" 2>/dev/null; then
    log "Fake server already running (PID $PID)"
    exit 0
  fi
  rm -f "$PID_FILE"
fi

log "Cloning gostack from $GOSTACK_REPO (ref: $GOSTACK_REF)..."
if [[ -d "$GOSTACK_DIR" ]]; then
  (cd "$GOSTACK_DIR" && git fetch origin && git checkout "$GOSTACK_REF" 2>/dev/null) || rm -rf "$GOSTACK_DIR"
fi
if [[ ! -d "$GOSTACK_DIR" ]]; then
  git clone --depth 1 --branch "$GOSTACK_REF" "$GOSTACK_REPO" "$GOSTACK_DIR"
fi

log "Building fake-openstack binary..."
(cd "$GOSTACK_DIR" && go build -o "${GOSTACK_DIR}/bin/fake-openstack" ./cmd/fake-openstack)

log "Starting fake OpenStack server on port $PORT..."
"${GOSTACK_DIR}/bin/fake-openstack" --port "$PORT" --pid-file "$PID_FILE" &

# Wait for gostack to write its PID file
for ((attempt = 1; attempt <= 10; attempt++)); do
  sleep 1
  [[ -f "$PID_FILE" ]] && break
done
if [[ ! -f "$PID_FILE" ]]; then
  log "ERROR: PID file not created; server may have failed to start"
  exit 1
fi
if ! kill -0 "$(cat "$PID_FILE")" 2>/dev/null; then
  log "ERROR: Server failed to start"
  exit 1
fi

log "Fake OpenStack server is running (PID $(cat "$PID_FILE"))"
log "Run cleanup.sh when all tests are done."
