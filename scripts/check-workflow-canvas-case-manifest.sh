#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
manifest="${AIOPS_WORKFLOW_CASE_MANIFEST:-${repo_root}/testdata/workflow_canvas_mcp/test-manifest.json}"
contract_results="${repo_root}/testdata/workflow_canvas_mcp/contract-results/phase0-known-failure-contract.json"

cd "${repo_root}"

validate_args=(validate --manifest "${manifest}")
if [[ -n "${AIOPS_WORKFLOW_MANIFEST_BASE:-}" ]]; then
  validate_args+=(--base "${AIOPS_WORKFLOW_MANIFEST_BASE}")
fi

go run ./cmd/workflow-test-manifest "${validate_args[@]}"
go run ./cmd/workflow-test-manifest gate \
  --manifest "${manifest}" \
  --results "${contract_results}"
go test ./internal/workflowtestmanifest -count=1
(cd pkg/runner && go test ./testsupport/workflowfixtures -count=1)
(cd web && npx vitest run \
  tests/workflowCanvasMcpManifest.test.ts \
  tests/helpers/workflowCanvasFixtures.test.js)
