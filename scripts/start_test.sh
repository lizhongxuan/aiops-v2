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
