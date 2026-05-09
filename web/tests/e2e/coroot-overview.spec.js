// @ts-check
import { test, expect } from "@playwright/test";

const SERVICES_RESPONSE = [
  { id: "svc-nginx", name: "nginx-frontend", status: "ok" },
  { id: "svc-api", name: "api-gateway", status: "warning" },
  { id: "svc-db", name: "mysql-primary", status: "critical" },
];

test.describe("CorootOverviewPage", () => {
  test.beforeEach(async ({ page }) => {
    await page.route("**/api/v1/coroot/api/v1/services", (route) =>
      route.fulfill({ json: SERVICES_RESPONSE })
    );
    await page.route("**/api/v1/coroot/api/v1/services/*/overview", (route) =>
      route.fulfill({ json: { id: "svc-nginx", name: "nginx-frontend", status: "ok", summary: {} } })
    );
    await page.route("**/api/v1/coroot/api/v1/topology", (route) =>
      route.fulfill({ json: { nodes: [], edges: [] } })
    );
    await page.route("**/api/v1/coroot/config", (route) =>
      route.fulfill({ json: { configured: true } })
    );
    await page.route("**/api/v1/session*", (route) =>
      route.fulfill({ json: { sessionId: "test-session" } })
    );
    await page.route("**/api/v1/chat/message", (route) =>
      route.fulfill({ json: { ok: true } })
    );
    await page.goto("/coroot");
  });

  test("page renders with title", async ({ page }) => {
    await expect(page.getByRole("heading", { name: "Coroot 监控总览" })).toBeVisible();
  });

  test("shows health stats via n-statistic", async ({ page }) => {
    await expect(page.locator(".ops-statistic").filter({ hasText: "健康" })).toContainText("1");
    await expect(page.locator(".ops-statistic").filter({ hasText: "告警" })).toContainText("1");
    await expect(page.locator(".ops-statistic").filter({ hasText: "异常" })).toContainText("1");
  });

  test("services tab shows service table", async ({ page }) => {
    const table = page.locator(".ops-data-table-table");
    await expect(table).toBeVisible();
    await expect(table.getByText("nginx-frontend")).toBeVisible();
    await expect(table.getByText("api-gateway")).toBeVisible();
    await expect(table.getByText("mysql-primary")).toBeVisible();
  });

  test("status values render in table", async ({ page }) => {
    const table = page.locator(".ops-data-table-table");
    await expect(table).toBeVisible();
    await expect(table.locator("td", { hasText: "ok" }).first()).toBeVisible();
    await expect(table.locator("td", { hasText: "warning" }).first()).toBeVisible();
    await expect(table.locator("td", { hasText: "critical" }).first()).toBeVisible();
  });

  test("clicking detail opens embed panel", async ({ page }) => {
    const table = page.locator(".ops-data-table-table");
    await table.getByRole("button", { name: "详情" }).first().click();
    await expect(page.locator(".embed-panel")).toBeVisible();
  });

  test("topology tab is accessible", async ({ page }) => {
    await page.locator(".ops-tabs-tab", { hasText: "拓扑视图" }).click();
    await expect(page.getByText("服务拓扑")).toBeVisible();
  });

  test("AI assistant button opens drawer", async ({ page }) => {
    await page.getByRole("button", { name: "AI 助手" }).click();
    await expect(page.locator(".monitor-ai-drawer")).toBeVisible();
  });

  test("AI drawer has 4 quick action buttons", async ({ page }) => {
    await page.getByRole("button", { name: "AI 助手" }).click();
    await expect(page.locator(".quick-actions .action-btn")).toHaveCount(4);
  });

  test("AI drawer quick action sends message", async ({ page }) => {
    await page.getByRole("button", { name: "AI 助手" }).click();
    await page.waitForTimeout(500);
    // Set up request promise before clicking
    const requestPromise = page.waitForRequest((req) =>
      req.url().includes("/api/v1/chat/message") && req.method() === "POST"
    );
    // The MonitorAIDrawer is inside an n-drawer; use force click to bypass overlay
    await page.locator(".monitor-ai-drawer .action-btn", { hasText: "解释当前面板" }).click({ force: true });
    const req = await requestPromise;
    const body = req.postDataJSON();
    expect(body.monitorContext).toBeDefined();
    expect(body.monitorContext.source).toBe("coroot");
  });
});
