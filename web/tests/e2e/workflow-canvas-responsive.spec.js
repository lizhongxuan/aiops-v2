// @ts-check
import { expect, test } from "@playwright/test";

import { openWorkflowStudioFixture } from "../helpers/workflowStudioPageFixture.js";

const viewports = [
  { width: 1440, height: 900 },
  { width: 1280, height: 720 },
  { width: 1274, height: 964 },
  { width: 1095, height: 863 },
  { width: 1024, height: 768 },
  { width: 820, height: 863 },
  { width: 768, height: 1024 },
  { width: 390, height: 844 },
  { width: 320, height: 720 },
];

const breakpointViewports = [
  { width: 1023, height: 768 },
  { width: 821, height: 863 },
];

test.describe("Workflow Canvas responsive contract", () => {
  for (const viewport of viewports) {
    test(`[RWD-001] ${viewport.width}x${viewport.height} avoids overflow and preserves Canvas under the workflow overlay`, async ({ page }) => {
      const health = await openWorkflowStudioFixture(page, viewport);
      await expect(page.getByTestId("runner-studio-page")).toBeVisible();
      await expect(page.getByTestId("runner-studio-canvas")).toBeVisible();
      await expect(page.getByTestId("workflow-context-tab-configuration")).toHaveCount(0);

      const geometry = await workspaceGeometry(page);
      expect(geometry.scrollWidth).toBeLessThanOrEqual(geometry.viewport + 1);
      expect(geometry.bodyScrollWidth).toBeLessThanOrEqual(geometry.viewport + 1);
      expect(geometry.canvas?.width || 0).toBeGreaterThan(0);
      if (viewport.width > 820) {
        await expect(page.getByTestId("workflow-context-dock")).toBeHidden();
        await expect(page.getByTestId("workflow-context-drawer-open")).toHaveCount(0);

        await page.getByTestId("workflow-workbench-trigger").click();
        const openGeometry = await workspaceGeometry(page);
        await expect(page.getByTestId("workflow-context-dock")).toBeVisible();
        expectSameCanvasGeometry(openGeometry.canvas, geometry.canvas);
        expect(openGeometry.dock?.width || 0).toBeGreaterThanOrEqual(460);
        expect(openGeometry.dock?.height || 0).toBeGreaterThanOrEqual((openGeometry.canvas?.height || 0) - 17);
        expect(Math.abs((openGeometry.dock?.right || 0) - ((openGeometry.canvas?.right || 0) - 8))).toBeLessThanOrEqual(1);
        expect((openGeometry.dock?.left || 0) < (openGeometry.canvas?.right || 0)).toBe(true);

        await page.getByTestId("workflow-context-drawer-close").click();
        await expect(page.getByTestId("workflow-context-dock")).toBeHidden();
        const closedGeometry = await workspaceGeometry(page);
        expectSameCanvasGeometry(closedGeometry.canvas, geometry.canvas);
      }
      await health.assertHealthy();
    });
  }

  for (const viewport of breakpointViewports) {
    test(`[RWD-005] ${viewport.width}px breakpoint preserves one-line command bar and overlay geometry`, async ({ page }) => {
      const health = await openWorkflowStudioFixture(page, viewport);
      const before = await workspaceGeometry(page);
      await expect(page.getByTestId("workflow-command-bar")).toBeVisible();
      await expect(page.getByTestId("workflow-context-dock")).toBeHidden();
      await page.getByTestId("workflow-workbench-trigger").click();
      await expect(page.getByTestId("workflow-context-dock")).toBeVisible();
      expectSameCanvasGeometry((await workspaceGeometry(page)).canvas, before.canvas);
      await health.assertHealthy();
    });
  }

  test("[RWD-002] desktop removes the outline and keeps the top bar on one row", async ({ page }) => {
    const health = await openWorkflowStudioFixture(page, { width: 1024, height: 768 });
    const canvas = page.getByTestId("workflow-context-canvas-pane");
    await expect(page.getByTestId("workflow-context-outline")).toHaveCount(0);
    await expect(page.getByTestId("workflow-outline-collapse")).toHaveCount(0);
    const canvasBox = await canvas.boundingBox();
    expect(canvasBox?.width || 0).toBeGreaterThan(0);
    const topbarCenterSpread = await page.getByTestId("workflow-command-bar").evaluate((element) => {
      const children = [...element.children].filter((child) => child.getBoundingClientRect().width > 0);
      const centers = children.map((child) => {
        const bounds = child.getBoundingClientRect();
        return bounds.top + bounds.height / 2;
      });
      return centers.length > 0 ? Math.max(...centers) - Math.min(...centers) : 0;
    });
    expect(topbarCenterSpread).toBeLessThanOrEqual(1);
    await health.assertHealthy();
  });

  test("[RWD-003] mobile modes retain AI input and return focus from full-screen configuration", async ({ page }) => {
    const health = await openWorkflowStudioFixture(page, { width: 390, height: 844 });
    const modes = page.getByRole("tablist", { name: "移动端工作区" });
    await expect(modes).toBeVisible();
    await expect(page.getByTestId("workflow-mobile-mode-canvas")).toHaveAttribute("aria-selected", "true");
    await expect(page.getByTestId("workflow-context-tab-configuration")).toHaveCount(0);
    await expect(page.getByTestId("workflow-node-sidebar")).toHaveCount(0);

    await page.getByTestId("workflow-mobile-mode-ai").click();
    const input = page.getByTestId("workflow-ai-input");
    await expect(input).toBeVisible();
    await input.fill("保留这段未发送的修改要求");
    await page.getByTestId("workflow-mobile-mode-canvas").click();
    const node = page.getByTestId("canvas-node-check-capacity");
    const sheet = page.getByTestId("workflow-mobile-configuration-sheet");
    await node.click();
    if (!(await sheet.isVisible())) await node.dblclick();
    await expect(sheet).toBeVisible();
    await expect(sheet).toHaveAttribute("aria-modal", "true");
    await sheet.getByRole("button", { name: "关闭配置" }).click();
    await expect(node).toBeFocused();

    await page.getByTestId("workflow-mobile-mode-ai").click();
    await expect(input).toHaveValue("保留这段未发送的修改要求");
    await expect(page.getByTestId("workflow-mobile-mode-changes")).toBeVisible();
    await expect(page.getByTestId("workflow-mobile-mode-run")).toBeVisible();
    await health.assertHealthy();
  });

  test("[RWD-004] mobile composer respects safe area and keeps input/send controls visible", async ({ page }) => {
    await openWorkflowStudioFixture(page, { width: 390, height: 844 });
    await page.getByTestId("workflow-mobile-mode-ai").click();
    const panel = page.getByTestId("workflow-ai-transport-panel");
    await expect(panel.getByTestId("workflow-ai-input")).toBeVisible();
    await expect(panel.getByTestId("workflow-ai-send")).toBeVisible();
    const bounds = await panel.locator("form").boundingBox();
    expect(bounds).not.toBeNull();
    expect((bounds?.y || 0) + (bounds?.height || 0)).toBeLessThanOrEqual(844);
  });

  test("[RWD-006] mobile keeps the single Primary above bottom navigation and opens More as a Bottom Sheet", async ({ page }) => {
    const health = await openWorkflowStudioFixture(page, { width: 390, height: 844 });
    const primary = page.getByTestId("workflow-primary-action");
    const navigation = page.getByRole("tablist", { name: "移动端工作区" });
    await expect(primary).toHaveCount(1);
    await expect(primary).toBeVisible();
    const primaryBounds = await primary.boundingBox();
    const navigationBounds = await navigation.boundingBox();
    expect(primaryBounds?.height || 0).toBeGreaterThanOrEqual(44);
    expect((primaryBounds?.y || 0) + (primaryBounds?.height || 0)).toBeLessThanOrEqual((navigationBounds?.y || 0) + 1);

    const more = page.getByTestId("workflow-action-menu-trigger");
    await more.click();
    const sheet = page.getByTestId("workflow-action-menu");
    await expect(sheet).toBeVisible();
    await expect(sheet).toHaveAttribute("data-side", "bottom");
    await expect.poll(async () => {
      const bounds = await sheet.boundingBox();
      return (bounds?.y || 0) + (bounds?.height || 0);
    }, { timeout: 500 }).toBeLessThanOrEqual(845);
    await page.keyboard.press("Escape");
    await expect(sheet).toBeHidden();
    await expect(more).toBeFocused();
    await health.assertHealthy();
  });
});

async function workspaceGeometry(page) {
  return page.evaluate(() => {
    const root = document.documentElement;
    const canvas = document.querySelector('[data-testid="workflow-context-canvas-pane"]')?.getBoundingClientRect();
    const dock = document.querySelector('[data-testid="workflow-context-dock"]')?.getBoundingClientRect();
    return {
      viewport: window.innerWidth,
      scrollWidth: root.scrollWidth,
      bodyScrollWidth: document.body.scrollWidth,
      canvas: canvas ? {
        top: canvas.top,
        right: canvas.right,
        bottom: canvas.bottom,
        left: canvas.left,
        width: canvas.width,
        height: canvas.height,
      } : null,
      dock: dock ? {
        top: dock.top,
        right: dock.right,
        bottom: dock.bottom,
        left: dock.left,
        width: dock.width,
        height: dock.height,
      } : null,
    };
  });
}

function expectSameCanvasGeometry(actual, expected) {
  expect(actual).not.toBeNull();
  expect(expected).not.toBeNull();
  expect(Math.abs((actual?.left || 0) - (expected?.left || 0))).toBeLessThanOrEqual(1);
  expect(Math.abs((actual?.right || 0) - (expected?.right || 0))).toBeLessThanOrEqual(1);
  expect(Math.abs((actual?.width || 0) - (expected?.width || 0))).toBeLessThanOrEqual(1);
}
