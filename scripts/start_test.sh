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

assert_fails_with() {
  local needle="$1"
  shift
  local output
  local status

  set +e
  output="$("$@" 2>&1)"
  status="$?"
  set -e

  if [[ "$status" == "0" ]]; then
    fail "expected command to fail"
  fi
  assert_contains "$output" "$needle"
}

help_output="$("$SCRIPT" --help)"
assert_contains "$help_output" "Usage:"
assert_contains "$help_output" "SKIP_WEB_BUILD=1"
assert_contains "$help_output" "AIOPS_HTTP_ADDR=:18080"
assert_contains "$help_output" "AIOPS_STORE_DRIVER=postgres"
assert_contains "$help_output" "AIOPS_ENV_FILE=.data/aiops.env"

preserve_path() {
  local path="$1"
  local backup

  backup="$(mktemp)"
  if [[ -e "$path" ]]; then
    cp "$path" "$backup"
    printf '%s:%s:present\n' "$path" "$backup"
  else
    rm -f "$backup"
    printf '%s::absent\n' "$path"
  fi
}

restore_preserved_path() {
  local record="$1"
  local path backup state

  IFS=: read -r path backup state <<<"$record"
  if [[ "$state" == "present" ]]; then
    cp "$backup" "$path"
    rm -f "$backup"
  else
    rm -f "$path"
  fi
}

mkdir -p "$ROOT_DIR/.data"
aiops_env_record="$(preserve_path "$ROOT_DIR/.data/aiops.env")"
env_local_record="$(preserve_path "$ROOT_DIR/.env.local")"
coroot_env_record="$(preserve_path "$ROOT_DIR/.data/coroot.env")"

cat >"$ROOT_DIR/.data/aiops.env" <<'EOF'
AIOPS_HTTP_ADDR=:19082
AIOPS_STORE_DRIVER=json
EOF
cat >"$ROOT_DIR/.env.local" <<'EOF'
AIOPS_HTTP_ADDR=:19083
EOF
cat >"$ROOT_DIR/.data/coroot.env" <<'EOF'
AIOPS_GRPC_ADDR=:19091
EOF

default_env_file_output="$(
  env -i \
  PATH="$PATH" \
  SKIP_WEB_BUILD=1 \
  SKIP_GO_BUILD=1 \
  "$SCRIPT" --dry-run
)"

restore_preserved_path "$aiops_env_record"
restore_preserved_path "$env_local_record"
restore_preserved_path "$coroot_env_record"

assert_contains "$default_env_file_output" "AIOPS_HTTP_ADDR=:19082"
assert_contains "$default_env_file_output" "AIOPS_STORE_DRIVER=json"
if [[ "$default_env_file_output" == *"AIOPS_GRPC_ADDR=:19091"* ]]; then
  fail "expected .data/coroot.env not to be loaded automatically"
fi

default_dry_run_output="$(
  SKIP_WEB_BUILD=1 \
  SKIP_GO_BUILD=1 \
  AIOPS_ENV_FILE=/tmp/aiops-v2-start-test-missing.env \
  "$SCRIPT" --dry-run
)"

assert_contains "$default_dry_run_output" "AIOPS_HTTP_ADDR=:18080"
assert_contains "$default_dry_run_output" "AIOPS_GRPC_ADDR=:18090"
assert_contains "$default_dry_run_output" "AIOPS_STORE_DRIVER=postgres"

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

env_file="$(mktemp)"
cat >"$env_file" <<'EOF'
AIOPS_HTTP_ADDR=:19081
AIOPS_STORE_DRIVER=json
EOF
env_file_output="$(
  env -i \
  PATH="$PATH" \
  AIOPS_ENV_FILE="$env_file" \
  SKIP_WEB_BUILD=1 \
  SKIP_GO_BUILD=1 \
  "$SCRIPT" --dry-run
)"
rm -f "$env_file"
assert_contains "$env_file_output" "AIOPS_HTTP_ADDR=:19081"
assert_contains "$env_file_output" "AIOPS_STORE_DRIVER=json"

assert_fails_with "AIOPS_POSTGRES_DSN is required when AIOPS_STORE_DRIVER=postgres" \
  env \
  SKIP_WEB_BUILD=1 \
  SKIP_GO_BUILD=1 \
  AIOPS_STORE_DRIVER=postgres \
  AIOPS_POSTGRES_DSN= \
  DATABASE_URL= \
  "$SCRIPT"

if command -v python3 >/dev/null 2>&1; then
  unused_port="$(
    python3 - <<'PY'
import socket

sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
sock.bind(("127.0.0.1", 0))
print(sock.getsockname()[1])
sock.close()
PY
  )"

  assert_fails_with "dependency unavailable: postgresql" \
    env \
    SKIP_WEB_BUILD=1 \
    SKIP_GO_BUILD=1 \
    AIOPS_STORE_DRIVER=postgres \
    AIOPS_POSTGRES_DSN="postgres://postgres:postgres@127.0.0.1:${unused_port}/aiops?sslmode=disable" \
    "$SCRIPT"

  env \
    SKIP_WEB_BUILD=1 \
    SKIP_GO_BUILD=1 \
    AIOPS_STORE_DRIVER=json \
    AIOPS_COROOT_BASE_URL="http://127.0.0.1:${unused_port}" \
    "$SCRIPT" --dry-run >/dev/null
fi
