// @ts-check
import { expect, test } from "@playwright/test";

import { openWorkflowStudioFixture } from "../helpers/workflowStudioPageFixture.js";

test.describe("Workflow AI real-user transport boundary", () => {
  test("uses a workflow overlay drawer beside the independent node sidebar and sends an immutable typed workflow scope", async ({ page }) => {
    /** @type {Array<Record<string, any>>} */
    const transportRequests = [];
    /** @type {string[]} */
    const requestedUrls = [];
    page.on("request", (request) => requestedUrls.push(request.url()));

    const health = await openWorkflowStudioFixture(page, { width: 1440, height: 900 });
    await page.route("**/api/v1/assistant/transport", async (route) => {
      transportRequests.push(route.request().postDataJSON());
      await route.fulfill({
        status: 200,
        contentType: "text/plain; charset=utf-8",
        body: "aui-state:[]\n",
      });
    });

    await expect(page.getByTestId("runner-studio-canvas")).toBeVisible();
    await page.getByTestId("canvas-node-check-capacity").click();
    const nodeSidebar = page.getByTestId("workflow-node-sidebar");
    await expect(nodeSidebar).toBeVisible();
    await expect(page.getByTestId("workflow-context-dock")).toBeHidden();
    await expect(page.getByTestId("workflow-context-tab-configuration")).toHaveCount(0);
    const canvasBeforeDrawer = await page.getByTestId("workflow-context-canvas-pane").boundingBox();
    await page.getByTestId("runner-toolbar-ai-generate").click();

    const dock = page.getByTestId("workflow-context-dock");
    const panel = page.getByTestId("workflow-ai-transport-panel");
    await expect(dock).toBeVisible();
    await expect(dock).toHaveAttribute("data-scope", "workflow");
    await expect(nodeSidebar).toHaveAttribute("aria-hidden", "true");
    expectSameCanvasGeometry(await page.getByTestId("workflow-context-canvas-pane").boundingBox(), canvasBeforeDrawer);
    await expect(panel).toBeVisible();
    await expect(panel.getByTestId("workflow-scope-workflow")).toHaveCount(0);
    await expect(panel.getByTestId("workflow-scope-selection")).toHaveCount(0);
    await expect(panel.getByLabel("本次发送范围")).toHaveCount(0);
    await expect(page.getByTestId("workflow-ai-drawer")).toHaveCount(0);
    await expect(page.locator('[data-testid^="workflow-ai-plan"], [data-testid^="workflow-ai-result"], [data-testid^="workflow-ai-event"]')).toHaveCount(0);

    const prompt = "只把已选容量检查步骤的阈值调整为 25，并先给我看变更";
    await panel.getByTestId("workflow-ai-input").fill(prompt);
    await expect(panel.getByTestId("workflow-ai-send")).toBeEnabled();
    await panel.getByTestId("workflow-ai-send").click();
    await expect.poll(() => transportRequests.length).toBe(1);

    const payload = transportRequests[0];
    expect(payload.commands).toHaveLength(1);
    expect(payload.commands[0]).toMatchObject({
      type: "add-message",
      message: {
        role: "user",
        metadata: { workflowId: "capacity-inspection" },
        parts: [{ type: "text", text: prompt }],
        workflowContext: {
          workflowId: "capacity-inspection",
          revision: {
            resourceVersion: "fixture-rv-1",
            semanticHash: "sha256:fixture-semantic-1",
            layoutHash: "sha256:fixture-layout-1",
          },
          selection: { nodeIds: ["check-capacity"], edgeIds: [] },
          environment: {
            name: "staging",
            bindingVersion: "sha256:fixture-environment-binding-1",
          },
        },
      },
    });

    // Collapse the overlay before interacting with the canvas. Changing the
    // live selection after Send must not mutate the already captured turn.
    await page.getByTestId("workflow-context-drawer-close").click();
    await expect(dock).toBeHidden();
    await expect(nodeSidebar).toHaveAttribute("aria-hidden", "false");
    expectSameCanvasGeometry(await page.getByTestId("workflow-context-canvas-pane").boundingBox(), canvasBeforeDrawer);
    await page.getByTestId("runner-node-panel-close").click();
    await expect(nodeSidebar).toHaveCount(0);
    await page.getByTestId("rf__node-capacity-ok").click();
    expect(payload.commands[0].message.workflowContext.selection).toEqual({ nodeIds: ["check-capacity"], edgeIds: [] });
    expect(requestedUrls.some((url) => /\/workflow-ai(?:\/|\?|$)|\/ai\/draft(?:\/|\?|$)/.test(url))).toBe(false);
    await health.assertHealthy();
  });
});

function expectSameCanvasGeometry(actual, expected) {
  expect(actual).not.toBeNull();
  expect(expected).not.toBeNull();
  expect(Math.abs((actual?.x || 0) - (expected?.x || 0))).toBeLessThanOrEqual(1);
  expect(Math.abs((actual?.width || 0) - (expected?.width || 0))).toBeLessThanOrEqual(1);
}
