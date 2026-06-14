// @ts-check
import { test, expect } from "@playwright/test";
import { openBrowserFixturePage } from "../helpers/uiFixtureHarness";

test.describe("tool progressive discovery fixture", () => {
  test("shows search, select, unloaded recovery, selected delta, and final evidence", async ({ page }) => {
    await openBrowserFixturePage(page, "/", "tool-progressive-discovery");

    await expect(page.getByText("synthetic_complex_tool_discovery_request")).toBeVisible();
    await page.getByRole("button", { name: /已处理/ }).click();
    await expect(page.getByText("tool_search mode=search")).toBeVisible();
    await expect(page.getByText("tool_search mode=select")).toBeVisible();
    await expect(page.getByText("selected tool delta: +synthetic.metrics.read")).toBeVisible();
    await expect(page.getByText("tool_unloaded recoverable error")).toBeVisible();
    await expect(page.getByText("final evidence: synthetic.metrics.read checked")).toBeVisible();
    await expect(page.getByText("final evidence: synthetic.audit.read not_checked")).toBeVisible();
    await expect(page.getByText("低置信说明")).toBeVisible();
  });
});
