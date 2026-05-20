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
  AIOPS_ENV_FILE=.data/aiops.env
                               unified KEY=VALUE file loaded without overriding explicit env vars
  AIOPS_STORE_DRIVER=postgres  persisted backend store, default postgres for this script
  AIOPS_POSTGRES_DSN=postgres://aiops:aiops@127.0.0.1:55432/aiops?sslmode=disable
                               PostgreSQL DSN used when AIOPS_STORE_DRIVER=postgres
  AIOPS_OTEL_ENABLED=1         when enabled, AIOPS_OTEL_ENDPOINT must be reachable
  AIOPS_RUNNER_STUDIO_UPSTREAM_URL=http://127.0.0.1:19080
                               when set, Runner Studio upstream must be reachable
  AIOPS_DEPENDENCY_TIMEOUT=2   dependency connection timeout in seconds
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

trim_spaces() {
  local value="$1"
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  printf '%s' "$value"
}

strip_optional_quotes() {
  local value="$1"
  if [[ "$value" == \"*\" && "$value" == *\" ]]; then
    value="${value:1:${#value}-2}"
  elif [[ "$value" == \'*\' && "$value" == *\' ]]; then
    value="${value:1:${#value}-2}"
  fi
  printf '%s' "$value"
}

load_env_file() {
  local file="$1"
  local line key value

  [[ -n "$file" && -f "$file" ]] || return 0

  while IFS= read -r line || [[ -n "$line" ]]; do
    line="$(trim_spaces "$line")"
    [[ -n "$line" && "$line" != \#* ]] || continue
    if [[ "$line" == export[[:space:]]* ]]; then
      line="$(trim_spaces "${line#export}")"
    fi
    if [[ "$line" =~ ^([A-Za-z_][A-Za-z0-9_]*)=(.*)$ ]]; then
      key="${BASH_REMATCH[1]}"
      value="$(trim_spaces "${BASH_REMATCH[2]}")"
      value="$(strip_optional_quotes "$value")"
      if [[ -z "${!key+x}" ]]; then
        export "$key=$value"
      fi
    fi
  done <"$file"
}

load_env_files() {
  AIOPS_ENV_FILE="${AIOPS_ENV_FILE:-.data/aiops.env}"
  export AIOPS_ENV_FILE
  load_env_file "$AIOPS_ENV_FILE"
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

lower() {
  printf '%s' "$1" | tr '[:upper:]' '[:lower:]'
}

truthy() {
  case "$(lower "${1:-}")" in
    1|true|yes|on|enabled)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

first_non_empty_env() {
  local key
  local value

  for key in "$@"; do
    value="${!key:-}"
    if [[ -n "$value" ]]; then
      printf '%s\n' "$value"
      return 0
    fi
  done

  return 1
}

default_port_for_scheme() {
  case "$(lower "$1")" in
    http)
      printf '80\n'
      ;;
    https)
      printf '443\n'
      ;;
    postgres|postgresql)
      printf '5432\n'
      ;;
    *)
      return 1
      ;;
  esac
}

url_host_port() {
  local url="$1"
  local scheme
  local rest
  local authority
  local host
  local port

  if [[ ! "$url" =~ ^([A-Za-z][A-Za-z0-9+.-]*)://(.+)$ ]]; then
    return 1
  fi
  scheme="${BASH_REMATCH[1]}"
  rest="${BASH_REMATCH[2]}"
  authority="${rest%%/*}"
  authority="${authority%%\?*}"
  authority="${authority##*@}"

  if [[ "$authority" =~ ^\[([^]]+)\](:([0-9]+))?$ ]]; then
    host="${BASH_REMATCH[1]}"
    port="${BASH_REMATCH[3]:-}"
  elif [[ "$authority" =~ ^([^:]+):([0-9]+)$ ]]; then
    host="${BASH_REMATCH[1]}"
    port="${BASH_REMATCH[2]}"
  else
    host="$authority"
    port="$(default_port_for_scheme "$scheme")"
  fi

  if [[ -z "$host" || -z "$port" ]]; then
    return 1
  fi

  printf '%s %s\n' "$host" "$port"
}

tcp_open() {
  local host="$1"
  local port="$2"
  local timeout="${AIOPS_DEPENDENCY_TIMEOUT:-2}"

  if command -v nc >/dev/null 2>&1; then
    nc -z -w "$timeout" "$host" "$port" >/dev/null 2>&1
    return
  fi

  (echo >/dev/tcp/"$host"/"$port") >/dev/null 2>&1
}

check_tcp_dependency() {
  local name="$1"
  local host="$2"
  local port="$3"

  if tcp_open "$host" "$port"; then
    log "dependency ready: $name ($host:$port)"
    return 0
  fi

  printf 'dependency unavailable: %s (%s:%s)\n' "$name" "$host" "$port" >&2
  printf 'start the required middleware first, then run ./scripts/start.sh again.\n' >&2
  return 1
}

check_url_dependency() {
  local name="$1"
  local url="$2"
  local endpoint
  local host
  local port
  local timeout="${AIOPS_DEPENDENCY_TIMEOUT:-2}"

  [[ -z "$url" ]] && return 0

  if command -v curl >/dev/null 2>&1; then
    if curl --noproxy '*' -sS -o /dev/null --connect-timeout "$timeout" --max-time "$((timeout + 3))" "$url"; then
      log "dependency ready: $name ($url)"
      return 0
    fi
  else
    if endpoint="$(url_host_port "$url")"; then
      host="${endpoint% *}"
      port="${endpoint##* }"
      if tcp_open "$host" "$port"; then
        log "dependency ready: $name ($host:$port)"
        return 0
      fi
    fi
  fi

  printf 'dependency unavailable: %s (%s)\n' "$name" "$url" >&2
  printf 'start the required middleware first, then run ./scripts/start.sh again.\n' >&2
  return 1
}

mysql_endpoint_from_dsn() {
  local dsn="$1"
  local endpoint
  local socket_path
  local host
  local port
  local unix_re='@unix\(([^)]*)\)'
  local tcp_re='@tcp\(([^)]*)\)'

  if [[ "$dsn" =~ $unix_re ]]; then
    socket_path="${BASH_REMATCH[1]}"
    printf 'unix %s\n' "$socket_path"
    return 0
  fi

  if [[ "$dsn" =~ $tcp_re ]]; then
    endpoint="${BASH_REMATCH[1]}"
    if [[ "$endpoint" =~ ^([^:]*):([0-9]+)$ ]]; then
      host="${BASH_REMATCH[1]:-127.0.0.1}"
      port="${BASH_REMATCH[2]}"
    else
      host="${endpoint:-127.0.0.1}"
      port="3306"
    fi
    printf 'tcp %s %s\n' "$host" "$port"
    return 0
  fi

  printf 'tcp 127.0.0.1 3306\n'
}

check_mysql_dependency() {
  local driver
  local dsn
  local endpoint
  local kind
  local host
  local port
  local socket_path

  driver="$(lower "${AIOPS_STORE_DRIVER:-}")"
  [[ "$driver" != "mysql" ]] && return 0

  dsn="${AIOPS_MYSQL_DSN:-}"
  if [[ -z "$dsn" ]]; then
    printf 'AIOPS_MYSQL_DSN is required when AIOPS_STORE_DRIVER=mysql\n' >&2
    return 1
  fi

  endpoint="$(mysql_endpoint_from_dsn "$dsn")"
  kind="${endpoint%% *}"
  if [[ "$kind" == "unix" ]]; then
    socket_path="${endpoint#unix }"
    if [[ -S "$socket_path" ]]; then
      log "dependency ready: mysql ($socket_path)"
      return 0
    fi
    printf 'dependency unavailable: mysql unix socket (%s)\n' "$socket_path" >&2
    printf 'start the required middleware first, then run ./scripts/start.sh again.\n' >&2
    return 1
  fi

  read -r _ host port <<<"$endpoint"
  check_tcp_dependency "mysql" "$host" "$port"
}

postgres_endpoint_from_dsn() {
  local dsn="$1"
  local endpoint
  local host="127.0.0.1"
  local port="5432"
  local part

  if endpoint="$(url_host_port "$dsn" 2>/dev/null)"; then
    printf 'tcp %s\n' "$endpoint"
    return 0
  fi

  for part in $dsn; do
    case "$part" in
      host=*)
        host="${part#host=}"
        ;;
      port=*)
        port="${part#port=}"
        ;;
    esac
  done

  if [[ "$host" == /* ]]; then
    printf 'unix %s/.s.PGSQL.%s\n' "$host" "$port"
    return 0
  fi

  printf 'tcp %s %s\n' "$host" "$port"
}

check_postgres_dependency() {
  local driver
  local dsn
  local endpoint
  local kind
  local host
  local port
  local socket_path

  driver="$(lower "${AIOPS_STORE_DRIVER:-}")"
  case "$driver" in
    postgres|postgresql)
      ;;
    *)
      return 0
      ;;
  esac

  dsn="$(first_non_empty_env AIOPS_POSTGRES_DSN DATABASE_URL || true)"
  if [[ -z "$dsn" ]]; then
    printf 'AIOPS_POSTGRES_DSN is required when AIOPS_STORE_DRIVER=postgres\n' >&2
    return 1
  fi

  endpoint="$(postgres_endpoint_from_dsn "$dsn")"
  kind="${endpoint%% *}"
  if [[ "$kind" == "unix" ]]; then
    socket_path="${endpoint#unix }"
    if [[ -S "$socket_path" ]]; then
      log "dependency ready: postgresql ($socket_path)"
      return 0
    fi
    printf 'dependency unavailable: postgresql unix socket (%s)\n' "$socket_path" >&2
    printf 'start the required middleware first, then run ./scripts/start.sh again.\n' >&2
    return 1
  fi

  read -r _ host port <<<"$endpoint"
  check_tcp_dependency "postgresql" "$host" "$port"
}

check_configured_dependencies() {
  local runner_upstream_url

  check_postgres_dependency
  check_mysql_dependency

  if truthy "${AIOPS_OTEL_ENABLED:-}"; then
    check_url_dependency "otel" "${AIOPS_OTEL_ENDPOINT:-http://localhost:6006/v1/traces}"
  fi

  runner_upstream_url="$(first_non_empty_env AIOPS_RUNNER_STUDIO_UPSTREAM_URL RUNNER_STUDIO_UPSTREAM_URL AIOPS_RUNNER_API_BASE_URL || true)"
  check_url_dependency "runner studio upstream" "$runner_upstream_url"
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

load_env_files

if [[ -n "${AIOPS_GRPC_ADDR+x}" ]]; then
  AIOPS_GRPC_ADDR_EXPLICIT=1
else
  AIOPS_GRPC_ADDR_EXPLICIT=0
fi

export AIOPS_DATA_DIR="${AIOPS_DATA_DIR:-.data}"
export AIOPS_HTTP_ADDR="${AIOPS_HTTP_ADDR:-:18080}"
export AIOPS_GRPC_ADDR="${AIOPS_GRPC_ADDR:-:18090}"
export AIOPS_WEB_DIST_DIR="${AIOPS_WEB_DIST_DIR:-web/dist}"
export AIOPS_STORE_DRIVER="${AIOPS_STORE_DRIVER:-postgres}"
case "$(lower "$AIOPS_STORE_DRIVER")" in
  postgres|postgresql)
    if [[ -z "${AIOPS_POSTGRES_DSN+x}" && -z "${DATABASE_URL+x}" ]]; then
      export AIOPS_POSTGRES_DSN="postgres://aiops:aiops@127.0.0.1:55432/aiops?sslmode=disable"
    fi
    ;;
esac

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
AIOPS_STORE_DRIVER=$AIOPS_STORE_DRIVER
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
  printf 'would check configured middleware dependencies before build/start\n'
  printf 'would start: %s\n' "$AIOPS_SERVER_BIN"
  exit 0
fi

check_configured_dependencies

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
