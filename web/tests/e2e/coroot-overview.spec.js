// @ts-check
import { test, expect } from "@playwright/test";
import { expectCorootWorkspace, installAiopsFixture, mockConfiguredCoroot } from "./coroot-helpers";

test.describe("Coroot main entry", () => {
  test("opens the embedded workspace instead of the legacy Coroot overview", async ({ page }) => {
    await installAiopsFixture(page);
    await mockConfiguredCoroot(page);

    await page.goto("/coroot");

    await expect(page).toHaveURL(/\/coroot\/p\/5hxbfx6p\/applications$/);
    await expectCorootWorkspace(page);
    await expect(page.locator("body")).not.toContainText("MCP 状态");
    await expect(page.locator("body")).not.toContainText("最近 Evidence");
    await expect(page.locator("body")).not.toContainText("最近发送到 AI Chat 的图表");
    await expect(page.locator("body")).not.toContainText("Coroot 观测");
  });
});
