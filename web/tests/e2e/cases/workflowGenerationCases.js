// @ts-check

import { expect } from "@playwright/test";

import {
  assertFullStackSpecHasNoRouteMocks,
  assertRealServerReady,
  attachFullStackEvidence,
  createRealPageGuard,
  fullRevision,
  readWorkflowGraph,
  resourceVersion,
  seedWorkflowFixture,
} from "../../helpers/workflowCanvasServerHarness.js";

/**
 * Real full-stack generation Cases that are supportable by the current
 * production path. These functions use the visible Workflow UI and the real
 * ai-server/RuntimeKernel/MCP/CommitStore path; setup is the only direct API
 * use and no Playwright route, HAR, target API mock, or fixed wait is used.
 */
export const workflowGenerationCaseImplementations = Object.freeze({
  "PW-002": changesOnlySelectedThreshold,
  "PW-006": appliesLatestDirectRevisionOnly,
  "PW-015": replacesCandidateAfterTypedPreviewFailure,
});

/**
 * A blocked Case stays absent from the implementation map. The evidence below
 * names the concrete production boundary; it must not be replaced by a
 * provider-only protocol that would make a mocked journey look green.
 */
export const workflowGenerationCaseBlockers = deepFreeze({
  "PW-001": {
    code: "AUTHORITATIVE_W0_REVISION_UNAVAILABLE",
    stage: "fixture_and_plan",
    requiredPath: ["authoritative_W0", "workflow.compile_plan", "review_plan", "preview_patch", "apply_patch"],
    observedGap: "W0 contains zero nodes, but the authoritative Visual Workflow create path rejects an empty graph before it can acquire the revision required by Workflow Chat. A local blank draft has no server revision and therefore cannot enter the MCP path.",
    codeEvidence: [
      "testdata/workflow_canvas_mcp/fixtures/W0.json:data.workflow.nodes=[]",
      "web/tests/helpers/workflowCanvasServerHarness.js:seedWorkflowFixture posts W0 unchanged",
      "pkg/runner/workflow/visual/validate.go:validateGraph emits nodes_required/start_required/executable_node_required",
    ],
  },
  "PW-029": {
    code: "ACTIVE_TURN_CONSTRAINT_CONSUMER_MISSING",
    stage: "runtime_steering",
    requiredPath: ["generation_in_flight", "pending_constraint", "invalidate_old_candidate", "preview_with_constraint"],
    observedGap: "The composer remains usable and RuntimeKernel persists the second message as PendingInputs, but the active model/tool loop never consumes PendingInputs. The constraint therefore cannot invalidate the old Candidate or participate in a new Preview.",
    codeEvidence: [
      "web/src/runner/WorkflowAiTransportPanel.tsx:workflow-ai-input/send are not disabled by active turn",
      "internal/runtimekernel/runtime_kernel.go:appendPendingInputToActiveTurn only appends and persists PendingInputs",
      "internal/runtimekernel/types.go:TurnSnapshot.PendingInputs has no active-loop consumption path",
    ],
  },
  "PW-030": {
    code: "QUEUED_POST_APPLY_TURN_CONSUMER_MISSING",
    stage: "apply_queue",
    requiredPath: ["apply_in_flight", "visible_queued_edit", "apply_receipt", "start_next_turn"],
    observedGap: "A second message can be recorded as PendingInputs while a regular turn is active, but it is neither projected as a user-visible queued edit nor dequeued into a new turn after the Apply receipt. The recorded provider boundary also cannot delay the synchronous workflow.apply_patch execution/receipt without mocking the target API.",
    codeEvidence: [
      "internal/runtimekernel/runtime_kernel.go:runningRegularTurnForPendingInput/appendPendingInputToActiveTurn",
      "internal/appui/transport_commands.go:applyAddMessage marks the current turn active but exposes no post-receipt dequeue",
      "web/src/runner/WorkflowAiTransportPanel.tsx:no PendingInputs queue presentation",
    ],
  },
});

/**
 * PW-002: the selected condition node is frozen into the sent Turn. The real
 * recorded provider emits one update_node.inputs operation, approval commits
 * it through RuntimeKernel, and every non-threshold graph field stays equal.
 *
 * @param {{page: import("@playwright/test").Page, request: import("@playwright/test").APIRequestContext}} fixtures
 * @param {import("@playwright/test").TestInfo} testInfo
 */
export async function changesOnlySelectedThreshold({ page, request }, testInfo) {
  assertFullStackSpecHasNoRouteMocks();
  await assertRealServerReady(request);
  const fixture = await seedValidW2(request, `pw002-${process.pid}-${testInfo.workerIndex}`);
  const before = await readWorkflowGraph(request, fixture.name);
  const revisionBefore = resourceVersion(before);
  expect(revisionBefore).not.toBe("");
  expect(nodeThreshold(before, "condition")).toBe(80);

  const guard = createRealPageGuard(page);
  await page.goto(`/runner/${encodeURIComponent(fixture.name)}`, { waitUntil: "domcontentloaded" });
  await expect(page.getByTestId("runner-studio-page")).toBeVisible();
  await expect(page.getByTestId("canvas-node-condition")).toBeVisible();
  await expect.poll(() => guard.latestWorkflowProjection()?.committed?.revision?.resourceVersion || "")
    .toBe(revisionBefore);

  await page.getByTestId("canvas-node-condition").click();
  await page.getByTestId("workflow-workbench-trigger").click();
  await page.getByTestId("workflow-context-tab-ai").click();
  const panel = page.getByTestId("workflow-ai-transport-panel");
  await expect(panel.getByTestId("workflow-scope-selection")).toHaveCount(0);

  const thresholdAfter = 95;
  const projection = currentProjection(guard, fixture.name);
  await panel.getByTestId("workflow-ai-input").fill(workflowApplyPrompt(projection, {
    workflowId: fixture.name,
    nodeId: "condition",
    operation: "threshold",
    threshold: thresholdAfter,
  }));
  await panel.getByTestId("workflow-ai-send").click();

  // Changing live selection after Send proves the Turn uses its frozen scope.
  await page.getByTestId("canvas-node-success").click();
  await page.getByTestId("runner-toolbar-ai-generate").click();
  await expect(page.getByTestId("runner-candidate-canvas-status")).toContainText("尚未保存");
  await page.getByTestId("workflow-context-tab-ai").click();
  const sentTurn = panel.locator("article[data-turn-id]").last();
  await expect(sentTurn).toContainText("本轮基于：condition");
  await expect(sentTurn).not.toContainText("本轮基于：success");
  await expect(panel.getByTestId("workflow-approval-card").last()).toHaveAttribute("data-approval-state", "pending");
  expect(resourceVersion(await readWorkflowGraph(request, fixture.name))).toBe(revisionBefore);
  expect(nodeThreshold(await readWorkflowGraph(request, fixture.name), "condition")).toBe(80);

  await panel.getByTestId("workflow-approval-accept").last().click();
  await expect(panel).toContainText("RECORDED_WORKFLOW_MCP_APPLY_OK", { timeout: 15_000 });
  await expect.poll(async () => resourceVersion(await readWorkflowGraph(request, fixture.name))).not.toBe(revisionBefore);
  const after = await readWorkflowGraph(request, fixture.name);
  const revisionAfter = resourceVersion(after);
  expect(nodeThreshold(after, "condition")).toBe(thresholdAfter);
  expectOnlyThresholdChanged(before, after, "condition");

  await page.reload({ waitUntil: "domcontentloaded" });
  await expect(page.getByTestId("canvas-node-condition")).toBeVisible();
  await expect.poll(() => guard.latestWorkflowProjection()?.committed?.revision?.resourceVersion || "")
    .toBe(revisionAfter);
  expect(nodeThreshold(await readWorkflowGraph(request, fixture.name), "condition")).toBe(thresholdAfter);
  await guard.assertHealthy();
  await attachFullStackEvidence(page, testInfo, {
    caseId: "PW-002",
    fixtureId: "W2",
    workflowId: fixture.name,
    frozenSelection: ["condition"],
    liveSelectionAfterSend: ["success"],
    thresholdBefore: 80,
    thresholdAfter,
    revisionBefore,
    revisionAfter,
    onlyTargetFieldChanged: true,
    durableAcrossReload: true,
  });
}

/**
 * PW-006: RuntimeKernel creates Agent Candidate v1 without an Apply call. A
 * visible keyboard move persists Candidate v2, then a node-inspector edit
 * persists Candidate v3. The first explicit Apply requests one server-owned
 * approval bound to the exact latest identity; once that live approval arrives,
 * the same Workspace owner opens the final Dialog. Only v3 may commit.
 *
 * @param {{page: import("@playwright/test").Page, request: import("@playwright/test").APIRequestContext}} fixtures
 * @param {import("@playwright/test").TestInfo} testInfo
 */
export async function appliesLatestDirectRevisionOnly({ page, request }, testInfo) {
  assertFullStackSpecHasNoRouteMocks();
  await assertRealServerReady(request);
  const fixture = await seedWorkflowFixture(request, "W1", `pw006-${process.pid}-${testInfo.workerIndex}`);
  const before = await readWorkflowGraph(request, fixture.name);
  const revisionBefore = resourceVersion(before);
  const committedLabel = nodeLabel(before, "action");
  const committedScript = nodeScript(before, "action");
  const committedPosition = nodePosition(before, "action");
  expect(revisionBefore).not.toBe("");

  const guard = createRealPageGuard(page);
  await page.goto(`/runner/${encodeURIComponent(fixture.name)}`, { waitUntil: "domcontentloaded" });
  await expect(page.getByTestId("runner-studio-page")).toBeVisible();
  await expect(page.getByTestId("canvas-node-action")).toBeVisible();
  await expect.poll(() => guard.latestWorkflowProjection()?.committed?.revision?.resourceVersion || "")
    .toBe(revisionBefore);

  await page.getByTestId("workflow-workbench-trigger").click();
  await page.getByTestId("workflow-context-tab-ai").click();
  const panel = page.getByTestId("workflow-ai-transport-panel");
  const candidateV1Label = "PW-006 Agent Candidate v1";
  await panel.getByTestId("workflow-ai-input").fill(workflowPreviewPrompt(
    currentProjection(guard, fixture.name),
    { workflowId: fixture.name, nodeId: "action", label: candidateV1Label },
  ));
  await panel.getByTestId("workflow-ai-send").click();
  await expect(panel).toContainText("RECORDED_WORKFLOW_MCP_PREVIEW_OK");
  await expect(page.getByTestId("runner-candidate-canvas-status")).toContainText("候选版本 1");
  await expect(page.getByTestId("canvas-node-action")).toContainText(candidateV1Label);
  await expect(panel.getByTestId("workflow-approval-card")).toHaveCount(0);

  // Candidate v1 is only a Ghost projection. The durable graph stays intact.
  let committed = await readWorkflowGraph(request, fixture.name);
  expect(resourceVersion(committed)).toBe(revisionBefore);
  expect(nodeLabel(committed, "action")).toBe(committedLabel);
  expect(nodeScript(committed, "action")).toBe(committedScript);

  let approvalRequestCount = 0;
  let approvalDecisionCount = 0;
  let reviseRequestCount = 0;
  page.on("request", (outgoing) => {
    if (outgoing.method() === "POST"
      && outgoing.url().endsWith("/api/runner-studio/workflow-candidate-revise")) {
      reviseRequestCount += 1;
    }
    if (outgoing.method() === "POST"
      && outgoing.url().endsWith("/api/runner-studio/workflow-candidate-apply-approval")) {
      approvalRequestCount += 1;
    }
    if (outgoing.method() === "POST"
      && outgoing.url().endsWith("/api/v1/assistant/transport")) {
      try {
        if (outgoing.postDataJSON()?.commands?.some((command) => command?.type === "aiops.approval-decision")) {
          approvalDecisionCount += 1;
        }
      } catch {
        // Invalid transport JSON is asserted by the missing decision below.
      }
    }
  });

  // First visible revise: change a node parameter through the real inspector.
  // It persists v2 but must not request approval yet.
  const candidateV3Script = "set -e\necho pw006-user-revised-v3";
  await page.getByTestId("canvas-node-action").click();
  await page.keyboard.press("Enter");
  await page.getByTestId("runner-node-panel-tab-script").click();
  await page.getByTestId("runner-node-script-editor").fill(candidateV3Script);
  const reviseV2ResponsePromise = page.waitForResponse((response) => (
    response.url().endsWith("/api/runner-studio/workflow-candidate-revise")
    && response.request().method() === "POST"
  ));
  await page.getByTestId("runner-node-panel-apply").click();
  const reviseV2Response = await reviseV2ResponsePromise;
  expect(reviseV2Response.status()).toBe(200);
  const revisedV2 = await reviseV2Response.json();
  expect(revisedV2).toMatchObject({
    kind: "preview",
    candidateRevision: 2,
    candidate: { currentRevision: 2 },
    changeSet: { candidateRevision: 2, actor: "user", parentChangeSetId: expect.any(String) },
  });
  await expect(page.getByTestId("runner-candidate-canvas-status")).toContainText("候选版本 2");
  expect(approvalRequestCount).toBe(0);

  // Second visible revise: nudge the Ghost node. React Flow emits one
  // authoritative update_layout operation chained from v2, still without an
  // approval until the operator explicitly Applies.
  const reviseV3ResponsePromise = page.waitForResponse((response) => (
    response.url().endsWith("/api/runner-studio/workflow-candidate-revise")
    && response.request().method() === "POST"
  ));
  await page.getByTestId("canvas-node-action").click();
  await page.keyboard.press("ArrowRight");
  const reviseV3Response = await reviseV3ResponsePromise;
  expect(reviseV3Response.status()).toBe(200);
  const revisedV3 = await reviseV3Response.json();
  expect(revisedV3).toMatchObject({
    kind: "preview",
    schemaVersion: "aiops.workflow.edit.v2",
    candidateRevision: 3,
    candidate: { currentRevision: 3 },
    changeSet: {
      candidateRevision: 3,
      actor: "user",
      parentChangeSetId: revisedV2.changeSet.id,
      base: fullRevision(currentProjection(guard, fixture.name)),
    },
  });
  const v3Layout = revisedV3.changeSet.operations.find((operation) => operation?.op === "update_layout");
  const movedAction = v3Layout?.layout?.nodes?.find((node) => node?.nodeId === "action");
  expect(movedAction?.position).toBeTruthy();
  expect(movedAction.position).not.toEqual(committedPosition);
  await expect(page.getByTestId("runner-candidate-canvas-status")).toContainText("候选版本 3");
  expect(reviseRequestCount).toBe(2);
  expect(approvalRequestCount).toBe(0);

  // The workspace shortcut emits the exact latest Candidate identity. The
  // Provider never sends mutation bindings from the browser.
  const approvalResponsePromise = page.waitForResponse((response) => (
    response.url().endsWith("/api/runner-studio/workflow-candidate-apply-approval")
    && response.request().method() === "POST"
  ));
  await page.getByTestId("runner-studio-page").click({ position: { x: 8, y: 8 } });
  await page.keyboard.press("Meta+Enter");

  const approvalResponse = await approvalResponsePromise;
  expect(approvalResponse.status()).toBe(202);
  const approvalRequest = approvalResponse.request().postDataJSON();
  expect(Object.keys(approvalRequest).sort()).toEqual([
    "action", "candidateId", "candidateRevision", "idempotencyKey", "schemaVersion", "workflowId",
  ]);
  expect(approvalRequest).toMatchObject({
    action: "request_apply_approval",
    candidateId: revisedV3.candidate.id,
    candidateRevision: 3,
    schemaVersion: "aiops.workflow.edit.v2",
    workflowId: fixture.name,
  });
  expect(approvalRequest).not.toHaveProperty("changeSetId");
  expect(approvalRequest).not.toHaveProperty("previewHash");
  expect(approvalRequest).not.toHaveProperty("expectedRevision");
  expect(approvalRequest).not.toHaveProperty("actionToken");
  const approval = await approvalResponse.json();
  expect(approval).toMatchObject({
    kind: "workflow_apply_approval",
    status: "blocked",
    candidate: {
      candidateId: revisedV3.candidate.id,
      candidateRevision: 3,
      changeSetId: revisedV3.changeSet.id,
      previewHash: revisedV3.changeSet.previewHash,
      expectedRevision: revisionBefore,
    },
  });
  expect(approvalRequestCount).toBe(1);
  await expect(page.getByTestId("runner-candidate-canvas-status")).toContainText("候选版本 3");

  // No superseded Candidate has mutated durable state before exact v3 approval.
  committed = await readWorkflowGraph(request, fixture.name);
  expect(resourceVersion(committed)).toBe(revisionBefore);
  expect(nodeLabel(committed, "action")).toBe(committedLabel);
  expect(nodeScript(committed, "action")).toBe(committedScript);

  const applyOwner = page.getByTestId("workflow-candidate-workspace-actions");
  const apply = applyOwner.getByTestId("workflow-candidate-apply");
  await expect(apply).toBeEnabled();
  await apply.click();
  const applyDialog = page.getByTestId("workflow-candidate-apply-dialog");
  await expect(applyDialog).toBeVisible();
  const confirmApply = applyDialog.getByTestId("workflow-candidate-confirm-apply");
  await expect(confirmApply).toBeFocused();
  await expect.poll(() => applyOwner.evaluate((element) => element.hasAttribute("inert"))).toBe(true);
  await page.keyboard.press("Escape");
  await expect(applyDialog).toBeHidden();
  await expect(apply).toBeFocused();
  await expect.poll(() => applyOwner.evaluate((element) => element.hasAttribute("inert"))).toBe(false);
  await apply.click();
  await expect(applyDialog).toBeVisible();
  await expect(confirmApply).toBeFocused();
  await confirmApply.click();

  await expect.poll(async () => resourceVersion(await readWorkflowGraph(request, fixture.name)))
    .not.toBe(revisionBefore);
  const after = await readWorkflowGraph(request, fixture.name);
  const revisionAfter = resourceVersion(after);
  expect(nodeLabel(after, "action")).toBe(candidateV1Label);
  expect(nodeScript(after, "action")).toBe(candidateV3Script);
  expect(nodePosition(after, "action")).toEqual(movedAction.position);
  await expect(page.getByTestId("runner-candidate-canvas-status")).toHaveCount(0);

  await page.reload({ waitUntil: "domcontentloaded" });
  await expect(page.getByTestId("canvas-node-action")).toContainText(candidateV1Label);
  await expect.poll(() => guard.latestWorkflowProjection()?.committed?.revision?.resourceVersion || "")
    .toBe(revisionAfter);
  const reloaded = await readWorkflowGraph(request, fixture.name);
  expect(nodeScript(reloaded, "action")).toBe(candidateV3Script);
  expect(nodePosition(reloaded, "action")).toEqual(movedAction.position);
  expect(approvalRequestCount).toBe(1);
  expect(approvalDecisionCount).toBe(1);
  await guard.assertHealthy();
  await attachFullStackEvidence(page, testInfo, {
    caseId: "PW-006",
    fixtureId: "W1",
    workflowId: fixture.name,
    revisionBefore,
    revisionAfter,
    candidateId: revisedV3.candidate.id,
    candidateRevisionBefore: 1,
    candidateRevisionApplied: 3,
    changeSetIdApplied: revisedV3.changeSet.id,
    previewHashApplied: revisedV3.changeSet.previewHash,
    approvalId: approval.approval.approvalId,
    approvalRequestCount,
    approvalDecisionCount,
    reviseRequestCount,
    visibleDirectRevisions: ["update_layout", "update_node"],
    browserMutationAuthorityFields: [],
    committedBeforeApproval: false,
    durableAcrossReload: true,
  });
}

/**
 * PW-015: a failed Preview in a later real Runtime turn must retain the old
 * Ghost Candidate. A third successful turn in the same workflow:* transport
 * thread must then replace it with a distinct current Candidate.
 *
 * @param {{page: import("@playwright/test").Page, request: import("@playwright/test").APIRequestContext}} fixtures
 * @param {import("@playwright/test").TestInfo} testInfo
 */
export async function replacesCandidateAfterTypedPreviewFailure({ page, request }, testInfo) {
  assertFullStackSpecHasNoRouteMocks();
  await assertRealServerReady(request);
  const fixture = await seedWorkflowFixture(request, "W1", `pw015-${process.pid}-${testInfo.workerIndex}`);
  const before = await readWorkflowGraph(request, fixture.name);
  const revisionBefore = resourceVersion(before);
  const committedLabel = nodeLabel(before, "action");
  const guard = createRealPageGuard(page);

  await page.goto(`/runner/${encodeURIComponent(fixture.name)}`, { waitUntil: "domcontentloaded" });
  await expect(page.getByTestId("runner-studio-page")).toBeVisible();
  await expect(page.getByTestId("canvas-node-action")).toBeVisible();
  await expect.poll(() => guard.latestWorkflowProjection()?.committed?.revision?.resourceVersion || "")
    .toBe(revisionBefore);
  await page.getByTestId("runner-toolbar-ai-generate").click();
  const panel = page.getByTestId("workflow-ai-transport-panel");

  const firstLabel = "PW-015 retained Candidate v1";
  await sendPreviewTurn(panel, workflowPreviewPrompt(currentProjection(guard, fixture.name), {
    workflowId: fixture.name, nodeId: "action", label: firstLabel,
  }));
  await expect(panel).toContainText("RECORDED_WORKFLOW_MCP_PREVIEW_OK");
  await expect(page.getByTestId("canvas-node-action")).toContainText(firstLabel);
  await expect(page.getByTestId("runner-candidate-canvas-status")).toContainText("候选版本 1");
  await page.getByTestId("workflow-context-tab-ai").click();
  const firstTurn = panel.locator("article[data-turn-id]").last();
  const firstCandidateDisplayId = await visibleCandidateDisplayId(firstTurn);

  const failedLabel = "PW-015 failed Preview must never project";
  await sendPreviewTurn(panel, workflowPreviewPrompt(currentProjection(guard, fixture.name), {
    workflowId: fixture.name, nodeId: "action", label: failedLabel, operation: "invalid",
  }));
  await expect(panel).toContainText("Workflow repair budget exhausted");
  await expect(panel).toContainText("The Candidate is retained");
  await expect(page.getByTestId("canvas-node-action")).toContainText(firstLabel);
  await expect(page.getByTestId("canvas-node-action")).not.toContainText(failedLabel);
  await expect(page.getByTestId("runner-candidate-canvas-status")).toContainText("候选版本 1");
  let committed = await readWorkflowGraph(request, fixture.name);
  expect(resourceVersion(committed)).toBe(revisionBefore);
  expect(nodeLabel(committed, "action")).toBe(committedLabel);

  const replacementLabel = "PW-015 replacement Candidate v2";
  await page.getByTestId("workflow-context-tab-ai").click();
  const thirdAddMessage = page.waitForRequest((outgoing) => {
    if (outgoing.method() !== "POST" || !outgoing.url().endsWith("/api/v1/assistant/transport")) return false;
    try {
      return outgoing.postDataJSON()?.commands?.some((command) => command?.type === "add-message") === true;
    } catch {
      return false;
    }
  });
  await panel.getByTestId("workflow-ai-input").fill(workflowPreviewPrompt(currentProjection(guard, fixture.name), {
    workflowId: fixture.name, nodeId: "action", label: replacementLabel,
  }));
  await panel.getByTestId("workflow-ai-send").click();
  const thirdRequest = await thirdAddMessage;
  expect(thirdRequest.postDataJSON()?.state?.threadId).toBe(`workflow:${fixture.name}`);
  await expect(panel).toContainText("RECORDED_WORKFLOW_MCP_PREVIEW_OK");
  await expect(page.getByTestId("canvas-node-action")).toContainText(replacementLabel);
  await expect(page.getByTestId("canvas-node-action")).not.toContainText(firstLabel);
  await expect(page.getByTestId("canvas-node-action")).not.toContainText(failedLabel);
  await page.getByTestId("workflow-context-tab-ai").click();
  const replacementTurn = panel.locator("article[data-turn-id]").last();
  const replacementCandidateDisplayId = await visibleCandidateDisplayId(replacementTurn);
  expect(replacementCandidateDisplayId).not.toBe(firstCandidateDisplayId);

  committed = await readWorkflowGraph(request, fixture.name);
  expect(resourceVersion(committed)).toBe(revisionBefore);
  expect(nodeLabel(committed, "action")).toBe(committedLabel);
  await guard.assertHealthy();
  await attachFullStackEvidence(page, testInfo, {
    caseId: "PW-015",
    fixtureId: "W1",
    workflowId: fixture.name,
    threadId: thirdRequest.postDataJSON()?.state?.threadId,
    revisionBefore,
    revisionAfter: resourceVersion(committed),
    firstCandidateDisplayId,
    replacementCandidateDisplayId,
    retainedCandidateAfterFailure: true,
    replacementProjected: true,
    failedCandidateProjected: false,
    committedMutationCount: 0,
  });
}

async function sendPreviewTurn(panel, prompt) {
  await panel.getByTestId("workflow-ai-input").fill(prompt);
  await panel.getByTestId("workflow-ai-send").click();
  await expect(panel.getByTestId("workflow-ai-input")).toBeEnabled();
}

async function visibleCandidateDisplayId(turn) {
  const artifact = turn.locator('[data-testid="workflow-change-artifact"][data-change-kind="preview"]').last();
  await artifact.scrollIntoViewIfNeeded();
  await expect(artifact).toBeVisible();
  const technical = artifact.getByTestId("workflow-change-technical");
  await technical.locator("summary").click();
  const candidateId = technical.locator("dt", { hasText: "候选 ID" }).locator("xpath=following-sibling::dd");
  await expect(candidateId).toBeVisible();
  return String(await candidateId.textContent()).trim();
}

async function seedValidW2(request, suffix) {
  const name = `complete-branch-${suffix}`.replace(/[^a-zA-Z0-9_.-]/g, "-");
  const graph = {
    version: "v1",
    workflow: {
      version: "v0.1",
      name,
      description: "full-stack valid W2 branch fixture",
      validation_env: "staging",
    },
    nodes: [
      {
        id: "start", type: "start", label: "Start", position: { x: 80, y: 220 },
        ports: [{ id: "next", type: "output", label: "next" }],
      },
      {
        id: "condition", type: "condition", label: "Threshold condition", position: { x: 340, y: 220 },
        step_name: "condition", step_id: "condition",
        ports: [
          { id: "in", type: "input", label: "in" },
          { id: "true", type: "output", label: "true" },
          { id: "false", type: "output", label: "false" },
        ],
        step: { name: "condition", action: "script.shell", args: { script: "set -e\ntest 80 -ge 80" } },
        condition: { if: "inputs.threshold >= 80", else: true },
        inputs: [{
          key: "threshold", type: "integer", label: "threshold", required: true,
          value_source: { type: "literal", value: 80 },
        }],
      },
      actionNode("success", "Success branch", 620, 100),
      actionNode("failure", "Failure branch", 620, 340),
      endNode("end-success", 900, 100),
      endNode("end-failure", 900, 340),
    ],
    edges: [
      { id: "e1", source: "start", source_port: "next", target: "condition", target_port: "in", kind: "next" },
      { id: "e2", source: "condition", source_port: "true", target: "success", target_port: "in", kind: "if" },
      { id: "e3", source: "condition", source_port: "false", target: "failure", target_port: "in", kind: "else" },
      { id: "e4", source: "success", source_port: "next", target: "end-success", target_port: "in", kind: "success" },
      { id: "e5", source: "failure", source_port: "next", target: "end-failure", target_port: "in", kind: "success" },
    ],
  };
  const response = await request.post("/api/runner-studio/workflows/graph", {
    data: {
      graph,
      labels: { source: "workflow-canvas-full-stack", fixture: "W2" },
      save_note: "seed valid W2 for Workflow generation full-stack test",
    },
  });
  const text = await response.text();
  if (response.status() !== 201) {
    throw new Error(`valid W2 setup failed (${response.status()}): ${text}`);
  }
  return { name, graph };
}

function actionNode(id, label, x, y) {
  return {
    id, type: "action", label, position: { x, y },
    step_name: id, step_id: id,
    ports: [{ id: "in", type: "input" }, { id: "next", type: "output" }],
    step: { name: id, action: "script.shell", args: { script: `set -e\necho ${id}` } },
  };
}

function endNode(id, x, y) {
  return { id, type: "end", label: "End", position: { x, y }, ports: [{ id: "in", type: "input" }] };
}

function currentProjection(guard, workflowId) {
  const projection = guard.latestWorkflowProjection();
  const projectedWorkflowId = projection?.committed?.workflowKey?.workflowId || projection?.workflowKey?.workflowId;
  if (!projection || projectedWorkflowId !== workflowId) throw new Error(`no live Workflow projection for ${workflowId}`);
  fullRevision(projection);
  return projection;
}

function workflowApplyPrompt(projection, { workflowId, nodeId, operation, threshold }) {
  const scope = workflowPromptScope(projection, workflowId);
  return [
    "WORKFLOW_MCP_APPLY",
    "tenant=runner-default",
    `workflow=${workflowId}`,
    `revision=${scope.revision.resourceVersion}`,
    `environment=${scope.environment}`,
    `node=${nodeId}`,
    `operation=${operation}`,
    `threshold=${threshold}`,
  ].join(" ");
}

function workflowPreviewPrompt(projection, { workflowId, nodeId, label, operation }) {
  const scope = workflowPromptScope(projection, workflowId);
  return [
    "WORKFLOW_MCP_PREVIEW",
    "tenant=runner-default",
    `workflow=${workflowId}`,
    `revision=${scope.revision.resourceVersion}`,
    `environment=${scope.environment}`,
    `node=${nodeId}`,
    ...(operation ? [`operation=${operation}`] : []),
    `label_b64=${Buffer.from(label, "utf8").toString("base64url")}`,
  ].join(" ");
}

function workflowPromptScope(projection, workflowId) {
  const revision = fullRevision(projection);
  const environment = projection?.runGateFacts?.environment?.bindingVersion;
  if (typeof environment !== "string" || !environment.trim()) {
    throw new Error(`${workflowId} has no server-owned environment binding version`);
  }
  return { revision, environment };
}

function nodeThreshold(payload, nodeId) {
  const node = graphNodes(payload).find((item) => item?.id === nodeId);
  const threshold = (node?.inputs || []).find((input) => input?.key === "threshold");
  return threshold?.value_source?.value;
}

function nodeLabel(payload, nodeId) {
  return String(graphNodes(payload).find((item) => item?.id === nodeId)?.label || "");
}

function nodeScript(payload, nodeId) {
  return String(graphNodes(payload).find((item) => item?.id === nodeId)?.step?.args?.script || "");
}

function nodePosition(payload, nodeId) {
  const position = graphNodes(payload).find((item) => item?.id === nodeId)?.position;
  return { x: Number(position?.x), y: Number(position?.y) };
}

function graphNodes(payload) {
  return payload?.nodes || payload?.graph?.nodes || [];
}

function expectOnlyThresholdChanged(before, after, nodeId) {
  expect(maskTargetThreshold({ workflow: after.workflow, nodes: after.nodes, edges: after.edges }, nodeId))
    .toEqual(maskTargetThreshold({ workflow: before.workflow, nodes: before.nodes, edges: before.edges }, nodeId));
}

function maskTargetThreshold(value, nodeId, insideTargetNode = false) {
  if (Array.isArray(value)) {
    return value.map((item) => maskTargetThreshold(item, nodeId, insideTargetNode));
  }
  if (!value || typeof value !== "object") return value;
  const target = insideTargetNode || value.id === nodeId;
  return Object.fromEntries(Object.entries(value).map(([key, item]) => {
    if (target && value.key === "threshold" && key === "value_source" && item && typeof item === "object") {
      return [key, { ...item, value: "__TARGET_THRESHOLD__" }];
    }
    return [key, maskTargetThreshold(item, nodeId, target)];
  }));
}

function deepFreeze(value) {
  Object.values(value).forEach((item) => {
    if (item && typeof item === "object") deepFreeze(item);
  });
  return Object.freeze(value);
}
