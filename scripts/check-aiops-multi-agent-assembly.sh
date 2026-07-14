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

require_file scripts/check-aiops-runtime-boundary.sh
require_file internal/agentassembly/admission.go
require_file internal/agentassembly/admission_test.go

bash scripts/check-aiops-runtime-boundary.sh

go test ./internal/agentassembly ./internal/agentmgr ./internal/eval \
  -run 'AgentKind|ProfileOnly|RuntimeAgent|Leakage|Eval|LearnedAsset' \
  -count=1

echo "PASS: aiops multi-agent assembly guards"
