#!/usr/bin/env bash
set -euo pipefail

run_checks=0
base_ref="HEAD"

while [ "$#" -gt 0 ]; do
  case "$1" in
    --run) run_checks=1; shift ;;
    -h|--help)
      cat <<'USAGE'
Usage: ./scripts/check-agent-tuning.sh [--run] [base-ref]

Prints the prompt/tool/policy/eval checks required by changed files.
With --run, executes the local preflight gate using existing tests and prompt-regression.
USAGE
      exit 0
      ;;
    *) base_ref="$1"; shift ;;
  esac
done

changed="$(
  {
    git diff --name-only "$base_ref" -- || true
    git ls-files --others --exclude-standard || true
  } | sort -u
)"

need_prompt=0
need_policy=0
need_eval=0
need_tool=0

if printf '%s\n' "$changed" | grep -q '^internal/promptcompiler/'; then
  need_prompt=1
fi
if printf '%s\n' "$changed" | grep -q '^internal/policyengine/\|internal/promptcompiler/runtime_policy_prompt.go'; then
  need_policy=1
fi
if printf '%s\n' "$changed" | grep -q '^testdata/eval_cases/\|^internal/eval/\|^cmd/agent-eval\|^internal/promptdiag/\|^cmd/prompt-diagnose'; then
  need_eval=1
fi
if printf '%s\n' "$changed" | grep -q '^internal/tooling/\|tool_registry.go'; then
  need_tool=1
fi

echo "Agent tuning check against ${base_ref}"
echo

if [ "$need_prompt" -eq 1 ]; then
  echo "- Prompt changed: run go test ./internal/promptcompiler -count=1"
  echo "- Prompt changed: run go run ./cmd/agent-eval -agent mock -priority P0 -cases testdata/eval_cases -out .data/eval_runs/prompt-p0-mock"
fi

if [ "$need_policy" -eq 1 ]; then
  echo "- Policy prompt or policy engine changed: run approval/policy eval cases before merging"
fi

if [ "$need_tool" -eq 1 ]; then
  echo "- Tool surface changed: run tool-calling eval cases before merging"
fi

if [ "$need_eval" -eq 1 ]; then
  echo "- Eval/diagnosis changed: run go test ./internal/eval ./cmd/agent-eval ./internal/promptdiag ./cmd/prompt-diagnose -count=1"
  echo "- Eval/diagnosis changed: run ./scripts/prompt-regression_test.sh"
fi

if [ "$need_prompt" -eq 0 ] && [ "$need_policy" -eq 0 ] && [ "$need_eval" -eq 0 ] && [ "$need_tool" -eq 0 ]; then
  echo "- No prompt/tool/policy/eval files changed."
fi

if [ "$run_checks" -eq 1 ]; then
  echo
  echo "Running local preflight gate..."
  go test ./internal/eval ./cmd/agent-eval ./cmd/agent-eval-case ./internal/promptdiag ./cmd/prompt-diagnose -count=1
  ./scripts/prompt-regression_test.sh
  if [ "$need_prompt" -eq 1 ]; then
    go test ./internal/promptcompiler -count=1
  fi
  if [ "$need_prompt" -eq 1 ] || [ "$need_eval" -eq 1 ] || [ "$need_tool" -eq 1 ] || [ "$need_policy" -eq 1 ]; then
    ./scripts/prompt-regression.sh \
      --agent mock \
      --priority P0 \
      --cases testdata/eval_cases \
      --trace-dir .data/model-input-traces \
      --out .data/prompt_optimization/preflight-$(date -u +%Y%m%dT%H%M%SZ) \
      --fail-on-regression
  fi
fi
