// @ts-check
import { test, expect } from "@playwright/test";

test.describe("Coroot 观测降级状态", () => {
  test("shows not-configured warning when Coroot is unconfigured", async ({ page }) => {
    await page.route("**/api/v1/coroot/config", (route) =>
      route.fulfill({ json: { configured: false } }),
    );
    await page.route("**/api/v1/mcp/servers", (route) => route.fulfill({ json: { items: [] } }));
    await page.route("**/api/v1/coroot/evidence", (route) => route.fulfill({ json: { items: [] } }));
    await page.route("**/api/v1/agent-ui-artifacts?source=coroot", (route) => route.fulfill({ json: { items: [] } }));
    await page.route("**/api/v1/session*", (route) => route.fulfill({ json: { sessionId: "test-session" } }));

    await page.goto("/coroot");

    await expect(page.getByTestId("coroot-not-configured")).toBeVisible();
    await expect(page.getByText("Coroot RCA 未启用")).toBeVisible();
  });
});
