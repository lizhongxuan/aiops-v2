#!/usr/bin/env bash
set -euo pipefail

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

require_file() {
  local file="$1"
  [[ -f "$file" ]] || fail "missing required file: $file"
}

scan_absent() {
  local label="$1"
  local pattern="$2"
  shift 2
  local files=("$@")
  if rg -n "$pattern" "${files[@]}"; then
    fail "$label"
  fi
}

adapter_files=(
  internal/tooling/eino_adapter.go
  internal/modelrouter/eino_message_adapter.go
)

for file in "${adapter_files[@]}" internal/agentmgr/factory.go internal/runtimekernel/runtime_kernel.go; do
  require_file "$file"
done

scan_absent \
  "Eino adapters must not carry business binding, policy, or prompt assembly branches" \
  'HostID|host_bound_ops|ResourceBinding|SessionTarget|RoleBinding|bound_host_id|bound_role|targetRole|pg_primary|pg_standby|PolicyEngine|policyengine|PendingApproval|ApprovalScope|PromptCompiler|CompileContext' \
  "${adapter_files[@]}"

scan_absent \
  "AgentFactory must not bypass PromptCompiler with direct Eino prompt constructors" \
  'schema\.(SystemMessage|UserMessage|AssistantMessage|ToolMessage)\(' \
  internal/agentmgr/factory.go

scan_absent \
  "RuntimeKernel must not depend on Eino DTOs or construct Eino messages directly" \
  'github\.com/cloudwego/eino/schema|schema\.(SystemMessage|UserMessage|AssistantMessage|ToolMessage)\(|\[\]\*schema\.Message|\*schema\.Message' \
  internal/runtimekernel/runtime_kernel.go internal/runtimekernel/model_input.go internal/runtimekernel/turn_loop.go

echo "PASS: aiops runtime boundary guards"
