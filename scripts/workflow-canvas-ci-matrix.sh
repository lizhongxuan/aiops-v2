#!/usr/bin/env bash
set -euo pipefail

profile="${1:-nightly}"
case "${profile}" in
  nightly|rc) ;;
  *) echo "usage: $0 <nightly|rc>" >&2; exit 2 ;;
esac

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
evidence_root="${AIOPS_WORKFLOW_EVIDENCE_DIR:-${RUNNER_TEMP:-${TMPDIR:-/tmp}}/workflow-canvas-evidence}"
matrix_dir="${evidence_root}/recorded-matrix"
mkdir -p "${matrix_dir}" "${evidence_root}/performance"
cd "${repo_root}"

run_logged() {
  local name="$1"
  shift
  "$@" 2>&1 | tee "${matrix_dir}/${name}.log"
}

scripts/workflow-canvas-ci-evidence.sh "${profile}" "chromium,firefox,webkit"
run_logged case-gate scripts/workflow-canvas-test-gate.sh "${profile}"
run_logged go-control-plane go test \
  ./internal/integrations/workfloweditor \
  ./internal/mcp/runtime \
  ./internal/server \
  ./internal/eval \
  -count=1
run_logged go-runner-full bash -c \
  'cd pkg/runner && go test ./workflowstore ./workflow/edit ./server/service ./server/app -count=1'
run_logged go-recovery-race bash -c \
  'cd pkg/runner && go test -race ./workflowstore ./workflow/edit ./server/app -count=1 && go test -race ./server/service -run "TestWorkflow(CommitRecovery|CommitFailpoint|CommitStoreTwoInstancesCAS|CommitStorePersistsCompleteSnapshotAndReceiptAcrossRestart|EventLogAppendReplayGapAndRestart|EventRecoveryReplaysCommittedWALHistoryAfterRestart|OutboxRetryAckAndRestartIsSeparateFromEventLog|CandidateFileStoreRestartRecoveryEligibilityAndAbandon|CandidateTransactionRecoversManifestAndEventProjection|ServiceCommitStoreIsSoleWritePathAndSurvivesRestart)" -count=1'
run_logged web-unit bash -c 'cd web && npx vitest run'
run_logged web-typecheck bash -c 'cd web && npm run typecheck'
run_logged web-build bash -c 'cd web && npm run build'
run_logged full-stack-cross-browser env \
  AIOPS_WORKFLOW_E2E_PROJECTS=chromium,firefox,webkit \
  PLAYWRIGHT_RETRIES=0 \
  scripts/run-workflow-canvas-e2e.sh
run_logged generation-recorded scripts/workflow-generation-eval.sh \
  recorded "${evidence_root}/generation-recorded"

node --input-type=module - \
  "${evidence_root}/generation-recorded/report.json" \
  "${evidence_root}/performance/workflow-generation.json" <<'NODE'
import fs from "node:fs";

const [reportPath, outputPath] = process.argv.slice(2);
const report = JSON.parse(fs.readFileSync(reportPath, "utf8"));
const performanceCase = report.workflowGeneration?.cases?.find((entry) => entry.caseId === "PERF-005");
if (!performanceCase || performanceCase.passed !== true) {
  throw new Error("recorded matrix did not pass required PERF-005");
}
const summary = {
  schemaVersion: "aiops.workflow-canvas.performance-evidence/v1",
  caseId: performanceCase.caseId,
  passed: performanceCase.passed,
  meanScore: performanceCase.meanScore,
  scoreVariance: performanceCase.scoreVariance,
  attemptScores: performanceCase.attemptScores,
};
fs.writeFileSync(outputPath, `${JSON.stringify(summary, null, 2)}\n`, { mode: 0o600 });
NODE

scripts/check-workflow-evidence-redaction.sh "${evidence_root}"
echo "workflow canvas ${profile} recorded matrix evidence: ${evidence_root}"
