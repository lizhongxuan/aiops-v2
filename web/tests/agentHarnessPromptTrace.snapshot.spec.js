import { expect, test } from "@playwright/test";

const tracePath = ".data/model-input-traces/sess-control/turn-control/iteration-001.json";

const traceDocument = {
  schemaVersion: "aiops.trace/v2",
  kind: "runtime_model_input",
  sessionId: "sess-control",
  turnId: "turn-control",
  iteration: 1,
  promptFingerprint: {
    absoluteSystemHash: "sha256:absolute-system-v1",
    roleProfileHash: "sha256:role-profile-v2",
    stableRuntimeContractHash: "sha256:runtime-contract-v1",
    stablePrefixHash: "sha256:stable-prefix-v1",
    turnStableHash: "sha256:turn-stable-v1",
    conversationHistoryHash: "sha256:history-v4",
    dynamicContextHash: "sha256:dynamic-context-v7",
    currentUserInputHash: "sha256:user-input-v1",
    modelInputHash: "sha256:model-input-v9",
  },
  previousPromptFingerprint: {
    absoluteSystemHash: "sha256:absolute-system-v1",
    roleProfileHash: "sha256:role-profile-v1",
    stableRuntimeContractHash: "sha256:runtime-contract-v1",
    stablePrefixHash: "sha256:stable-prefix-v1",
    turnStableHash: "sha256:turn-stable-v1",
    conversationHistoryHash: "sha256:history-v3",
    dynamicContextHash: "sha256:dynamic-context-v6",
    currentUserInputHash: "sha256:user-input-v1",
    modelInputHash: "sha256:model-input-v8",
  },
  stepContext: {
    hash: "sha256:step-context-008",
    turnAssemblyHash: "sha256:turn-assembly-001",
    checkpointRef: "checkpoint:approval-42",
    toolSurfaceFingerprint: "sha256:tool-surface-008",
    toolPolicyHash: "sha256:tool-policy-003",
    stepReference: { transition: { revisions: [{ kind: "approval_resumed" }] } },
    modelInput: [{ index: 0, providerRole: "user", semanticRole: "user", content: "敏感动态原文不应出现在控制链视图" }],
  },
  toolSurface: {
    modelVisibleTools: ["inspect_service", "restart_service"],
    dispatchableTools: ["inspect_service"],
    hiddenReasons: { restart_service: "approval_context_stale" },
  },
};

const controlChain = {
  schemaVersion: "aiops.canonical-rollout.v1",
  sessionId: "sess-control",
  turnId: "turn-control",
  available: true,
  headRef: { sequence: 4, eventId: "event:approval-decided", hash: "sha256:event-approval-decided" },
  events: [
      {
        sequence: 1,
        kind: "admission",
        eventId: "event:admission",
        hash: "sha256:event-admission",
        owner: "admission",
      },
      {
        sequence: 2,
        kind: "assembly",
        eventId: "event:assembly",
        hash: "sha256:event-assembly",
        owner: "assembly",
        turnAssemblyHash: "sha256:turn-assembly-001",
      },
      {
        sequence: 3,
        kind: "prompt",
        eventId: "event:prompt",
        hash: "sha256:event-prompt",
        owner: "prompt",
        turnAssemblyHash: "sha256:turn-assembly-001",
        stepContextHash: "sha256:step-context-008",
      },
      {
        sequence: 4,
        kind: "approval_decided",
        eventId: "event:approval-decided",
        hash: "sha256:event-approval-decided",
        owner: "approval",
        turnAssemblyHash: "sha256:turn-assembly-001",
        stepContextHash: "sha256:step-context-008",
        stepRevisionKind: "approval_resumed",
        sourceRefs: ["checkpoint:approval-42"],
        payloadRefs: {
          approvalId: "approval-42",
          actionTokenHash: "sha256:approval-token-42",
          permissionHash: "sha256:permission-4",
          mismatchFields: ["permission"],
          checkpointRef: "checkpoint:approval-42",
        },
      },
    ],
};

test("Prompt Trace highlights the first canonical control-chain divergence owner", async ({ page }) => {
  await page.route("**/api/v1/debug/model-input-traces**", async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname.endsWith("/file")) {
      await route.fulfill({ json: { content: JSON.stringify(traceDocument) } });
      return;
    }
    if (url.searchParams.get("includeControlChain") === "true") {
      await route.fulfill({ json: { traces: [], controlChain } });
      return;
    }
    await route.fulfill({
      json: {
        selectedId: tracePath,
        traces: [{
          id: tracePath,
          sessionId: "sess-control",
          turnId: "turn-control",
          iteration: 1,
          messageCount: 1,
          visibleTools: ["inspect_service", "restart_service"],
          createdAt: "2026-07-14T06:00:00Z",
          userPromptPreview: "诊断本次 Turn 的控制链分歧",
          relativePath: tracePath,
          jsonPath: tracePath,
          markdownPath: tracePath.replace(/\.json$/, ".md"),
        }],
      },
    });
  });

  await page.goto("/debug/prompts", { waitUntil: "networkidle" });
  await page.getByTestId("prompt-trace-llm-card").click();
  await page.getByRole("button", { name: "控制链", exact: true }).click();

  const panel = page.getByTestId("prompt-trace-control-chain");
  await expect(panel).toBeVisible();
  await expect(page.getByTestId("prompt-trace-first-divergence-owner")).toContainText("approval");
  await expect(panel).not.toContainText("敏感动态原文");
  await expect(page.locator('[data-testid^="prompt-trace-hash-"]')).toHaveCount(9);
  await expect(page.getByTestId("prompt-trace-hash-absoluteSystemHash")).toContainText("L0");
  await expect(page.getByTestId("prompt-trace-hash-dynamicContextHash")).toContainText("L5");
  await expect(page.getByTestId("prompt-trace-hash-currentUserInputHash")).toContainText("L6");
  const bindings = page.getByTestId("prompt-trace-control-bindings");
  await expect(bindings).toContainText("permission");
  await expect(bindings).toContainText("checkpoint:approval-42");
  await expect(bindings).toContainText("visible-only [restart_service]");
  await expect(page.getByTestId("prompt-trace-control-ref-approval")).toContainText("approval-42");
  await expect(page.getByTestId("prompt-trace-control-ref-token")).toContainText("sha256:a");
  await expect(page.getByTestId("prompt-trace-control-ref-rollout")).toContainText("#4 · event:approval-decided");
  await expect(page.getByTestId("prompt-trace-control-ref-rollout")).toHaveAttribute("title", /^#4 · event:approval-decided · sha256:e/);

  await expect(panel).toHaveScreenshot("agent-harness-prompt-trace-control-chain.png");
  await expect(bindings).toHaveScreenshot("agent-harness-prompt-trace-bindings.png");
});
