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

if rg -n 'writeModelInputDebugTrace|ModelInputDebugTraceRequest|modeltrace\.Write\(' internal/runtimekernel internal/promptdiag internal/eval --glob '!**/*_test.go'; then
  fail "v1 trace production writer path remains"
fi

echo "PASS: single runtime path guards"
