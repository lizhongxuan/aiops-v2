// @ts-check
import { expect, test } from "@playwright/test";

import { openWorkflowStudioFixture } from "../helpers/workflowStudioPageFixture.js";

test.describe("Workflow Studio run recovery", () => {
  test("[UX-PW-008] a failed node run retries its exact target without losing AI or node context", async ({ page }) => {
    const health = await openWorkflowStudioFixture(page, { width: 1274, height: 964 });
    const runIds = ["run-node-failed", "run-node-retry"];
    /** @type {Record<string, unknown>[]} */
    const runRequests = [];

    await page.route("**/api/runner-studio/runs", async (route) => {
      const body = /** @type {Record<string, unknown>} */ (route.request().postDataJSON());
      runRequests.push(body);
      const runId = runIds[Math.min(runRequests.length - 1, runIds.length - 1)];
      await route.fulfill({ json: { run_id: runId, status: "running" } });
    });
    await page.route(`**/api/runner-studio/runs/${runIds[0]}/events/history`, (route) => route.fulfill({
      json: [
        { type: "run_start", run_id: runIds[0], status: "running", timestamp: "2026-07-24T08:00:00Z" },
        {
          type: "node.failed",
          run_id: runIds[0],
          node_id: "check-capacity",
          status: "failed",
          error: "fixture remote unavailable",
          timestamp: "2026-07-24T08:00:01Z",
        },
        {
          type: "run.failed",
          run_id: runIds[0],
          status: "failed",
          error: "fixture remote unavailable",
          timestamp: "2026-07-24T08:00:01Z",
        },
      ],
    }));
    await page.route(`**/api/runner-studio/runs/${runIds[1]}/events/history`, (route) => route.fulfill({
      json: [
        { type: "run_start", run_id: runIds[1], status: "running", timestamp: "2026-07-24T08:01:00Z" },
        {
          type: "step_finish",
          run_id: runIds[1],
          node_id: "check-capacity",
          status: "success",
          timestamp: "2026-07-24T08:01:01Z",
        },
        { type: "run_finish", run_id: runIds[1], status: "success", timestamp: "2026-07-24T08:01:01Z" },
      ],
    }));

    const canvas = page.getByTestId("runner-studio-canvas");
    const node = page.getByTestId("canvas-node-check-capacity");
    await expect(canvas).toBeVisible();
    const initialNodeIds = await canvas.locator('[data-testid^="canvas-node-"]').evaluateAll((elements) =>
      elements.map((element) => element.getAttribute("data-testid")).filter(Boolean).sort(),
    );

    await page.getByTestId("workflow-workbench-trigger").click();
    const aiDraft = page.getByTestId("workflow-ai-input");
    await aiDraft.fill("保留这段未发送的故障排查要求");
    await page.getByTestId("workflow-context-drawer-close").click();

    await node.click();
    const nodePanel = page.getByTestId("runner-node-panel");
    await expect(nodePanel).toBeVisible();
    await expect(page.getByTestId("runner-node-panel-title")).toHaveText("检查磁盘容量");
    await expect(page.getByTestId("runner-node-target-labels-input")).toHaveValue("production");
    await expect(node).toHaveClass(/selected/);

    await page.getByTestId("runner-node-panel-run").click();
    await expect.poll(() => runRequests.length).toBe(1);

    await page.getByTestId("workflow-workbench-trigger").click();
    await page.getByTestId("workflow-context-tab-run").click();
    const inspector = page.getByTestId("workflow-run-inspector");
    await expect(inspector).toBeVisible();
    await expect(inspector.getByText("节点 check-capacity", { exact: true })).toBeVisible();
    await inspector.locator("ol button").first().click();
    const retry = page.getByTestId("workflow-run-retry");
    await expect(retry).toBeVisible();
    await retry.click();
    await expect.poll(() => runRequests.length).toBe(2);

    expect(runRequests[1]).toEqual(runRequests[0]);
    expect(runRequests[1]).toMatchObject({
      workflow_name: "capacity-inspection",
      node_id: "check-capacity",
      run_scope: "single_node",
      vars: {},
      triggered_by: "ui",
      risk_acknowledged: true,
    });
    expect(runRequests[1].graph).toEqual(runRequests[0].graph);

    await page.getByTestId("workflow-context-tab-ai").click();
    await expect(aiDraft).toHaveValue("保留这段未发送的故障排查要求");
    await page.getByTestId("workflow-context-drawer-close").click();
    await expect(canvas.locator('[data-testid^="canvas-node-"]')).toHaveCount(initialNodeIds.length);
    await expect.poll(async () => canvas.locator('[data-testid^="canvas-node-"]').evaluateAll((elements) =>
      elements.map((element) => element.getAttribute("data-testid")).filter(Boolean).sort(),
    )).toEqual(initialNodeIds);
    await expect(node).toHaveClass(/selected/);
    await expect(nodePanel).toBeVisible();
    await expect(page.getByTestId("runner-node-panel-title")).toHaveText("检查磁盘容量");
    await expect(page.getByTestId("runner-node-target-labels-input")).toHaveValue("production");
    await health.assertHealthy();
  });
});
