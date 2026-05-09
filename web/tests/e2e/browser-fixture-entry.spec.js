// @ts-check
import { test, expect } from "@playwright/test";
import { openBrowserFixturePage } from "../helpers/uiFixtureHarness";

test.describe("browser fixture entry", () => {
  test("loads chat fixture state and sessions without route mocks", async ({ page }) => {
    await openBrowserFixturePage(page, "/", "chat");

    await expect(page.locator(".chat-container")).toBeVisible({ timeout: 5000 });
    await expect(page.locator("body")).toContainText("nginx 中间件的状态");
    await expect(page.locator("body")).toContainText("Nginx chat");
  });

  test("loads protocol fixture state and sessions without route mocks", async ({ page }) => {
    await openBrowserFixturePage(page, "/protocol?promptDebug=1", "protocol");

    const pageRoot = page.getByTestId("protocol-workspace-page");
    await expect(pageRoot).toBeVisible({ timeout: 5000 });
    await expect(pageRoot).toContainText("nginx 巡检计划");
    await expect(pageRoot).toContainText("等待审批");

    await page.getByTestId("protocol-prompt-debug-button").click();
    await expect(page.getByTestId("protocol-prompt-debug-drawer")).toContainText("Tool Visibility");
  });
});
