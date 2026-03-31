#!/usr/bin/env bash
# Cleanup: stop the fake OpenStack server and remove cloned gostack, binaries, PID file.
# Run this after all integration tests complete (make integration-test does this automatically).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INTEGRATION_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
GOSTACK_DIR="${INTEGRATION_DIR}/.gostack"
PID_FILE="${PID_FILE:-/tmp/fake_os_server.pid}"

log() { echo "[cleanup] $*" >&2; }

# Stop the fake server
if [[ -f "$PID_FILE" ]]; then
  PID=$(cat "$PID_FILE" 2>/dev/null || true)
  if [[ -n "$PID" ]]; then
    if kill -0 "$PID" 2>/dev/null; then
      log "Stopping fake OpenStack server (PID $PID)..."
      kill "$PID" 2>/dev/null || true
      sleep 1
      kill -9 "$PID" 2>/dev/null || true
    fi
  fi
  rm -f "$PID_FILE"
  log "Removed PID file"
fi

# Remove cloned gostack and binaries
if [[ -d "$GOSTACK_DIR" ]]; then
  log "Removing cloned gostack ($GOSTACK_DIR)..."
  rm -rf "$GOSTACK_DIR"
  log "Cleanup complete"
fi
