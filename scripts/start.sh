#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DRY_RUN=0

usage() {
  cat <<'EOF'
Usage:
  ./scripts/start.sh [--dry-run]

Starts aiops-v2 through the production-like single-process entry:
  1. build web/dist
  2. build cmd/ai-server
  3. run ai-server, which serves web/dist directly

Environment overrides:
  AIOPS_HTTP_ADDR=:18080       HTTP listen address, default :8080
  AIOPS_GRPC_ADDR=:18090       gRPC listen address, default :18090
  AIOPS_DATA_DIR=.data         persisted state directory, default .data
  AIOPS_WEB_DIST_DIR=web/dist  frontend dist directory, default web/dist
  AIOPS_SERVER_BIN=.data/bin/ai-server
  SKIP_WEB_BUILD=1             skip npm build
  SKIP_GO_BUILD=1              skip go build

Examples:
  ./scripts/start.sh
  AIOPS_HTTP_ADDR=:18080 ./scripts/start.sh
  SKIP_WEB_BUILD=1 SKIP_GO_BUILD=1 ./scripts/start.sh
EOF
}

log() {
  printf '[aiops-v2] %s\n' "$*"
}

require_command() {
  local name="$1"

  if ! command -v "$name" >/dev/null 2>&1; then
    printf 'missing required command: %s\n' "$name" >&2
    exit 127
  fi
}

http_url() {
  local addr="$1"

  if [[ "$addr" == :* ]]; then
    printf 'http://127.0.0.1%s\n' "$addr"
    return
  fi

  if [[ "$addr" == 0.0.0.0:* ]]; then
    printf 'http://127.0.0.1:%s\n' "${addr##*:}"
    return
  fi

  printf 'http://%s\n' "$addr"
}

for arg in "$@"; do
  case "$arg" in
    --dry-run)
      DRY_RUN=1
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      printf 'unknown argument: %s\n\n' "$arg" >&2
      usage >&2
      exit 2
      ;;
  esac
done

cd "$ROOT_DIR"

export AIOPS_DATA_DIR="${AIOPS_DATA_DIR:-.data}"
export AIOPS_HTTP_ADDR="${AIOPS_HTTP_ADDR:-:8080}"
export AIOPS_GRPC_ADDR="${AIOPS_GRPC_ADDR:-:18090}"
export AIOPS_WEB_DIST_DIR="${AIOPS_WEB_DIST_DIR:-web/dist}"

SKIP_WEB_BUILD="${SKIP_WEB_BUILD:-0}"
SKIP_GO_BUILD="${SKIP_GO_BUILD:-0}"
AIOPS_SERVER_BIN="${AIOPS_SERVER_BIN:-.data/bin/ai-server}"

if [[ "$DRY_RUN" == "1" ]]; then
  cat <<EOF
ROOT_DIR=$ROOT_DIR
AIOPS_HTTP_ADDR=$AIOPS_HTTP_ADDR
AIOPS_GRPC_ADDR=$AIOPS_GRPC_ADDR
AIOPS_DATA_DIR=$AIOPS_DATA_DIR
AIOPS_WEB_DIST_DIR=$AIOPS_WEB_DIST_DIR
AIOPS_SERVER_BIN=$AIOPS_SERVER_BIN
SKIP_WEB_BUILD=$SKIP_WEB_BUILD
SKIP_GO_BUILD=$SKIP_GO_BUILD
EOF
  if [[ "$SKIP_WEB_BUILD" == "1" ]]; then
    printf 'would skip: npm --prefix web run build\n'
  else
    printf 'would run: npm --prefix web run build\n'
  fi
  if [[ "$SKIP_GO_BUILD" == "1" ]]; then
    printf 'would skip: go build -o %s ./cmd/ai-server\n' "$AIOPS_SERVER_BIN"
  else
    printf 'would run: go build -o %s ./cmd/ai-server\n' "$AIOPS_SERVER_BIN"
  fi
  printf 'would start: %s\n' "$AIOPS_SERVER_BIN"
  exit 0
fi

if [[ "$SKIP_WEB_BUILD" != "1" ]]; then
  require_command npm
fi
if [[ "$SKIP_GO_BUILD" != "1" ]]; then
  require_command go
fi

mkdir -p "$AIOPS_DATA_DIR" "$(dirname "$AIOPS_SERVER_BIN")"

if [[ "$SKIP_WEB_BUILD" == "1" ]]; then
  log "skip web build"
else
  log "build frontend: npm --prefix web run build"
  npm --prefix web run build
fi

if [[ "$SKIP_GO_BUILD" == "1" ]]; then
  log "skip go build"
  if [[ ! -x "$AIOPS_SERVER_BIN" ]]; then
    printf 'server binary is not executable: %s\n' "$AIOPS_SERVER_BIN" >&2
    printf 'run without SKIP_GO_BUILD=1 first, or set AIOPS_SERVER_BIN to an executable binary.\n' >&2
    exit 1
  fi
else
  log "build backend: go build -o $AIOPS_SERVER_BIN ./cmd/ai-server"
  go build -o "$AIOPS_SERVER_BIN" ./cmd/ai-server
fi

log "data dir: $AIOPS_DATA_DIR"
log "web dist: $AIOPS_WEB_DIST_DIR"
log "http: $(http_url "$AIOPS_HTTP_ADDR")"
log "start server"
exec "$AIOPS_SERVER_BIN"
