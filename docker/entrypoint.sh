#!/usr/bin/env sh
set -eu

: "${AIOPS_HTTP_ADDR:=:8080}"
: "${AIOPS_GRPC_ADDR:=:18090}"
: "${AIOPS_DATA_DIR:=/var/lib/aiops}"
: "${AIOPS_WEB_DIST_DIR:=/app/web/dist}"

export AIOPS_HTTP_ADDR
export AIOPS_GRPC_ADDR
export AIOPS_DATA_DIR
export AIOPS_WEB_DIST_DIR

mkdir -p "$AIOPS_DATA_DIR"

exec "$@"
