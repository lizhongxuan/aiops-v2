#!/usr/bin/env bash
set -u -o pipefail

agent="mock"
server_url="http://127.0.0.1:8080"
poll_timeout="2m"
poll_interval="1s"
out_root=".data/self-optimization-lab"
core_cases="testdata/eval_cases"
synthetic_cases="testdata/self_optimization/eval_cases"
priority=""
loop=0
max_runs=1
interval_seconds=3600
duration_hours=0
run_go_tests=1
run_core=1
run_synthetic=1
llm_suggestions=0
fail_fast=0
fail_on_regression=0
standalone=0
dashboard=1
asset_draft=1
allow_real_llm=0
allow_remote_host=0

usage() {
  cat <<'USAGE'
Usage: ./scripts/self-optimization-lab.sh [options]

Runs a safe, repeatable AIOps self-optimization lab. The default mode is
offline-only: deterministic Go tests, ops-manual retrieval golden cases,
memory/learning tests, and prompt regression against mock agent cases.

Options:
  --agent mock|server              Agent adapter for prompt-regression.
  --server-url URL                 Base URL for --agent server.
  --poll-timeout DURATION          Server eval poll timeout.
  --poll-interval DURATION         Server eval poll interval.
  --out DIR                        Output root. Default .data/self-optimization-lab.
  --core-cases DIR                 Core eval cases. Default testdata/eval_cases.
  --synthetic-cases DIR            Synthetic user journey cases.
  --priority P0|P1|P2              Optional eval priority filter.
  --loop                           Repeat until --max-runs or --duration-hours.
  --interval-seconds N             Sleep between loop runs. Default 3600.
  --max-runs N                     Max loop runs. 0 means unlimited while duration allows.
  --duration-hours N               Stop loop after N hours.
  --skip-go-tests                  Skip deterministic Go safety/retrieval tests.
  --skip-core                      Skip core prompt regression.
  --skip-synthetic                 Skip synthetic user journey prompt regression.
  --llm-suggestions                Ask prompt-diagnose for summary-only LLM suggestions.
                                    Requires --allow-real-llm and AIOPS_LAB_LLM_*.
  --fail-on-regression             Fail a run when prompt-diagnose detects regression.
  --fail-fast                      Stop the current run on first failing step.
  --standalone                     Run the standalone selfopt subsystem instead of the legacy lab flow.
  --no-dashboard                   Standalone mode: skip static dashboard generation.
  --no-asset-draft                 Standalone mode: skip candidate asset draft generation.
  --allow-real-llm                 Allow explicit real LLM configuration for suggestions.
  --allow-remote-host              Standalone mode: allow explicit remote host configuration.
  -h, --help                       Show this help.

Examples:
  ./scripts/self-optimization-lab.sh
  ./scripts/self-optimization-lab.sh --loop --duration-hours 24 --interval-seconds 1800
  ./scripts/self-optimization-lab.sh --agent server --server-url http://127.0.0.1:18080 --loop --duration-hours 24
USAGE
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --agent) agent="${2:?missing --agent value}"; shift 2 ;;
    --server-url) server_url="${2:?missing --server-url value}"; shift 2 ;;
    --poll-timeout) poll_timeout="${2:?missing --poll-timeout value}"; shift 2 ;;
    --poll-interval) poll_interval="${2:?missing --poll-interval value}"; shift 2 ;;
    --out) out_root="${2:?missing --out value}"; shift 2 ;;
    --core-cases) core_cases="${2:?missing --core-cases value}"; shift 2 ;;
    --synthetic-cases) synthetic_cases="${2:?missing --synthetic-cases value}"; shift 2 ;;
    --priority) priority="${2:?missing --priority value}"; shift 2 ;;
    --loop) loop=1; max_runs=0; shift ;;
    --interval-seconds) interval_seconds="${2:?missing --interval-seconds value}"; shift 2 ;;
    --max-runs) max_runs="${2:?missing --max-runs value}"; shift 2 ;;
    --duration-hours) duration_hours="${2:?missing --duration-hours value}"; shift 2 ;;
    --skip-go-tests) run_go_tests=0; shift ;;
    --skip-core) run_core=0; shift ;;
    --skip-synthetic) run_synthetic=0; shift ;;
    --llm-suggestions) llm_suggestions=1; shift ;;
    --fail-on-regression) fail_on_regression=1; shift ;;
    --fail-fast) fail_fast=1; shift ;;
    --standalone) standalone=1; shift ;;
    --no-dashboard) dashboard=0; shift ;;
    --no-asset-draft) asset_draft=0; shift ;;
    --allow-real-llm) allow_real_llm=1; shift ;;
    --allow-remote-host) allow_remote_host=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "self-optimization-lab: unknown argument $1" >&2; exit 2 ;;
  esac
done

if ! [[ "$interval_seconds" =~ ^[0-9]+$ ]]; then
  echo "self-optimization-lab: --interval-seconds must be an integer" >&2
  exit 2
fi
if ! [[ "$max_runs" =~ ^[0-9]+$ ]]; then
  echo "self-optimization-lab: --max-runs must be an integer" >&2
  exit 2
fi
if ! [[ "$duration_hours" =~ ^[0-9]+$ ]]; then
  echo "self-optimization-lab: --duration-hours must be an integer" >&2
  exit 2
fi
if [ "$llm_suggestions" -eq 1 ]; then
  if [ "$allow_real_llm" -ne 1 ]; then
    echo "self-optimization-lab: --llm-suggestions requires --allow-real-llm and AIOPS_LAB_LLM_*" >&2
    exit 2
  fi
  if [ -z "${AIOPS_LAB_LLM_BASE_URL:-}" ] || [ -z "${AIOPS_LAB_LLM_API_KEY:-}" ] || [ -z "${AIOPS_LAB_LLM_MODEL:-}" ]; then
    echo "self-optimization-lab: --llm-suggestions requires AIOPS_LAB_LLM_BASE_URL, AIOPS_LAB_LLM_API_KEY, and AIOPS_LAB_LLM_MODEL" >&2
    exit 2
  fi
fi

mkdir -p "$out_root/baselines"
history_path="$out_root/history.json"
start_epoch="$(date -u +%s)"
end_epoch=0
if [ "$duration_hours" -gt 0 ]; then
  end_epoch=$((start_epoch + duration_hours * 3600))
fi

standalone_changed_files() {
  if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    {
      git diff --name-only --no-ext-diff
      git diff --cached --name-only --no-ext-diff
      git ls-files --others --exclude-standard
    } 2>/dev/null | awk 'NF' | sort -u | paste -sd, -
  fi
}

run_standalone_once() {
  local seq="$1"
  local stamp
  stamp="$(date -u +%Y%m%dT%H%M%SZ)"
  local run_id="self-opt-${stamp}-${seq}"
  local cmd=(
    go run ./selfopt/cmd/selfopt
    --run-id "$run_id"
    --cases "$synthetic_cases"
    --out "$out_root"
    --changed "$(standalone_changed_files)"
  )
  if [ "$dashboard" -eq 1 ]; then
    cmd+=(--dashboard)
  fi
  if [ "$asset_draft" -eq 1 ]; then
    cmd+=(--asset-draft)
  fi
  if [ "$llm_suggestions" -eq 1 ]; then
    cmd+=(--llm-suggestions)
  fi
  if [ "$allow_real_llm" -eq 1 ]; then
    cmd+=(--allow-real-llm)
  fi
  if [ "$allow_remote_host" -eq 1 ]; then
    cmd+=(--allow-remote-host)
  fi
  if ! "${cmd[@]}"; then
    return 1
  fi
  if [ "$fail_on_regression" -eq 1 ]; then
    local latest
    latest="$(cat "$out_root/latest_run.txt" 2>/dev/null || true)"
    if [ -n "$latest" ] && command -v jq >/dev/null 2>&1; then
      local decision
      decision="$(jq -r '.gate.decision // "pass"' "$latest/scorecard.json" 2>/dev/null || echo pass)"
      [ "$decision" != "block" ] || return 1
    fi
  fi
  return 0
}

if [ "$standalone" -eq 1 ]; then
  run_count=0
  last_status=0
  while :; do
    run_count=$((run_count + 1))
    run_standalone_once "$run_count"
    last_status=$?
    if [ "$loop" -ne 1 ]; then
      exit "$last_status"
    fi
    if [ "$max_runs" -gt 0 ] && [ "$run_count" -ge "$max_runs" ]; then
      exit "$last_status"
    fi
    if [ "$end_epoch" -gt 0 ] && [ "$(date -u +%s)" -ge "$end_epoch" ]; then
      exit "$last_status"
    fi
    sleep "$interval_seconds"
  done
fi

step_statuses=()
run_dir=""
run_log=""
prompt_regression_cmd=()

append_step_status() {
  step_statuses+=("$1:$2")
}

run_step() {
  local name="$1"
  shift
  local log_path="$run_dir/${name}.log"
  printf '[%s] START %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$name" | tee -a "$run_log"
  "$@" >"$log_path" 2>&1
  local status=$?
  if [ "$status" -eq 0 ]; then
    printf '[%s] PASS  %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$name" | tee -a "$run_log"
    append_step_status "$name" "PASS"
  else
    printf '[%s] FAIL  %s status=%s log=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$name" "$status" "$log_path" | tee -a "$run_log"
    append_step_status "$name" "FAIL"
    if [ "$fail_fast" -eq 1 ]; then
      return "$status"
    fi
  fi
  return 0
}

build_prompt_regression_command() {
  local cases_dir="$1"
  local out_dir="$2"
  local run_id="$3"
  local baseline="$4"
  prompt_regression_cmd=(
    bash ./scripts/prompt-regression.sh
    --agent "$agent"
    --cases "$cases_dir"
    --out "$out_dir"
    --run-id "$run_id"
    --history "$history_path"
    --draft-failed-cases
  )
  if [ -n "$priority" ]; then
    prompt_regression_cmd+=(--priority "$priority")
  fi
  if [ -s "$baseline" ]; then
    prompt_regression_cmd+=(--baseline "$baseline")
  fi
  if [ "$agent" = "server" ]; then
    prompt_regression_cmd+=(--server-url "$server_url" --poll-timeout "$poll_timeout" --poll-interval "$poll_interval")
  fi
  if [ "$llm_suggestions" -eq 1 ]; then
    prompt_regression_cmd+=(--llm-suggestions)
  fi
  if [ "$fail_on_regression" -eq 1 ]; then
    prompt_regression_cmd+=(--fail-on-regression)
  fi
}

maybe_seed_baseline() {
  local report="$1"
  local baseline="$2"
  if [ -s "$baseline" ] || [ ! -s "$report" ]; then
    return 0
  fi
  cp "$report" "$baseline"
}

append_diag_section() {
  local title="$1"
  local diag_path="$2"
  local summary_path="$3"
  {
    printf '\n## %s\n\n' "$title"
    if [ -s "$diag_path" ] && command -v jq >/dev/null 2>&1; then
      jq -r '
        "- total=\(.summary.total) passed=\(.summary.passed) failed=\(.summary.failed) avgScore=\(.summary.avgScore)" ,
        (if (.summary.worse // 0) > 0 then "- regressions=\(.summary.worse)" else empty end),
        (if (.suggestions | length) > 0 then "\n### Suggestions\n" else empty end),
        (.suggestions[]? | "- `\(.area)`: \(.action)")
      ' "$diag_path"
      local failed
      failed="$(jq -r '.cases[]? | select(.passed == false or .movement == "worse") | "- `\(.caseId)`: score=\(.score) rootCause=\(.likelyRootCause) failed=\((.failedChecks // []) | join(","))"' "$diag_path")"
      if [ -n "$failed" ]; then
        printf '\n### Failed Or Worse Cases\n\n%s\n' "$failed"
      fi
    else
      printf -- '- diagnosis: `%s`\n' "$diag_path"
      printf -- '- install `jq` for inline case/suggestion summaries.\n'
    fi
  } >> "$summary_path"
}

write_run_summary() {
  local summary_path="$run_dir/summary.zh.md"
  {
    printf '# AIOps 自优化实验室 Run\n\n'
    printf -- '- Run ID: `%s`\n' "$1"
    printf -- '- Agent: `%s`\n' "$agent"
    printf -- '- Started: `%s`\n' "$2"
    printf -- '- Output: `%s`\n' "$run_dir"
    printf '\n## Step Status\n\n'
    for item in "${step_statuses[@]}"; do
      printf -- '- `%s`\n' "$item"
    done
    printf '\n## Key Artifacts\n\n'
    printf -- '- Core diagnosis: `%s`\n' "$run_dir/prompt-regression-core/diagnosis.zh.md"
    printf -- '- Synthetic diagnosis: `%s`\n' "$run_dir/prompt-regression-synthetic/diagnosis.zh.md"
    printf -- '- Improvement backlog: `%s`\n' "$run_dir/improvement-backlog.zh.md"
  } > "$summary_path"

  append_diag_section "Core Prompt Regression" "$run_dir/prompt-regression-core/diagnosis.json" "$summary_path"
  append_diag_section "Synthetic User Journeys" "$run_dir/prompt-regression-synthetic/diagnosis.json" "$summary_path"
}

write_improvement_backlog() {
  local backlog_path="$run_dir/improvement-backlog.zh.md"
  {
    printf '# 自优化 Backlog\n\n'
    printf '本文件由 self-optimization lab 生成，只记录改进候选，不自动修改 prompt、手册、Workflow 或策略。\n\n'
    printf '## 默认处理顺序\n\n'
    printf '1. 先处理安全回归：审批、ActionToken、高风险阻断、敏感信息泄漏。\n'
    printf '2. 再处理检索误召回：跨对象手册、缺失参数 direct_execute、错误 adapt。\n'
    printf '3. 再处理诊断质量：证据不足却高置信、工具失败被当健康、当前证据被旧记忆覆盖。\n'
    printf '4. 最后处理表达和 prompt 成本：回答过泛、trace 过大、无验证闭环。\n'
  } > "$backlog_path"

  if command -v jq >/dev/null 2>&1; then
    for diag in "$run_dir"/prompt-regression-*/diagnosis.json; do
      [ -s "$diag" ] || continue
      {
        printf '\n## %s\n\n' "$diag"
        jq -r '
          .cases[]?
          | select(.passed == false or .movement == "worse")
          | "### \(.caseId)\n- score: \(.score)\n- likelyRootCause: \(.likelyRootCause)\n- failedChecks: \((.failedChecks // []) | join(", "))\n- artifacts: \(.artifacts.answerPath // "-")\n"
        ' "$diag"
      } >> "$backlog_path"
    done
  fi
}

run_once() {
  local seq="$1"
  local stamp
  stamp="$(date -u +%Y%m%dT%H%M%SZ)"
  local run_id="self-opt-${stamp}-${seq}"
  run_dir="$out_root/$run_id"
  run_log="$run_dir/run.log"
  step_statuses=()
  mkdir -p "$run_dir"

  cat > "$run_dir/manifest.json" <<EOF
{
  "run_id": "$run_id",
  "agent": "$agent",
  "server_url": "$server_url",
  "core_cases": "$core_cases",
  "synthetic_cases": "$synthetic_cases",
  "priority": "$priority",
  "started_at": "$stamp",
  "safety_mode": "offline_by_default_no_prod_mutation"
}
EOF

  if [ "$run_go_tests" -eq 1 ]; then
    run_step "go-test-eval-memory-promptdiag" \
      go test ./internal/memory ./internal/eval ./cmd/agent-eval ./internal/promptdiag ./cmd/prompt-diagnose ./cmd/agent-eval-case -count=1 || return $?
    run_step "go-test-opsmanual-retrieval-learning" \
      go test ./internal/opsmanual -run 'TestHybridRetrievalGoldenCases|TestSearchOpsManuals|TestLearningSummary|TestManualCandidateValidator|TestGenerateCandidate' -count=1 || return $?
  fi

  local core_baseline="$out_root/baselines/core-report.json"
  if [ "$run_core" -eq 1 ]; then
    build_prompt_regression_command "$core_cases" "$run_dir/prompt-regression-core" "$run_id-core" "$core_baseline"
    run_step "prompt-regression-core" "${prompt_regression_cmd[@]}" || return $?
    maybe_seed_baseline "$run_dir/prompt-regression-core/eval/report.json" "$core_baseline"
  fi

  local synthetic_baseline="$out_root/baselines/synthetic-report.json"
  if [ "$run_synthetic" -eq 1 ]; then
    build_prompt_regression_command "$synthetic_cases" "$run_dir/prompt-regression-synthetic" "$run_id-synthetic" "$synthetic_baseline"
    run_step "prompt-regression-synthetic" "${prompt_regression_cmd[@]}" || return $?
    maybe_seed_baseline "$run_dir/prompt-regression-synthetic/eval/report.json" "$synthetic_baseline"
  fi

  write_improvement_backlog
  write_run_summary "$run_id" "$stamp"
  printf '%s\n' "$run_dir" > "$out_root/latest_run.txt"

  local failed=0
  for item in "${step_statuses[@]}"; do
    case "$item" in
      *:FAIL) failed=$((failed + 1)) ;;
    esac
  done
  if [ "$failed" -gt 0 ]; then
    return 1
  fi
  return 0
}

run_count=0
last_status=0
while :; do
  run_count=$((run_count + 1))
  run_once "$run_count"
  last_status=$?

  if [ "$loop" -ne 1 ]; then
    exit "$last_status"
  fi
  if [ "$max_runs" -gt 0 ] && [ "$run_count" -ge "$max_runs" ]; then
    exit "$last_status"
  fi
  if [ "$end_epoch" -gt 0 ] && [ "$(date -u +%s)" -ge "$end_epoch" ]; then
    exit "$last_status"
  fi
  sleep "$interval_seconds"
done
