// @ts-check

import { expect } from "@playwright/test";

import {
  assertFullStackSpecHasNoRouteMocks,
  assertRealServerReady,
  attachFullStackEvidence,
  commitManualWorkflowCommand,
  createRealPageGuard,
  fullRevision,
  readWorkflowGraph,
  resourceVersion,
  seedWorkflowFixture,
} from "../../helpers/workflowCanvasServerHarness.js";

/**
 * Cases in this module must exercise the real ai-server and must never install
 * a Playwright route/HAR interceptor for a target API. A missing product path
 * is represented in workflowCandidateCaseBlockers instead of being fake-passed.
 */
export const workflowCandidateCaseImplementations = Object.freeze({
  "PW-005": locatesChatChangeFromGhostNode,
  "PW-012": completesHistoryUndoRedoApprovalJourney,
});

/**
 * Machine-readable activation blockers for candidate-oriented Phase 3 cases.
 * `requiredPath` is the journey the manifest requires; `observedGap` names the
 * exact production boundary that currently prevents an honest full-stack Case.
 */
export const workflowCandidateCaseBlockers = deepFreeze({
  "PW-001": {
    code: "WORKFLOW_CREATE_PLAN_COMPILE_PATH_MISSING",
    stage: "model_to_mcp",
    requiredPath: ["W0", "compile_plan", "preview_patch", "approval", "apply_patch"],
    observedGap: "The deterministic real provider can only preview one existing-node label update or delete; it cannot compile and preview a conditional graph from W0.",
  },
  "PW-002": {
    code: "SELECTED_THRESHOLD_MUTATION_PATH_MISSING",
    stage: "model_to_mcp",
    requiredPath: ["frozen_selection", "threshold_only_patch", "approval", "apply_patch"],
    observedGap: "The deterministic real provider exposes no threshold operation and the full-stack fixture materializer does not preserve a W2 threshold field.",
  },
  "PW-004": {
    code: "CHAT_ARTIFACT_FOCUS_ACTION_MISSING",
    stage: "transport_artifact",
    requiredPath: ["chat_change_item", "workflow.focus_object", "canvas_focus", "field_diff"],
    observedGap: "The real workflow.preview_patch artifact has no server-authored workflow.focus_object action, so no Chat change item can initiate Canvas focus.",
  },
  "PW-006": {
    code: "REVISED_CANDIDATE_REAUTHORIZATION_MISSING",
    stage: "approval",
    requiredPath: ["preview_v1", "direct_revise_v2", "invalidate_v1", "approve_v2", "apply_v2"],
    observedGap: "Direct revise creates a new previewHash and correctly invalidates the old approval, but the current UI/provider cannot request an exact replacement approval for that revised Candidate.",
  },
  "PW-008": {
    code: "STALE_CANDIDATE_REBASE_UI_MISSING",
    stage: "candidate_workspace",
    requiredPath: ["candidate_preview", "disjoint_layout_commit", "safe_rebase", "preserve_layout"],
    observedGap: "After a disjoint committed revision arrives, the Canvas fail-closes the stale Candidate projection and disables direct revise, so the backend safe-rebase path is not reachable through the real UI.",
  },
  "PW-009": {
    code: "THREE_WAY_CONFLICT_RECOMPUTE_UI_MISSING",
    stage: "conflict_resolution",
    requiredPath: ["same_field_drift", "three_way_diff", "keep_user_value", "recompute_candidate"],
    observedGap: "The backend can return typed conflicting paths, but the real Candidate UI has no complete three-way choice and recompute journey that preserves the user's editable value.",
  },
  "PW-011": {
    code: "TRUSTED_SECRET_CONFIGURATION_INGRESS_MISSING",
    stage: "configuration",
    requiredPath: ["W5", "configuration_required", "trusted_secret_binding", "redacted_chat_dom_logs"],
    observedGap: "W5 now materializes http.request with secret_ref=null and no credential value. Production still lacks a server-owned configuration resolver, configuration_required dry-run result, and an authorized SecretRef selector/binder UI/API; test-only outcome or Secret injection cannot replace that path.",
  },
});

/**
 * PW-005: selecting a real ghost Candidate node must locate the exact Chat
 * change block. Setup itself creates a typed authoritative transaction so the
 * evidence covers CommitStore, receipt, event, AppSnapshot and DOM before the
 * RuntimeKernel/MCP preview path is exercised.
 *
 * @param {{page: import("@playwright/test").Page, request: import("@playwright/test").APIRequestContext}} fixtures
 * @param {import("@playwright/test").TestInfo} testInfo
 */
export async function locatesChatChangeFromGhostNode({ page, request }, testInfo) {
  assertFullStackSpecHasNoRouteMocks();
  await assertRealServerReady(request);

  const suffix = `pw005-${process.pid}-${testInfo.workerIndex}`;
  const fixture = await seedWorkflowFixture(request, "W1", suffix);
  const seeded = await readWorkflowGraph(request, fixture.name);
  const seedRevision = resourceVersion(seeded);
  expect(seedRevision).not.toBe("");

  const baselineLabel = "PW-005 committed baseline";
  const baseline = await commitManualWorkflowCommand(request, {
    workflowId: fixture.name,
    expectedRevision: seedRevision,
    idempotencyKey: `pw005-baseline-${process.pid}-${testInfo.workerIndex}`,
    reason: "PW-005 establishes an authoritative committed baseline before previewing a ghost change",
    command: { kind: "update_node", nodeId: "action", nodePatch: { label: baselineLabel } },
  });
  const baselineRevision = String(baseline.receipt.revisionAfter.resourceVersion);
  expect(baselineRevision).not.toBe(seedRevision);
  await expect.poll(async () => resourceVersion(await readWorkflowGraph(request, fixture.name))).toBe(baselineRevision);

  const guard = createRealPageGuard(page);
  await page.goto(`/runner/${encodeURIComponent(fixture.name)}`, { waitUntil: "domcontentloaded" });
  await expect(page.getByTestId("runner-studio-page")).toBeVisible();
  await expect(page.getByTestId("runner-studio-canvas")).toBeVisible();
  await expect(page.getByTestId("canvas-node-action")).toContainText(baselineLabel);
  await expect.poll(() => guard.latestWorkflowProjection()?.committed?.revision?.resourceVersion || "")
    .toBe(baselineRevision);

  const projection = currentProjection(guard, fixture.name);
  const candidateLabel = "PW-005 ghost candidate";
  await openWorkflowPanelTab(page, "ai");
  const panel = page.getByTestId("workflow-ai-transport-panel");
  await panel.getByTestId("workflow-ai-input").fill(workflowApplyPrompt(projection, {
    workflowId: fixture.name,
    nodeId: "action",
    label: candidateLabel,
  }));
  await panel.getByTestId("workflow-ai-send").click();

  const ghost = page.getByTestId("canvas-node-action");
  await expect(page.getByTestId("runner-candidate-canvas-status")).toContainText("尚未保存");
  await expect(ghost).toHaveAttribute("data-candidate-change", "changed");
  await expect(ghost).toContainText(candidateLabel);
  const previewArtifact = panel.locator('[data-testid="workflow-change-artifact"][data-change-kind="preview"]');
  await expect(previewArtifact).toHaveCount(1);
  await expect(panel.getByTestId("workflow-approval-card")).toHaveAttribute("data-approval-state", "pending");

  // Canvas -> Review -> Chat is entirely event/projector driven. Re-open the
  // AI tab after Candidate activation so the exact focused block is visible.
  await ghost.click();
  await expect(page.getByTestId("workflow-context-tab-review")).toHaveAttribute("aria-selected", "true");
  await expect(page.getByTestId("workflow-change-review")).toBeVisible();
  await page.getByTestId("workflow-context-tab-ai").click();
  const focused = panel.getByTestId("workflow-focused-change");
  await expect(focused).toBeVisible();
  await expect(focused).toHaveAttribute("aria-current", "true");
  await expect(focused).toHaveAttribute("data-workflow-change-item", /:0$/);
  await expect(focused).toContainText("已定位到第 1 项修改");
  await expect(focused.locator('[data-testid="workflow-change-artifact"][data-change-kind="preview"]')).toHaveCount(1);

  const committedAfter = await readWorkflowGraph(request, fixture.name);
  expect(resourceVersion(committedAfter)).toBe(baselineRevision);
  expect(nodeLabel(committedAfter, "action")).toBe(baselineLabel);
  await guard.assertHealthy();
  await attachFullStackEvidence(page, testInfo, {
    caseId: "PW-005",
    workflowId: fixture.name,
    revisionBeforeBaseline: seedRevision,
    revisionCommitted: baselineRevision,
    baselineTransactionId: baseline.receipt.transactionId,
    baselineEventId: baseline.event.eventId,
    baselineEventSequence: baseline.event.sequence,
    candidateCommitted: false,
    candidateNodeId: "action",
    candidateLabel,
    focusedChangeItem: await focused.getAttribute("data-workflow-change-item"),
    path: "Canvas ghost -> WorkflowSurfaceFocus -> exact AssistantTransport artifact block",
  });
}

/**
 * PW-012: History recovery is a normal controlled Runtime approval journey.
 * The browser submits only the persisted Candidate selector, confirms through
 * the Workspace-owned final Dialog and AssistantTransport, then observes the
 * durable undo/redo transactions after reload; it never sends a mutation
 * binding or ActionToken.
 *
 * @param {{page: import("@playwright/test").Page, request: import("@playwright/test").APIRequestContext}} fixtures
 * @param {import("@playwright/test").TestInfo} testInfo
 */
export async function completesHistoryUndoRedoApprovalJourney({ page, request }, testInfo) {
  assertFullStackSpecHasNoRouteMocks();
  await assertRealServerReady(request);

  const suffix = `pw012-${process.pid}-${testInfo.workerIndex}`;
  const fixture = await seedWorkflowFixture(request, "W1", suffix);
  const initial = await readWorkflowGraph(request, fixture.name);
  const initialRevision = resourceVersion(initial);
  const firstLabel = "PW-012 first durable value";
  const secondLabel = "PW-012 second durable value";
  const first = await commitManualWorkflowCommand(request, {
    workflowId: fixture.name,
    expectedRevision: initialRevision,
    idempotencyKey: `pw012-first-${process.pid}-${testInfo.workerIndex}`,
    reason: "PW-012 establishes the first durable value",
    command: { kind: "update_node", nodeId: "action", nodePatch: { label: firstLabel } },
  });
  const firstRevision = String(first.receipt.revisionAfter.resourceVersion);
  const second = await commitManualWorkflowCommand(request, {
    workflowId: fixture.name,
    expectedRevision: firstRevision,
    idempotencyKey: `pw012-second-${process.pid}-${testInfo.workerIndex}`,
    reason: "PW-012 establishes the second durable value",
    command: { kind: "update_node", nodeId: "action", nodePatch: { label: secondLabel } },
  });
  const secondRevision = String(second.receipt.revisionAfter.resourceVersion);

  const guard = createRealPageGuard(page);
  await page.goto(`/runner/${encodeURIComponent(fixture.name)}`, { waitUntil: "domcontentloaded" });
  await expect(page.getByTestId("runner-studio-page")).toBeVisible();
  await expect(page.getByTestId("canvas-node-action")).toContainText(secondLabel);
  await expect.poll(() => guard.latestWorkflowProjection()?.committed?.revision?.resourceVersion || "").toBe(secondRevision);

  // Establish the exact Workflow AssistantTransport session used by the
  // server-owned controlled approval turn.
  await openWorkflowPanelTab(page, "ai");
  const panel = page.getByTestId("workflow-ai-transport-panel");
  await panel.getByTestId("workflow-ai-input").fill(
    `WORKFLOW_MCP_READ tenant=runner-default workflow=${fixture.name} revision=${secondRevision} 请读取当前工作流摘要`,
  );
  await panel.getByTestId("workflow-ai-send").click();
  await expect(panel).toContainText("RECORDED_WORKFLOW_MCP_READ_OK");

  let approvalRequests = 0;
  let approvalDecisions = 0;
  const approvalRequestBodies = [];
  page.on("request", (outgoing) => {
    if (outgoing.method() !== "POST") return;
    if (outgoing.url().endsWith("/api/runner-studio/workflow-candidate-apply-approval")) {
      approvalRequests += 1;
      approvalRequestBodies.push(outgoing.postDataJSON());
    }
    if (outgoing.url().endsWith("/api/v1/assistant/transport")) {
      try {
        if (outgoing.postDataJSON()?.commands?.some((command) => command?.type === "aiops.approval-decision")) approvalDecisions += 1;
      } catch {
        // Invalid transport JSON is asserted by the missing decision below.
      }
    }
  });

  await openWorkflowPanelTab(page, "changes");
  const secondEntry = page.locator(`[data-transaction-id="${second.receipt.transactionId}"]`);
  await expect(secondEntry).toBeVisible();
  await secondEntry.getByRole("button", { name: "撤销", exact: true }).click();
  await expect(page.getByTestId("runner-candidate-canvas-status")).toContainText("尚未保存");
  expect(resourceVersion(await readWorkflowGraph(request, fixture.name))).toBe(secondRevision);

  const undoApprovalResponse = page.waitForResponse((response) => (
    response.url().endsWith("/api/runner-studio/workflow-candidate-apply-approval")
    && response.request().method() === "POST"
  ));
  await page.getByTestId("workflow-candidate-request-approval").click();
  expect((await undoApprovalResponse).status()).toBe(202);
  await confirmCandidateApplyDialog(page);
  await expect.poll(async () => resourceVersion(await readWorkflowGraph(request, fixture.name))).not.toBe(secondRevision);
  const afterUndo = await readWorkflowGraph(request, fixture.name);
  const undoRevision = resourceVersion(afterUndo);
  expect(nodeLabel(afterUndo, "action")).toBe(firstLabel);
  await expect(page.getByTestId("runner-candidate-canvas-status")).toHaveCount(0);

  // Reload proves the undo receipt and history lineage are durable, then the
  // real Redo button creates a fresh Candidate and a fresh exact approval.
  await page.reload({ waitUntil: "domcontentloaded" });
  await expect(page.getByTestId("runner-studio-page")).toBeVisible();
  await expect(page.getByTestId("canvas-node-action")).toContainText(firstLabel);
  await expect.poll(() => guard.latestWorkflowProjection()?.committed?.revision?.resourceVersion || "").toBe(undoRevision);
  await openWorkflowPanelTab(page, "changes");
  const redoEntry = page.locator("li[data-transaction-id]").filter({
    has: page.locator("button", { hasText: "重做" }),
  });
  await expect(redoEntry).toHaveCount(1);
  const undoTransactionId = await redoEntry.getAttribute("data-transaction-id");
  expect(undoTransactionId).toBeTruthy();
  await redoEntry.locator("summary").click();
  const redoButton = redoEntry.getByRole("button", { name: "重做", exact: true });
  await expect(redoButton).toBeVisible();
  await redoButton.click();
  await expect(page.getByTestId("runner-candidate-canvas-status")).toContainText("尚未保存");
  expect(resourceVersion(await readWorkflowGraph(request, fixture.name))).toBe(undoRevision);

  const redoApprovalResponse = page.waitForResponse((response) => (
    response.url().endsWith("/api/runner-studio/workflow-candidate-apply-approval")
    && response.request().method() === "POST"
  ));
  await page.getByTestId("workflow-candidate-request-approval").click();
  expect((await redoApprovalResponse).status()).toBe(202);
  await confirmCandidateApplyDialog(page);
  await expect.poll(async () => resourceVersion(await readWorkflowGraph(request, fixture.name))).not.toBe(undoRevision);
  const afterRedo = await readWorkflowGraph(request, fixture.name);
  const redoRevision = resourceVersion(afterRedo);
  expect(nodeLabel(afterRedo, "action")).toBe(secondLabel);
  await expect(page.getByTestId("runner-candidate-canvas-status")).toHaveCount(0);

  await page.reload({ waitUntil: "domcontentloaded" });
  await expect(page.getByTestId("canvas-node-action")).toContainText(secondLabel);
  await expect.poll(() => guard.latestWorkflowProjection()?.committed?.revision?.resourceVersion || "").toBe(redoRevision);
  expect(approvalRequests).toBe(2);
  expect(approvalDecisions).toBe(2);
  for (const body of approvalRequestBodies) {
    expect(Object.keys(body).sort()).toEqual([
      "action", "candidateId", "candidateRevision", "idempotencyKey", "schemaVersion", "workflowId",
    ]);
    expect(body).not.toHaveProperty("changeSetId");
    expect(body).not.toHaveProperty("previewHash");
    expect(body).not.toHaveProperty("actionToken");
  }
  await guard.assertHealthy();
  await attachFullStackEvidence(page, testInfo, {
    caseId: "PW-012",
    workflowId: fixture.name,
    sourceTransactions: [first.receipt.transactionId, second.receipt.transactionId],
    undoTransactionId,
    revisions: { initialRevision, firstRevision, secondRevision, undoRevision, redoRevision },
    values: { firstLabel, secondLabel },
    approvalRequests,
    approvalDecisions,
    browserMutationAuthorityFields: [],
    durableAcrossReload: true,
  });
}

/**
 * Confirms the exact live Candidate through the Workspace-owned final Dialog.
 *
 * @param {import("@playwright/test").Page} page
 */
async function confirmCandidateApplyDialog(page) {
  const applyOwner = page.getByTestId("workflow-candidate-workspace-actions");
  const apply = applyOwner.getByTestId("workflow-candidate-apply");
  await expect(apply).toBeEnabled();
  await apply.click();
  const dialog = page.getByTestId("workflow-candidate-apply-dialog");
  await expect(dialog).toBeVisible();
  const confirm = dialog.getByTestId("workflow-candidate-confirm-apply");
  await expect(confirm).toBeFocused();
  await expect.poll(() => applyOwner.evaluate((element) => element.hasAttribute("inert"))).toBe(true);
  await confirm.click();
}

/**
 * Opens the shared Workflow Workbench when needed and selects one of its
 * current three top-level tasks. This deliberately avoids removed
 * toolbar/history compatibility selectors.
 *
 * @param {import("@playwright/test").Page} page
 * @param {"ai" | "changes" | "run"} tab
 */
async function openWorkflowPanelTab(page, tab) {
  const dock = page.getByTestId("workflow-context-dock");
  if (!await dock.isVisible()) {
    await page.getByTestId("workflow-workbench-trigger").click();
  }
  await page.getByTestId(`workflow-context-tab-${tab}`).click();
}

function currentProjection(guard, workflowId) {
  const projection = guard.latestWorkflowProjection();
  const projectedWorkflowId = projection?.committed?.workflowKey?.workflowId || projection?.workflowKey?.workflowId;
  if (!projection || projectedWorkflowId !== workflowId) throw new Error(`no live Workflow projection for ${workflowId}`);
  fullRevision(projection);
  return projection;
}

function workflowApplyPrompt(projection, { workflowId, nodeId, label }) {
  const revision = fullRevision(projection);
  const environment = projection?.runGateFacts?.environment?.bindingVersion;
  if (typeof environment !== "string" || !environment.trim()) {
    throw new Error("PW-005 requires the server-owned environment binding version");
  }
  return [
    "WORKFLOW_MCP_APPLY",
    "tenant=runner-default",
    `workflow=${workflowId}`,
    `revision=${revision.resourceVersion}`,
    `environment=${environment}`,
    `node=${nodeId}`,
    `label_b64=${Buffer.from(label, "utf8").toString("base64url")}`,
  ].join(" ");
}

function nodeLabel(payload, nodeId) {
  const nodes = payload?.nodes || payload?.graph?.nodes || [];
  return String(nodes.find((node) => node?.id === nodeId)?.label || "");
}

function deepFreeze(value) {
  Object.values(value).forEach((item) => {
    if (item && typeof item === "object") deepFreeze(item);
  });
  return Object.freeze(value);
}
