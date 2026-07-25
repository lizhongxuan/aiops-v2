// @ts-check
import { expect, test } from "@playwright/test";
import { installWorkflowStudioFixture } from "./helpers/workflowStudioPageFixture.js";

test("scheme C command bar, node sidebar, and workflow overlay visual baseline", async ({ page }) => {
  const consoleErrors = [];
  page.on("console", (message) => {
    if (message.type() === "error") consoleErrors.push(message.text());
  });
  await page.setViewportSize({ width: 1440, height: 900 });
  await installWorkflowStudioFixture(page);
  await page.goto("/runner/capacity-inspection", { waitUntil: "networkidle" });

  const commandBar = page.getByTestId("workflow-command-bar");
  await expect(commandBar).toBeVisible();
  await expect(page.getByTestId("app-shell-header").getByTestId("workflow-command-bar")).toHaveCount(1);
  await expect(page.getByTestId("runner-studio-shell").getByTestId("workflow-command-bar")).toHaveCount(0);
  await expect(commandBar.getByTestId("workflow-save-status")).toContainText("已自动保存");
  await expect(commandBar.getByTestId("workflow-lifecycle-trace")).toContainText("检查");
  await expect(commandBar.getByTestId("workflow-primary-action")).toContainText("发布工作流");
  await expect(page.getByTestId("runner-toolbar-save")).toHaveCount(0);
  await expect(page.getByTestId("runner-toolbar-run")).toHaveCount(0);
  await expect(page.getByTestId("runner-toolbar-run-details")).toHaveCount(0);
  await expect(page.getByTestId("runner-toolbar-ai-generate")).toHaveCount(0);
  await expect(page.getByTestId("workflow-primary-action")).toHaveCount(1);
  await expect(page.getByTestId("workflow-workbench-trigger")).toBeVisible();
  await expect(page.getByTestId("runner-studio-canvas")).toBeVisible();
  await expect(page.getByTestId("workflow-context-dock")).toBeHidden();
  await expect(page.getByTestId("workflow-context-drawer-open")).toHaveCount(0);
  await expect(page.getByTestId("workflow-context-tab-configuration")).toHaveCount(0);
  await page.getByTestId("canvas-node-check-capacity").click();
  await expect(page.getByTestId("workflow-node-sidebar")).toBeVisible();
  await expect(page.getByTestId("runner-node-panel")).toBeVisible();
  await page.getByTestId("runner-node-panel-tab-script").click();
  await expect(page.getByTestId("runner-node-script-editor")).toHaveValue("print({'free_percent': 42})");
  await expect(page.getByTestId("runner-node-panel-delete")).toBeVisible();
  const canvasWithNodeSidebar = await page.getByTestId("workflow-context-canvas-pane").boundingBox();
  await page.evaluate(() => document.fonts.ready);

  await expect(page).toHaveScreenshot("workflow-scheme-c-node-sidebar.png", {
    animations: "disabled",
    caret: "hide",
    fullPage: true,
  });

  await expect(page.getByTestId("workflow-context-outline")).toHaveCount(0);
  await expect(page.getByTestId("workflow-outline-collapse")).toHaveCount(0);
  await expect(page.getByTestId("workflow-outline-expand")).toHaveCount(0);
  expect(canvasWithNodeSidebar?.width || 0).toBeGreaterThanOrEqual(600);

  await page.getByTestId("workflow-workbench-trigger").click();
  await expect(page.getByTestId("workflow-context-dock")).toBeVisible();
  await expect(page.getByTestId("workflow-node-sidebar")).toHaveAttribute("aria-hidden", "true");
  await expect(page.getByRole("tablist", { name: "工作流面板" })).toBeVisible();
  await expect(page.getByTestId("workflow-context-tab-changes")).toBeVisible();
  await expect(page.getByTestId("workflow-context-tab-run")).toBeVisible();
  await expect(page.getByTestId("workflow-context-tab-history")).toHaveCount(0);
  await expect(page.getByTestId("workflow-context-tab-review")).toHaveCount(0);
  expectSameCanvasGeometry(await page.getByTestId("workflow-context-canvas-pane").boundingBox(), canvasWithNodeSidebar);
  await expect.poll(
    () => overlayClearance(page, "canvas-node-check-capacity"),
    { timeout: 300, message: "选中节点应在 300ms 内让开工作流浮层" },
  ).toBeGreaterThanOrEqual(23);

  await expect(page).toHaveScreenshot("workflow-scheme-c-overlay.png", {
    animations: "disabled",
    caret: "hide",
    fullPage: true,
  });

  await page.getByTestId("workflow-context-drawer-close").click();
  await expect(page.getByTestId("workflow-context-dock")).toBeHidden();
  await expect(page.getByTestId("workflow-node-sidebar")).toBeVisible();
  expectSameCanvasGeometry(await page.getByTestId("workflow-context-canvas-pane").boundingBox(), canvasWithNodeSidebar);
  expect(consoleErrors).toEqual([]);
});

async function overlayClearance(page, nodeTestId) {
  return page.evaluate((testId) => {
    const node = document.querySelector(`[data-testid="${testId}"]`)?.getBoundingClientRect();
    const drawer = document.querySelector('[data-testid="workflow-context-dock"]:not([hidden])')?.getBoundingClientRect();
    return node && drawer ? Math.floor(drawer.left - node.right) : -1;
  }, nodeTestId);
}

function expectSameCanvasGeometry(actual, expected) {
  expect(actual).not.toBeNull();
  expect(expected).not.toBeNull();
  expect(Math.abs((actual?.x || 0) - (expected?.x || 0))).toBeLessThanOrEqual(1);
  expect(Math.abs((actual?.width || 0) - (expected?.width || 0))).toBeLessThanOrEqual(1);
}
