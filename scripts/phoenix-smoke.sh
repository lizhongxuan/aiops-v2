#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

export AIOPS_OTEL_ENABLED="${AIOPS_OTEL_ENABLED:-1}"
export AIOPS_OTEL_ENDPOINT="${AIOPS_OTEL_ENDPOINT:-http://localhost:6006/v1/traces}"
export AIOPS_DEBUG_MODEL_INPUT_TRACE="${AIOPS_DEBUG_MODEL_INPUT_TRACE:-1}"
export AIOPS_DEBUG_MODEL_INPUT_TRACE_DIR="${AIOPS_DEBUG_MODEL_INPUT_TRACE_DIR:-.data/model-input-traces}"

echo "Phoenix smoke config:"
echo "  AIOPS_OTEL_ENABLED=$AIOPS_OTEL_ENABLED"
echo "  AIOPS_OTEL_ENDPOINT=$AIOPS_OTEL_ENDPOINT"
echo "  AIOPS_DEBUG_MODEL_INPUT_TRACE=$AIOPS_DEBUG_MODEL_INPUT_TRACE"
echo "  AIOPS_DEBUG_MODEL_INPUT_TRACE_DIR=$AIOPS_DEBUG_MODEL_INPUT_TRACE_DIR"
echo

echo "Running focused observability tests..."
go test ./internal/observability -run 'TestConfigFromEnv|TestInit|TestRuntimeObserver' -count=1

echo "Running focused runtime trace tests..."
go test ./internal/runtimekernel -run 'TestRunTurnObserverRecordsTurnAndModelCall|TestToolDispatcherObserverRecordsToolOutcome|TestRunTurn_CreatesRootSpanForEachTurn|TestRunTurn_EmitsIterationStageActivityUpdates|TestToolDispatcher|TestDispatch' -count=1

cat <<'EOF'

Manual E2E check:

1. Start Phoenix in a separate terminal:
   docker run --rm -p 6006:6006 -p 4317:4317 arizephoenix/phoenix:latest

2. Start aiops server in another terminal:
   AIOPS_OTEL_ENABLED=1 \
   AIOPS_OTEL_ENDPOINT=http://localhost:6006/v1/traces \
   AIOPS_DEBUG_MODEL_INPUT_TRACE=1 \
   AIOPS_DATA_DIR=.data \
   ./scripts/start.sh

3. Run one short agent turn from the normal chat/API path.

4. Open Phoenix:
   http://localhost:6006

5. Check the latest aiops-v2-agent trace:
   - agent.turn root span exists.
   - model_call child span exists.
   - tool_call.<tool_name> child span exists when the turn calls tools.
   - model_call has trace.file and trace.diff attributes.

6. Open the local prompt trace referenced by trace.file under:
   .data/model-input-traces/

This script does not start long-running Phoenix or aiops server processes.
EOF
