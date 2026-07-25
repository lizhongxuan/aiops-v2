#!/usr/bin/env bash
set -euo pipefail

profile="${1:-}"
browsers="${2:-}"
case "${profile}" in
  pr|nightly|rc) ;;
  *) echo "usage: $0 <pr|nightly|rc> [browser-list]" >&2; exit 2 ;;
esac

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
manifest="${AIOPS_WORKFLOW_CASE_MANIFEST:-${repo_root}/testdata/workflow_canvas_mcp/test-manifest.json}"
evidence_root="${AIOPS_WORKFLOW_EVIDENCE_DIR:-${RUNNER_TEMP:-${TMPDIR:-/tmp}}/workflow-canvas-evidence}"
output="${evidence_root}/environment.json"
commit_sha="${GITHUB_SHA:-$(git -C "${repo_root}" rev-parse HEAD)}"
runner_os="${RUNNER_OS:-$(uname -s)}"
runner_arch="${RUNNER_ARCH:-$(uname -m)}"

mkdir -p "${evidence_root}"
node --input-type=module - \
  "${manifest}" \
  "${output}" \
  "${profile}" \
  "${browsers}" \
  "${commit_sha}" \
  "${runner_os}" \
  "${runner_arch}" <<'NODE'
import crypto from "node:crypto";
import fs from "node:fs";

const [manifestPath, outputPath, profile, browser, commitSHA, runnerOS, arch] = process.argv.slice(2);
const raw = fs.readFileSync(manifestPath);
const manifest = JSON.parse(raw.toString("utf8"));
const environment = {
  schemaVersion: "aiops.workflow-canvas.ci-environment/v1",
  profile,
  runnerOS,
  arch,
  commitSHA,
  buildSHA: commitSHA,
  activePhase: manifest.currentActivationPhase,
  capabilityTags: Array.isArray(manifest.approvedCapabilityTags) ? manifest.approvedCapabilityTags : [],
  manifestDigest: `sha256:${crypto.createHash("sha256").update(raw).digest("hex")}`,
  startedAt: new Date().toISOString(),
  counts: { manifestCases: Array.isArray(manifest.cases) ? manifest.cases.length : 0 },
};
if (browser) environment.browser = browser;
fs.writeFileSync(outputPath, `${JSON.stringify(environment, null, 2)}\n`, { mode: 0o600 });
NODE

echo "workflow canvas ${profile} evidence environment: ${output}"
