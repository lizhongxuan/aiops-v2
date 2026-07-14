#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

scripts/test-aiops-harness-contract-boundaries.sh
scripts/test-aiops-change-budget.sh
scripts/check-aiops-harness-contract-boundaries.sh
scripts/check-aiops-change-budget.sh
go test ./internal/runtimekernel -run 'TestAssemblyBoundarySingleBuilder|TestRuntimeStepContextSingleProductionBuilder|TestTurnAssemblyShadowBuildsOnceBeforePromptAndProvider|TestStepToolRouterProviderAndDispatcherShareFingerprint|TestRunTurnProviderRequestAndDispatcherDecisionShareToolSurfaceFingerprint|TestBuildRuntimeStepContextCreatesProviderRequestSnapshot|TestRuntimeStepContextOwnsModelInputProviderRequestAndToolSurface|TestRunTurn_RefreshesToolsBetweenIterations' -count=1
go test ./internal/promptinput -run 'TestBuildFirstStepUsesL0L6OrderAndCurrentUserLast' -count=1
go test ./internal/promptcompiler -run 'TestPromptEnvelopeV2ValidatesLogicalLayerOrder|TestApplyPromptSectionCacheMarksHitMissAndInvalidated' -count=1

go test ./internal/runtimekernel/toolfailure -count=1
go test ./internal/runtimekernel/state -count=1
go test ./internal/runtimekernel -run 'TestAIChatHarnessGoldenCases|Test.*HarnessContract|Test.*FinalContract|Test.*FinalEvidence|Test.*RollbackContract|TestRawToolCallsFromAssistantText|Test.*SessionTarget' -count=1
go test ./internal/agentassembly ./internal/tooling -run 'Test.*ToolSurface.*|Test.*HiddenReason' -count=1
go test ./internal/appui -run 'TestAppUIRuntimeLifecycleHasUniqueWriter|TestTransportProjector|TestApprovalService|TestAgentEventProjector' -count=1
go test ./internal/server -run 'TestAssistantTransport|TestAssistantTransportStories' -count=1
go test ./internal/specialinputmemory -count=1
go test ./internal/appui ./internal/runtimekernel ./internal/resourcebinding -run 'SpecialInput|MemoryReadPlan|ExecutionScope|RoleBinding|TransportCommandsSpecialInput|Correction|Forget|Confirm|Conflict' -count=1
go test ./internal/eval -run 'TestServerAgent|Test.*Harness.*Golden|TestHarnessServerAgent' -count=1
if rg -n '/api/v1/state|/api/v1/chat/message' internal/eval cmd/agent-eval -g '!**/*_test.go'; then
	echo "agent eval must use AssistantTransport instead of legacy chat/state endpoints" >&2
	exit 1
fi
npm --prefix web run test -- src/transport/aiopsTransportConverter.test.ts src/chat/components/ProcessTranscript.test.tsx
npm --prefix web run test -- src/transport/aiopsTransportRuntime.test.ts src/chat/inputMentions.test.ts src/chat/components/SpecialInputContextBar.test.tsx src/chat/components/HostMentionInlineOverlay.css.test.ts
scripts/check-aiops-single-assistant-message.sh

if rg -n 'specialInputContext.*(final|markdown)|markdown.*specialInputContext|final(Text|Output).*specialInputContext' web/src internal -g '!**/*_test.go' -g '!*.test.*'; then
	echo "special input context must come from typed transport/read plan state, not final markdown/text parsing" >&2
	exit 1
fi

if rg -n 'strings\.Contains\([^,\n]*(finalText|FinalOutput|assistantText)' internal web/src -g '!**/*_test.go' -g '!*.test.*'; then
	echo "final text must not be used as a structured state source" >&2
	exit 1
fi

go test ./cmd/ai-server -run TestRegisterBuiltinPluginsRegistersCorootToolsWithoutStartupEnv -count=1
go test ./internal/runtimekernel/toolfailure -count=1
go test ./internal/runtimekernel/state -count=1
go test ./internal/runtimekernel -run 'Test.*ActiveTurnMigration|Test.*ToolAttempt|TestReadOnlyRetry|TestAIChatHarnessGoldenCases|TestToolDispatcher.*ActionToken|TestRecoverTurn|TestCancelTurn' -count=1
go test ./internal/actionproposal -count=1
go test ./internal/mcp/... -count=1
go test ./internal/appui -run 'TestAppUIRuntimeLifecycleHasUniqueWriter|TestCapabilitySnapshot|TestAgentProfile.*Preview|TestTransportProjector|TestAgentEventProjector' -count=1
go test ./internal/featureflag -count=1
scripts/check-aiops-single-assistant-message.sh
if [[ -d cmd/aiops-active-turn-migrate ]]; then
	go test ./cmd/aiops-active-turn-migrate -count=1
fi
if [[ -d cmd/aiops-migrate-assistant-message ]]; then
	go test ./cmd/aiops-migrate-assistant-message -count=1
fi
go test ./...
