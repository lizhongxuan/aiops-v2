// @ts-check

import { expect } from "@playwright/test";

import {
  assertFullStackSpecHasNoRouteMocks,
  assertRealServerReady,
  attachFullStackEvidence,
  commitManualWorkflowCommand,
  createRealPageGuard,
  readWorkflowGraph,
  resourceVersion,
  seedWorkflowFixture,
} from "../../helpers/workflowCanvasServerHarness.js";

/**
 * Real full-stack scale/lifecycle Cases. A Case appears here only when its
 * required user journey is reachable through production browser behavior.
 */
export const workflowScaleLifecycleCaseImplementations = Object.freeze({
  "PW-028": navigatesLargeWorkflowDiffWithoutInteractionStalls,
});

/**
 * Precise remaining product boundaries. These are intentionally not test
 * implementations: no route/HAR interception, reducer injection, forged
 * lifecycle artifact, or test-only target endpoint may turn them green.
 */
export const workflowScaleLifecycleCaseBlockers = deepFreeze({
  "PW-011": {
    code: "TRUSTED_SECRET_CONFIGURATION_INGRESS_MISSING",
    stage: "configuration_postcheck",
    requiredPath: ["W5", "configuration_required", "trusted_secret_binding", "redacted_chat_dom_logs"],
    verifiedFixtureBoundary: "W5 now materializes the cloud connector with secret_ref=null and no credential value; it is never replaced by script.shell.",
    observedGap: "W5 can be stored as http.request with secret_ref=null, but WorkflowEditFacade.DryRunWorkflow maps the production Visual simulator only to passed or failed. No server-owned configuration resolver can produce configuration_required, and no trusted SecretRef selector/binder UI or API exists; outcome injection or a test Secret must not fake this journey.",
  },
  "PW-032": {
    code: "PUBLISHED_REVISION_CONTROL_PLANE_PATH_DEFERRED",
    stage: "phase_5_publish_execute",
    requiredPath: ["publish_current_revision", "edit_new_draft", "run_published_revision", "dry_run_current_draft"],
    verifiedRolloutBoundary: "The full-stack rollout is internal_apply and fail-closed: workflow_publish and workflow_execute remain false.",
    observedGap: "No production control-plane publish command writes a published lifecycle artifact, and RunnerStudioPage does not wire an immutable published-run callback; the legacy visual publish endpoint cannot be treated as the missing Phase 5 MCP lifecycle path.",
  },
  "PW-033": {
    code: "POSTCHECK_FAILURE_EXECUTOR_PATH_MISSING",
    stage: "postcheck_execution",
    requiredPath: ["successful_committed_apply", "draft_non_production_postcheck", "saved_with_issues", "publish_and_run_closed"],
    verifiedExecutorBoundary: "WorkflowEditFacade delegates to VisualWorkflowService.DryRunWithOptions, which performs validation and simulation but no action execution; after successful validation it always returns a valid dry-run result.",
    observedGap: "The public workflow-dry-run endpoint accepts only workflowId, complete server-derived basis, and environment. It exposes no outcome injection and the production executor has no naturally reachable failed post-check after a successful commit, so saved_with_issues cannot be honestly produced from the browser.",
  },
});

// Alias kept explicit for orchestrators that want to report only unresolved
// scale/lifecycle work without knowing this module's naming convention.
export const workflowScaleLifecycleRemainingBlockers = workflowScaleLifecycleCaseBlockers;

/**
 * PW-028 builds the real W4 Graph through the embedded Runner API, creates a
 * 64-operation reversible transaction as test setup, enters Undo through the
 * visible History UI, then performs 36 visible keyboard revisions on the
 * Ghost Candidate. The resulting authoritative Candidate has exactly 100
 * operations and is navigated through the rendered review controls.
 *
 * @param {{page: import("@playwright/test").Page, request: import("@playwright/test").APIRequestContext}} fixtures
 * @param {import("@playwright/test").TestInfo} testInfo
 */
export async function navigatesLargeWorkflowDiffWithoutInteractionStalls({ page, request }, testInfo) {
  // Building 100 authoritative operations includes 36 real Candidate revise
  // round-trips against the 500/700 Graph. Keep enough budget for a cold CI
  // host while all responsiveness assertions below retain tight UI limits.
  testInfo.setTimeout(900_000);
  assertFullStackSpecHasNoRouteMocks();
  await assertRealServerReady(request);

  const suffix = `pw028-${process.pid}-${testInfo.workerIndex}`;
  const fixture = await seedWorkflowFixture(request, "W4", suffix);
  const seeded = await readWorkflowGraph(request, fixture.name);
  const seedRevision = resourceVersion(seeded);
  expect(seedRevision).not.toBe("");
  expect(seeded.nodes).toHaveLength(500);
  expect(seeded.edges).toHaveLength(700);

  const setupCommands = Array.from({ length: 64 }, (_, offset) => {
    const nodeId = largeNodeId(offset + 1, 500);
    return { kind: "update_node", nodeId, nodePatch: { label: `PW-028 committed ${nodeId}` } };
  });
  const setup = await commitManualWorkflowCommand(request, {
    workflowId: fixture.name,
    expectedRevision: seedRevision,
    idempotencyKey: `pw028-batch-${process.pid}-${testInfo.workerIndex}`,
    reason: "PW-028 establishes one reversible 64-operation large-graph transaction",
    commands: setupCommands,
  });
  const committedRevision = String(setup.receipt.revisionAfter.resourceVersion);
  expect(committedRevision).not.toBe(seedRevision);

  const guard = createRealPageGuard(page);
  await page.goto(`/runner/${encodeURIComponent(fixture.name)}`, { waitUntil: "domcontentloaded" });
  await expect(page.getByTestId("runner-studio-page")).toBeVisible();
  await expect(page.getByTestId("runner-studio-canvas")).toBeVisible();
  await expect(page.getByRole("navigation", { name: "工作流结构导航" })).toContainText("500 个步骤 · 700 条连线", { timeout: 60_000 });
  // React Flow may cull a boundary node at the fitted viewport edge. The
  // authoritative API and Outline prove all 500 nodes; the Canvas assertion
  // ensures the large rendered surface is present without coupling the Case to
  // a particular viewport-culling implementation.
  expect(await page.locator('.runner-canvas-node[data-testid^="canvas-node-"]').count()).toBeGreaterThanOrEqual(490);
  await expect.poll(() => guard.latestWorkflowProjection()?.committed?.revision?.resourceVersion || "", { timeout: 30_000 })
    .toBe(committedRevision);

  // Enter the Candidate from the same visible History control an operator uses.
  await page.getByTestId("workflow-context-tab-history").click();
  const historyEntry = page.locator(`[data-transaction-id="${setup.receipt.transactionId}"]`);
  await expect(historyEntry).toBeVisible();
  const undoResponsePromise = page.waitForResponse((response) => (
    response.request().method() === "POST"
    && response.url().endsWith("/api/runner-studio/workflow-history-actions")
  ), { timeout: 90_000 });
  await historyEntry.getByRole("button", { name: "撤销此操作" }).click();
  const undoResponse = await undoResponsePromise;
  expect(undoResponse.status()).toBe(200);
  await expect(page.getByTestId("runner-candidate-canvas-status")).toContainText("尚未保存", { timeout: 30_000 });
  await page.getByTestId("workflow-context-tab-review").click();
  const review = page.getByTestId("workflow-change-review");
  await expect(review).toContainText("候选版本 1 · 共 64 项修改");
  expect(resourceVersion(await readWorkflowGraph(request, fixture.name))).toBe(committedRevision);

  // React Flow keyboard nudges are real visible Candidate edits. Await every
  // authoritative revise response and rendered Candidate revision; there is no
  // blind wait and no direct call to the Candidate mutation endpoint.
  // The fitted 500-node surface spans far beyond the physical viewport. Use
  // the visible Review navigation to locate the first changed object exactly,
  // then perform keyboard edits on that on-screen Ghost node.
  const revisedNode = page.getByTestId(`canvas-node-${largeNodeId(64, 500)}`);
  await review.locator("ol button").first().click();
  await expect(revisedNode).toBeInViewport({ timeout: 15_000 });
  let reviseRequestCount = 0;
  page.on("request", (outgoing) => {
    if (outgoing.method() === "POST" && outgoing.url().endsWith("/api/runner-studio/workflow-candidate-revise")) reviseRequestCount += 1;
  });
  for (let index = 0; index < 36; index += 1) {
    await revisedNode.click();
    const responsePromise = page.waitForResponse((response) => (
      response.request().method() === "POST"
      && response.url().endsWith("/api/runner-studio/workflow-candidate-revise")
    ));
    await page.keyboard.press("ArrowRight");
    const response = await responsePromise;
    expect(response.status()).toBe(200);
    await expect(review).toContainText(`候选版本 ${index + 2} · 共 ${index + 65} 项修改`, { timeout: 15_000 });
  }
  expect(reviseRequestCount).toBe(36);
  await expect(review.locator("ol > li")).toHaveCount(100);
  await expect(review).toContainText("共 100 项修改");
  expect(resourceVersion(await readWorkflowGraph(request, fixture.name))).toBe(committedRevision);

  // A 100-row review list intentionally scrolls. Bring the real footer
  // navigation into view as an operator would before measuring the 99 visible
  // Next interactions; do not rely on a hidden element or programmatic state.
  await expect(review).toContainText("1 / 100");
  const next = review.getByTestId("workflow-review-next");
  await next.scrollIntoViewIfNeeded();
  await expect(next).toBeVisible();
  await installLongTaskProbe(page);
  const navigationStartedAt = await browserNow(page);
  for (let index = 1; index < 100; index += 1) {
    await next.click({ timeout: 5_000 });
    await expect(review).toBeVisible();
  }
  await expect(review).toContainText("100 / 100");
  await expect(next).toBeDisabled();
  await expect(revisedNode).toHaveClass(/selected/);
  const navigationDurationMs = (await browserNow(page)) - navigationStartedAt;

  // Input remains interactive while the 100-item Candidate and 500/700 Canvas
  // stay mounted.
  await page.getByTestId("workflow-context-tab-ai").click();
  const input = page.getByTestId("workflow-ai-input");
  const inputStartedAt = await browserNow(page);
  await input.fill("PW-028 large diff input remains responsive");
  await expect(input).toHaveValue("PW-028 large diff input remains responsive");
  const inputDurationMs = (await browserNow(page)) - inputStartedAt;

  const viewport = page.locator(".react-flow__viewport");
  const zoomIn = page.locator(".react-flow__controls-zoomin");
  const transformBeforeZoom = await viewport.getAttribute("style");
  const zoomStartedAt = await browserNow(page);
  await zoomIn.click();
  await expect.poll(() => viewport.getAttribute("style")).not.toBe(transformBeforeZoom);
  const zoomDurationMs = (await browserNow(page)) - zoomStartedAt;

  const performance = await stopLongTaskProbe(page);
  expect(navigationDurationMs).toBeLessThan(30_000);
  expect(inputDurationMs).toBeLessThan(3_000);
  expect(zoomDurationMs).toBeLessThan(5_000);
  expect(performance.maxLongTaskMs).toBeLessThan(2_000);
  await guard.assertHealthy();
  await attachFullStackEvidence(page, testInfo, {
    caseId: "PW-028",
    fixtureId: "W4",
    workflowId: fixture.name,
    nodeCount: 500,
    edgeCount: 700,
    candidateOperationCount: 100,
    candidateRevision: 37,
    visibleCandidateRevisionCount: 36,
    reviseRequestCount,
    committedRevision,
    committedGraphUnchanged: true,
    performance: {
      navigationDurationMs: Math.round(navigationDurationMs),
      inputDurationMs: Math.round(inputDurationMs),
      zoomDurationMs: Math.round(zoomDurationMs),
      ...performance,
    },
  });
}

function largeNodeId(index, nodeCount) {
  return `node-${String(index).padStart(String(nodeCount - 1).length, "0")}`;
}

async function browserNow(page) {
  return page.evaluate(() => performance.now());
}

async function installLongTaskProbe(page) {
  await page.evaluate(() => {
    const target = /** @type {any} */ (window);
    target.__pw028LongTasks = [];
    target.__pw028LongTaskObserver?.disconnect?.();
    if (typeof PerformanceObserver === "undefined" || !PerformanceObserver.supportedEntryTypes?.includes("longtask")) return;
    target.__pw028LongTaskObserver = new PerformanceObserver((list) => {
      target.__pw028LongTasks.push(...list.getEntries().map((entry) => entry.duration));
    });
    target.__pw028LongTaskObserver.observe({ type: "longtask", buffered: false });
  });
}

async function stopLongTaskProbe(page) {
  return page.evaluate(() => {
    const target = /** @type {any} */ (window);
    target.__pw028LongTaskObserver?.takeRecords?.().forEach((entry) => target.__pw028LongTasks.push(entry.duration));
    target.__pw028LongTaskObserver?.disconnect?.();
    const durations = Array.isArray(target.__pw028LongTasks) ? target.__pw028LongTasks : [];
    return {
      longTaskCount: durations.length,
      maxLongTaskMs: Math.max(0, ...durations.map((value) => Number(value) || 0)),
    };
  });
}

function deepFreeze(value) {
  if (!value || typeof value !== "object" || Object.isFrozen(value)) return value;
  Object.freeze(value);
  for (const nested of Object.values(value)) deepFreeze(nested);
  return value;
}
