// @ts-check

import { expect } from "@playwright/test";

import {
  assertFullStackSpecHasNoRouteMocks,
  assertRealServerReady,
  attachFullStackEvidence,
  commitManualWorkflowCommand,
  createRealPageGuard,
  fullRevision,
  readApplicationState,
  readWorkflowGraph,
  resourceVersion,
  seedWorkflowFixture,
} from "../../helpers/workflowCanvasServerHarness.js";

/**
 * Resilience Cases exercise the real ai-server, persisted Runner graph,
 * AssistantTransport and browser DOM. They deliberately do not install a
 * Playwright route/HAR interceptor for any target API.
 */
export const workflowResilienceCaseImplementations = Object.freeze({
  "PW-008": rebasesSafeLayoutDrift,
  "PW-017": recoversAfterApplyResponseLoss,
  "PW-022": locatesExactBlockingDiagnosticField,
  "PW-025": protectsCandidateDuringWorkflowSwitch,
  "PW-026": keepsManualEditingWithAgentOff,
});

/**
 * All five Cases now have a real product path. Keep this export even when it
 * is empty so the manifest integrator can merge Case modules uniformly.
 */
export const workflowResilienceCaseBlockers = deepFreeze({});

/**
 * PW-008: a Candidate may remain editable across an authoritative layout-only
 * revision. The browser sends the latest complete revision and the server is
 * the final path-disjoint rebase arbiter; the committed graph is never replaced
 * by browser-composed state.
 *
 * @param {{page: import("@playwright/test").Page, request: import("@playwright/test").APIRequestContext}} fixtures
 * @param {import("@playwright/test").TestInfo} testInfo
 */
export async function rebasesSafeLayoutDrift({ page, request }, testInfo) {
  assertFullStackSpecHasNoRouteMocks();
  await assertRealServerReady(request);
  const fixture = await seedWorkflowFixture(request, "W1", caseSuffix("pw008", testInfo));
  const before = await readWorkflowGraph(request, fixture.name);
  const revisionBefore = resourceVersion(before);
  const committedLabel = nodeLabel(before, "action");
  const committedScript = nodeScript(before, "action");
  const endBefore = nodePosition(before, "end");
  const guard = await openRealWorkflow(page, request, fixture.name, revisionBefore);

  const candidateLabel = "PW-008 candidate survives layout drift";
  const panel = await createRealCandidate(page, guard, fixture.name, candidateLabel);
  const candidateStatus = page.getByTestId("runner-candidate-canvas-status");
  await expect(candidateStatus).toContainText("候选版本 1");
  await expect(page.getByTestId("canvas-node-action")).toContainText(candidateLabel);

  const movedEnd = { x: endBefore.x + 137, y: endBefore.y + 83 };
  const layoutCommit = await commitManualWorkflowCommand(request, {
    workflowId: fixture.name,
    expectedRevision: revisionBefore,
    idempotencyKey: `pw008-layout-${process.pid}-${testInfo.workerIndex}`,
    reason: "PW-008 simulates another user moving a path-disjoint node",
    command: { kind: "move", layout: { nodes: [{ nodeId: "end", position: movedEnd }] } },
  });
  const layoutRevision = String(layoutCommit.receipt.revisionAfter.resourceVersion);
  expect(layoutRevision).not.toBe(revisionBefore);
  await expect.poll(async () => resourceVersion(await readWorkflowGraph(request, fixture.name))).toBe(layoutRevision);
  await expect.poll(() => currentProjection(guard, fixture.name).committed?.revision?.resourceVersion || "").toBe(layoutRevision);
  await expect(candidateStatus).toContainText("候选版本 1");
  await expect(candidateStatus).not.toContainText("候选预览已阻止");
  await expect(page.getByTestId("canvas-node-action")).toContainText(candidateLabel);

  const expectedAfterLayout = fullRevision(currentProjection(guard, fixture.name));
  const revisedScript = "set -e\necho pw008-candidate-rebased";
  // Candidate clicks intentionally focus the matching Review diff first.
  // Enter is the documented canvas shortcut for opening configuration of the
  // selected node, and avoids racing React Flow's selection re-render between
  // the two physical clicks of a double-click.
  await page.getByTestId("canvas-node-action").click();
  await page.keyboard.press("Enter");
  await page.getByTestId("runner-node-panel-tab-script").click();
  await page.getByTestId("runner-node-script-editor").fill(revisedScript);
  const reviseResponsePromise = page.waitForResponse((response) => (
    response.url().endsWith("/api/runner-studio/workflow-candidate-revise")
    && response.request().method() === "POST"
  ));
  await page.getByTestId("runner-node-panel-apply").click();
  const reviseResponse = await reviseResponsePromise;
  expect(reviseResponse.status()).toBe(200);
  const reviseRequest = reviseResponse.request().postDataJSON();
  const revised = await reviseResponse.json();
  expect(reviseRequest).toMatchObject({
    schemaVersion: "aiops.workflow.edit.v2",
    workflowId: fixture.name,
    expected: expectedAfterLayout,
    intent: "用户调整候选节点参数",
  });
  expect(reviseRequest.baseChangeSetId).toBeTruthy();
  expect(revised).toMatchObject({
    kind: "preview",
    schemaVersion: "aiops.workflow.edit.v2",
    candidateRevision: 2,
    rebased: true,
    changeSet: {
      candidateRevision: 2,
      parentChangeSetId: reviseRequest.baseChangeSetId,
      base: expectedAfterLayout,
    },
  });
  await expect(candidateStatus).toContainText("候选版本 2");
  await expect(page.getByTestId("runner-node-script-editor")).toHaveValue(revisedScript);

  const committedAfter = await readWorkflowGraph(request, fixture.name);
  expect(resourceVersion(committedAfter)).toBe(layoutRevision);
  expect(nodePosition(committedAfter, "end")).toEqual(movedEnd);
  expect(nodeLabel(committedAfter, "action")).toBe(committedLabel);
  expect(nodeScript(committedAfter, "action")).toBe(committedScript);
  await guard.assertHealthy();
  await attachFullStackEvidence(page, testInfo, {
    caseId: "PW-008",
    workflowId: fixture.name,
    revisionBefore,
    layoutRevision,
    layoutTransactionId: layoutCommit.receipt.transactionId,
    movedNode: { nodeId: "end", before: endBefore, after: movedEnd },
    latestExpectedRevisionSent: reviseRequest.expected,
    candidateRevision: revised.candidateRevision,
    candidateRebased: revised.rebased,
    candidateCommitted: false,
    committedSemanticValuePreserved: true,
  });
  // Keep the transport panel referenced until all DOM assertions settle; this
  // also proves the real Candidate originated from AssistantTransport.
  await expect(panel).toBeAttached();
}

/**
 * PW-017: the server commits an approved Apply, but Chromium aborts delivery of
 * that one real response after response headers arrive. Reload must recover the
 * authoritative graph and exactly one durable receipt. CDP Fetch is used only
 * as a browser transport fault boundary; it does not fabricate a response.
 *
 * @param {{page: import("@playwright/test").Page, request: import("@playwright/test").APIRequestContext}} fixtures
 * @param {import("@playwright/test").TestInfo} testInfo
 */
export async function recoversAfterApplyResponseLoss({ page, request }, testInfo) {
  assertFullStackSpecHasNoRouteMocks();
  await assertRealServerReady(request);
  if (testInfo.project.name !== "chromium") {
    throw new Error("PW-017 requires Chromium CDP to lose one real response without page.route or a synthetic response");
  }
  const fixture = await seedWorkflowFixture(request, "W1", caseSuffix("pw017", testInfo));
  const before = await readWorkflowGraph(request, fixture.name);
  const revisionBefore = resourceVersion(before);
  const guard = await openRealWorkflow(page, request, fixture.name, revisionBefore);
  const historyBefore = currentProjection(guard, fixture.name).durableHistory?.length || 0;
  const label = "PW-017 committed although response was lost";
  const panel = await createRealCandidate(page, guard, fixture.name, label);
  await page.getByTestId("workflow-context-tab-ai").click();
  const approval = panel.getByTestId("workflow-approval-accept").last();
  await expect(approval).toBeVisible();

  const cdp = await page.context().newCDPSession(page);
  /** @type {Set<string>} */
  const pausedRequestIds = new Set();
  /** @type {(event: any) => void} */
  let resolveApplyResponse;
  /** @type {(error: Error) => void} */
  let rejectApplyResponse;
  const applyResponsePaused = new Promise((resolve, reject) => {
    resolveApplyResponse = resolve;
    rejectApplyResponse = reject;
  });
  const onPaused = async (event) => {
    pausedRequestIds.add(event.requestId);
    try {
      if (event.responseStatusCode && isApprovalDecisionPostData(event.request?.postData)) {
        resolveApplyResponse(event);
        return;
      }
      await continueRealResponse(cdp, event.requestId);
      pausedRequestIds.delete(event.requestId);
    } catch (error) {
      rejectApplyResponse(error instanceof Error ? error : new Error(String(error)));
    }
  };
  cdp.on("Fetch.requestPaused", onPaused);
  await cdp.send("Fetch.enable", {
    patterns: [{ urlPattern: "*api/v1/assistant/transport*", requestStage: "Response" }],
  });

  /** @type {any} */
  let pausedApply;
  try {
    await approval.click();
    pausedApply = await applyResponsePaused;
    expect(pausedApply.responseStatusCode).toBe(200);

    // The handler has already committed before it starts the HTTP response.
    // The independent API context proves durable truth while the page still
    // has no response body.
    await expect.poll(async () => resourceVersion(await readWorkflowGraph(request, fixture.name))).not.toBe(revisionBefore);
    const committed = await readWorkflowGraph(request, fixture.name);
    const revisionAfter = resourceVersion(committed);
    expect(nodeLabel(committed, "action")).toBe(label);

    const failedRequestPromise = page.waitForEvent("requestfailed", {
      predicate: (failed) => isApprovalDecisionRequest(failed),
    });
    await cdp.send("Fetch.failRequest", { requestId: pausedApply.requestId, errorReason: "Aborted" });
    pausedRequestIds.delete(pausedApply.requestId);
    const failedRequest = await failedRequestPromise;
    expect(failedRequest.failure()?.errorText || "").toMatch(/abort|failed/i);

    await cdp.send("Fetch.disable");
    await page.reload({ waitUntil: "domcontentloaded" });
    await expect(page.getByTestId("runner-studio-page")).toBeVisible();
    await expect(page.getByTestId("canvas-node-action")).toContainText(label);
    await expect(page.getByTestId("runner-candidate-canvas-status")).toHaveCount(0);
    await expect.poll(() => currentProjection(guard, fixture.name).committed?.revision?.resourceVersion || "").toBe(revisionAfter);

    const recovered = currentProjection(guard, fixture.name);
    const matchingReceipts = (recovered.receipts || []).filter((receipt) => (
      receipt.revisionBefore?.resourceVersion === revisionBefore
      && receipt.revisionAfter?.resourceVersion === revisionAfter
    ));
    expect(matchingReceipts).toHaveLength(1);
    const history = recovered.durableHistory || [];
    expect(history).toHaveLength(historyBefore + 1);
    expect(history.filter((entry) => entry.transactionId === matchingReceipts[0].transactionId)).toHaveLength(1);
    expect(new Set(history.map((entry) => entry.transactionId)).size).toBe(history.length);

    const stable = await readWorkflowGraph(request, fixture.name);
    expect(resourceVersion(stable)).toBe(revisionAfter);
    expect(nodeLabel(stable, "action")).toBe(label);
    expect(guard.consoleProblems).toEqual([]);
    const expectedFailures = guard.networkProblems.filter((problem) => problem.includes("/api/v1/assistant/transport"));
    expect(expectedFailures).toHaveLength(1);
    expect(guard.networkProblems).toEqual(expectedFailures);
    await attachFullStackEvidence(page, testInfo, {
      caseId: "PW-017",
      workflowId: fixture.name,
      revisionBefore,
      revisionAfter,
      applyResponseStatusBeforeAbort: pausedApply.responseStatusCode,
      browserFailure: failedRequest.failure()?.errorText,
      recoveredReceipt: matchingReceipts[0],
      historyBefore,
      historyAfter: history.length,
      exactlyOnce: true,
      syntheticResponseUsed: false,
    });
  } finally {
    cdp.off("Fetch.requestPaused", onPaused);
    for (const requestId of pausedRequestIds) {
      await continueRealResponse(cdp, requestId).catch(() => undefined);
    }
    await cdp.send("Fetch.disable").catch(() => undefined);
    await cdp.detach().catch(() => undefined);
  }
}

/**
 * PW-022: a real invalid Candidate diagnostic must locate the exact node and
 * editor field, not merely open a generic configuration panel.
 *
 * @param {{page: import("@playwright/test").Page, request: import("@playwright/test").APIRequestContext}} fixtures
 * @param {import("@playwright/test").TestInfo} testInfo
 */
export async function locatesExactBlockingDiagnosticField({ page, request }, testInfo) {
  assertFullStackSpecHasNoRouteMocks();
  await assertRealServerReady(request);
  const fixture = await seedWorkflowFixture(request, "W1", caseSuffix("pw022", testInfo));
  const before = await readWorkflowGraph(request, fixture.name);
  const revisionBefore = resourceVersion(before);
  const committedScript = nodeScript(before, "action");
  const guard = await openRealWorkflow(page, request, fixture.name, revisionBefore);
  await createRealCandidate(page, guard, fixture.name, "PW-022 exact diagnostic target");

  await page.getByTestId("canvas-node-action").click();
  await page.keyboard.press("Enter");
  await page.getByTestId("runner-node-panel-tab-script").click();
  await page.getByTestId("runner-node-script-editor").fill("");
  const reviseResponsePromise = page.waitForResponse((response) => (
    response.url().endsWith("/api/runner-studio/workflow-candidate-revise")
    && response.request().method() === "POST"
  ));
  await page.getByTestId("runner-node-panel-apply").click();
  const reviseResponse = await reviseResponsePromise;
  expect(reviseResponse.status()).toBe(200);
  const revised = await reviseResponse.json();
  const exactDiagnostics = revised?.changeSet?.validation?.diagnostics?.filter((diagnostic) => (
    diagnostic?.nodeId === "action" && diagnostic?.field === "step.args.script"
  )) || [];
  expect(revised?.changeSet?.validation?.valid).toBe(false);
  expect(exactDiagnostics).toHaveLength(1);

  await page.getByTestId("workflow-context-tab-review").click();
  const issue = page.getByTestId("workflow-validation-issue-candidate-0");
  await expect(issue).toBeVisible();
  await expect(issue).toHaveAttribute("data-workflow-node-id", "action");
  await expect(issue).toHaveAttribute("data-workflow-field", "step.args.script");
  await issue.click();

  await expect(page.getByTestId("workflow-context-tab-configuration")).toHaveAttribute("aria-selected", "true");
  await expect(page.getByTestId("canvas-node-action")).toHaveClass(/\bselected\b/);
  await expect(page.getByTestId("runner-node-panel-tab-script")).toHaveClass(/\bactive\b/);
  const editor = page.getByTestId("runner-node-script-editor");
  await expect(editor).toHaveValue("");
  await expect(editor).toHaveAttribute("data-workflow-diagnostic-focus", "step.args.script");
  await expect(editor).toBeFocused();

  const committedAfter = await readWorkflowGraph(request, fixture.name);
  expect(resourceVersion(committedAfter)).toBe(revisionBefore);
  expect(nodeScript(committedAfter, "action")).toBe(committedScript);
  await guard.assertHealthy();
  await attachFullStackEvidence(page, testInfo, {
    caseId: "PW-022",
    workflowId: fixture.name,
    revision: revisionBefore,
    diagnostic: exactDiagnostics[0],
    focusedNodeId: await issue.getAttribute("data-workflow-node-id"),
    focusedField: await editor.getAttribute("data-workflow-diagnostic-focus"),
    exactEditorFocused: await editor.evaluate((element) => document.activeElement === element),
    committedUnchanged: true,
  });
}

/**
 * PW-025: leaving a workflow with a live Candidate requires an explicit
 * keep or server-confirmed discard decision. Keeping must not navigate; discard
 * must occur exactly once before navigation.
 *
 * @param {{page: import("@playwright/test").Page, request: import("@playwright/test").APIRequestContext}} fixtures
 * @param {import("@playwright/test").TestInfo} testInfo
 */
export async function protectsCandidateDuringWorkflowSwitch({ page, request }, testInfo) {
  assertFullStackSpecHasNoRouteMocks();
  await assertRealServerReady(request);
  const suffix = caseSuffix("pw025", testInfo);
  const first = await seedWorkflowFixture(request, "W1", `${suffix}-first`);
  const second = await seedWorkflowFixture(request, "W1", `${suffix}-second`);
  const firstBefore = await readWorkflowGraph(request, first.name);
  const firstRevision = resourceVersion(firstBefore);
  const firstLabel = nodeLabel(firstBefore, "action");
  const guard = await openRealWorkflow(page, request, first.name, firstRevision);
  const candidateLabel = "PW-025 candidate must not be silently lost";
  await createRealCandidate(page, guard, first.name, candidateLabel);
  await page.getByTestId("workflow-context-tab-ai").click();

  /** @type {Array<Record<string, any>>} */
  const discardBodies = [];
  page.on("request", (outgoing) => {
    if (outgoing.method() !== "POST" || !outgoing.url().endsWith("/api/runner-studio/workflow-candidate-actions")) return;
    try {
      const body = outgoing.postDataJSON();
      if (body?.action === "discard") discardBodies.push(body);
    } catch {
      // The strict count and body assertions below fail if this is malformed.
    }
  });

  await page.getByTestId("runner-back-to-library").click();
  const keepDialog = page.getByTestId("workflow-candidate-switch-confirm");
  await expect(keepDialog).toBeVisible();
  await expect(keepDialog).toContainText("工作流列表");
  await keepDialog.getByTestId("workflow-candidate-switch-keep").click();
  await expect(keepDialog).toHaveCount(0);
  await expect(page).toHaveURL(new RegExp(`/runner/${escapeRegExp(encodeURIComponent(first.name))}$`));
  await expect(page.getByTestId("canvas-node-action")).toContainText(candidateLabel);
  expect(discardBodies).toHaveLength(0);

  await page.getByTestId("runner-back-to-library").click();
  const discardDialog = page.getByTestId("workflow-candidate-switch-confirm");
  await expect(discardDialog).toBeVisible();
  const discardResponsePromise = page.waitForResponse((response) => (
    response.url().endsWith("/api/runner-studio/workflow-candidate-actions")
    && response.request().method() === "POST"
  ));
  await discardDialog.getByTestId("workflow-candidate-switch-discard").click();
  const discardResponse = await discardResponsePromise;
  expect(discardResponse.status()).toBe(200);
  await expect(discardDialog).toHaveCount(0);
  await expect(page).toHaveURL(/\/runner$/);
  await expect(page.getByTestId("runner-workflow-library")).toBeVisible();
  await page.locator(".runner-studio-workflow", { hasText: second.name }).click();
  await expect(page).toHaveURL(new RegExp(`/runner/${escapeRegExp(encodeURIComponent(second.name))}$`));
  await expect(page.getByTestId("runner-workflow-title-display")).toContainText(second.name);
  await expect(page.getByTestId("canvas-node-action")).toBeVisible();

  expect(discardBodies).toHaveLength(1);
  expect(discardBodies[0]).toMatchObject({
    schemaVersion: "aiops.workflow.edit.v2",
    workflowId: first.name,
    action: "discard",
    candidateRevision: 1,
    expected: { resourceVersion: firstRevision },
  });
  expect(discardBodies[0].candidateId).toBeTruthy();
  expect(discardBodies[0].changeSetId).toBeTruthy();
  expect(discardBodies[0].previewHash).toBeTruthy();
  expect(discardBodies[0].idempotencyKey).toBeTruthy();
  const firstAfter = await readWorkflowGraph(request, first.name);
  expect(resourceVersion(firstAfter)).toBe(firstRevision);
  expect(nodeLabel(firstAfter, "action")).toBe(firstLabel);
  await guard.assertHealthy();
  await attachFullStackEvidence(page, testInfo, {
    caseId: "PW-025",
    sourceWorkflowId: first.name,
    targetWorkflowId: second.name,
    keepDecisionNavigated: false,
    discardRequestCount: discardBodies.length,
    discardBinding: discardBodies[0],
    sourceCommittedUnchanged: true,
    targetVisibleAfterServerDiscard: true,
  });
}

/**
 * PW-026: when the server is genuinely restarted at disabled rollout, every
 * Workflow Agent capability and AI entry point stays off while normal manual
 * node insertion/connection and server validation remain usable.
 *
 * @param {{page: import("@playwright/test").Page, request: import("@playwright/test").APIRequestContext}} fixtures
 * @param {import("@playwright/test").TestInfo} testInfo
 */
export async function keepsManualEditingWithAgentOff({ page, request }, testInfo) {
  assertFullStackSpecHasNoRouteMocks();
  const ready = await assertRealServerReady(request);
  const capabilitySnapshot = /** @type {any} */ (ready.state)?.config?.workflowCapabilities;
  expect(capabilitySnapshot).toMatchObject({
    schemaVersion: "aiops.workflow.rollout.v1",
    stage: "disabled",
  });
  expect(Object.keys(capabilitySnapshot?.capabilities || {}).length).toBeGreaterThan(0);
  expect(Object.values(capabilitySnapshot.capabilities).every((enabled) => enabled === false)).toBe(true);

  const fixture = await seedWorkflowFixture(request, "W1", caseSuffix("pw026", testInfo));
  const before = await readWorkflowGraph(request, fixture.name);
  const revisionBefore = resourceVersion(before);
  const beforeNodeIds = new Set(graphNodes(before).map((node) => String(node.id)));
  const guard = createRealPageGuard(page);
  await page.goto(`/runner/${encodeURIComponent(fixture.name)}`, { waitUntil: "domcontentloaded" });
  await expect(page.getByTestId("runner-studio-page")).toBeVisible();
  await expect(page.getByTestId("runner-studio-canvas")).toBeVisible();
  await expect(page.getByTestId("runner-toolbar-ai-generate")).toHaveCount(0);
  await expect(page.getByTestId("workflow-context-tab-ai")).toHaveCount(0);
  await expect(page.getByTestId("runner-node-picker-toggle")).toBeVisible();

  const addResponsePromise = page.waitForResponse((response) => {
    if (!response.url().endsWith("/api/runner-studio/workflow-commands") || response.request().method() !== "POST") return false;
    try {
      const body = response.request().postDataJSON();
      return commandList(body).some((command) => command?.kind === "add_node");
    } catch {
      return false;
    }
  });
  // W1's action -> end edge is an explicit success branch, so the generic
  // palette cannot safely infer an insertion point. Exercise the visible e1
  // edge insertion affordance instead: this is the manual operation that
  // atomically adds the node and reconnects both sides of start -> action.
  const edgeHitTarget = page.locator('.react-flow__edge[data-id="e1"] .runner-flow-edge-hover-path');
  await expect(edgeHitTarget).toHaveCount(1);
  await edgeHitTarget.hover({ force: true });
  await expect(page.getByTestId("runner-edge-insert-e1")).toBeVisible();
  await page.getByTestId("runner-edge-insert-e1").click();
  const edgePicker = page.getByTestId("runner-edge-node-picker");
  await expect(edgePicker).toBeVisible();
  await edgePicker.getByTestId("edge-catalog-action-script-shell").click();
  const addResponse = await addResponsePromise;
  expect(addResponse.status()).toBe(200);
  const addRequest = addResponse.request().postDataJSON();
  const commands = commandList(addRequest);
  const addCommands = commands.filter((command) => command?.kind === "add_node");
  const connectCommands = commands.filter((command) => command?.kind === "connect_edge");
  expect(addCommands).toHaveLength(1);
  expect(connectCommands.length).toBeGreaterThanOrEqual(2);
  const addedNodeId = String(addCommands[0]?.node?.id || "");
  expect(addedNodeId).not.toBe("");
  expect(beforeNodeIds.has(addedNodeId)).toBe(false);
  const addResult = await addResponse.json();
  const revisionAfterAdd = String(addResult?.receipt?.revisionAfter?.resourceVersion || "");
  expect(revisionAfterAdd).not.toBe("");
  expect(revisionAfterAdd).not.toBe(revisionBefore);
  await expect.poll(async () => resourceVersion(await readWorkflowGraph(request, fixture.name))).toBe(revisionAfterAdd);
  await expect(page.getByTestId(`canvas-node-${addedNodeId}`)).toBeVisible();

  const afterAdd = await readWorkflowGraph(request, fixture.name);
  const edgesAfterAdd = graphEdges(afterAdd);
  expect(edgesAfterAdd.some((edge) => edge.source === "start" && edge.target === addedNodeId)).toBe(true);
  expect(edgesAfterAdd.some((edge) => edge.source === addedNodeId && edge.target === "action")).toBe(true);

  const validateResponsePromise = page.waitForResponse((response) => (
    response.url().endsWith(`/api/runner-studio/workflows/${encodeURIComponent(fixture.name)}/validate`)
    && response.request().method() === "POST"
  ));
  await page.getByTestId("runner-toolbar-more").click();
  await page.getByTestId("runner-toolbar-validate").click();
  const validateResponse = await validateResponsePromise;
  expect(validateResponse.status()).toBe(200);
  const validation = await validateResponse.json();
  expect(validation?.valid ?? validation?.data?.valid).toBe(true);
  await expect(page.getByTestId("workflow-command-error-toast")).toHaveCount(0);
  await expect(page.getByTestId("workflow-validation-status")).toBeVisible();

  const stateAfter = await readApplicationState(request);
  expect(stateAfter?.config?.workflowCapabilities).toEqual(capabilitySnapshot);
  await guard.assertHealthy();
  await attachFullStackEvidence(page, testInfo, {
    caseId: "PW-026",
    workflowId: fixture.name,
    rolloutStage: capabilitySnapshot.stage,
    enabledCapabilities: Object.entries(capabilitySnapshot.capabilities).filter(([, enabled]) => enabled).map(([name]) => name),
    aiToolbarVisible: false,
    manualNodeId: addedNodeId,
    manualCommands: commands.map((command) => command.kind),
    revisionBefore,
    revisionAfterAdd,
    validationValid: validation?.valid ?? validation?.data?.valid,
  });
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

async function createRealCandidate(page, guard, workflowId, label) {
  const projection = currentProjection(guard, workflowId);
  await page.getByTestId("runner-toolbar-ai-generate").click();
  const panel = page.getByTestId("workflow-ai-transport-panel");
  await expect(panel).toBeVisible();
  await panel.getByTestId("workflow-ai-input").fill(workflowApplyPrompt(projection, { workflowId, nodeId: "action", label }));
  await panel.getByTestId("workflow-ai-send").click();
  await expect(page.getByTestId("runner-candidate-canvas-status")).toContainText("尚未保存");
  await expect(page.getByTestId("canvas-node-action")).toHaveAttribute("data-candidate-change", "changed");
  await expect(page.getByTestId("canvas-node-action")).toContainText(label);
  await expect(panel.getByTestId("workflow-approval-card").last()).toHaveAttribute("data-approval-state", "pending");
  return panel;
}

function workflowApplyPrompt(projection, { workflowId, nodeId, label }) {
  const revision = fullRevision(projection);
  const environment = projection?.runGateFacts?.environment?.bindingVersion;
  if (typeof environment !== "string" || !environment.trim()) {
    throw new Error("Workflow resilience Case requires the server-owned environment binding version");
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

function currentProjection(guard, workflowId) {
  const projection = guard.latestWorkflowProjection();
  const projectedWorkflowId = projection?.committed?.workflowKey?.workflowId || projection?.workflowKey?.workflowId;
  if (!projection || projectedWorkflowId !== workflowId) throw new Error(`no live Workflow projection for ${workflowId}`);
  fullRevision(projection);
  return projection;
}

async function continueRealResponse(cdp, requestId) {
  try {
    await cdp.send("Fetch.continueResponse", { requestId });
  } catch {
    await cdp.send("Fetch.continueRequest", { requestId });
  }
}

function isApprovalDecisionPostData(value) {
  if (typeof value !== "string" || !value.trim()) return false;
  try {
    return JSON.parse(value)?.commands?.some((command) => command?.type === "aiops.approval-decision") === true;
  } catch {
    return false;
  }
}

function isApprovalDecisionRequest(request) {
  if (request.method() !== "POST" || !request.url().endsWith("/api/v1/assistant/transport")) return false;
  try {
    return request.postDataJSON()?.commands?.some((command) => command?.type === "aiops.approval-decision") === true;
  } catch {
    return false;
  }
}

function graphNodes(payload) {
  return payload?.nodes || payload?.graph?.nodes || [];
}

function graphEdges(payload) {
  return payload?.edges || payload?.graph?.edges || [];
}

function nodeLabel(payload, nodeId) {
  return String(graphNodes(payload).find((node) => node?.id === nodeId)?.label || "");
}

function nodeScript(payload, nodeId) {
  return String(graphNodes(payload).find((node) => node?.id === nodeId)?.step?.args?.script || "");
}

function nodePosition(payload, nodeId) {
  const position = graphNodes(payload).find((node) => node?.id === nodeId)?.position;
  if (!position || !Number.isFinite(position.x) || !Number.isFinite(position.y)) {
    throw new Error(`workflow node ${nodeId} has no finite position`);
  }
  return { x: Number(position.x), y: Number(position.y) };
}

function commandList(payload) {
  if (Array.isArray(payload?.commands)) return payload.commands;
  return payload?.command ? [payload.command] : [];
}

function caseSuffix(prefix, testInfo) {
  return `${prefix}-${process.pid}-${testInfo.workerIndex}`;
}

function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function deepFreeze(value) {
  Object.values(value).forEach((item) => {
    if (item && typeof item === "object") deepFreeze(item);
  });
  return Object.freeze(value);
}
