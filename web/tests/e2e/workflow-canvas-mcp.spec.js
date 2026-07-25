// @ts-check
import { expect, test } from "@playwright/test";

import {
  workflowCandidateCaseBlockers,
  workflowCandidateCaseImplementations,
} from "./cases/workflowCandidateCases.js";
import {
  workflowGenerationCaseBlockers,
  workflowGenerationCaseImplementations,
} from "./cases/workflowGenerationCases.js";
import {
  workflowResilienceCaseBlockers,
  workflowResilienceCaseImplementations,
} from "./cases/workflowResilienceCases.js";
import {
  workflowScaleLifecycleCaseBlockers,
  workflowScaleLifecycleCaseImplementations,
} from "./cases/workflowScaleLifecycleCases.js";
import {
  assertFullStackSpecHasNoRouteMocks,
  assertRealServerReady,
  attachFullStackEvidence,
  commitManualNodeLabel,
  commitManualWorkflowCommand,
  createRealPageGuard,
  loadWorkflowCanvasManifest,
  readWorkflowGraph,
  resourceVersion,
  seedWorkflowFixture,
  validateWorkflowNodeLabel,
  workflowCaseActivation,
} from "../helpers/workflowCanvasServerHarness.js";

const realServerEnabled = process.env.AIOPS_WORKFLOW_FULL_STACK_E2E === "1";
const manifest = loadWorkflowCanvasManifest();
const resolvedCandidateCaseIds = new Set(["PW-004", "PW-005", "PW-009"]);
const unresolvedWorkflowCandidateCaseBlockers = Object.fromEntries(
  Object.entries(workflowCandidateCaseBlockers).filter(([caseId]) => !resolvedCandidateCaseIds.has(caseId)),
);

test.describe("Workflow Canvas MCP full-stack (real ai-server; no page.route)", () => {
  test.skip(!realServerEnabled, "run through scripts/run-workflow-canvas-e2e.sh with an isolated real ai-server");

  test("[HARNESS-001] real Runner Application, CommitStore, Domain Event, AppSnapshot, and Canvas DOM", async ({ page, request }, testInfo) => {
    assertFullStackSpecHasNoRouteMocks();
    await assertRealServerReady(request);
    const fixture = await seedWorkflowFixture(request, "W1", `${process.pid}-${testInfo.workerIndex}`);

    // Bind and snapshot the authoritative Workflow before the browser joins
    // /ws. The first AppSnapshot must replay projector state; it must not rely
    // on the browser being connected when the binding broadcast occurred.
    const before = await readWorkflowGraph(request, fixture.name);
    const beforeRevision = resourceVersion(before);
    expect(beforeRevision).not.toBe("");

    const guard = createRealPageGuard(page);
    await page.goto(`/runner/${encodeURIComponent(fixture.name)}`, { waitUntil: "domcontentloaded" });
    await expect(page).toHaveTitle(/AIOps/);
    await expect(page.locator("#app")).not.toBeEmpty();
    await expect(page.getByTestId("runner-studio-page")).toBeVisible();
    await expect(page.getByTestId("canvas-node-action")).toBeVisible();
    await expect.poll(() => guard.latestWorkflowProjection()?.committed?.revision?.resourceVersion)
      .toBe(beforeRevision);

    const label = "Authoritative full-stack label";
    const validation = await validateWorkflowNodeLabel(request, {
      workflowId: fixture.name,
      expectedRevision: beforeRevision,
      nodeId: "action",
      label,
      patchId: `harness-validation-${process.pid}-${testInfo.workerIndex}`,
    });
    const committed = await commitManualNodeLabel(request, {
      workflowId: fixture.name,
      expectedRevision: beforeRevision,
      nodeId: "action",
      label,
      idempotencyKey: `harness-label-${process.pid}-${testInfo.workerIndex}`,
    });
    const afterRevision = committed.receipt.revisionAfter.resourceVersion;
    expect(afterRevision).not.toBe(beforeRevision);
    await expect.poll(() => guard.latestWorkflowProjection()?.committed?.revision?.resourceVersion)
      .toBe(afterRevision);
    await expect.poll(async () => {
      const graph = await readWorkflowGraph(request, fixture.name);
      return graph.nodes?.find((node) => node.id === "action")?.label;
    }).toBe(label);

    await expect(page.getByTestId("canvas-node-action")).toContainText(label);
    await guard.assertHealthy();
    await attachFullStackEvidence(page, testInfo, {
      fixtureId: "W1",
      workflowId: fixture.name,
      revisionBefore: beforeRevision,
      revisionAfter: afterRevision,
      transactionId: committed.receipt.transactionId,
      eventId: committed.event.eventId,
      eventSequence: committed.event.sequence,
      validation,
    });
  });

  test("[HARNESS-002] real Chat UI, RuntimeKernel, recorded provider, AssistantTransport, and DOM", async ({ page, request }, testInfo) => {
    assertFullStackSpecHasNoRouteMocks();
    await assertRealServerReady(request);
    const guard = createRealPageGuard(page);
    await page.goto("/", { waitUntil: "domcontentloaded" });
    await expect(page).toHaveTitle(/AIOps/);
    await expect(page.locator("#app")).not.toBeEmpty();
    const composer = page.getByTestId("omnibar-input");
    await expect(composer).toBeVisible();
    await composer.fill("WORKFLOW_HARNESS_FINAL_ONLY 请只返回录制模型确认文本");
    await page.getByTestId("omnibar-primary-action").click();
    await expect(page.locator("body")).toContainText("RECORDED_WORKFLOW_MODEL_OK");
    await expect.poll(async () => {
      const state = await request.get("/api/v1/state");
      const payload = await state.json();
      return payload.status || payload.runtime?.turn?.phase;
    }).toMatch(/idle|completed/);
    await guard.assertHealthy();
    await attachFullStackEvidence(page, testInfo, { model: "recorded-workflow-model", runtime: "completed" });
  });

  test("[HARNESS-003] real Workflow panel freezes scope, executes MCP through RuntimeKernel, and resumes", async ({ page, request }, testInfo) => {
    assertFullStackSpecHasNoRouteMocks();
    await assertRealServerReady(request);
    const fixture = await seedWorkflowFixture(request, "W1", `workflow-chat-${process.pid}-${testInfo.workerIndex}`);
    const before = await readWorkflowGraph(request, fixture.name);
    const revision = resourceVersion(before);
    expect(revision).not.toBe("");

    const guard = createRealPageGuard(page);
    await page.goto(`/runner/${encodeURIComponent(fixture.name)}`, { waitUntil: "domcontentloaded" });
    await expect(page.getByTestId("runner-studio-page")).toBeVisible();
    await expect(page.getByTestId("canvas-node-action")).toBeVisible();
    await page.getByTestId("runner-toolbar-ai-generate").click();
    const panel = page.getByTestId("workflow-ai-transport-panel");
    await expect(panel).toBeVisible();
    await expect(panel.getByTestId("workflow-scope-workflow")).toHaveCount(0);
    await expect(panel.getByLabel("本次发送范围")).toHaveCount(0);
    await expect(page.getByTestId("runner-workflow-title-display")).toContainText(fixture.name);

    const prompt = `WORKFLOW_MCP_READ tenant=runner-default workflow=${fixture.name} revision=${revision} 请读取当前工作流摘要`;
    await panel.getByTestId("workflow-ai-input").fill(prompt);
    await expect(panel.getByTestId("workflow-ai-send")).toBeEnabled();
    await panel.getByTestId("workflow-ai-send").click();
    await expect(panel).toContainText("RECORDED_WORKFLOW_MCP_READ_OK");
    const turn = panel.locator("article[data-turn-id]").last();
    const turnId = await turn.getAttribute("data-turn-id");
    expect(turnId).toBeTruthy();
    await expect(turn).toContainText("本轮基于：整个工作流");

    await page.reload({ waitUntil: "domcontentloaded" });
    await expect(page.getByTestId("canvas-node-action")).toBeVisible();
    await page.getByTestId("runner-toolbar-ai-generate").click();
    const resumed = page.getByTestId("workflow-ai-transport-panel");
    await expect(resumed).toContainText("RECORDED_WORKFLOW_MCP_READ_OK");
    await expect(resumed.locator(`article[data-turn-id="${turnId}"]`)).toContainText("本轮基于：整个工作流");
    await guard.assertHealthy();
    await attachFullStackEvidence(page, testInfo, {
      fixtureId: "W1",
      workflowId: fixture.name,
      revision,
      turnId,
      path: "WorkflowAiTransportPanel -> RuntimeKernel -> workflow.get_context -> AssistantTransport resume",
    });
  });
});

const phaseCases = [
  ["PW-001", "W0 creates a conditional workflow through review, Preview, and Apply"],
  ["PW-002", "freezes selection and changes only the selected threshold"],
  ["PW-003", "keeps the sent turn selection after the operator changes selection"],
  ["PW-004", "locates the Canvas object from a Chat change item"],
  ["PW-005", "locates the Chat change item from a ghost node"],
  ["PW-006", "applies the latest directly revised Candidate only"],
  ["PW-007", "discards Candidate without changing committed state"],
  ["PW-008", "rebases safe layout drift without losing coordinates"],
  ["PW-009", "shows a three-way same-field conflict and preserves user input"],
  ["PW-010", "requires exact approval for deleting an existing node"],
  ["PW-011", "routes W5 missing Secret through trusted configuration without disclosure"],
  ["PW-012", "keeps Undo, refresh, Redo, history, and revisions consistent"],
  ["PW-013", "resyncs snapshot after an interrupted event connection"],
  ["PW-014", "reports requested and cancelled states accurately"],
  ["PW-015", "retains Candidate after a Tool failure and re-Preview"],
  ["PW-016", "deduplicates a double Apply into one transaction"],
  ["PW-017", "recovers an unknown Apply response from the receipt"],
  ["PW-018", "opens the workflow drawer without resizing the Canvas"],
  ["PW-019", "preserves operator focus without automatic Follow AI controls"],
  ["PW-020", "does not delete an edge on double-click and supports undo"],
  ["PW-021", "rejects an incompatible edge before graph mutation"],
  ["PW-022", "locates the exact node field from a blocking diagnostic"],
  ["PW-023", "shows accurate check, configuration, and ready states"],
  ["PW-024", "hides raw tools and JSON until readable technical details are opened"],
  ["PW-025", "requires keep or discard when leaving a workflow with an active Candidate"],
  ["PW-026", "keeps manual editing usable when the Agent capability is off"],
  ["PW-027", "shows only useful first actions and real examples on an empty Canvas"],
  ["PW-028", "navigates a 100-item W4 diff without interaction stalls"],
  ["PW-029", "accepts a new constraint while generating and invalidates the old Candidate"],
  ["PW-030", "queues a second edit until the active Apply receipt arrives"],
  ["PW-031", "invalidates check and publish readiness after a semantic edit"],
  ["PW-032", "runs the published revision separately from current draft dry-run"],
  ["PW-033", "keeps a committed graph while reporting failed post-check honestly"],
];

const phaseCaseGaps = {
  "PW-001": "the recorded real model only exposes workflow.get_context and cannot create a reviewable mutation candidate",
  "PW-002": "the recorded real model cannot generate a selected-field mutation candidate",
  "PW-006": "the recorded real model cannot generate and revise an apply-authorized Candidate",
  "PW-008": "no browser-accessible layout rebase operation currently produces a candidate with safe external drift",
  "PW-011": "the full-stack fixture materializer cannot provision W5 configuration/Secret requirements into the real Runner graph",
  "PW-015": "the recorded provider exposes no deterministic mutation Tool failure and re-Preview sequence",
  "PW-017": "the real harness has no network-fault injection boundary for losing only the Apply response without route interception",
  "PW-022": "the fixture materializer has no blocking diagnostic with an exact node-field location",
  "PW-026": "workflow_agent_ui is a startup capability and the current full-stack harness cannot restart the server inside one Case with that capability disabled",
  "PW-028": "the W4 recipe is not materialized into a real 500-node graph by the current harness",
  "PW-029": "the recorded provider has no long-running candidate generation turn that can accept a second constraint",
  "PW-030": "the recorded provider cannot create an apply-authorized Candidate or a delayed Apply receipt",
  "PW-032": "the real control-plane projection does not yet expose a published revision runnable independently from the current draft",
  "PW-033": "the harness has no non-test-only way to force a post-check failure after a successful committed mutation",
  ...unresolvedWorkflowCandidateCaseBlockers,
  ...workflowGenerationCaseBlockers,
  ...workflowResilienceCaseBlockers,
  ...workflowScaleLifecycleCaseBlockers,
};

const phaseCaseImplementations = {
  ...workflowCandidateCaseImplementations,
  ...workflowGenerationCaseImplementations,
  ...workflowResilienceCaseImplementations,
  ...workflowScaleLifecycleCaseImplementations,
  "PW-003": freezesSentSelection,
  "PW-004": locatesCanvasObjectFromChatChange,
  "PW-007": discardsUndoCandidate,
  "PW-009": preservesCandidateOnSameFieldConflict,
  "PW-010": requiresExactApprovalForDelete,
  "PW-013": resyncsAfterBrowserReconnect,
  "PW-014": cancelsPendingApprovalWithoutMutation,
  "PW-016": deduplicatesDoubleApply,
  "PW-018": keepsWorkflowDrawerOverlayOffCanvasLayout,
  "PW-019": preservesOperatorFocusWithoutFollowAi,
  "PW-020": preservesEdgeOnDoubleClickAndPreviewsUndo,
  "PW-021": rejectsSelfConnectionBeforeMutation,
  "PW-023": showsAuthoritativeGateStates,
  "PW-024": keepsWorkflowReadOutputReadable,
  "PW-027": showsUsefulRealActionPalette,
  "PW-031": invalidatesDryRunReadinessAfterSemanticEdit,
};

test.describe("Workflow Canvas MCP manifest-driven real-user cases", () => {
  for (const [caseId, title] of phaseCases) {
    const activation = workflowCaseActivation(caseId, manifest);
    const define = activation.active ? test : test.skip;
    define(`[${caseId}] ${title}`, async ({ page, request }, testInfo) => {
      const implementation = phaseCaseImplementations[caseId];
      test.fixme(!implementation, caseGapReason(caseId));
      await implementation?.({ page, request }, testInfo);
    });
  }
});

function caseGapReason(caseId) {
  const gap = phaseCaseGaps[caseId];
  if (!gap) return `${caseId} has no honest real full-stack implementation yet`;
  if (typeof gap === "string") return gap;
  return `${gap.code}: ${gap.observedGap}`;
}

async function freezesSentSelection({ page, request }, testInfo) {
  assertFullStackSpecHasNoRouteMocks();
  await assertRealServerReady(request);
  const fixture = await seedWorkflowFixture(request, "W1", caseSuffix("pw003", testInfo));
  const guard = await openRealWorkflow(page, request, fixture.name);

  await page.getByTestId("canvas-node-action").click();
  await page.getByTestId("runner-toolbar-ai-generate").click();
  const panel = page.getByTestId("workflow-ai-transport-panel");
  await expect(panel.getByTestId("workflow-scope-selection")).toHaveCount(0);
  const revision = resourceVersion(await readWorkflowGraph(request, fixture.name));
  await panel.getByTestId("workflow-ai-input").fill(workflowReadPrompt(fixture.name, revision));
  await panel.getByTestId("workflow-ai-send").click();
  await page.getByTestId("canvas-node-start").click();
  await page.getByTestId("runner-toolbar-ai-generate").click();
  await expect(panel).toContainText("RECORDED_WORKFLOW_MCP_READ_OK");
  const sentTurn = panel.locator("article[data-turn-id]").last();
  await expect(sentTurn).toContainText("本轮基于：action");
  await expect(sentTurn).not.toContainText("本轮基于：start");
  await guard.assertHealthy();
  await attachFullStackEvidence(page, testInfo, { caseId: "PW-003", workflowId: fixture.name, frozenSelection: ["action"] });
}

async function locatesCanvasObjectFromChatChange({ page, request }, testInfo) {
  assertFullStackSpecHasNoRouteMocks();
  await assertRealServerReady(request);
  const fixture = await seedWorkflowFixture(request, "W1", caseSuffix("pw004", testInfo));
  const beforeGraph = await readWorkflowGraph(request, fixture.name);
  const revisionBefore = resourceVersion(beforeGraph);
  const committedLabel = beforeGraph.nodes?.find((node) => node.id === "action")?.label;
  expect(typeof committedLabel).toBe("string");
  const candidateLabel = "PW-004 candidate label from Chat";
  const guard = await openRealWorkflow(page, request, fixture.name, revisionBefore);

  await page.getByTestId("runner-toolbar-ai-generate").click();
  const panel = page.getByTestId("workflow-ai-transport-panel");
  await panel.getByTestId("workflow-ai-input").fill(workflowApplyPrompt(currentProjection(guard, fixture.name), {
    workflowId: fixture.name,
    nodeId: "action",
    label: candidateLabel,
  }));
  await panel.getByTestId("workflow-ai-send").click();

  await expect(page.getByTestId("workflow-context-tab-review")).toHaveAttribute("aria-selected", "true");
  await expect(page.getByTestId("runner-candidate-canvas-status")).toContainText("尚未保存");
  await page.getByTestId("workflow-context-tab-ai").click();
  const previewArtifact = panel.locator('[data-testid="workflow-change-artifact"][data-change-kind="preview"]');
  await expect(previewArtifact).toHaveCount(1);
  const focusAction = previewArtifact.getByRole("button", { name: "定位节点", exact: true });
  await expect(focusAction).toHaveCount(1);
  await page.evaluate(() => {
    window.addEventListener("aiops:workflow-change-action", (event) => {
      if (event instanceof CustomEvent) Reflect.set(window, "__pw004WorkflowChangeAction", event.detail);
    }, { once: true });
  });
  await focusAction.click();

  const actionDetail = await page.evaluate(() => Reflect.get(window, "__pw004WorkflowChangeAction"));
  expect(actionDetail).toMatchObject({
    action: {
      id: "workflow-focus-operation-0",
      intent: "workflow.focus_object",
      label: "定位节点",
      mutation: false,
      target: { nodeId: "action", operationIndex: 0 },
    },
  });

  await expect(page.getByTestId("workflow-context-tab-review")).toHaveAttribute("aria-selected", "true");
  const focusedNode = page.getByTestId("canvas-node-action");
  await expect(focusedNode).toHaveClass(/\bselected\b/);
  await expect(focusedNode).toHaveAttribute("data-candidate-change", "changed");
  await expect(focusedNode).toContainText(candidateLabel);

  const review = page.getByTestId("workflow-change-review");
  await expect(review).toBeVisible();
  const activeOperation = review.getByRole("button", { name: "1. 更新节点 action", exact: true });
  await expect(activeOperation).toHaveAttribute("aria-current", "step");
  await expect(review.getByRole("navigation", { name: "逐项审阅" })).toContainText("1 / 1");
  const fieldDiff = review.getByRole("region", { name: "字段差异" });
  await expect(fieldDiff).toBeVisible();
  await expect(fieldDiff).toContainText("label");
  await expect(fieldDiff).toContainText(String(committedLabel));
  await expect(fieldDiff).toContainText(candidateLabel);

  await expect.poll(async () => {
    const graph = await readWorkflowGraph(request, fixture.name);
    return {
      revision: resourceVersion(graph),
      label: graph.nodes?.find((node) => node.id === "action")?.label,
    };
  }).toEqual({ revision: revisionBefore, label: committedLabel });
  await guard.assertHealthy();
  await attachFullStackEvidence(page, testInfo, {
    caseId: "PW-004",
    workflowId: fixture.name,
    revisionBefore,
    committedLabel,
    candidateLabel,
    actionIntent: "workflow.focus_object",
    actionTarget: { nodeId: "action", operationIndex: 0 },
    activeReviewOperation: 0,
    committedUnchanged: true,
  });
}

async function discardsUndoCandidate({ page, request }, testInfo) {
  assertFullStackSpecHasNoRouteMocks();
  await assertRealServerReady(request);
  const history = await seedLabelTransaction(request, testInfo, "pw007", "PW-007 committed label");
  const guard = await openRealWorkflow(page, request, history.workflowId, history.revisionAfter);
  await expect(page.getByTestId("canvas-node-action")).toContainText(history.label);

  await page.getByTestId("workflow-context-tab-history").click();
  const entry = page.locator(`[data-transaction-id="${history.transactionId}"]`);
  await expect(entry).toBeVisible();
  await entry.getByRole("button", { name: "撤销此操作" }).click();
  await expect(page.getByTestId("runner-candidate-canvas-status")).toContainText("尚未保存");
  await expect(page.getByTestId("workflow-candidate-action-bar")).toBeVisible();
  await expect(page.getByTestId("canvas-node-action")).not.toContainText(history.label);

  await page.getByTestId("workflow-candidate-action-bar").getByRole("button", { name: "放弃修改" }).click();
  await expect(page.getByTestId("runner-candidate-canvas-status")).toHaveCount(0);
  await expect(page.getByTestId("canvas-node-action")).toContainText(history.label);
  await expect.poll(async () => graphFacts(await readWorkflowGraph(request, history.workflowId))).toEqual({
    revision: history.revisionAfter,
    nodes: ["action", "end", "start"],
    edges: ["e1", "e2"],
  });
  await guard.assertHealthy();
  await attachFullStackEvidence(page, testInfo, { caseId: "PW-007", ...history, committedUnchanged: true });
}

async function resyncsAfterBrowserReconnect({ page, request }, testInfo) {
  assertFullStackSpecHasNoRouteMocks();
  await assertRealServerReady(request);
  const fixture = await seedWorkflowFixture(request, "W1", caseSuffix("pw013", testInfo));
  const before = await readWorkflowGraph(request, fixture.name);
  const revisionBefore = resourceVersion(before);
  const guard = await openRealWorkflow(page, request, fixture.name, revisionBefore);
  expect(guard.workflowSocketOpenCount()).toBeGreaterThan(0);

  const closeCountBefore = guard.workflowSocketCloseCount();
  await page.goto("about:blank", { waitUntil: "domcontentloaded" });
  await expect.poll(() => guard.workflowSocketCloseCount()).toBeGreaterThan(closeCountBefore);
  const committed = await commitManualNodeLabel(request, {
    workflowId: fixture.name,
    expectedRevision: revisionBefore,
    nodeId: "action",
    label: "PW-013 resynced label",
    idempotencyKey: `pw013-${process.pid}-${testInfo.workerIndex}`,
  });
  const revisionAfter = committed.receipt.revisionAfter.resourceVersion;
  await expect.poll(async () => resourceVersion(await readWorkflowGraph(request, fixture.name))).toBe(revisionAfter);
  await page.goto(`/runner/${encodeURIComponent(fixture.name)}`, { waitUntil: "domcontentloaded" });
  await expect(page.getByTestId("runner-studio-page")).toBeVisible();
  await expect.poll(() => guard.latestWorkflowProjection()?.committed?.revision?.resourceVersion || "").toBe(revisionAfter);
  await expect(page.getByTestId("canvas-node-action")).toContainText("PW-013 resynced label");
  await attachFullStackEvidence(page, testInfo, {
    caseId: "PW-013",
    workflowId: fixture.name,
    revisionBefore,
    revisionAfter,
    socketCloses: guard.workflowSocketCloseCount() - closeCountBefore,
    resyncedFromFreshSnapshotAfterRouteReturn: true,
  });
}

async function cancelsPendingApprovalWithoutMutation({ page, request }, testInfo) {
  assertFullStackSpecHasNoRouteMocks();
  await assertRealServerReady(request);
  const fixture = await seedWorkflowFixture(request, "W1", caseSuffix("pw014", testInfo));
  const before = graphFacts(await readWorkflowGraph(request, fixture.name));
  const guard = await openRealWorkflow(page, request, fixture.name, before.revision);
  const candidateLabel = "PW-014 canceled candidate";

  await page.getByTestId("runner-toolbar-ai-generate").click();
  const panel = page.getByTestId("workflow-ai-transport-panel");
  await panel.getByTestId("workflow-ai-input").fill(workflowApplyPrompt(currentProjection(guard, fixture.name), {
    workflowId: fixture.name,
    nodeId: "action",
    label: candidateLabel,
  }));
  await panel.getByTestId("workflow-ai-send").click();

  await expect(page.getByTestId("workflow-context-tab-review")).toHaveAttribute("aria-selected", "true");
  await page.getByTestId("workflow-context-tab-ai").click();
  const approval = panel.getByTestId("workflow-approval-card").last();
  await expect(approval).toHaveAttribute("data-approval-state", "pending");
  await expect(panel.getByTestId("workflow-ai-status")).toHaveText("等待你的操作");
  await expect(panel.getByTestId("workflow-approval-accept").last()).toBeVisible();
  await expect(panel.getByTestId("workflow-approval-reject").last()).toBeVisible();
  const turnId = await panel.locator("article[data-turn-id]").last().getAttribute("data-turn-id");
  expect(turnId).toBeTruthy();
  expect(graphFacts(await readWorkflowGraph(request, fixture.name))).toEqual(before);

  let approvalDecisionRequests = 0;
  let stopRequests = 0;
  page.on("request", (outgoing) => {
    if (isApprovalDecisionRequest(outgoing)) approvalDecisionRequests += 1;
    if (isStopRequest(outgoing)) stopRequests += 1;
  });
  const stopResponsePromise = page.waitForResponse((response) => isStopRequest(response.request()));
  await panel.getByTestId("workflow-ai-stop").click();
  expect((await stopResponsePromise).status()).toBe(200);
  await expect(panel.getByTestId("workflow-ai-status")).toHaveText("已取消");
  await expect(panel.getByTestId("workflow-ai-stop")).toHaveCount(0);
  await expect(panel.getByTestId("workflow-approval-accept")).toHaveCount(0);
  await expect(panel.getByTestId("workflow-approval-reject")).toHaveCount(0);
  await expect(approval).toHaveAttribute("data-approval-state", "canceled");
  expect(approvalDecisionRequests).toBe(0);
  expect(stopRequests).toBe(1);
  await expect.poll(async () => graphFacts(await readWorkflowGraph(request, fixture.name))).toEqual(before);

  await page.reload({ waitUntil: "domcontentloaded" });
  await expect(page.getByTestId("runner-studio-page")).toBeVisible();
  await expect(page.getByTestId("canvas-node-action")).toBeVisible();
  await page.getByTestId("runner-toolbar-ai-generate").click();
  const resumed = page.getByTestId("workflow-ai-transport-panel");
  await expect(resumed.getByTestId("workflow-ai-status")).toHaveText("已取消");
  await expect(resumed.locator(`article[data-turn-id="${turnId}"]`)).toBeVisible();
  await expect(resumed.getByTestId("workflow-approval-accept")).toHaveCount(0);
  await expect(resumed.getByTestId("workflow-approval-reject")).toHaveCount(0);
  await expect.poll(async () => graphFacts(await readWorkflowGraph(request, fixture.name))).toEqual(before);
  await guard.assertHealthy();
  await attachFullStackEvidence(page, testInfo, {
    caseId: "PW-014",
    workflowId: fixture.name,
    turnId,
    revision: before.revision,
    approvalObserved: "requested/blocked",
    cancellationPersistedAfterReload: true,
    approvalActionsAfterCancellation: 0,
    approvalDecisionRequests,
    stopRequests,
    committedGraphUnchanged: true,
  });
}

async function preservesCandidateOnSameFieldConflict({ page, request }, testInfo) {
  assertFullStackSpecHasNoRouteMocks();
  await assertRealServerReady(request);
  const fixture = await seedWorkflowFixture(request, "W1", caseSuffix("pw009", testInfo));
  const before = await readWorkflowGraph(request, fixture.name);
  const revisionBefore = resourceVersion(before);
  const guard = await openRealWorkflow(page, request, fixture.name, revisionBefore);
  const candidateLabel = "PW-009 retained candidate draft";
  const authoritativeLabel = "PW-009 authoritative manual value";

  await page.getByTestId("runner-toolbar-ai-generate").click();
  const panel = page.getByTestId("workflow-ai-transport-panel");
  await panel.getByTestId("workflow-ai-input").fill(workflowApplyPrompt(currentProjection(guard, fixture.name), {
    workflowId: fixture.name,
    nodeId: "action",
    label: candidateLabel,
  }));
  await panel.getByTestId("workflow-ai-send").click();

  await expect(page.getByTestId("workflow-context-tab-review")).toHaveAttribute("aria-selected", "true");
  await expect(page.getByTestId("workflow-change-review")).toContainText("更新节点 action");
  await expect(page.getByTestId("runner-candidate-canvas-status")).toContainText("尚未保存");
  await expect(page.getByTestId("canvas-node-action")).toHaveAttribute("data-candidate-change", "changed");
  await expect(page.getByTestId("canvas-node-action")).toContainText(candidateLabel);
  await page.getByTestId("workflow-context-tab-ai").click();
  const approval = panel.getByTestId("workflow-approval-card");
  await expect(approval).toHaveAttribute("data-approval-state", "pending");
  await expect(panel.getByTestId("workflow-approval-accept")).toBeVisible();

  const drift = await commitManualNodeLabel(request, {
    workflowId: fixture.name,
    expectedRevision: revisionBefore,
    nodeId: "action",
    label: authoritativeLabel,
    idempotencyKey: `pw009-drift-${process.pid}-${testInfo.workerIndex}`,
  });
  const revisionAfterDrift = drift.receipt.revisionAfter.resourceVersion;
  expect(revisionAfterDrift).not.toBe(revisionBefore);
  await expect.poll(() => guard.latestWorkflowProjection()?.committed?.revision?.resourceVersion || "").toBe(revisionAfterDrift);
  await expect.poll(async () => {
    const graph = await readWorkflowGraph(request, fixture.name);
    return graph.nodes?.find((node) => node.id === "action")?.label;
  }).toBe(authoritativeLabel);
  await expect(page.getByTestId("runner-candidate-canvas-status")).toContainText("候选预览已阻止");
  await expect(page.getByTestId("canvas-node-action")).toContainText(authoritativeLabel);

  const approvalResponsePromise = page.waitForResponse((response) => isApprovalDecisionRequest(response.request()));
  await panel.getByTestId("workflow-approval-accept").click();
  expect((await approvalResponsePromise).status()).toBe(200);
  await expect(panel).toContainText("RECORDED_WORKFLOW_MCP_APPLY_ERROR");
  const conflictArtifact = panel.locator('[data-testid="workflow-change-artifact"][data-change-kind="conflict"]');
  await expect(conflictArtifact).toHaveCount(1);
  await expect(conflictArtifact).toContainText("画布已发生变化");
  await expect(conflictArtifact).toContainText("需要处理");

  await expect.poll(async () => {
    const graph = await readWorkflowGraph(request, fixture.name);
    return {
      revision: resourceVersion(graph),
      label: graph.nodes?.find((node) => node.id === "action")?.label,
    };
  }).toEqual({ revision: revisionAfterDrift, label: authoritativeLabel });
  await page.getByTestId("workflow-context-tab-review").click();
  const review = page.getByTestId("workflow-change-review");
  await expect(review).toBeVisible();
  await expect(review).toContainText("更新节点 action");
  await expect(review).toContainText("候选版本 1");
  await expect(page.getByText("当前候选信息不完整，无法安全应用。请让 AI 重新生成待确认修改。")).toBeVisible();
  await expect(page.getByTestId("workflow-candidate-action-bar")).toHaveCount(0);
  await expect(page.getByTestId("runner-candidate-canvas-status")).toContainText("候选预览已阻止");
  await expect(page.getByTestId("canvas-node-action")).toContainText(authoritativeLabel);
  await guard.assertHealthy();
  await attachFullStackEvidence(page, testInfo, {
    caseId: "PW-009",
    workflowId: fixture.name,
    candidateLabel,
    authoritativeLabel,
    revisionBefore,
    revisionAfterDrift,
    driftTransactionId: drift.receipt.transactionId,
    conflictArtifactVisible: true,
    candidateRetainedForReview: true,
    authoritativeValuePreserved: true,
  });
}

async function requiresExactApprovalForDelete({ page, request }, testInfo) {
  assertFullStackSpecHasNoRouteMocks();
  await assertRealServerReady(request);
  const fixture = await seedWorkflowFixture(request, "W1", caseSuffix("pw010", testInfo));
  const seededRevision = resourceVersion(await readWorkflowGraph(request, fixture.name));
  await commitManualWorkflowCommand(request, {
    workflowId: fixture.name,
    expectedRevision: seededRevision,
    idempotencyKey: `pw010-survivor-${process.pid}-${testInfo.workerIndex}`,
    commands: [
      {
        kind: "add_node",
        node: {
          id: "delete-survivor",
          type: "action",
          position: { x: 320, y: 360 },
          label: "PW-010 surviving route",
          step: { name: "delete-survivor", action: "script.shell", args: { script: "set -e\necho surviving-route" } },
        },
      },
      { kind: "connect_edge", edge: { id: "e3", source: "start", target: "delete-survivor", kind: "next" } },
      { kind: "connect_edge", edge: { id: "e4", source: "delete-survivor", target: "end", kind: "success" } },
    ],
  });
  const before = graphFacts(await readWorkflowGraph(request, fixture.name));
  const guard = await openRealWorkflow(page, request, fixture.name, before.revision);
  const projection = currentProjection(guard, fixture.name);
  await page.getByTestId("runner-toolbar-ai-generate").click();
  const panel = page.getByTestId("workflow-ai-transport-panel");
  await panel.getByTestId("workflow-ai-input").fill(workflowApplyPrompt(projection, {
    workflowId: fixture.name,
    nodeId: "action",
    operation: "delete",
  }));
  await panel.getByTestId("workflow-ai-send").click();

  await expect(panel).toContainText("工具已生成候选预览");
  await expect(page.getByTestId("workflow-context-tab-review")).toHaveAttribute("aria-selected", "true");
  await page.getByTestId("workflow-context-tab-ai").click();
  await expect(panel).toBeVisible();
  await expect(panel.getByTestId("workflow-approval-accept")).toBeVisible();
  expect(graphFacts(await readWorkflowGraph(request, fixture.name))).toEqual(before);
  await expect(page.getByTestId("canvas-node-action")).toBeVisible();

  let approvalDecisionRequests = 0;
  page.on("request", (outgoing) => {
    if (isApprovalDecisionRequest(outgoing)) approvalDecisionRequests += 1;
  });
  const approvalResponsePromise = page.waitForResponse((response) => isApprovalDecisionRequest(response.request()));
  await panel.getByTestId("workflow-approval-accept").click();
  expect((await approvalResponsePromise).status()).toBe(200);
  await expect(panel).toContainText("RECORDED_WORKFLOW_MCP_APPLY_OK", { timeout: 15_000 });
  await expect.poll(async () => graphFacts(await readWorkflowGraph(request, fixture.name)).nodes).toEqual(["delete-survivor", "end", "start"]);
  const after = graphFacts(await readWorkflowGraph(request, fixture.name));
  expect(after.revision).not.toBe(before.revision);
  expect(approvalDecisionRequests).toBe(1);
  await expect(page.getByTestId("canvas-node-action")).toHaveCount(0);
  await guard.assertHealthy();
  await attachFullStackEvidence(page, testInfo, {
    caseId: "PW-010",
    workflowId: fixture.name,
    revisionBefore: before.revision,
    revisionAfter: after.revision,
    exactApprovalObservedBeforeMutation: true,
    deletedNodeId: "action",
  });
}

async function deduplicatesDoubleApply({ page, request }, testInfo) {
  assertFullStackSpecHasNoRouteMocks();
  await assertRealServerReady(request);
  const fixture = await seedWorkflowFixture(request, "W1", caseSuffix("pw016", testInfo));
  const before = graphFacts(await readWorkflowGraph(request, fixture.name));
  const guard = await openRealWorkflow(page, request, fixture.name, before.revision);
  const beforeHistoryCount = currentProjection(guard, fixture.name).durableHistory?.length || 0;
  const label = "PW-016 one idempotent transaction";
  await page.getByTestId("runner-toolbar-ai-generate").click();
  const panel = page.getByTestId("workflow-ai-transport-panel");
  await panel.getByTestId("workflow-ai-input").fill(workflowApplyPrompt(currentProjection(guard, fixture.name), {
    workflowId: fixture.name,
    nodeId: "action",
    label,
  }));
  await panel.getByTestId("workflow-ai-send").click();
  await expect(page.getByTestId("workflow-context-tab-review")).toHaveAttribute("aria-selected", "true");
  await page.getByTestId("workflow-context-tab-ai").click();
  await expect(panel).toBeVisible();
  const approve = panel.getByTestId("workflow-approval-accept");
  await expect(approve).toBeVisible();
  let approvalDecisionRequests = 0;
  page.on("request", (outgoing) => {
    if (isApprovalDecisionRequest(outgoing)) approvalDecisionRequests += 1;
  });
  const approvalResponsePromise = page.waitForResponse((response) => isApprovalDecisionRequest(response.request()));
  await approve.dblclick();

  expect((await approvalResponsePromise).status()).toBe(200);
  await expect(panel).toContainText("RECORDED_WORKFLOW_MCP_APPLY_OK", { timeout: 15_000 });
  await expect.poll(async () => (await readWorkflowGraph(request, fixture.name)).nodes?.find((node) => node.id === "action")?.label).toBe(label);
  const after = graphFacts(await readWorkflowGraph(request, fixture.name));
  await expect.poll(() => currentProjection(guard, fixture.name).durableHistory?.length || 0).toBe(beforeHistoryCount + 1);
  const history = currentProjection(guard, fixture.name).durableHistory || [];
  expect(new Set(history.map((entry) => entry.transactionId)).size).toBe(history.length);
  expect(approvalDecisionRequests).toBe(1);
  await guard.assertHealthy();
  await attachFullStackEvidence(page, testInfo, {
    caseId: "PW-016",
    workflowId: fixture.name,
    revisionBefore: before.revision,
    revisionAfter: after.revision,
    historyBefore: beforeHistoryCount,
    historyAfter: history.length,
    doubleApprovalClicks: 2,
    durableTransactionsAdded: 1,
  });
}

async function keepsWorkflowDrawerOverlayOffCanvasLayout({ page, request }, testInfo) {
  assertFullStackSpecHasNoRouteMocks();
  await assertRealServerReady(request);
  const fixture = await seedWorkflowFixture(request, "W1", caseSuffix("pw018", testInfo));
  const guard = await openRealWorkflow(page, request, fixture.name);
  const canvas = page.getByTestId("workflow-context-canvas-pane");
  const drawer = page.getByTestId("workflow-context-dock");
  await expect(drawer).toBeHidden();
  const canvasBefore = await canvas.boundingBox();
  expect(canvasBefore).toBeTruthy();

  await page.getByTestId("runner-toolbar-ai-generate").click();
  const panel = page.getByTestId("workflow-ai-transport-panel");
  await expect(drawer).toBeVisible();
  await expect(panel).toBeVisible();
  await expect(page.getByTestId("workflow-context-tab-configuration")).toHaveCount(0);
  const canvasOpen = await canvas.boundingBox();
  const drawerOpen = await drawer.boundingBox();
  expectCanvasGeometryStable(canvasOpen, canvasBefore);
  expect(drawerOpen?.width || 0).toBeGreaterThanOrEqual(320);
  expect((drawerOpen?.x || 0) < (canvasOpen?.x || 0) + (canvasOpen?.width || 0)).toBe(true);
  await expect(page.getByTestId("runner-node-picker-toggle")).toBeVisible();
  await expect(page.getByTestId("runner-canvas-fullscreen-toggle")).toBeVisible();

  await page.getByTestId("workflow-context-drawer-close").click();
  await expect(drawer).toBeHidden();
  const canvasClosed = await canvas.boundingBox();
  expectCanvasGeometryStable(canvasClosed, canvasBefore);
  await guard.assertHealthy();
  await attachFullStackEvidence(page, testInfo, {
    caseId: "PW-018",
    workflowId: fixture.name,
    canvasBefore,
    canvasOpen,
    canvasClosed,
    drawerWidth: drawerOpen?.width,
  });
}

function expectCanvasGeometryStable(actual, expected) {
  expect(actual).toBeTruthy();
  expect(expected).toBeTruthy();
  expect(Math.abs((actual?.x || 0) - (expected?.x || 0))).toBeLessThanOrEqual(1);
  expect(Math.abs((actual?.width || 0) - (expected?.width || 0))).toBeLessThanOrEqual(1);
}

async function preservesOperatorFocusWithoutFollowAi({ page, request }, testInfo) {
  assertFullStackSpecHasNoRouteMocks();
  await assertRealServerReady(request);
  const fixture = await seedWorkflowFixture(request, "W1", caseSuffix("pw019", testInfo));
  const guard = await openRealWorkflow(page, request, fixture.name);
  const candidateLabel = "PW-019 exact follow target";

  await page.getByTestId("runner-toolbar-ai-generate").click();
  const panel = page.getByTestId("workflow-ai-transport-panel");
  await panel.getByTestId("workflow-ai-input").fill(workflowApplyPrompt(currentProjection(guard, fixture.name), {
    workflowId: fixture.name,
    nodeId: "action",
    label: candidateLabel,
  }));
  const canvas = page.getByTestId("runner-canvas-dropzone");
  const canvasBox = await canvas.boundingBox();
  expect(canvasBox).toBeTruthy();
  const viewport = canvas.locator(".react-flow__viewport");
  const viewportBeforeOperator = await viewport.getAttribute("style");
  const center = { x: canvasBox.x + canvasBox.width / 2, y: canvasBox.y + canvasBox.height / 2 };

  await panel.getByTestId("workflow-ai-send").click();
  await expect(page.getByTestId("workflow-follow-ai")).toHaveCount(0);
  await expect(page.getByText(/跟随 AI/)).toHaveCount(0);
  await page.mouse.move(center.x, center.y);
  await page.mouse.wheel(0, -520);
  await expect(page.getByTestId("runner-candidate-canvas-status")).toHaveCount(0);
  await page.getByTestId("canvas-node-start").click();
  await expect(page.getByTestId("canvas-node-start")).toHaveClass(/\bselected\b/);
  await page.mouse.move(center.x, center.y);
  await page.mouse.down();
  await page.mouse.move(center.x + 96, center.y + 64, { steps: 6 });
  await page.mouse.up();
  await expect.poll(async () => viewport.getAttribute("style")).not.toBe(viewportBeforeOperator);
  const pausedViewport = await viewport.getAttribute("style");

  await expect(page.getByTestId("runner-candidate-canvas-status")).toContainText("尚未保存");
  await expect(page.getByTestId("canvas-node-action")).toHaveAttribute("data-candidate-change", "changed");
  await expect(page.getByTestId("canvas-node-start")).toHaveClass(/\bselected\b/);
  expect(await viewport.getAttribute("style")).toBe(pausedViewport);
  await page.getByTestId("canvas-node-action").click();
  await expect(page.getByTestId("canvas-node-action")).toHaveClass(/\bselected\b/);
  await expect(page.getByTestId("canvas-node-start")).not.toHaveClass(/\bselected\b/);
  expect(await viewport.getAttribute("style")).toBe(pausedViewport);

  await guard.assertHealthy();
  await attachFullStackEvidence(page, testInfo, {
    caseId: "PW-019",
    workflowId: fixture.name,
    candidateLabel,
    selectedWhileCandidateArrived: "start",
    selectedCandidateManually: "action",
    viewportBeforeOperator,
    preservedViewport: pausedViewport,
    followControlRemoved: true,
    backgroundCandidateDidNotStealFocus: true,
  });
}

async function preservesEdgeOnDoubleClickAndPreviewsUndo({ page, request }, testInfo) {
  assertFullStackSpecHasNoRouteMocks();
  await assertRealServerReady(request);
  const history = await seedLabelTransaction(request, testInfo, "pw020", "PW-020 history anchor");
  const alternatePath = await commitManualWorkflowCommand(request, {
    workflowId: history.workflowId,
    expectedRevision: history.revisionAfter,
    idempotencyKey: `pw020-alternate-${process.pid}-${testInfo.workerIndex}`,
    commands: [
      {
        kind: "add_node",
        node: {
          id: "alternate",
          type: "action",
          position: { x: 320, y: 360 },
          label: "PW-020 alternate route",
          step: { name: "alternate", action: "script.shell", args: { script: "set -e\necho alternate-route" } },
        },
      },
      { kind: "connect_edge", edge: { id: "e3", source: "start", target: "alternate", kind: "next" } },
      { kind: "connect_edge", edge: { id: "e4", source: "alternate", target: "action", kind: "success" } },
    ],
  });
  history.revisionAfter = alternatePath.receipt.revisionAfter.resourceVersion;
  const guard = await openRealWorkflow(page, request, history.workflowId, history.revisionAfter);
  const edgeHitTarget = page.locator('.react-flow__edge[data-id="e1"] .runner-flow-edge-hover-path');
  await expect(edgeHitTarget).toHaveCount(1);
  await edgeHitTarget.dblclick({ force: true });
  expect(graphFacts(await readWorkflowGraph(request, history.workflowId)).edges).toContain("e1");
  await edgeHitTarget.click({ button: "right", force: true });
  await expect(page.getByTestId("runner-edge-menu")).toBeVisible();

  const commandResponse = page.waitForResponse((response) => {
    if (!response.url().endsWith("/api/runner-studio/workflow-commands") || response.request().method() !== "POST") return false;
    try { return response.request().postDataJSON()?.command?.kind === "disconnect_edge"; } catch { return false; }
  });
  await page.getByTestId("runner-edge-delete").click();
  const deletion = await (await commandResponse).json();
  const revisionAfterDelete = deletion?.receipt?.revisionAfter?.resourceVersion;
  const deleteTransactionId = deletion?.receipt?.transactionId;
  expect(revisionAfterDelete).toBeTruthy();
  expect(deleteTransactionId).toBeTruthy();
  await expect.poll(async () => graphFacts(await readWorkflowGraph(request, history.workflowId)).edges).toEqual(["e2", "e3", "e4"]);

  await page.getByTestId("workflow-context-tab-history").click();
  const deleteEntry = page.locator(`[data-transaction-id="${deleteTransactionId}"]`);
  await expect(deleteEntry).toBeVisible();
  await deleteEntry.getByRole("button", { name: "撤销此操作" }).click();
  await expect(page.getByTestId("runner-candidate-canvas-status")).toContainText("尚未保存");
  expect(graphFacts(await readWorkflowGraph(request, history.workflowId))).toMatchObject({ revision: revisionAfterDelete, edges: ["e2", "e3", "e4"] });
  await guard.assertHealthy();
  await attachFullStackEvidence(page, testInfo, { caseId: "PW-020", workflowId: history.workflowId, deleteTransactionId, revisionAfterDelete, doubleClickCommittedMutation: false, undoIsCandidate: true });
}

async function rejectsSelfConnectionBeforeMutation({ page, request }, testInfo) {
  assertFullStackSpecHasNoRouteMocks();
  await assertRealServerReady(request);
  const fixture = await seedWorkflowFixture(request, "W1", caseSuffix("pw021", testInfo));
  const before = graphFacts(await readWorkflowGraph(request, fixture.name));
  const guard = await openRealWorkflow(page, request, fixture.name, before.revision);
  const action = page.getByTestId("canvas-node-action");
  const source = action.locator(".react-flow__handle.source").first();
  const target = action.locator(".react-flow__handle.target").first();
  const sourceBox = await source.boundingBox();
  const targetBox = await target.boundingBox();
  expect(sourceBox).toBeTruthy();
  expect(targetBox).toBeTruthy();
  await page.mouse.move(sourceBox.x + sourceBox.width / 2, sourceBox.y + sourceBox.height / 2);
  await page.mouse.down();
  await page.mouse.move(targetBox.x + targetBox.width / 2, targetBox.y + targetBox.height / 2, { steps: 8 });
  await page.mouse.up();
  await expect(page.getByRole("alert")).toContainText("不能连接到自身");
  expect(graphFacts(await readWorkflowGraph(request, fixture.name))).toEqual(before);
  await guard.assertHealthy();
  await attachFullStackEvidence(page, testInfo, { caseId: "PW-021", workflowId: fixture.name, rejected: "self_connection", committedUnchanged: true });
}

async function showsAuthoritativeGateStates({ page, request }, testInfo) {
  assertFullStackSpecHasNoRouteMocks();
  await assertRealServerReady(request);
  const history = await seedLabelTransaction(request, testInfo, "pw023", "PW-023 validated label");
  const guard = await openRealWorkflow(page, request, history.workflowId, history.revisionAfter);
  await page.getByTestId("workflow-context-tab-run").click();
  const run = page.getByTestId("workflow-run-panel");
  await expect(run).toHaveAttribute("data-gate-state", "validated");
  await expect(run.getByTestId("workflow-draft-dry-run")).toBeEnabled();
  await run.getByTestId("workflow-draft-dry-run").click();
  await expect(run).toHaveAttribute("data-gate-state", "current");
  await expect(run.getByTestId("workflow-run-gate-status")).toContainText("检查和试运行已通过");
  await guard.assertHealthy();
  await attachFullStackEvidence(page, testInfo, { caseId: "PW-023", workflowId: history.workflowId, states: ["validated", "current"] });
}

async function keepsWorkflowReadOutputReadable({ page, request }, testInfo) {
  assertFullStackSpecHasNoRouteMocks();
  await assertRealServerReady(request);
  const fixture = await seedWorkflowFixture(request, "W1", caseSuffix("pw024", testInfo));
  const revision = resourceVersion(await readWorkflowGraph(request, fixture.name));
  const guard = await openRealWorkflow(page, request, fixture.name, revision);
  await page.getByTestId("runner-toolbar-ai-generate").click();
  const panel = page.getByTestId("workflow-ai-transport-panel");
  await panel.getByTestId("workflow-ai-input").fill(workflowReadPrompt(fixture.name, revision));
  await panel.getByTestId("workflow-ai-send").click();
  await expect(panel).toContainText("RECORDED_WORKFLOW_MCP_READ_OK");
  await expect(panel.locator("pre")).toHaveCount(0);
  await expect(panel.getByText(/\{"schemaVersion"|"structuredContent"|"workflowKey":/)).toHaveCount(0);
  await guard.assertHealthy();
  await attachFullStackEvidence(page, testInfo, { caseId: "PW-024", workflowId: fixture.name, readableFinal: true, visibleRawJson: false });
}

async function showsUsefulRealActionPalette({ page, request }, testInfo) {
  assertFullStackSpecHasNoRouteMocks();
  await assertRealServerReady(request);
  const fixture = await seedWorkflowFixture(request, "W1", caseSuffix("pw027", testInfo));
  const guard = await openRealWorkflow(page, request, fixture.name);
  await page.getByTestId("runner-node-picker-toggle").click();
  const palette = page.getByTestId("runner-node-picker");
  await expect(palette).toBeVisible();
  await expect(palette.getByTestId("catalog-action-script-shell")).toBeVisible();
  await expect(palette.getByTestId("catalog-action-script-python")).toBeVisible();
  await expect(palette.getByTestId("catalog-action-builtin-tcp_ping")).toHaveCount(0);
  await expect(palette.getByPlaceholder("搜索节点")).toBeVisible();
  await guard.assertHealthy();
  await attachFullStackEvidence(page, testInfo, { caseId: "PW-027", workflowId: fixture.name, usefulActions: ["script.shell", "script.python"], disallowedHidden: ["builtin.tcp_ping"] });
}

async function invalidatesDryRunReadinessAfterSemanticEdit({ page, request }, testInfo) {
  assertFullStackSpecHasNoRouteMocks();
  await assertRealServerReady(request);
  const history = await seedLabelTransaction(request, testInfo, "pw031", "PW-031 baseline");
  const guard = await openRealWorkflow(page, request, history.workflowId, history.revisionAfter);
  await page.getByTestId("workflow-context-tab-run").click();
  const run = page.getByTestId("workflow-run-panel");
  await expect(run).toHaveAttribute("data-gate-state", "validated");
  await run.getByTestId("workflow-draft-dry-run").click();
  await expect(run).toHaveAttribute("data-gate-state", "current");
  await expect(run).toHaveAttribute("data-publish-allowed", "true");

  await page.getByTestId("canvas-node-action").click();
  await page.getByTestId("runner-node-panel-tab-script").click();
  await page.getByTestId("runner-node-script-editor").fill("set -e\necho pw031-semantic-change");
  const commandResponse = page.waitForResponse((response) => response.url().endsWith("/api/runner-studio/workflow-commands") && response.request().method() === "POST");
  await page.getByTestId("runner-node-panel-apply").click();
  const mutation = await (await commandResponse).json();
  const revisionAfter = mutation?.receipt?.revisionAfter?.resourceVersion;
  expect(revisionAfter).toBeTruthy();
  await expect.poll(async () => resourceVersion(await readWorkflowGraph(request, history.workflowId))).toBe(revisionAfter);
  await page.getByTestId("workflow-context-tab-run").click();
  await expect(run).toHaveAttribute("data-gate-state", "validated");
  await expect(run).toHaveAttribute("data-publish-allowed", "false");
  await expect(run.getByTestId("workflow-draft-dry-run")).toBeEnabled();
  await guard.assertHealthy();
  await attachFullStackEvidence(page, testInfo, { caseId: "PW-031", workflowId: history.workflowId, revisionBefore: history.revisionAfter, revisionAfter, gateBefore: "current", gateAfter: "validated", publishAllowedAfter: false });
}

async function seedLabelTransaction(request, testInfo, prefix, label) {
  const fixture = await seedWorkflowFixture(request, "W1", caseSuffix(prefix, testInfo));
  const before = await readWorkflowGraph(request, fixture.name);
  const revisionBefore = resourceVersion(before);
  const committed = await commitManualNodeLabel(request, {
    workflowId: fixture.name,
    expectedRevision: revisionBefore,
    nodeId: "action",
    label,
    idempotencyKey: `${prefix}-${process.pid}-${testInfo.workerIndex}`,
  });
  return {
    workflowId: fixture.name,
    label,
    revisionBefore,
    revisionAfter: committed.receipt.revisionAfter.resourceVersion,
    transactionId: committed.receipt.transactionId,
  };
}

async function openRealWorkflow(page, request, workflowId, expectedRevision) {
  const guard = createRealPageGuard(page);
  await page.goto(`/runner/${encodeURIComponent(workflowId)}`, { waitUntil: "domcontentloaded" });
  await expect(page.getByTestId("runner-studio-page")).toBeVisible();
  await expect(page.getByTestId("runner-studio-canvas")).toBeVisible();
  await expect(page.getByTestId("canvas-node-action")).toBeVisible();
  const revision = expectedRevision || resourceVersion(await readWorkflowGraph(request, workflowId));
  await expect.poll(() => guard.latestWorkflowProjection()?.committed?.revision?.resourceVersion || "").toBe(revision);
  return guard;
}

function workflowReadPrompt(workflowId, revision) {
  return `WORKFLOW_MCP_READ tenant=runner-default workflow=${workflowId} revision=${revision} 请读取当前工作流摘要`;
}

function isApprovalDecisionRequest(request) {
  if (request.method() !== "POST" || !request.url().endsWith("/api/v1/assistant/transport")) return false;
  try {
    return request.postDataJSON()?.commands?.some((command) => command?.type === "aiops.approval-decision") === true;
  } catch {
    return false;
  }
}

function isStopRequest(request) {
  if (request.method() !== "POST" || !request.url().endsWith("/api/v1/assistant/transport")) return false;
  try {
    return request.postDataJSON()?.commands?.some((command) => command?.type === "aiops.stop") === true;
  } catch {
    return false;
  }
}

function workflowApplyPrompt(projection, { workflowId, nodeId, label = "unused", operation = "update" }) {
  const revision = fullProjectionRevision(projection);
  const environment = projection?.runGateFacts?.environment?.bindingVersion;
  if (typeof environment !== "string" || !environment.trim()) throw new Error("Workflow Apply Case requires a server-owned environment binding version");
  const labelBase64 = Buffer.from(label, "utf8").toString("base64url");
  return [
    "WORKFLOW_MCP_APPLY",
    "tenant=runner-default",
    `workflow=${workflowId}`,
    `revision=${revision.resourceVersion}`,
    `environment=${environment}`,
    `node=${nodeId}`,
    `label_b64=${labelBase64}`,
    ...(operation === "delete" ? ["operation=delete"] : []),
  ].join(" ");
}

function currentProjection(guard, workflowId) {
  const projection = guard.latestWorkflowProjection();
  const projectedWorkflowId = projection?.committed?.workflowKey?.workflowId || projection?.workflowKey?.workflowId;
  if (!projection || projectedWorkflowId !== workflowId) throw new Error(`no live Workflow projection for ${workflowId}`);
  fullProjectionRevision(projection);
  return projection;
}

function fullProjectionRevision(projection) {
  const revision = projection?.committed?.revision;
  if (![revision?.resourceVersion, revision?.semanticHash, revision?.layoutHash].every((value) => typeof value === "string" && value.trim())) {
    throw new Error(`Workflow projection has no complete committed revision: ${JSON.stringify(revision || {})}`);
  }
  return revision;
}

function graphFacts(payload) {
  return {
    revision: resourceVersion(payload),
    nodes: (payload?.nodes || payload?.graph?.nodes || []).map((node) => String(node.id)).sort(),
    edges: (payload?.edges || payload?.graph?.edges || []).map((edge) => String(edge.id || `${edge.source}-${edge.target}-${edge.kind || "next"}`)).sort(),
  };
}

function caseSuffix(prefix, testInfo) {
  return `${prefix}-${process.pid}-${testInfo.workerIndex}`;
}
