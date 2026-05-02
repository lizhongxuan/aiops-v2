#!/usr/bin/env bash
set -euo pipefail

base_ref="${1:-HEAD}"

changed="$(git diff --name-only "$base_ref" -- || true)"

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
if printf '%s\n' "$changed" | grep -q '^testdata/eval_cases/\|^internal/eval/\|^cmd/agent-eval'; then
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
  echo "- Eval changed: run go test ./internal/eval ./cmd/agent-eval -count=1"
fi

if [ "$need_prompt" -eq 0 ] && [ "$need_policy" -eq 0 ] && [ "$need_eval" -eq 0 ] && [ "$need_tool" -eq 0 ]; then
  echo "- No prompt/tool/policy/eval files changed."
fi
