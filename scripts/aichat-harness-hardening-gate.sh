#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

go test ./cmd/ai-server -run TestRegisterBuiltinPluginsRegistersCorootToolsWithoutStartupEnv -count=1
go test ./internal/runtimekernel/toolfailure -count=1
go test ./internal/runtimekernel/state -count=1
go test ./internal/runtimekernel -run 'Test.*ActiveTurnMigration|Test.*ToolAttempt|TestReadOnlyRetry|TestAIChatHarnessGoldenCases|TestToolDispatcher.*ActionToken|TestRecoverTurn|TestCancelTurn' -count=1
go test ./internal/actionproposal -count=1
go test ./internal/mcp/... -count=1
go test ./internal/appui -run 'TestCapabilitySnapshot|TestAgentProfile.*Preview|TestTransportProjector|TestAgentEventProjector' -count=1
go test ./internal/featureflag -count=1
if [[ -d cmd/aiops-active-turn-migrate ]]; then
	go test ./cmd/aiops-active-turn-migrate -count=1
fi
go test ./...
