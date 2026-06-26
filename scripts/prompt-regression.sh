#!/usr/bin/env bash
set -euo pipefail

agent="mock"
server_url="http://127.0.0.1:18080"
priority=""
cases="testdata/eval_cases"
trace_dir=".data/model-input-traces"
out=""
baseline=""
run_id=""
poll_timeout="2m"
poll_interval="1s"
fail_on_worse=0
fail_on_regression=0
draft_failed_cases=0
failed_from=""
worse_from=""
history_path=""
llm_suggestions=0
llm_base_url="${AIOPS_LAB_LLM_BASE_URL:-${AIOPS_LLM_BASE_URL:-}}"
llm_api_key="${AIOPS_LAB_LLM_API_KEY:-${AIOPS_LLM_API_KEY:-}}"
llm_model="${AIOPS_LAB_LLM_MODEL:-${AIOPS_LLM_MODEL:-}}"
case_ids=()
case_id_count=0
tmp_root=""

cleanup() {
  if [ -n "$tmp_root" ]; then
    rm -rf "$tmp_root"
  fi
}
trap cleanup EXIT

while [ "$#" -gt 0 ]; do
  case "$1" in
    --agent) agent="${2:?missing --agent value}"; shift 2 ;;
    --server-url) server_url="${2:?missing --server-url value}"; shift 2 ;;
    --priority) priority="${2:?missing --priority value}"; shift 2 ;;
    --cases) cases="${2:?missing --cases value}"; shift 2 ;;
    --trace-dir) trace_dir="${2:?missing --trace-dir value}"; shift 2 ;;
    --out) out="${2:?missing --out value}"; shift 2 ;;
    --baseline) baseline="${2:?missing --baseline value}"; shift 2 ;;
    --run-id) run_id="${2:?missing --run-id value}"; shift 2 ;;
    --poll-timeout) poll_timeout="${2:?missing --poll-timeout value}"; shift 2 ;;
    --poll-interval) poll_interval="${2:?missing --poll-interval value}"; shift 2 ;;
    --fail-on-worse) fail_on_worse=1; shift ;;
    --fail-on-regression) fail_on_regression=1; shift ;;
    --draft-failed-cases) draft_failed_cases=1; shift ;;
    --case-id) case_ids+=("${2:?missing --case-id value}"); case_id_count=$((case_id_count + 1)); shift 2 ;;
    --failed-from) failed_from="${2:?missing --failed-from value}"; shift 2 ;;
    --worse-from) worse_from="${2:?missing --worse-from value}"; shift 2 ;;
    --history) history_path="${2:?missing --history value}"; shift 2 ;;
    --llm-suggestions) llm_suggestions=1; shift ;;
    --llm-base-url) llm_base_url="${2:?missing --llm-base-url value}"; shift 2 ;;
    --llm-api-key) llm_api_key="${2:?missing --llm-api-key value}"; shift 2 ;;
    --llm-model) llm_model="${2:?missing --llm-model value}"; shift 2 ;;
    -h|--help)
      cat <<'USAGE'
Usage: ./scripts/prompt-regression.sh [options]

Runs existing cmd/agent-eval, then runs cmd/prompt-diagnose on the generated report.

Options:
  --agent mock|server
  --server-url URL
  --priority P0|P1|P2
  --cases DIR
  --trace-dir DIR
  --out DIR
  --baseline PATH
  --run-id ID
  --poll-timeout DURATION
  --poll-interval DURATION
  --fail-on-worse
  --fail-on-regression
  --draft-failed-cases
  --case-id ID              Run only one case id. Can be repeated.
  --failed-from REPORT      Run failed cases from an agent-eval report.json.
  --worse-from REPORT       Run worse cases from a report with baselineComparison.
  --history PATH            Append run metadata to this history.json.
  --llm-suggestions         Generate summary-only LLM suggestions.
  --llm-base-url URL        OpenAI-compatible base URL for --llm-suggestions.
  --llm-api-key KEY         API key for --llm-suggestions. Prefer AIOPS_LAB_LLM_API_KEY env.
  --llm-model MODEL         Model for --llm-suggestions.
USAGE
      exit 0
      ;;
    *) echo "prompt-regression: unknown argument $1" >&2; exit 2 ;;
  esac
done

if [ -z "$out" ]; then
  out=".data/prompt_optimization/run-$(date -u +%Y%m%dT%H%M%SZ)"
fi
if [ -z "$run_id" ]; then
  run_id="prompt-regression-$(date -u +%Y%m%dT%H%M%SZ)"
fi

if [ "$case_id_count" -gt 0 ] || [ -n "$failed_from" ] || [ -n "$worse_from" ]; then
  if ! command -v jq >/dev/null 2>&1; then
    echo "prompt-regression: jq is required for --case-id/--failed-from/--worse-from filtering" >&2
    exit 2
  fi
  tmp_root="$(mktemp -d)"
  selected_ids="$tmp_root/selected-case-ids.txt"
  matched_ids="$tmp_root/matched-case-ids.txt"
  tmp_cases="$tmp_root/cases"
  : > "$selected_ids"
  : > "$matched_ids"
  mkdir -p "$tmp_cases"
  if [ "$case_id_count" -gt 0 ]; then
    for id in "${case_ids[@]}"; do
      printf '%s\n' "$id" >> "$selected_ids"
    done
  fi
  if [ -n "$failed_from" ]; then
    jq -r '.cases[]? | select(.passed == false) | .caseId' "$failed_from" >> "$selected_ids"
  fi
  if [ -n "$worse_from" ]; then
    jq -r '.baselineComparison.cases[]? | select(.status == "worse") | .caseId' "$worse_from" >> "$selected_ids"
  fi
  sort -u "$selected_ids" -o "$selected_ids"
  if [ ! -s "$selected_ids" ]; then
    echo "prompt-regression: no target case ids selected" >&2
    exit 2
  fi
  while IFS= read -r file; do
    id="$(jq -r '.id // empty' "$file")"
    if [ -n "$id" ] && grep -Fxq "$id" "$selected_ids"; then
      cp "$file" "$tmp_cases/"
      printf '%s\n' "$id" >> "$matched_ids"
    fi
  done < <(find "$cases" -maxdepth 1 -type f -name '*.json' | sort)
  sort -u "$matched_ids" -o "$matched_ids"
  missing="$(comm -23 "$selected_ids" "$matched_ids" || true)"
  if [ -n "$missing" ]; then
    echo "prompt-regression: selected case ids not found in $cases:" >&2
    printf '%s\n' "$missing" >&2
    exit 2
  fi
  cases="$tmp_cases"
fi

eval_out="${out}/eval"
mkdir -p "$out"

bash scripts/check-no-core-string-semantics.sh
bash scripts/check-aiops-agent-runtime-context-single-path.sh
go test ./internal/promptcompiler -run 'TestPromptBaselineBudgetByProfile|Test.*NoCrossProfile|TestRuntimePolicyOnlyContainsDynamicState|TestToolSurfaceSummaryBudget' -count=1
go test ./internal/eval -run TestAgentRuntimeContextOptimizationTrace -count=1

eval_args=(
  go run ./cmd/agent-eval
  -agent "$agent"
  -cases "$cases"
  -out "$eval_out"
  -run-id "$run_id"
)
if [ -n "$priority" ]; then
  eval_args+=(-priority "$priority")
fi
if [ -n "$baseline" ]; then
  eval_args+=(-baseline "$baseline")
fi
if [ "$agent" = "server" ]; then
  eval_args+=(
    -server-url "$server_url"
    -poll-timeout "$poll_timeout"
    -poll-interval "$poll_interval"
  )
fi

"${eval_args[@]}"

diag_args=(
  go run ./cmd/prompt-diagnose
  -report "${eval_out}/report.json"
  -cases "$cases"
  -trace-dir "$trace_dir"
  -out "$out"
)
if [ -n "$history_path" ]; then
  diag_args+=(-history "$history_path")
fi
if [ -n "$baseline" ]; then
  diag_args+=(-baseline "$baseline")
fi
if [ "$fail_on_worse" -eq 1 ]; then
  diag_args+=(-fail-on-worse)
fi
if [ "$fail_on_regression" -eq 1 ]; then
  diag_args+=(-fail-on-regression)
fi
if [ "$draft_failed_cases" -eq 1 ]; then
  diag_args+=(-draft-cases-out "$out/draft-cases")
fi
if [ "$llm_suggestions" -eq 1 ]; then
  diag_args+=(-llm-suggestions)
  if [ -n "$llm_base_url" ]; then
    export AIOPS_LLM_BASE_URL="$llm_base_url"
  fi
  if [ -n "$llm_api_key" ]; then
    export AIOPS_LLM_API_KEY="$llm_api_key"
  fi
  if [ -n "$llm_model" ]; then
    export AIOPS_LLM_MODEL="$llm_model"
  fi
fi

"${diag_args[@]}"

mkdir -p .data/prompt_optimization
printf '%s\n' "$out" > .data/prompt_optimization/latest_run.txt

echo "prompt regression output: $out"
