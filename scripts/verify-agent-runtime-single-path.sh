#!/usr/bin/env bash
set -euo pipefail

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

if rg -n 'CompileForEino|BuildResult\s*\{[^}]*Messages|type\s+EinoKernel|NewEinoKernel|EinoKernelConfig|type\s+RuntimeKernel\s*=\s*EinoKernel' internal --glob '!**/*_test.go'; then
  fail "old runtime or prompt symbols remain"
fi

if rg -n 'chatModel\.(Stream|Generate)|generateModelResponse' internal/runtimekernel --glob '!**/*_test.go'; then
  fail "runtimekernel still calls provider model directly"
fi

if rg -n 'writeModelInputDebugTrace|ModelInputDebugTraceRequest' internal/runtimekernel internal/promptdiag internal/eval --glob '!**/*_test.go'; then
  fail "v1 trace production writer path remains"
fi

if rg -n 'modeltrace\.Write\(' internal --glob '!**/*_test.go'; then
  fail "production code still writes legacy modeltrace schema v1"
fi

if rg -n 'github\.com/cloudwego/eino/schema|schema\.Message|\[\]\*schema\.Message|\*schema\.Message' internal/promptcompiler internal/promptinput internal/runtimekernel internal/modeltrace --glob '!**/*_test.go'; then
  fail "prompt/runtime/trace production code still depends on Eino schema DTOs"
fi

if rg -n 'outputPreview|json:"preview|Preview\s+string|\.Preview|Preview:' \
  internal/runtimekernel/runtime_kernel.go \
  internal/runtimekernel/model_input.go \
  internal/runtimekernel/resource_identity.go \
  internal/runtimekernel/context_artifact.go \
  internal/runtimekernel/context_artifact_reader.go \
  internal/runtimekernel/observation_state.go \
  internal/runtimekernel/types.go \
  --glob '!**/*_test.go'; then
  fail "persisted runtime state still carries preview fields"
fi

if rg -n 'outputPreview|json:"preview|Preview\s+string|\.Preview|Preview:' internal/promptcompiler internal/promptinput internal/modelrouter/provider_request*.go --glob '!**/*_test.go'; then
  fail "prompt/provider request path still carries preview fields"
fi

bash scripts/check-aiops-runtime-boundary.sh

echo "PASS: single runtime path guards"
