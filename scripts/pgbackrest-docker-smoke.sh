#!/usr/bin/env bash
set -euo pipefail

BASE_IMAGE="${AIOPS_PGBACKREST_SMOKE_BASE_IMAGE:-postgres:16-alpine}"
IMAGE_NAME="${AIOPS_PGBACKREST_SMOKE_IMAGE:-aiops-pgbackrest-smoke:local}"
CONTAINER_NAME="${AIOPS_PGBACKREST_SMOKE_CONTAINER:-aiops-pgbackrest-smoke-$$}"
CPU_LIMIT="${AIOPS_PGBACKREST_SMOKE_CPUS:-1}"
MEMORY_LIMIT="${AIOPS_PGBACKREST_SMOKE_MEMORY:-1g}"
KEEP="${KEEP_PGBACKREST_SMOKE:-0}"
KEEP_IMAGE="${KEEP_PGBACKREST_SMOKE_IMAGE:-0}"
DRY_RUN=0

usage() {
  cat <<'EOF'
Usage:
  ./scripts/pgbackrest-docker-smoke.sh [--dry-run]

Builds a disposable Docker image with PostgreSQL 16 and pgBackRest, then runs:
  1. initdb with archive_command configured for pgBackRest
  2. create stanza and run pgBackRest check
  3. create test data and run a full backup
  4. verify pgBackRest info/check
  5. remove PGDATA, restore from backup, and compare restored data count/hash

Environment overrides:
  AIOPS_PGBACKREST_SMOKE_IMAGE=aiops-pgbackrest-smoke:local
  AIOPS_PGBACKREST_SMOKE_BASE_IMAGE=postgres:16-alpine
  AIOPS_PGBACKREST_SMOKE_CONTAINER=aiops-pgbackrest-smoke-<pid>
  AIOPS_PGBACKREST_SMOKE_CPUS=1
  AIOPS_PGBACKREST_SMOKE_MEMORY=1g
  KEEP_PGBACKREST_SMOKE=1          keep temp build context and failed container
  KEEP_PGBACKREST_SMOKE_IMAGE=1    keep the built image tag after the run
EOF
}

log() {
  printf '[pgbackrest-smoke] %s\n' "$*"
}

truthy() {
  case "${1:-}" in
    1|true|TRUE|yes|YES|on|ON)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

require_command() {
  local name="$1"

  if ! command -v "$name" >/dev/null 2>&1; then
    printf 'missing required command: %s\n' "$name" >&2
    exit 127
  fi
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run)
      DRY_RUN=1
      shift
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      printf 'unknown argument: %s\n' "$1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

require_command docker

if [[ "$DRY_RUN" == "1" ]]; then
  cat <<EOF
image: $IMAGE_NAME
base_image: $BASE_IMAGE
container: $CONTAINER_NAME
limits: cpus=$CPU_LIMIT memory=$MEMORY_LIMIT
phases:
  - build PostgreSQL 16 + pgBackRest smoke image
  - initdb and enable WAL archiving through pgBackRest
  - create stanza, check archive health, and run full backup
  - verify pgBackRest info/check after backup
  - delete PGDATA, restore from pgBackRest, and compare count/hash
EOF
  exit 0
fi

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/aiops-pgbackrest-smoke.XXXXXX")"

cleanup() {
  if ! truthy "$KEEP"; then
    docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true
    rm -rf "$TMP_DIR"
  else
    log "kept build context: $TMP_DIR"
  fi

  if ! truthy "$KEEP_IMAGE"; then
    docker rmi "$IMAGE_NAME" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

cat >"$TMP_DIR/Dockerfile" <<'DOCKERFILE'
ARG BASE_IMAGE=postgres:16-alpine
FROM ${BASE_IMAGE}

RUN if command -v apk >/dev/null 2>&1; then \
    apk add --no-cache ca-certificates pgbackrest; \
  elif command -v apt-get >/dev/null 2>&1; then \
    apt-get update \
      && apt-get install -y --no-install-recommends ca-certificates pgbackrest \
      && rm -rf /var/lib/apt/lists/*; \
  else \
    echo "unsupported base image: missing apk or apt-get" >&2; \
    exit 1; \
  fi

COPY run-pgbackrest-smoke.sh /usr/local/bin/run-pgbackrest-smoke.sh
RUN chmod 0755 /usr/local/bin/run-pgbackrest-smoke.sh

USER root
ENTRYPOINT ["/usr/local/bin/run-pgbackrest-smoke.sh"]
DOCKERFILE

cat >"$TMP_DIR/run-pgbackrest-smoke.sh" <<'CONTAINER_SCRIPT'
#!/usr/bin/env bash
set -euo pipefail

export PATH="/usr/lib/postgresql/16/bin:$PATH"

STANZA="demo"
APP_DB="appdb"
PGDATA="/var/lib/postgresql/data/pgdata"
PGBACKREST_CONFIG="/etc/pgbackrest/pgbackrest.conf"
POSTGRES_LOG="/tmp/postgres.log"
RESTORE_LOG="/tmp/postgres-restored.log"

log() {
  printf '[container] %s\n' "$*"
}

as_postgres() {
  su -s /bin/bash postgres -c "$*"
}

query_scalar() {
  as_postgres "psql -At -d '$APP_DB' -v ON_ERROR_STOP=1 -c \"$1\""
}

stop_postgres() {
  if [[ -d "$PGDATA" ]]; then
    as_postgres "pg_ctl -D '$PGDATA' -m fast -w stop" >/dev/null 2>&1 || true
  fi
}
trap stop_postgres EXIT

log "prepare directories"
install -d -o postgres -g postgres -m 0700 /var/lib/postgresql/data
install -d -o postgres -g postgres -m 0750 /var/lib/pgbackrest /var/log/pgbackrest
install -d -o postgres -g postgres -m 0750 /etc/pgbackrest

log "initdb"
as_postgres "initdb -D '$PGDATA' --encoding=UTF8"

cat >>"$PGDATA/postgresql.conf" <<EOF
listen_addresses = 'localhost'
port = 5432
archive_mode = on
archive_command = 'pgbackrest --stanza=$STANZA archive-push %p'
wal_level = replica
max_wal_senders = 3
EOF

cat >"$PGBACKREST_CONFIG" <<EOF
[global]
repo1-path=/var/lib/pgbackrest
repo1-retention-full=2
log-level-console=info
log-level-file=detail
log-path=/var/log/pgbackrest
start-fast=y
process-max=2

[$STANZA]
pg1-path=$PGDATA
pg1-port=5432
pg1-user=postgres
EOF
chown -R postgres:postgres /etc/pgbackrest /var/lib/pgbackrest /var/log/pgbackrest "$PGDATA"

log "start postgres"
as_postgres "pg_ctl -D '$PGDATA' -w -l '$POSTGRES_LOG' start"
as_postgres "createdb '$APP_DB'"

log "create test data"
cat >/tmp/seed.sql <<'SQL'
CREATE TABLE smoke_data (
  id integer PRIMARY KEY,
  payload text NOT NULL
);

INSERT INTO smoke_data(id, payload)
SELECT g, 'pgbackrest-row-' || g::text
FROM generate_series(1, 50) AS g;

CHECKPOINT;
SQL
as_postgres "psql -d '$APP_DB' -v ON_ERROR_STOP=1 -f /tmp/seed.sql"

BEFORE_COUNT="$(query_scalar 'SELECT count(*) FROM smoke_data;')"
BEFORE_HASH="$(query_scalar "SELECT md5(string_agg(payload, ',' ORDER BY id)) FROM smoke_data;")"
log "before backup count=$BEFORE_COUNT hash=$BEFORE_HASH"

log "create stanza"
as_postgres "pgbackrest --stanza=$STANZA stanza-create"

log "check stanza and archiving"
as_postgres "pgbackrest --stanza=$STANZA check"

log "run full backup"
as_postgres "pgbackrest --stanza=$STANZA --type=full backup"

log "verify backup info"
as_postgres "pgbackrest --stanza=$STANZA info" | tee /tmp/pgbackrest-info.txt
grep -q 'status: ok' /tmp/pgbackrest-info.txt
grep -q 'full backup:' /tmp/pgbackrest-info.txt

log "re-run pgBackRest check after backup"
as_postgres "pgbackrest --stanza=$STANZA check"

log "simulate PGDATA loss"
as_postgres "pg_ctl -D '$PGDATA' -m fast -w stop"
mv "$PGDATA" "${PGDATA}.lost"
install -d -o postgres -g postgres -m 0700 "$PGDATA"

log "restore backup"
as_postgres "pgbackrest --stanza=$STANZA restore"

log "start restored postgres"
as_postgres "pg_ctl -D '$PGDATA' -w -l '$RESTORE_LOG' start"

AFTER_COUNT="$(query_scalar 'SELECT count(*) FROM smoke_data;')"
AFTER_HASH="$(query_scalar "SELECT md5(string_agg(payload, ',' ORDER BY id)) FROM smoke_data;")"
log "after restore count=$AFTER_COUNT hash=$AFTER_HASH"

if [[ "$BEFORE_COUNT" != "$AFTER_COUNT" ]]; then
  printf 'count mismatch: before=%s after=%s\n' "$BEFORE_COUNT" "$AFTER_COUNT" >&2
  exit 1
fi

if [[ "$BEFORE_HASH" != "$AFTER_HASH" ]]; then
  printf 'hash mismatch: before=%s after=%s\n' "$BEFORE_HASH" "$AFTER_HASH" >&2
  exit 1
fi

cat <<EOF
PGBACKREST_DOCKER_SMOKE_RESULT_BEGIN
status=passed
stanza=$STANZA
database=$APP_DB
rows=$AFTER_COUNT
hash=$AFTER_HASH
PGBACKREST_DOCKER_SMOKE_RESULT_END
EOF
CONTAINER_SCRIPT

log "build image: $IMAGE_NAME from $BASE_IMAGE"
docker build --build-arg "BASE_IMAGE=$BASE_IMAGE" -t "$IMAGE_NAME" "$TMP_DIR"

docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true

docker_run_args=(
  --name "$CONTAINER_NAME"
  --cpus "$CPU_LIMIT"
  --memory "$MEMORY_LIMIT"
)
if ! truthy "$KEEP"; then
  docker_run_args+=(--rm)
fi

log "run container: $CONTAINER_NAME"
docker run "${docker_run_args[@]}" "$IMAGE_NAME"
