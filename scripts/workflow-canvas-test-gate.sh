#!/usr/bin/env bash
set -euo pipefail

profile="${1:-pr}"
case "${profile}" in
  pr|nightly|rc|red-evidence) ;;
  *)
    echo "usage: $0 <pr|nightly|rc|red-evidence>" >&2
    exit 2
    ;;
esac

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
manifest="${AIOPS_WORKFLOW_CASE_MANIFEST:-${repo_root}/testdata/workflow_canvas_mcp/test-manifest.json}"
results="${AIOPS_WORKFLOW_CASE_RESULTS:-${repo_root}/testdata/workflow_canvas_mcp/contract-results/phase0-known-failure-contract.json}"
evidence_root="${AIOPS_WORKFLOW_EVIDENCE_DIR:-${RUNNER_TEMP:-${TMPDIR:-/tmp}}/workflow-canvas-evidence}"
run_dir="${evidence_root}/case-gate-${profile}"
mkdir -p "${run_dir}"

cd "${repo_root}"
validate_args=(validate --manifest "${manifest}")
if [[ -n "${AIOPS_WORKFLOW_MANIFEST_BASE:-}" ]]; then
  validate_args+=(--base "${AIOPS_WORKFLOW_MANIFEST_BASE}")
fi
go run ./cmd/workflow-test-manifest "${validate_args[@]}"

run_check() {
  local name="$1"
  shift
  ("$@") >"${run_dir}/${name}.log" 2>&1
}

run_check manifest-go go test ./internal/workflowtestmanifest -count=1 &
pid_manifest=$!
run_check runner-fixtures bash -c 'cd pkg/runner && go test ./testsupport/workflowfixtures -count=1' &
pid_runner=$!
run_check web-fixtures bash -c 'cd web && npx vitest run tests/workflowCanvasMcpManifest.test.ts tests/helpers/workflowCanvasFixtures.test.js' &
pid_web=$!

failed=0
for entry in "manifest-go:${pid_manifest}" "runner-fixtures:${pid_runner}" "web-fixtures:${pid_web}"; do
  name="${entry%%:*}"
  pid="${entry##*:}"
  if ! wait "${pid}"; then
    echo "${name} failed; see ${run_dir}/${name}.log" >&2
    failed=1
  fi
done
if [[ "${failed}" -ne 0 ]]; then
  exit 1
fi

go run ./cmd/workflow-test-manifest gate \
  --manifest "${manifest}" \
  --results "${results}" \
  --profile "${profile}" \
  --json-report "${run_dir}/case-gate.json" \
  --junit-report "${run_dir}/case-gate.junit.xml"

echo "workflow canvas ${profile} gate evidence: ${run_dir}"
