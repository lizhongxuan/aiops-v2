#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "usage: $0 recorded|live <out-dir>" >&2
  exit 2
}

[[ $# -eq 2 ]] || usage
mode="$1"
out_dir="$2"
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cases_dir="${repo_root}/testdata/eval_cases/workflow_canvas"
baseline_path="${AIOPS_WORKFLOW_EVAL_BASELINE:-}"
require_baseline="${AIOPS_WORKFLOW_REQUIRE_BASELINE:-0}"
max_regressions="${AIOPS_WORKFLOW_MAX_BASELINE_REGRESSIONS:-0}"

case "${require_baseline}" in
  0|1) ;;
  *) echo "AIOPS_WORKFLOW_REQUIRE_BASELINE must be 0 or 1" >&2; exit 2 ;;
esac
[[ "${max_regressions}" =~ ^[0-9]+$ ]] || {
  echo "AIOPS_WORKFLOW_MAX_BASELINE_REGRESSIONS must be a non-negative integer" >&2
  exit 2
}

case "${mode}" in
  recorded)
    agent="mock"
    repetitions=1
    ;;
  live)
    : "${AIOPS_LIVE_MODEL_BASE_URL:?AIOPS_LIVE_MODEL_BASE_URL is required}"
    : "${AIOPS_LIVE_MODEL_API_KEY:?AIOPS_LIVE_MODEL_API_KEY is required}"
    : "${AIOPS_LIVE_MODEL_MODEL:?AIOPS_LIVE_MODEL_MODEL is required}"
    agent="live-model"
    repetitions="${AIOPS_LIVE_MODEL_REPETITIONS:-3}"
    [[ "${repetitions}" =~ ^[0-9]+$ ]] && (( repetitions >= 3 )) || {
      echo "AIOPS_LIVE_MODEL_REPETITIONS must be an integer >= 3" >&2
      exit 2
    }
    if [[ "${require_baseline}" == "1" && -z "${baseline_path}" ]]; then
      echo "AIOPS_WORKFLOW_EVAL_BASELINE is required for this live release evaluation" >&2
      exit 2
    fi
    ;;
  *) usage ;;
esac

mkdir -p "${out_dir}"
cd "${repo_root}"

go test ./internal/eval -run 'TestWorkflowScorer|GEN-' -count=1
eval_args=(
  -agent "${agent}"
  -workflow-mode "${mode}"
  -repetitions "${repetitions}"
  -cases "${cases_dir}"
  -out "${out_dir}"
)
if [[ -n "${baseline_path}" ]]; then
  [[ -f "${baseline_path}" ]] || {
    echo "workflow eval baseline does not exist: ${baseline_path}" >&2
    exit 2
  }
  eval_args+=( -baseline "${baseline_path}" )
fi

set +e
go run ./cmd/agent-eval "${eval_args[@]}"
eval_status=$?
set -e

if [[ "${eval_status}" -eq 0 ]]; then
  set +e
  node --input-type=module - \
    "${out_dir}/report.json" \
    "${mode}" \
    "${repetitions}" \
    "${require_baseline}" \
    "${max_regressions}" \
    "${baseline_path}" <<'NODE'
import fs from "node:fs";

const [reportPath, expectedMode, expectedRepetitionsRaw, requireBaselineRaw, maxRegressionsRaw, baselinePath] = process.argv.slice(2);
const report = JSON.parse(fs.readFileSync(reportPath, "utf8"));
const generation = report.workflowGeneration;
const expectedRepetitions = Number(expectedRepetitionsRaw);
const requireBaseline = requireBaselineRaw === "1";
const maxRegressions = Number(maxRegressionsRaw);

if (!generation || generation.mode !== expectedMode || generation.repetitions !== expectedRepetitions) {
  throw new Error("workflow generation report mode/repetition metadata is incomplete");
}
if (generation.gate?.passed !== true) {
  throw new Error("workflow generation quality gate did not pass");
}
if (expectedMode === "live" && expectedRepetitions < 3) {
  throw new Error("live workflow generation report has fewer than three repetitions");
}
if (baselinePath) {
  const baseline = JSON.parse(fs.readFileSync(baselinePath, "utf8"));
  if (baseline.workflowGeneration?.mode !== "recorded" || baseline.workflowGeneration?.gate?.passed !== true) {
    throw new Error("release baseline must be a passing recorded workflow generation report");
  }
}
if (requireBaseline) {
  const summary = report.baselineComparison?.summary;
  if (!summary) throw new Error("required workflow generation baseline comparison is missing");
  if (summary.worse > maxRegressions || summary.missing !== 0 || summary.new !== 0) {
    throw new Error(`workflow generation baseline regressed: worse=${summary.worse} missing=${summary.missing} new=${summary.new}`);
  }
}
NODE
  validation_status=$?
  set -e
  if [[ "${validation_status}" -ne 0 ]]; then
    eval_status="${validation_status}"
  fi
fi

scripts/check-workflow-evidence-redaction.sh "${out_dir}"
echo "workflow generation ${mode} report: ${out_dir}/report.json"
exit "${eval_status}"
