import fs from "node:fs";
import path from "node:path";

const MANIFEST_SCHEMA = "aiops.workflow-canvas.case-manifest/v1";
const FIXTURE_SCHEMA = "aiops.workflow-canvas.fixture/v1";
const DEFAULT_MANIFEST_PATH = path.resolve(process.cwd(), "../testdata/workflow_canvas_mcp/test-manifest.json");

function expectedCaseIds() {
  const groups = [
    ["DOM", 12], ["MCP", 15], ["TXN", 17], ["UI", 27], ["EVT", 10], ["SEC", 15],
    ["PW", 33], ["RWD", 4], ["VIS", 3], ["A11Y", 6], ["PERF", 5], ["FI", 8], ["GEN", 14],
  ];
  return groups.flatMap(([prefix, count]) =>
    Array.from({ length: count }, (_, index) => `${prefix}-${String(index + 1).padStart(3, "0")}`),
  );
}

export function validateWorkflowCanvasManifest(manifest) {
  const errors = [];
  if (manifest?.schemaVersion !== MANIFEST_SCHEMA) errors.push(`schemaVersion must be ${MANIFEST_SCHEMA}`);
  if (!Number.isInteger(manifest?.currentActivationPhase) || manifest.currentActivationPhase < 0 || manifest.currentActivationPhase > 5) {
    errors.push("currentActivationPhase must be between 0 and 5");
  }
  const fixtures = new Set();
  for (const entry of manifest?.fixtures ?? []) {
    if (!entry.id || !entry.kind || !entry.path) errors.push("fixture requires id, kind, and path");
    if (fixtures.has(entry.id)) errors.push(`duplicate fixture ${entry.id}`);
    fixtures.add(entry.id);
  }
  const cases = new Set();
  for (const entry of manifest?.cases ?? []) {
    for (const field of [
      "id", "priority", "activationPhase", "activationState", "suite", "selector", "fixtures", "browsers",
      "capabilityTags", "owner", "evidence", "expectedOutcome",
    ]) {
      if (!Object.hasOwn(entry, field)) errors.push(`${entry.id ?? "unknown case"} lacks field ${field}`);
    }
    if (cases.has(entry.id)) errors.push(`duplicate case ${entry.id}`);
    cases.add(entry.id);
    if (!/^(P0|P1|P2)$/.test(entry.priority ?? "")) errors.push(`${entry.id} has invalid priority`);
    if (!Number.isInteger(entry.activationPhase) || entry.activationPhase < 0 || entry.activationPhase > 5) {
      errors.push(`${entry.id} has invalid activationPhase`);
    }
    if (!["active", "planned"].includes(entry.activationState)) errors.push(`${entry.id} has invalid activationState`);
    if (entry.activationPhase > manifest.currentActivationPhase && entry.activationState !== "planned") {
      errors.push(`${entry.id} future case must be planned`);
    }
    if (!entry.suite || !entry.selector || !entry.owner || !entry.fixtures?.length || !entry.evidence?.length || !entry.capabilityTags?.length) {
      errors.push(`${entry.id} lacks routing metadata`);
    }
    for (const fixture of entry.fixtures ?? []) {
      if (!fixtures.has(fixture)) errors.push(`${entry.id} references unknown fixture ${fixture}`);
    }
    if (entry.expectedOutcome === "known_failure") {
      if (entry.activationState !== "active" || !entry.expectedFailureSignature || !Number.isInteger(entry.greenActivationPhase)) {
        errors.push(`${entry.id} has invalid known_failure contract`);
      }
      if (entry.greenActivationPhase <= manifest.currentActivationPhase) {
        errors.push(`${entry.id} must convert to pass at its greenActivationPhase`);
      }
    } else if (entry.expectedOutcome !== "pass") {
      errors.push(`${entry.id} has invalid expectedOutcome`);
    }
  }
  for (const id of expectedCaseIds()) {
    if (!cases.has(id)) errors.push(`design case missing from manifest: ${id}`);
  }
  if (cases.size !== expectedCaseIds().length) errors.push("manifest includes an unknown or duplicate design case");
  if (errors.length) throw new Error(errors.sort().join("; "));
  return manifest;
}

export function loadWorkflowCanvasManifest(manifestPath = DEFAULT_MANIFEST_PATH) {
  const manifest = JSON.parse(fs.readFileSync(manifestPath, "utf8"));
  Object.defineProperty(manifest, "__manifestPath", { value: manifestPath, enumerable: false });
  return validateWorkflowCanvasManifest(manifest);
}

export function selectWorkflowCanvasCases(manifest, options = {}) {
  validateWorkflowCanvasManifest(manifest);
  const phase = options.phase ?? manifest.currentActivationPhase;
  if (phase < manifest.currentActivationPhase) {
    throw new Error(`phase ${phase} cannot lower repository phase ${manifest.currentActivationPhase}`);
  }
  const priorities = new Set(options.priorities ?? ["P0", "P1"]);
  return manifest.cases
    .filter((entry) => entry.activationState === "active" && entry.activationPhase <= phase && priorities.has(entry.priority))
    .sort((left, right) => left.id.localeCompare(right.id));
}

export function loadWorkflowCanvasFixture(id, manifest = loadWorkflowCanvasManifest()) {
  const entry = manifest.fixtures.find((candidate) => candidate.id === id);
  if (!entry) throw new Error(`unknown workflow fixture ${id}`);
  const manifestPath = manifest.__manifestPath ?? DEFAULT_MANIFEST_PATH;
  const root = path.dirname(manifestPath);
  const fixturePath = path.resolve(root, entry.path);
  if (!fixturePath.startsWith(`${path.resolve(root)}${path.sep}`)) throw new Error(`fixture ${id} escapes manifest root`);
  const fixture = JSON.parse(fs.readFileSync(fixturePath, "utf8"));
  if (fixture.schemaVersion !== FIXTURE_SCHEMA || fixture.fixtureId !== id || fixture.kind !== entry.kind || fixture.data == null) {
    throw new Error(`fixture ${id} does not match its catalog entry`);
  }
  return fixture;
}
