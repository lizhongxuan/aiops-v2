#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
evidence_root="${AIOPS_WORKFLOW_EVIDENCE_DIR:-${RUNNER_TEMP:-${TMPDIR:-/tmp}}/workflow-canvas-evidence}"
run_id="${AIOPS_WORKFLOW_E2E_RUN_ID:-$(date -u +%Y%m%dT%H%M%SZ)-$$}"
run_dir="${evidence_root}/full-stack-${run_id}"
data_dir="${run_dir}/data"
server_bin="${AIOPS_WORKFLOW_E2E_SERVER_BIN:-${run_dir}/bin/ai-server}"
mkdir -p "${run_dir}/bin" "${data_dir}" "${run_dir}/playwright"

free_port() {
  python3 - <<'PY'
import socket
sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
sock.bind(("127.0.0.1", 0))
print(sock.getsockname()[1])
sock.close()
PY
}

http_port="${AIOPS_WORKFLOW_E2E_HTTP_PORT:-$(free_port)}"
grpc_port="${AIOPS_WORKFLOW_E2E_GRPC_PORT:-$(free_port)}"
model_port="${AIOPS_WORKFLOW_E2E_MODEL_PORT:-$(free_port)}"
base_url="http://127.0.0.1:${http_port}"
model_origin="http://127.0.0.1:${model_port}"
model_pid=""
server_pid=""
server_log_file=""
rollout_stage="${AIOPS_WORKFLOW_E2E_ROLLOUT_STAGE:-internal_apply}"
requested_case_grep=""
requested_cases=()
if [[ -n "${AIOPS_WORKFLOW_E2E_CASES:-}" ]]; then
  IFS=',' read -r -a requested_cases <<<"${AIOPS_WORKFLOW_E2E_CASES}"
  requested_case_patterns=()
  for requested_case in "${requested_cases[@]}"; do
    requested_case="${requested_case//[[:space:]]/}"
    if [[ ! "${requested_case}" =~ ^(PW|HARNESS)-[0-9]{3}$ ]]; then
      echo "unsupported Workflow Canvas E2E Case id: ${requested_case}" >&2
      exit 2
    fi
    requested_case_patterns+=("\\[${requested_case}\\]")
  done
  requested_case_grep="$(IFS='|'; echo "${requested_case_patterns[*]}")"
fi
case "${rollout_stage}" in
  internal_apply)
    rollout_payload='{"workflow":{"rollout":{"schemaVersion":"aiops.workflow.rollout.v1","stage":"internal_apply","saltVersion":"workflow-canvas-e2e-v1","rolloutPercentage":100,"allowTenants":["runner-default"],"denyTenants":[],"features":{"workflow_domain_events":true,"workflow_plan_ir":false,"workflow_elk_layout":false,"workflow_edit_grants":false,"workflow_external_mcp":false,"workflow_mcp_apps":false}}}}'
    ;;
  disabled)
    if [[ "${#requested_cases[@]}" -ne 1 || "${requested_cases[0]//[[:space:]]/}" != "PW-026" ]]; then
      echo "disabled Workflow rollout is reserved for an isolated explicit PW-026 real-user Case" >&2
      exit 2
    fi
    rollout_payload='{"workflow":{"rollout":{"schemaVersion":"aiops.workflow.rollout.v1","stage":"disabled","saltVersion":"workflow-canvas-e2e-v1","rolloutPercentage":100,"allowTenants":["runner-default"],"denyTenants":[],"features":{"workflow_domain_events":false,"workflow_plan_ir":false,"workflow_elk_layout":false,"workflow_edit_grants":false,"workflow_external_mcp":false,"workflow_mcp_apps":false}}}}'
    ;;
  *)
    echo "unsupported Workflow Canvas E2E rollout stage: ${rollout_stage}" >&2
    exit 2
    ;;
esac

stop_process() {
  local process_name="$1"
  local process_id="$2"
  local attempts="${AIOPS_WORKFLOW_E2E_STOP_ATTEMPTS:-150}"
  local index

  [[ -n "${process_id}" ]] || return 0
  if kill -0 "${process_id}" 2>/dev/null; then
    kill -TERM "${process_id}" 2>/dev/null || true
    for ((index = 0; index < attempts; index += 1)); do
      if ! kill -0 "${process_id}" 2>/dev/null; then
        break
      fi
      # This is a bounded process-exit probe, not a blind restart delay.
      sleep 0.1
    done
    if kill -0 "${process_id}" 2>/dev/null; then
      echo "${process_name} did not stop gracefully; forcing termination" >&2
      kill -KILL "${process_id}" 2>/dev/null || true
    fi
  fi
  wait "${process_id}" 2>/dev/null || true
}

stop_ai_server() {
  stop_process "ai-server" "${server_pid}"
  server_pid=""
  server_log_file=""
}

cleanup() {
  stop_ai_server
  stop_process "recorded model server" "${model_pid}"
  model_pid=""
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

wait_for_http() {
  local url="$1"
  local process_id="$2"
  local log_file="$3"
  local attempts="${AIOPS_WORKFLOW_E2E_HEALTH_ATTEMPTS:-180}"
  local index
  for ((index = 0; index < attempts; index += 1)); do
    if curl --noproxy '*' --fail --silent --show-error --max-time 2 "${url}" >/dev/null 2>&1; then
      return 0
    fi
    if ! kill -0 "${process_id}" 2>/dev/null; then
      echo "process exited before health check: ${url}" >&2
      tail -80 "${log_file}" >&2 || true
      return 1
    fi
    sleep 1
  done
  echo "health check did not become ready: ${url}" >&2
  tail -80 "${log_file}" >&2 || true
  return 1
}

start_ai_server() {
  local log_file="$1"
  local start_skip_web_build="$2"
  local start_skip_go_build="$3"

  if [[ -n "${server_pid}" ]]; then
    echo "refusing to start a second ai-server process" >&2
    return 1
  fi
  server_log_file="${log_file}"
  AIOPS_ENV_FILE="${run_dir}/missing.env" \
  AIOPS_STORE_DRIVER=json \
  AIOPS_DATA_DIR="${data_dir}" \
  AIOPS_HTTP_ADDR="127.0.0.1:${http_port}" \
  AIOPS_GRPC_ADDR="127.0.0.1:${grpc_port}" \
  AIOPS_WEB_DIST_DIR="${repo_root}/web/dist" \
  AIOPS_SERVER_BIN="${server_bin}" \
  SKIP_WEB_BUILD="${start_skip_web_build}" \
  SKIP_GO_BUILD="${start_skip_go_build}" \
  SKIP_NODE_ARTIFACT_BUILD=1 \
  scripts/start.sh >"${server_log_file}" 2>&1 &
  server_pid="$!"
  wait_for_http "${base_url}/api/v1/runtime-settings" "${server_pid}" "${server_log_file}"
}

verify_workflow_rollout_response() {
  local response_file="$1"
  local expected_stage="$2"
  node --input-type=module - "${response_file}" "${expected_stage}" <<'NODE'
import fs from "node:fs";

const [responsePath, expectedStage] = process.argv.slice(2);
const payload = JSON.parse(fs.readFileSync(responsePath, "utf8"));
const rollout = payload?.settings?.workflow?.rollout;
const expected = {
  schemaVersion: "aiops.workflow.rollout.v1",
  stage: expectedStage,
  saltVersion: "workflow-canvas-e2e-v1",
  rolloutPercentage: 100,
  allowTenants: ["runner-default"],
};
for (const [key, value] of Object.entries(expected)) {
  if (JSON.stringify(rollout?.[key]) !== JSON.stringify(value)) {
    throw new Error(`persisted workflow rollout ${key} does not match the full-stack E2E contract`);
  }
}
if (rollout?.denyTenants !== undefined && (!Array.isArray(rollout.denyTenants) || rollout.denyTenants.length !== 0)) {
  throw new Error("persisted workflow rollout denyTenants must be empty");
}
const expectedDomainEvents = expectedStage === "internal_apply";
if (rollout?.features?.workflow_domain_events !== expectedDomainEvents) {
  throw new Error(`persisted workflow rollout domain-event flag does not match ${expectedStage}`);
}
for (const phaseCapability of ["workflow_plan_ir", "workflow_elk_layout", "workflow_edit_grants", "workflow_external_mcp", "workflow_mcp_apps"]) {
  if (rollout?.features?.[phaseCapability] !== false) {
    throw new Error(`persisted Workflow E2E rollout must keep deferred capability ${phaseCapability} disabled`);
  }
}
if (!Array.isArray(payload?.restartRequiredKeys) || !payload.restartRequiredKeys.includes("workflow.rollout")) {
  throw new Error("runtime settings response must declare workflow.rollout restart-required");
}
NODE
}

verify_workflow_control_plane_after_restart() {
  local state_file="$1"
  local mcp_file="$2"
  local expected_stage="$3"
  node --input-type=module - "${state_file}" "${mcp_file}" "${expected_stage}" <<'NODE'
import fs from "node:fs";

const [statePath, mcpPath, expectedStage] = process.argv.slice(2);
const state = JSON.parse(fs.readFileSync(statePath, "utf8"));
const snapshot = state?.config?.workflowCapabilities;
if (snapshot?.schemaVersion !== "aiops.workflow.rollout.v1" || snapshot?.stage !== expectedStage || snapshot?.decision !== "allow_list") {
  throw new Error(`AppSnapshot does not expose the restarted ${expectedStage} allow-list decision`);
}
const mcp = JSON.parse(fs.readFileSync(mcpPath, "utf8"));
const runner = mcp?.items?.find((item) => item?.name === "runner");
if (expectedStage === "disabled") {
  const enabled = Object.entries(snapshot?.capabilities || {}).filter(([, value]) => value === true).map(([name]) => name);
  if (enabled.length !== 0) throw new Error(`disabled rollout exposed Workflow capabilities: ${enabled.join(",")}`);
  if (runner && runner.disabled !== true && runner.status === "connected" && Number(runner.toolCount) > 0) {
    throw new Error("disabled rollout exposed the connected Runner Workflow MCP tool surface");
  }
  process.exit(0);
}
const requiredCapabilities = ["workflow_agent_ui", "workflow_mcp_read", "workflow_mcp_preview", "workflow_mcp_mutation", "workflow_domain_events"];
for (const capability of requiredCapabilities) {
  if (snapshot?.capabilities?.[capability] !== true) {
    throw new Error(`AppSnapshot is missing required capability ${capability}`);
  }
}
for (const deferredCapability of ["workflow_delete", "workflow_publish", "workflow_execute", "workflow_plan_ir", "workflow_elk_layout", "workflow_edit_grants", "workflow_external_mcp", "workflow_mcp_apps"]) {
  if (snapshot?.capabilities?.[deferredCapability] !== false) {
    throw new Error(`internal_apply rollout exposed deferred capability ${deferredCapability}`);
  }
}
if (!runner || runner.disabled === true || runner.status !== "connected" || !Number.isInteger(runner.toolCount) || runner.toolCount < 5) {
  throw new Error("restarted MCP registry does not expose the connected Runner Workflow Editor tool surface");
}
NODE
}

cd "${repo_root}"
command -v node >/dev/null 2>&1
command -v npm >/dev/null 2>&1
command -v npx >/dev/null 2>&1
command -v go >/dev/null 2>&1
command -v curl >/dev/null 2>&1
command -v python3 >/dev/null 2>&1

node --check web/tests/helpers/recordedWorkflowModelServer.js
node --test web/tests/helpers/recordedWorkflowModelServer.test.js
node --check web/tests/helpers/workflowCanvasServerHarness.js
node --test web/tests/helpers/workflowCanvasServerHarness.test.js
node --check web/tests/e2e/cases/workflowCandidateCases.js
node --check web/tests/e2e/cases/workflowGenerationCases.js
node --check web/tests/e2e/cases/workflowResilienceCases.js
node --check web/tests/e2e/cases/workflowScaleLifecycleCases.js
node --check web/tests/e2e/workflow-canvas-mcp.spec.js
if rg -n '\.(route|routeFromHAR)\s*\(' \
  web/tests/e2e/workflow-canvas-mcp.spec.js \
  web/tests/e2e/cases/workflowCandidateCases.js \
  web/tests/e2e/cases/workflowGenerationCases.js \
  web/tests/e2e/cases/workflowResilienceCases.js \
  web/tests/e2e/cases/workflowScaleLifecycleCases.js; then
  echo "full-stack Workflow Canvas spec must not register route mocks" >&2
  exit 1
fi

node web/tests/helpers/recordedWorkflowModelServer.js \
  --host 127.0.0.1 \
  --port "${model_port}" \
  --record "${run_dir}/recorded-model-requests.jsonl" \
  >"${run_dir}/recorded-model.log" 2>&1 &
model_pid="$!"
wait_for_http "${model_origin}/healthz" "${model_pid}" "${run_dir}/recorded-model.log"

skip_web_build="${AIOPS_WORKFLOW_E2E_SKIP_WEB_BUILD:-0}"
skip_go_build="${AIOPS_WORKFLOW_E2E_SKIP_GO_BUILD:-0}"
start_ai_server "${run_dir}/ai-server-bootstrap.log" "${skip_web_build}" "${skip_go_build}"

# Workflow MCP registration is intentionally a startup decision. Persist the
# explicit test rollout on a fresh store, then gracefully restart the same
# server/data directory before configuring or exercising the model transport.
curl --noproxy '*' --fail --silent --show-error \
  -X PATCH \
  -H 'Content-Type: application/json' \
  --data "${rollout_payload}" \
  "${base_url}/api/v1/runtime-settings" >"${run_dir}/workflow-rollout-bootstrap-response.json"
verify_workflow_rollout_response "${run_dir}/workflow-rollout-bootstrap-response.json" "${rollout_stage}"

stop_ai_server
start_ai_server "${run_dir}/ai-server.log" 1 1
curl --noproxy '*' --fail --silent --show-error \
  "${base_url}/api/v1/runtime-settings" >"${run_dir}/workflow-rollout-after-restart.json"
verify_workflow_rollout_response "${run_dir}/workflow-rollout-after-restart.json" "${rollout_stage}"
curl --noproxy '*' --fail --silent --show-error \
  "${base_url}/api/v1/state" >"${run_dir}/workflow-app-snapshot-after-restart.json"
curl --noproxy '*' --fail --silent --show-error \
  "${base_url}/api/v1/mcp/servers" >"${run_dir}/workflow-mcp-registry-after-restart.json"
verify_workflow_control_plane_after_restart \
  "${run_dir}/workflow-app-snapshot-after-restart.json" \
  "${run_dir}/workflow-mcp-registry-after-restart.json" \
  "${rollout_stage}"

curl --noproxy '*' --fail --silent --show-error \
  -X PUT \
  -H 'Content-Type: application/json' \
  --data "{\"provider\":\"openai\",\"model\":\"recorded-workflow-model\",\"baseURL\":\"${model_origin}/v1\",\"apiKey\":\"workflow-e2e-placeholder\",\"maxContextTokens\":200000,\"requestTimeoutMs\":30000}" \
  "${base_url}/api/v1/llm-config" >"${run_dir}/llm-config-response.json"

export PLAYWRIGHT_BASE_URL="${base_url}"
export PLAYWRIGHT_SKIP_WEB_SERVER=1
export AIOPS_WORKFLOW_FULL_STACK_E2E=1
export AIOPS_RECORDED_WORKFLOW_MODEL_BASE_URL="${model_origin}/v1"
export PLAYWRIGHT_HTML_OPEN=never
export PLAYWRIGHT_HTML_OUTPUT_DIR="${run_dir}/html-report"

playwright_projects="${AIOPS_WORKFLOW_E2E_PROJECTS:-chromium}"
project_args=()
IFS=',' read -r -a requested_projects <<<"${playwright_projects}"
for project in "${requested_projects[@]}"; do
  project="${project//[[:space:]]/}"
  case "${project}" in
    chromium|firefox|webkit) project_args+=("--project=${project}") ;;
    *) echo "unsupported Workflow Canvas Playwright project: ${project}" >&2; exit 2 ;;
  esac
done
if [[ -n "${requested_case_grep}" ]]; then
  project_args+=("--grep=${requested_case_grep}")
fi

(
  cd web
  npx playwright test tests/e2e/workflow-canvas-mcp.spec.js \
    "${project_args[@]}" \
    --workers=1 \
    --trace=retain-on-failure \
    --output="${run_dir}/playwright" \
    --reporter=line,html
)

node --input-type=module - "${repo_root}/testdata/workflow_canvas_mcp/test-manifest.json" "${run_dir}/case-summary.json" <<'NODE'
import fs from "node:fs";
const [manifestPath, outputPath] = process.argv.slice(2);
const manifest = JSON.parse(fs.readFileSync(manifestPath, "utf8"));
const workflowCases = manifest.cases.filter((entry) => entry.id.startsWith("PW-"));
const summary = {
  schemaVersion: "aiops.workflow-canvas.playwright-summary/v1",
  fullStackSmokeCases: 3,
  fullStackManifestCases: workflowCases.length,
  activeManifestCases: workflowCases.filter((entry) => entry.activationState === "active" && entry.activationPhase <= manifest.currentActivationPhase).length,
  plannedManifestCases: workflowCases.filter((entry) => entry.activationState !== "active" || entry.activationPhase > manifest.currentActivationPhase).length,
  fixtureMockSpec: "web/tests/e2e/workflow-ai-real-user.spec.js",
  fullStackSpec: "web/tests/e2e/workflow-canvas-mcp.spec.js",
};
fs.writeFileSync(outputPath, `${JSON.stringify(summary, null, 2)}\n`);
process.stdout.write(`${JSON.stringify(summary)}\n`);
NODE

# The isolated server persists its provider configuration so restart tests can
# exercise the real store. Stop both processes before replacing credential
# fields in the evidence copy; uploaded artifacts must never contain even test
# credentials, and live credentials are never written by this harness.
cleanup
server_pid=""
model_pid=""
node --input-type=module - "${run_dir}" <<'NODE'
import fs from "node:fs";
import path from "node:path";

const root = path.resolve(process.argv[2]);
const secretKey = /^(?:authorization|proxy-authorization|api[_-]?key|access[_-]?token|refresh[_-]?token|client[_-]?secret|password)$/i;
function redact(value) {
  if (Array.isArray(value)) return value.map(redact);
  if (!value || typeof value !== "object") return value;
  return Object.fromEntries(Object.entries(value).map(([key, child]) => [key, secretKey.test(key) ? "[REDACTED]" : redact(child)]));
}
function walk(directory) {
  for (const entry of fs.readdirSync(directory, { withFileTypes: true })) {
    const file = path.join(directory, entry.name);
    if (entry.isDirectory()) walk(file);
    else if (entry.isFile() && /\.json$/i.test(file)) {
      try {
        const value = JSON.parse(fs.readFileSync(file, "utf8"));
        fs.writeFileSync(file, `${JSON.stringify(redact(value), null, 2)}\n`);
      } catch {
        // Non-JSON assets are checked separately and are never treated as safe.
      }
    }
  }
}
walk(root);
NODE

"${repo_root}/scripts/check-workflow-evidence-redaction.sh" "${run_dir}"

echo "workflow canvas full-stack evidence: ${run_dir}"
