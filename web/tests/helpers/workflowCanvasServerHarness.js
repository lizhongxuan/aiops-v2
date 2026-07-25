// @ts-check

import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const helperDir = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(helperDir, "../../..");
const defaultManifestPath = path.join(repoRoot, "testdata/workflow_canvas_mcp/test-manifest.json");
const fullStackSpecPath = path.join(repoRoot, "web/tests/e2e/workflow-canvas-mcp.spec.js");

const TARGET_API_PREFIXES = [
  "/api/v1/chat/",
  "/api/v1/assistant/",
  "/api/v1/state",
  "/api/runner-studio/workflow-ai/",
  "/api/runner-studio/workflow-commands",
  "/api/runner-studio/workflows",
];

export function loadWorkflowCanvasManifest() {
  const manifestPath = process.env.AIOPS_WORKFLOW_CASE_MANIFEST || defaultManifestPath;
  const manifest = readJSON(manifestPath);
  if (manifest?.schemaVersion !== "aiops.workflow-canvas.case-manifest/v1" || !Array.isArray(manifest.cases)) {
    throw new Error(`invalid Workflow Canvas manifest: ${manifestPath}`);
  }
  return manifest;
}

export function workflowCaseActivation(caseId, manifest = loadWorkflowCanvasManifest()) {
  const definition = manifest.cases.find((candidate) => candidate.id === caseId);
  if (!definition) throw new Error(`Workflow Canvas manifest does not define ${caseId}`);
  const explicitlyRequested = requestedWorkflowCases();
  if (explicitlyRequested.size > 0) {
    return {
      definition,
      phase: Number(definition.activationPhase),
      active: explicitlyRequested.has("*") || explicitlyRequested.has(caseId),
      reason: `explicit AIOPS_WORKFLOW_E2E_CASES selection: ${[...explicitlyRequested].join(",")}`,
    };
  }
  const requested = Number.parseInt(process.env.AIOPS_WORKFLOW_E2E_PHASE || String(manifest.currentActivationPhase), 10);
  const phase = Number.isFinite(requested) ? Math.max(Number(manifest.currentActivationPhase), requested) : Number(manifest.currentActivationPhase);
  return {
    definition,
    phase,
    active: definition.activationState === "active" && Number(definition.activationPhase) <= phase,
    reason: `${definition.activationState} at phase ${definition.activationPhase}; current phase ${phase}`,
  };
}

export function requestedWorkflowCases() {
  return new Set(String(process.env.AIOPS_WORKFLOW_E2E_CASES || "")
    .split(",")
    .map((value) => value.trim().toUpperCase())
    .filter(Boolean));
}

export function assertFullStackSpecHasNoRouteMocks() {
  const source = readFileSync(fullStackSpecPath, "utf8");
  if (/\.(?:route|routeFromHAR)\s*\(/.test(source)) {
    throw new Error("full-stack Workflow Canvas spec must not register Playwright route mocks");
  }
}

export function createRealPageGuard(page) {
  const consoleProblems = [];
  const networkProblems = [];
  const workflowProjections = [];
  let workflowSocketOpenCount = 0;
  let workflowSocketCloseCount = 0;
  page.on("console", (message) => {
    if (message.type() === "error" || message.type() === "warning") {
      consoleProblems.push(`${message.type()}: ${message.text()}`);
    }
  });
  page.on("pageerror", (error) => consoleProblems.push(`pageerror: ${error.message}`));
  page.on("requestfailed", (request) => {
    networkProblems.push(`${request.method()} ${request.url()} ${request.failure()?.errorText || "request failed"}`);
  });
  page.on("response", (response) => {
    if (response.status() >= 400 && !isIgnorableResponse(response)) {
      networkProblems.push(`${response.request().method()} ${response.url()} HTTP ${response.status()}`);
    }
  });
  page.on("websocket", (socket) => {
    const pathname = new URL(socket.url()).pathname;
    if (pathname !== "/ws") return;
    workflowSocketOpenCount += 1;
    socket.on("close", () => { workflowSocketCloseCount += 1; });
    socket.on("framereceived", (frame) => {
      const payload = typeof frame.payload === "string" ? frame.payload : frame.payload.toString("utf8");
      try {
        const message = JSON.parse(payload);
        if (message?.config?.workflowWorkspace) workflowProjections.push(message.config.workflowWorkspace);
      } catch {
        // Heartbeats and non-JSON frames are not Workflow projection evidence.
      }
    });
  });
  return {
    consoleProblems,
    networkProblems,
    workflowProjections,
    latestWorkflowProjection() {
      return workflowProjections.at(-1);
    },
    workflowSocketOpenCount() {
      return workflowSocketOpenCount;
    },
    workflowSocketCloseCount() {
      return workflowSocketCloseCount;
    },
    async assertHealthy() {
      const frameworkError = page.locator([
        "[data-nextjs-dialog-overlay]",
        "vite-error-overlay",
        "#webpack-dev-server-client-overlay",
      ].join(", "));
      const runtimeErrorText = page.getByText("Uncaught Runtime Error", { exact: true });
      if (await frameworkError.count() || await runtimeErrorText.count()) throw new Error("framework error layer is visible");
      if (consoleProblems.length) throw new Error(`unexpected browser console output:\n${consoleProblems.join("\n")}`);
      if (networkProblems.length) throw new Error(`unexpected failed browser requests:\n${networkProblems.join("\n")}`);
    },
  };
}

export async function assertRealServerReady(request) {
  const state = await getJSON(request, "/api/v1/state");
  if (!state || typeof state !== "object" || !("kind" in state)) throw new Error("real ai-server state endpoint returned no application state");
  const workflows = await getJSON(request, "/api/runner-studio/workflows");
  if (!workflows || typeof workflows !== "object") throw new Error("real embedded Runner endpoint is unavailable");
  return { state, workflows };
}

export async function seedWorkflowFixture(request, fixtureId, uniqueSuffix) {
  const { name, graph } = materializeWorkflowFixture(fixtureId, uniqueSuffix);
  const created = await postJSON(request, "/api/runner-studio/workflows/graph", {
    graph,
    labels: { source: "workflow-canvas-full-stack", fixture: fixtureId },
    save_note: `seed ${fixtureId} for isolated full-stack test`,
  }, 201);
  return { name, graph: created.graph || graph, created };
}

/**
 * Converts a manifest Workflow fixture into the exact Graph sent to the real
 * embedded Runner API. Recipe fixtures are expanded here so scale tests never
 * rely on a browser route, HAR, or an in-memory React fixture.
 *
 * W5 is intentionally materialized with an identifier-only missing Secret
 * reference. The current production validator rejects it before a trusted
 * configuration flow can be entered; the corresponding Playwright blocker
 * records that product boundary instead of weakening the fixture to a shell
 * action or putting a credential value in the Graph.
 */
export function materializeWorkflowFixture(fixtureId, uniqueSuffix) {
  const fixture = loadWorkflowFixture(fixtureId);
  const source = fixture?.data?.workflow;
  if (!source || typeof source.name !== "string" || !source.name.trim()) {
    throw new Error(`${fixtureId} has no materializable workflow identity`);
  }
  const name = `${source.name}-${uniqueSuffix}`.replace(/[^a-zA-Z0-9_.-]/g, "-");
  const recipe = fixture?.data?.recipe;
  let nodes;
  let edges;
  let ui;

  if (recipe?.kind === "seeded-dag") {
    ({ nodes, edges } = materializeSeededDAG(recipe));
    ui = {
      fixture_id: fixtureId,
      fixture_recipe: "seeded-dag",
      fixture_seed: recipe.seed,
      fixture_diagnostic_count: recipe.diagnosticCount,
    };
  } else if (fixtureId === "W5" && source.configurationState === "configuration_required") {
    ({ nodes, edges } = materializeMissingSecretWorkflow(source));
    ui = {
      fixture_id: fixtureId,
      fixture_configuration_state: "configuration_required",
      fixture_secret_refs: [],
    };
  } else if (Array.isArray(source.nodes) && Array.isArray(source.edges)) {
    nodes = source.nodes.map((node, index) => fixtureNode(node, index));
    edges = source.edges.map((edge) => fixtureEdge(edge));
  } else {
    throw new Error(`${fixtureId} cannot be materialized as a full graph fixture`);
  }

  return {
    name,
    graph: {
      version: "v1",
      workflow: {
        version: "v0.1",
        name,
        description: `full-stack fixture ${fixtureId}`,
        validation_env: "staging",
      },
      nodes,
      edges,
      ...(ui ? { ui } : {}),
    },
  };
}

export async function readWorkflowGraph(request, workflowId) {
  return getJSON(request, `/api/runner-studio/workflows/${encodeURIComponent(workflowId)}/graph`);
}

export async function readApplicationState(request) {
  return getJSON(request, "/api/v1/state");
}

export function workflowProjection(statePayload, workflowId) {
  const projection = statePayload?.config?.workflowWorkspace;
  const projectedWorkflowId = projection?.committed?.workflowKey?.workflowId || projection?.workflowKey?.workflowId;
  return projectedWorkflowId === workflowId ? projection : undefined;
}

export function fullRevision(projection) {
  const revision = projection?.committed?.revision;
  const values = [revision?.resourceVersion, revision?.semanticHash, revision?.layoutHash];
  if (values.some((value) => typeof value !== "string" || !value.trim())) {
    throw new Error(`Workflow projection has no complete committed revision: ${JSON.stringify(revision || {})}`);
  }
  return {
    resourceVersion: revision.resourceVersion,
    semanticHash: revision.semanticHash,
    layoutHash: revision.layoutHash,
  };
}

export async function validateWorkflowNodeLabel(request, { workflowId, expectedRevision, nodeId, label, patchId }) {
  const patch = {
    id: patchId,
    baseRevision: expectedRevision,
    operations: [{ op: "update_node", nodeId, fields: { label } }],
  };
  const result = await postJSON(request, "/api/runner-studio/workflow-ai/validate", {
    workflowId,
    baseRevision: expectedRevision,
    patch,
  });
  const errors = Array.isArray(result?.errors) ? result.errors : [];
  if (result?.valid !== true || errors.length) {
    throw new Error(`real Workflow validation rejected the typed label patch: ${JSON.stringify(result)}`);
  }
  return result;
}

export async function commitManualNodeLabel(request, { workflowId, expectedRevision, nodeId, label, idempotencyKey }) {
  return commitManualWorkflowCommand(request, {
    workflowId,
    expectedRevision,
    idempotencyKey,
    reason: "full-stack harness verifies authoritative manual edit plumbing",
    command: { kind: "update_node", nodeId, nodePatch: { label } },
  });
}

/**
 * @param {import("@playwright/test").APIRequestContext} request
 * @param {{
 *   workflowId: string,
 *   expectedRevision: string,
 *   idempotencyKey: string,
 *   reason?: string,
 *   command?: Record<string, unknown>,
 *   commands?: Array<Record<string, unknown>>,
 *   confirmed?: boolean,
 * }} options
 */
export async function commitManualWorkflowCommand(request, {
  workflowId,
  expectedRevision,
  idempotencyKey,
  reason = "full-stack Playwright verifies an authoritative manual workflow command",
  command,
  commands,
  confirmed,
}) {
  const result = await postJSON(request, "/api/runner-studio/workflow-commands", {
    schemaVersion: "aiops.workflow.edit.v2",
    workflowId,
    expectedRevision,
    idempotencyKey,
    reason,
    ...(confirmed === true ? { confirmed: true } : {}),
    ...(command ? { command } : { commands }),
  });
  if (!result?.receipt?.transactionId || !result?.receipt?.revisionAfter?.resourceVersion) {
    throw new Error(`workflow command returned no authoritative receipt: ${JSON.stringify(result)}`);
  }
  if (!result?.event?.eventId || !Number.isFinite(result?.event?.sequence)) {
    throw new Error(`workflow command returned no domain event: ${JSON.stringify(result)}`);
  }
  return result;
}

export async function attachFullStackEvidence(page, testInfo, facts) {
  const screenshot = await page.screenshot({ fullPage: true });
  await testInfo.attach("full-stack-page.png", { body: screenshot, contentType: "image/png" });
  await testInfo.attach("workflow-facts.json", {
    body: Buffer.from(`${JSON.stringify(facts, null, 2)}\n`, "utf8"),
    contentType: "application/json",
  });
}

export function resourceVersion(graphPayload) {
  return String(graphPayload?.ui?.resource_version || graphPayload?.ui?.resourceVersion || "").trim();
}

export function isTargetAPI(urlValue) {
  const pathname = new URL(urlValue, "http://127.0.0.1").pathname;
  return TARGET_API_PREFIXES.some((prefix) => pathname === prefix || pathname.startsWith(prefix));
}

function loadWorkflowFixture(fixtureId) {
  return readJSON(path.join(repoRoot, "testdata/workflow_canvas_mcp/fixtures", `${fixtureId}.json`));
}

function fixtureNode(node, index) {
  const base = {
    id: String(node.id),
    type: String(node.type),
    position: { x: 120 + index * 260, y: 180 },
  };
  if (node.type !== "action") return base;
  return {
    ...base,
    label: "Recorded DNS check",
    step: {
      name: String(node.id),
      action: "script.shell",
      args: { script: "set -e\necho recorded-workflow-harness" },
    },
  };
}

function fixtureEdge(edge) {
  return {
    id: String(edge.id),
    source: String(edge.source),
    target: String(edge.target),
    kind: edge.source === "start" ? "next" : "success",
  };
}

function materializeSeededDAG(recipe) {
  const nodeCount = Number(recipe.nodeCount);
  const edgeCount = Number(recipe.edgeCount);
  const seed = Number(recipe.seed);
  if (!Number.isSafeInteger(nodeCount) || nodeCount < 3 || !Number.isSafeInteger(edgeCount) || edgeCount < nodeCount - 1 || !Number.isSafeInteger(seed)) {
    throw new Error("seeded-dag fixture requires a safe seed, at least three nodes, and a connected edge budget");
  }
  const maximumEdges = (nodeCount * (nodeCount - 1)) / 2;
  if (edgeCount > maximumEdges) throw new Error("seeded-dag fixture edge budget exceeds the DAG maximum");

  const width = String(nodeCount - 1).length;
  const nodeId = (index) => `node-${String(index).padStart(width, "0")}`;
  const nodes = Array.from({ length: nodeCount }, (_, index) => {
    const base = {
      id: nodeId(index),
      type: index === 0 ? "start" : index === nodeCount - 1 ? "end" : "action",
      label: index === 0 ? "Start" : index === nodeCount - 1 ? "End" : `Scale node ${String(index).padStart(width, "0")}`,
      position: { x: 120 + Math.floor(index / 10) * 260, y: 100 + (index % 10) * 150 },
    };
    if (base.type !== "action") return base;
    return {
      ...base,
      step: {
        name: nodeId(index),
        action: "script.shell",
        args: { script: `set -e\necho ${nodeId(index)}` },
      },
    };
  });

  const edges = [];
  const used = new Set();
  const addEdge = (sourceIndex, targetIndex) => {
    const identity = `${sourceIndex}:${targetIndex}`;
    if (sourceIndex < 0 || targetIndex >= nodeCount || sourceIndex >= targetIndex || used.has(identity)) return false;
    used.add(identity);
    edges.push({
      id: `edge-${String(edges.length).padStart(String(edgeCount - 1).length, "0")}`,
      source: nodeId(sourceIndex),
      target: nodeId(targetIndex),
      kind: sourceIndex === 0 ? "next" : "success",
    });
    return true;
  };
  for (let index = 0; index < nodeCount - 1; index += 1) addEdge(index, index + 1);

  const random = seededRandom(seed);
  let attempts = 0;
  const attemptBudget = Math.max(edgeCount * 50, 10_000);
  while (edges.length < edgeCount && attempts < attemptBudget) {
    attempts += 1;
    // Keep the unique start edge stable. Extra forward-only action edges retain
    // a connected, cycle-free graph while exercising the real large-DAG path.
    const sourceIndex = 1 + Math.floor(random() * (nodeCount - 2));
    const targetIndex = sourceIndex + 1 + Math.floor(random() * (nodeCount - sourceIndex - 1));
    addEdge(sourceIndex, targetIndex);
  }
  if (edges.length !== edgeCount) throw new Error(`seeded-dag materializer produced ${edges.length}/${edgeCount} edges`);
  return { nodes, edges };
}

function materializeMissingSecretWorkflow(source) {
  const connector = Array.isArray(source.nodes) ? source.nodes.find((node) => node?.type === "action") : undefined;
  if (!connector || connector.args?.secretRef !== null || !Array.isArray(source.secretRefs) || source.secretRefs.length !== 0) {
    throw new Error("W5 must describe one connector with an explicitly missing Secret reference and no Secret values");
  }
  return {
    nodes: [
      { id: "start", type: "start", label: "Start", position: { x: 120, y: 180 } },
      {
        id: String(connector.id),
        type: "action",
        label: "Cloud connector (configuration required)",
        position: { x: 380, y: 180 },
        step: {
          name: String(connector.id),
          action: "http.request",
          args: {
            method: "GET",
            url: "https://connector.invalid/health",
            auth: { type: "bearer", secret_ref: null },
          },
        },
      },
      { id: "end", type: "end", label: "End", position: { x: 640, y: 180 } },
    ],
    edges: [
      { id: "start-connector", source: "start", target: String(connector.id), kind: "next" },
      { id: "connector-end", source: String(connector.id), target: "end", kind: "success" },
    ],
  };
}

function seededRandom(seed) {
  let state = seed >>> 0;
  return () => {
    state = (Math.imul(state, 1_664_525) + 1_013_904_223) >>> 0;
    return state / 0x1_0000_0000;
  };
}

async function getJSON(request, url) {
  const response = await request.get(url);
  return parseJSONResponse(response, url, 200);
}

async function postJSON(request, url, data, expectedStatus = 200) {
  const response = await request.post(url, { data });
  return parseJSONResponse(response, url, expectedStatus);
}

async function parseJSONResponse(response, url, expectedStatus) {
  const text = await response.text();
  let payload = {};
  try {
    payload = text ? JSON.parse(text) : {};
  } catch {
    throw new Error(`${url} returned non-JSON HTTP ${response.status()}: ${text.slice(0, 300)}`);
  }
  if (response.status() !== expectedStatus) {
    throw new Error(`${url} returned HTTP ${response.status()}, expected ${expectedStatus}: ${JSON.stringify(payload)}`);
  }
  return payload;
}

function readJSON(filePath) {
  return JSON.parse(readFileSync(filePath, "utf8"));
}

function isIgnorableResponse(response) {
  const pathname = new URL(response.url()).pathname;
  return response.status() === 404 && pathname === "/favicon.ico";
}
