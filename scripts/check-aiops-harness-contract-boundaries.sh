#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

fail=0

if [[ -n "${AIOPS_HARNESS_SCAN_ROOTS:-}" ]]; then
  IFS=':' read -r -a scan_roots <<< "${AIOPS_HARNESS_SCAN_ROOTS}"
else
  scan_roots=("${REPO_ROOT}")
fi

run_rg() {
  local pattern="$1"
  shift

  local -a scan_paths=()
  local root
  local path
  for root in "${scan_roots[@]}"; do
    for path in "$@"; do
      scan_paths+=("${root%/}/${path}")
    done
  done

  rg -n -P "$pattern" "${scan_paths[@]}" \
    --glob '!**/*_test.go' \
    --glob '!**/*.test.ts' \
    --glob '!**/*.test.tsx' \
    --glob '!web/src/pages/debug/**' \
    --glob '!web/src/pages/PromptTracePage.tsx' \
    --glob '!scripts/check-aiops-harness-contract-boundaries.sh'
}

check_absent() {
  local name="$1"
  local owner="$2"
  local pattern="$3"
  shift 3

  local matches
  if matches="$(run_rg "$pattern" "$@" 2>/dev/null)"; then
    echo "ERROR: forbidden harness boundary pattern found: ${name}" >&2
    echo "owner: ${owner}" >&2
    echo "${matches}" >&2
    fail=1
  fi
}

check_required() {
  local name="$1"
  local owner="$2"
  local pattern="$3"
  shift 3

  if ! run_rg "$pattern" "$@" >/dev/null 2>&1; then
    echo "ERROR: required harness boundary pattern missing: ${name}" >&2
    echo "owner: ${owner}" >&2
    echo "searched: $*" >&2
    fail=1
  fi
}

check_absent \
  "markdown-derived runtime process or status" \
  "frontend transport/appui" \
  '(?i)((parse|extract|infer|derive)[A-Za-z0-9_]*(process|tool|approval|verified|status)[A-Za-z0-9_]*(markdown|finalText|final_markdown)|(markdown|finalText|final_markdown)[A-Za-z0-9_]*(parse|extract|infer|derive)[A-Za-z0-9_]*(process|tool|approval|verified|status))' \
  web/src/chat web/src/transport internal/appui

check_absent \
  "synthetic completed HostOps success path" \
  "appui hostops/runtime projection" \
  '(?i)(hostops|host_ops|HostOps).{0,120}(Status:\s*"completed"|Status:\s*AiopsTransportTurnStatusCompleted|status\s*=\s*"completed"|synthetic.*success|success.*synthetic)' \
  internal/appui

check_absent \
  "direct tool execution bypassing ToolDispatcher" \
  "runtimekernel dispatcher" \
  '(?i)(RunToolDirect|ExecuteToolDirect|DirectToolExecution|toolRegistry\.Execute|registry\.ExecuteTool|lookupTool\([^)]*\)\.Execute)' \
  internal/runtimekernel internal/appui

check_absent \
  "approval decision fallback RunTurn" \
  "appui approval service" \
  '\.RunTurn\(' \
  internal/appui/approval_service.go

check_required \
  "approval decisions resume the existing turn" \
  "appui approval service" \
  'ResumeTurn\(' \
  internal/appui/approval_service.go

check_absent \
  "UI verified state inferred from final markdown" \
  "frontend final projection" \
  '(?i)(finalText|markdown|MessageMarkdown|parseAnswerSections).{0,120}("verified"|"已验证"|verified)' \
  web/src/chat web/src/transport

if [[ "${fail}" -ne 0 ]]; then
  exit 1
fi
