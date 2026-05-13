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
  AIOPS_HTTP_ADDR=:18080       HTTP listen address, default :18080
  AIOPS_GRPC_ADDR=:18090       gRPC listen address, default :18090
  AIOPS_GRPC_AUTO_PORT=1       when AIOPS_GRPC_ADDR is unset, use :18190+ if :18090 is busy
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

addr_port() {
  local addr="$1"

  if [[ "$addr" =~ :([0-9]+)$ ]]; then
    printf '%s\n' "${BASH_REMATCH[1]}"
    return 0
  fi

  return 1
}

addr_with_port() {
  local addr="$1"
  local port="$2"

  if [[ "$addr" == :* ]]; then
    printf ':%s\n' "$port"
    return
  fi

  printf '%s:%s\n' "${addr%:*}" "$port"
}

port_in_use() {
  local port="$1"

  if command -v lsof >/dev/null 2>&1; then
    lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1
    return
  fi

  if command -v nc >/dev/null 2>&1; then
    nc -z 127.0.0.1 "$port" >/dev/null 2>&1
    return
  fi

  return 1
}

select_available_grpc_addr() {
  local addr="$1"
  local port

  if ! port="$(addr_port "$addr")"; then
    printf '%s\n' "$addr"
    return 0
  fi

  if ! port_in_use "$port"; then
    printf '%s\n' "$addr"
    return 0
  fi

  local candidate="${AIOPS_GRPC_FALLBACK_ADDR:-:18190}"
  local candidate_port
  if ! candidate_port="$(addr_port "$candidate")"; then
    candidate="$(addr_with_port "$addr" "$((port + 100))")"
    candidate_port="$(addr_port "$candidate")"
  fi

  for _ in {1..50}; do
    if ! port_in_use "$candidate_port"; then
      printf '%s\n' "$candidate"
      return 0
    fi
    candidate_port="$((candidate_port + 1))"
    candidate="$(addr_with_port "$candidate" "$candidate_port")"
  done

  printf 'no available gRPC port found near %s\n' "$candidate" >&2
  return 1
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

if [[ -n "${AIOPS_GRPC_ADDR+x}" ]]; then
  AIOPS_GRPC_ADDR_EXPLICIT=1
else
  AIOPS_GRPC_ADDR_EXPLICIT=0
fi

export AIOPS_DATA_DIR="${AIOPS_DATA_DIR:-.data}"
export AIOPS_HTTP_ADDR="${AIOPS_HTTP_ADDR:-:18080}"
export AIOPS_GRPC_ADDR="${AIOPS_GRPC_ADDR:-:18090}"
export AIOPS_WEB_DIST_DIR="${AIOPS_WEB_DIST_DIR:-web/dist}"

SKIP_WEB_BUILD="${SKIP_WEB_BUILD:-0}"
SKIP_GO_BUILD="${SKIP_GO_BUILD:-0}"
AIOPS_SERVER_BIN="${AIOPS_SERVER_BIN:-.data/bin/ai-server}"
AIOPS_GRPC_AUTO_PORT="${AIOPS_GRPC_AUTO_PORT:-1}"
GRPC_AUTO_SELECTED=0
GRPC_AUTO_ORIGINAL="$AIOPS_GRPC_ADDR"

if [[ "$AIOPS_GRPC_AUTO_PORT" == "1" && "$AIOPS_GRPC_ADDR_EXPLICIT" == "0" ]]; then
  AIOPS_GRPC_ADDR="$(select_available_grpc_addr "$AIOPS_GRPC_ADDR")"
  export AIOPS_GRPC_ADDR
  if [[ "$AIOPS_GRPC_ADDR" != "$GRPC_AUTO_ORIGINAL" ]]; then
    GRPC_AUTO_SELECTED=1
  fi
fi

if [[ "$DRY_RUN" == "1" ]]; then
  cat <<EOF
ROOT_DIR=$ROOT_DIR
AIOPS_HTTP_ADDR=$AIOPS_HTTP_ADDR
AIOPS_GRPC_ADDR=$AIOPS_GRPC_ADDR
AIOPS_GRPC_AUTO_PORT=$AIOPS_GRPC_AUTO_PORT
AIOPS_DATA_DIR=$AIOPS_DATA_DIR
AIOPS_WEB_DIST_DIR=$AIOPS_WEB_DIST_DIR
AIOPS_SERVER_BIN=$AIOPS_SERVER_BIN
SKIP_WEB_BUILD=$SKIP_WEB_BUILD
SKIP_GO_BUILD=$SKIP_GO_BUILD
EOF
  if [[ "$GRPC_AUTO_SELECTED" == "1" ]]; then
    printf 'auto-selected gRPC listen address: %s was busy, using %s\n' "$GRPC_AUTO_ORIGINAL" "$AIOPS_GRPC_ADDR"
  fi
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
if [[ "$GRPC_AUTO_SELECTED" == "1" ]]; then
  log "grpc: $AIOPS_GRPC_ADDR (auto-selected because $GRPC_AUTO_ORIGINAL is busy)"
else
  log "grpc: $AIOPS_GRPC_ADDR"
fi
log "start server"
exec "$AIOPS_SERVER_BIN"
