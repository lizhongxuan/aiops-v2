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
  local -a rg_args=(-n -P)
  local root
  local path
  if [[ "${AIOPS_HARNESS_MULTILINE_SCAN:-0}" == "1" ]]; then
    rg_args+=(-U)
  fi
  for root in "${scan_roots[@]}"; do
    for path in "$@"; do
      scan_paths+=("${root%/}/${path}")
    done
  done

  rg "${rg_args[@]}" "$pattern" "${scan_paths[@]}" \
    --glob '!**/*_test.go' \
    --glob '!**/*.test.ts' \
    --glob '!**/*.test.tsx' \
    --glob '!web/src/pages/debug/**' \
    --glob '!web/src/pages/PromptTracePage.tsx' \
    --glob '!scripts/check-aiops-harness-contract-boundaries.sh'
}

run_rg_multiline() {
  AIOPS_HARNESS_MULTILINE_SCAN=1 run_rg "$@"
}

check_absent() {
  local name="$1"
  local owner="$2"
  local pattern="$3"
  shift 3

  local matches
  local rc
  if matches="$(run_rg "$pattern" "$@" 2>&1)"; then
    rc=0
  else
    rc=$?
  fi

  case "${rc}" in
    0)
      echo "ERROR: forbidden harness boundary pattern found: ${name}" >&2
      echo "owner: ${owner}" >&2
      echo "${matches}" >&2
      fail=1
      ;;
    1)
      ;;
    *)
      echo "ERROR: harness boundary scan failed: ${name}" >&2
      echo "owner: ${owner}" >&2
      echo "rg exit code: ${rc}" >&2
      echo "${matches}" >&2
      fail=1
      ;;
  esac
}

check_absent_multiline() {
  local name="$1"
  local owner="$2"
  local pattern="$3"
  shift 3

  local matches
  local rc
  if matches="$(run_rg_multiline "$pattern" "$@" 2>&1)"; then
    rc=0
  else
    rc=$?
  fi

  case "${rc}" in
    0)
      echo "ERROR: forbidden harness boundary pattern found: ${name}" >&2
      echo "owner: ${owner}" >&2
      echo "${matches}" >&2
      fail=1
      ;;
    1)
      ;;
    *)
      echo "ERROR: harness boundary scan failed: ${name}" >&2
      echo "owner: ${owner}" >&2
      echo "rg exit code: ${rc}" >&2
      echo "${matches}" >&2
      fail=1
      ;;
  esac
}

check_required() {
  local name="$1"
  local owner="$2"
  local pattern="$3"
  shift 3

  local matches
  local rc
  if matches="$(run_rg "$pattern" "$@" 2>&1)"; then
    rc=0
  else
    rc=$?
  fi

  case "${rc}" in
    0)
      ;;
    1)
      echo "ERROR: required harness boundary pattern missing: ${name}" >&2
      echo "owner: ${owner}" >&2
      echo "searched: $*" >&2
      fail=1
      ;;
    *)
      echo "ERROR: harness boundary scan failed: ${name}" >&2
      echo "owner: ${owner}" >&2
      echo "rg exit code: ${rc}" >&2
      echo "${matches}" >&2
      fail=1
      ;;
  esac
}

check_absent \
	"agent eval legacy state endpoint" \
	"eval AssistantTransport adapter" \
	'/api/v1/state' \
	internal/eval cmd/agent-eval

check_absent \
	"control state derived from final text or markdown" \
	"runtime/appui/web typed control facts" \
	'(?i)^\s*if\s*(?:\(\s*)?(?:strings\.Contains\(\s*(finalText|FinalOutput|assistantText|markdown)\s*,\s*"[^"]*(approved|approval|blocked|completed|failed|verified|pending|denied|success|error|running)|(finalText|FinalOutput|assistantText|markdown)\.(includes|match|test)\(\s*"[^"]*(approved|approval|blocked|completed|failed|verified|pending|denied|success|error|running))' \
	internal/runtimekernel internal/appui web/src

check_absent_multiline \
	"control state derived from final text or markdown" \
	"runtime/appui/web typed control facts" \
	'(?msx)^\h*(?:const\h+|let\h+|var\h+)?(?<control_alias>[A-Za-z_][A-Za-z0-9_]*)\h*(?::=|=)\h*(?:strings\.TrimSpace\(\h*)?(?:finalText|assistantText|markdown|(?:[A-Za-z_][A-Za-z0-9_]*\.)*FinalOutput)\h*\)?\h*;?\h*$(?:(?:\R)(?!\h*(?://|/\*|\*)).*){0,20}?\R\h*if\h*(?:\(\h*)?(?:strings\.Contains\(\h*\k<control_alias>\h*,\h*"[^"]*(?:approved|approval|blocked|completed|failed|verified|pending|denied|success|error|running)|\k<control_alias>\.(?:includes|match|test)\(\h*"[^"]*(?:approved|approval|blocked|completed|failed|verified|pending|denied|success|error|running))' \
	internal/runtimekernel internal/appui web/src

check_required \
	"TurnAssembly before prompt production marker" \
	"runtimekernel turn admission" \
	'(?m)^\s*k\.observeRuntimeStage\([^\n]*"turn_assembly_built"\)\s*$' \
	internal/runtimekernel/runtime_kernel.go

check_required \
	"StepToolRouter provider request wiring" \
	"runtimekernel step builder" \
	'(?m)^\s*Tools:\s+providerToolSpecsFromRuntimeToolSurface\(toolSurface\),?\s*$' \
	internal/runtimekernel/step_builder.go

check_required \
	"StepToolRouter provider surface adapter" \
	"runtimekernel step builder" \
	'(?m)^\s*return\s+providerToolSpecsFromStepToolRouter\(surface\)\s*$' \
	internal/runtimekernel/step_builder.go

check_required \
	"StepToolRouter dispatcher binding marker" \
	"runtimekernel dispatcher" \
	'(?m)^\s*WithStepToolRouter\(runtimeToolSurface\)\.\s*$' \
	internal/runtimekernel/runtime_kernel.go

check_required \
	"model input L0/L1 first validator" \
	"promptinput model input validator" \
	'model input must begin with L0 then L1' \
	internal/promptinput/model_input_validation.go

check_required \
	"model input L6 last validator" \
	"promptinput model input validator" \
	'model input L6 must be last' \
	internal/promptinput/model_input_validation.go

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
