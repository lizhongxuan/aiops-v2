#!/usr/bin/env bash
set -euo pipefail

export AIOPS_HTTP_ADDR="${AIOPS_HTTP_ADDR:-127.0.0.1:18083}"
export AIOPS_GRPC_ADDR="${AIOPS_GRPC_ADDR:-127.0.0.1:18093}"

http_url="http://${AIOPS_HTTP_ADDR}"
server_bin="${AIOPS_SERVER_BIN:-.data/bin/ai-server}"

"$server_bin" &
server_pid=$!

cleanup() {
  if kill -0 "$server_pid" >/dev/null 2>&1; then
    kill "$server_pid" >/dev/null 2>&1 || true
  fi
}
trap cleanup INT TERM EXIT

for _ in $(seq 1 120); do
  if curl -fsS "$http_url/api/v1/runtime-settings" >/dev/null 2>&1; then
    break
  fi
  if ! kill -0 "$server_pid" >/dev/null 2>&1; then
    wait "$server_pid"
  fi
  sleep 0.5
done

curl -fsS -X PATCH "$http_url/api/v1/runtime-settings" \
  -H 'Content-Type: application/json' \
  --data '{"debug":{"modelInputTrace":true,"finalState":true,"transportProjection":true}}' >/dev/null || true

wait "$server_pid"
