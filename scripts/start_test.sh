#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRIPT="$ROOT_DIR/scripts/start.sh"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

assert_contains() {
  local haystack="$1"
  local needle="$2"

  if [[ "$haystack" != *"$needle"* ]]; then
    fail "expected output to contain: $needle"
  fi
}

help_output="$("$SCRIPT" --help)"
assert_contains "$help_output" "Usage:"
assert_contains "$help_output" "SKIP_WEB_BUILD=1"
assert_contains "$help_output" "AIOPS_HTTP_ADDR=:18080"
assert_contains "$help_output" "AIOPS_GRPC_AUTO_PORT=1"

default_dry_run_output="$(
  SKIP_WEB_BUILD=1 \
  SKIP_GO_BUILD=1 \
  AIOPS_GRPC_AUTO_PORT=0 \
  "$SCRIPT" --dry-run
)"

assert_contains "$default_dry_run_output" "AIOPS_HTTP_ADDR=:18080"
assert_contains "$default_dry_run_output" "AIOPS_GRPC_ADDR=:18090"
assert_contains "$default_dry_run_output" "AIOPS_GRPC_AUTO_PORT=0"

dry_run_output="$(
  SKIP_WEB_BUILD=1 \
  SKIP_GO_BUILD=1 \
  AIOPS_HTTP_ADDR=:18080 \
  AIOPS_DATA_DIR=.data-test \
  AIOPS_WEB_DIST_DIR=/tmp/aiops-web-dist \
  "$SCRIPT" --dry-run
)"

assert_contains "$dry_run_output" "AIOPS_HTTP_ADDR=:18080"
assert_contains "$dry_run_output" "AIOPS_DATA_DIR=.data-test"
assert_contains "$dry_run_output" "AIOPS_WEB_DIST_DIR=/tmp/aiops-web-dist"
assert_contains "$dry_run_output" "SKIP_WEB_BUILD=1"
assert_contains "$dry_run_output" "SKIP_GO_BUILD=1"
assert_contains "$dry_run_output" "would start:"

if command -v python3 >/dev/null 2>&1; then
  ready_file="$(mktemp)"
  python3 - "$ready_file" <<'PY' &
import pathlib
import socket
import sys
import time

sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
try:
    sock.bind(("127.0.0.1", 18090))
except OSError:
    pathlib.Path(sys.argv[1]).write_text("already-busy")
    sys.exit(0)
sock.listen(1)
pathlib.Path(sys.argv[1]).write_text("ready")
try:
    while True:
        time.sleep(1)
finally:
    sock.close()
PY
  listener_pid="$!"
  cleanup_listener() {
    kill "$listener_pid" >/dev/null 2>&1 || true
    wait "$listener_pid" 2>/dev/null || true
    rm -f "$ready_file"
  }
  trap cleanup_listener EXIT

  for _ in {1..50}; do
    [[ -s "$ready_file" ]] && break
    sleep 0.1
  done

  auto_port_output="$(
    SKIP_WEB_BUILD=1 \
    SKIP_GO_BUILD=1 \
    "$SCRIPT" --dry-run
  )"

  if [[ "$auto_port_output" == *"AIOPS_GRPC_ADDR=:18090"* ]]; then
    fail "expected auto-selected gRPC address when :18090 is busy"
  fi
  assert_contains "$auto_port_output" "auto-selected gRPC listen address"
  cleanup_listener
  trap - EXIT
fi
