// @ts-check
import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";

import { openWorkflowStudioFixture } from "../helpers/workflowStudioPageFixture.js";

test.describe("Workflow Canvas accessibility contract", () => {
  test("[A11Y-004] Canvas, node sidebar, and workflow drawer have no critical or serious axe violations", async ({ page }) => {
    const health = await openWorkflowStudioFixture(page, { width: 1440, height: 900 });
    await expect(page.getByTestId("runner-studio-canvas")).toBeVisible();
    let results = await new AxeBuilder({ page }).include('[data-testid="runner-studio-page"]').analyze();
    expect(severe(results.violations)).toEqual([]);

    await page.getByTestId("canvas-node-check-capacity").click();
    await expect(page.getByTestId("workflow-node-sidebar")).toBeVisible();
    await expect(page.getByTestId("runner-node-panel")).toBeVisible();
    results = await new AxeBuilder({ page }).include('[data-testid="runner-studio-page"]').analyze();
    expect(severe(results.violations)).toEqual([]);

    await page.getByTestId("workflow-workbench-trigger").click();
    await expect(page.getByTestId("workflow-context-dock")).toBeVisible();
    await expect(page.getByTestId("workflow-node-sidebar")).toHaveAttribute("aria-hidden", "true");
    results = await new AxeBuilder({ page }).include('[data-testid="runner-studio-page"]').analyze();
    expect(severe(results.violations)).toEqual([]);
    await health.assertHealthy();
  });

  test("[A11Y-001] keyboard reaches the node sidebar separator and workflow-only drawer tabs", async ({ page }) => {
    await openWorkflowStudioFixture(page, { width: 1440, height: 900 });
    const node = page.getByTestId("canvas-node-check-capacity");
    await node.click();
    await expect(node).toBeFocused();
    await expect(page.getByTestId("workflow-node-sidebar")).toBeVisible();
    await expect(page.getByTestId("runner-node-panel")).toBeVisible();
    await expect(page.getByTestId("workflow-context-tab-configuration")).toHaveCount(0);

    const separator = page.getByTestId("workflow-node-sidebar-separator");
    await separator.focus();
    const before = Number(await separator.getAttribute("aria-valuenow"));
    await page.keyboard.press("ArrowLeft");
    await expect(separator).not.toHaveAttribute("aria-valuenow", String(before));

    const workbenchTrigger = page.getByTestId("workflow-workbench-trigger");
    await workbenchTrigger.focus();
    await page.keyboard.press("Enter");
    await expect(page.getByTestId("workflow-context-dock")).toBeVisible();
    await expect(page.getByTestId("workflow-node-sidebar")).toHaveAttribute("aria-hidden", "true");
    await page.getByTestId("workflow-context-tab-ai").focus();
    await page.keyboard.press("ArrowRight");
    await expect(page.getByTestId("workflow-context-tab-changes")).toBeFocused();
    await expect(page.getByTestId("workflow-context-tab-changes")).toHaveAttribute("aria-selected", "true");

    await page.keyboard.press("Escape");
    await expect(page.getByTestId("workflow-context-dock")).toBeHidden();
    await expect(workbenchTrigger).toBeFocused();
    await expect(page.getByTestId("workflow-node-sidebar")).toBeVisible();
  });

  test("[A11Y-005] node sidebar separator ignores hover and resizes only while the primary button is held", async ({ page }) => {
    const health = await openWorkflowStudioFixture(page, { width: 1274, height: 964 });
    await page.getByTestId("canvas-node-check-capacity").click();
    const separator = page.getByTestId("workflow-node-sidebar-separator");
    await expect(separator).toBeVisible();
    const initial = Number(await separator.getAttribute("aria-valuenow"));
    const initialBox = await separator.boundingBox();
    expect(initialBox).not.toBeNull();

    await page.mouse.move((initialBox?.x || 0) + 4, (initialBox?.y || 0) + 120);
    await page.mouse.move((initialBox?.x || 0) + 2, (initialBox?.y || 0) + 260, { steps: 4 });
    await expect(separator).toHaveAttribute("aria-valuenow", String(initial));

    await page.mouse.move((initialBox?.x || 0) + 4, (initialBox?.y || 0) + 180);
    await page.mouse.down();
    await page.mouse.move((initialBox?.x || 0) - 64, (initialBox?.y || 0) + 180, { steps: 5 });
    await page.mouse.up();
    const resized = Number(await separator.getAttribute("aria-valuenow"));
    expect(resized).toBeGreaterThan(initial);

    const resizedBox = await separator.boundingBox();
    await page.mouse.move((resizedBox?.x || 0) + 4, (resizedBox?.y || 0) + 220);
    await page.mouse.move((resizedBox?.x || 0) + 2, (resizedBox?.y || 0) + 320, { steps: 4 });
    await expect(separator).toHaveAttribute("aria-valuenow", String(resized));
    await health.assertHealthy();
  });

  test("[A11Y-002] More menu supports keyboard navigation, excludes lifecycle duplicates, and restores focus", async ({ page }) => {
    const health = await openWorkflowStudioFixture(page, { width: 1440, height: 900 });
    const trigger = page.getByTestId("workflow-action-menu-trigger");
    await trigger.focus();
    await page.keyboard.press("Enter");

    const menu = page.getByTestId("workflow-action-menu");
    await expect(menu).toBeVisible();
    await expect(menu).toContainText("文件");
    await expect(menu).not.toContainText(/检查工作流|试运行草稿|发布工作流|运行已发布版本/);
    const menuItems = menu.getByRole("menuitem");
    await expect(menuItems.first()).toBeFocused();
    const menuItemLayout = await menuItems.evaluateAll((items) => items.map((item) => {
      const rect = item.getBoundingClientRect();
      return { width: rect.width, top: rect.top };
    }));
    expect(menuItemLayout.every((item) => item.width >= 180)).toBe(true);
    expect(menuItemLayout.slice(1).every((item, index) => item.top > menuItemLayout[index].top)).toBe(true);
    await page.keyboard.press("ArrowDown");
    await expect(menuItems.nth(1)).toBeFocused();

    await page.keyboard.press("Escape");
    await expect(menu).toBeHidden();
    await expect(trigger).toBeFocused();
    await health.assertHealthy();
  });

  test("[A11Y-003] workflow status uses text/icon semantics and honors reduced motion", async ({ page }) => {
    await page.emulateMedia({ reducedMotion: "reduce" });
    await openWorkflowStudioFixture(page, { width: 1024, height: 768 });
    await page.getByTestId("workflow-workbench-trigger").click();
    await page.getByTestId("workflow-context-tab-run").click();
    const statusNodes = page.locator('[role="status"]:visible, [role="alert"]:visible');
    await expect(statusNodes.first()).toBeVisible();
    const animated = await page.evaluate(() => [...document.querySelectorAll("*")].filter((element) => {
      const style = getComputedStyle(element);
      return style.animationDuration !== "0s" && style.animationIterationCount === "infinite";
    }).map((element) => ({
      tag: element.tagName,
      className: element.getAttribute("class") || "",
      testId: element.getAttribute("data-testid") || "",
      animationName: getComputedStyle(element).animationName,
    })));
    expect(animated).toEqual([]);
  });

  test("[A11Y-006] 200% reflow-equivalent viewport keeps the current action and AI composer operable", async ({ page }) => {
    const health = await openWorkflowStudioFixture(page, { width: 720, height: 450 });
    const rootMetrics = await page.evaluate(() => ({
      viewportWidth: window.innerWidth,
      documentWidth: document.documentElement.scrollWidth,
      bodyWidth: document.body.scrollWidth,
    }));
    expect(rootMetrics.viewportWidth).toBe(720);
    expect(rootMetrics.documentWidth).toBeLessThanOrEqual(rootMetrics.viewportWidth + 1);
    expect(rootMetrics.bodyWidth).toBeLessThanOrEqual(rootMetrics.viewportWidth + 1);

    await expect(page.getByTestId("workflow-primary-action")).toBeVisible();
    await expect(page.getByRole("tablist", { name: "移动端工作区" })).toBeVisible();
    await page.getByTestId("workflow-mobile-mode-ai").click();
    await expect(page.getByTestId("workflow-ai-input")).toBeVisible();
    await expect(page.getByTestId("workflow-ai-send")).toBeVisible();
    await page.getByTestId("workflow-ai-input").fill("200% 缩放下保留这段输入");
    await page.getByTestId("workflow-mobile-mode-canvas").click();
    await page.getByTestId("workflow-mobile-mode-ai").click();
    await expect(page.getByTestId("workflow-ai-input")).toHaveValue("200% 缩放下保留这段输入");
    await health.assertHealthy();
  });
});

function severe(violations) {
  return violations
    .filter((violation) => violation.impact === "critical" || violation.impact === "serious")
    .map((violation) => ({ id: violation.id, impact: violation.impact, targets: violation.nodes.map((node) => node.target) }));
}
