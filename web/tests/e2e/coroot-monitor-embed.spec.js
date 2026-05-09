// @ts-check
import { test, expect } from "@playwright/test";

const COROOT_CONFIG_RESPONSE = {
  configured: true,
  baseUrl: "/api/v1/coroot/",
  iframeMode: true,
};

const SERVICES_RESPONSE = [
  { id: "svc-nginx", name: "nginx-frontend", status: "ok" },
  { id: "svc-api", name: "api-gateway", status: "warning" },
  { id: "svc-db", name: "mysql-primary", status: "critical" },
];

async function setupRoutes(page) {
  await page.route("**/api/v1/coroot/config", (route) =>
    route.fulfill({ json: COROOT_CONFIG_RESPONSE }),
  );
  await page.route("**/api/v1/coroot/api/v1/services", (route) =>
    route.fulfill({ json: SERVICES_RESPONSE }),
  );
  await page.route("**/api/v1/coroot/api/v1/services/*/overview", (route) =>
    route.fulfill({
      json: { id: "svc-nginx", name: "nginx-frontend", status: "ok", summary: {} },
    }),
  );
  await page.route("**/api/v1/coroot/api/v1/topology", (route) =>
    route.fulfill({ json: { nodes: [], edges: [] } }),
  );
  await page.route("**/api/v1/coroot/", (route) => {
    if (route.request().resourceType() === "document") {
      return route.fulfill({
        contentType: "text/html",
        body: "<html><body>Coroot Dashboard</body></html>",
      });
    }
    return route.fulfill({ body: "" });
  });
  await page.route("**/api/v1/session*", (route) =>
    route.fulfill({ json: { sessionId: "test-session" } }),
  );
  await page.route("**/api/v1/chat/message", (route) =>
    route.fulfill({ json: { ok: true } }),
  );
}

test.describe("Coroot Monitor Embed E2E", () => {
  test.beforeEach(async ({ page }) => {
    await setupRoutes(page);
  });

  test("sidebar contains Coroot nav item and navigates to /coroot", async ({ page }) => {
    await page.goto("/");
    // Naive UI n-menu renders items as .ops-menu-item
    const navItem = page.locator(".ops-menu-item", { hasText: "Coroot 监控" });
    await expect(navItem).toBeVisible();
    await navItem.click();
    await expect(page).toHaveURL(/\/coroot/);
    await expect(page.getByRole("heading", { name: "Coroot 监控总览" })).toBeVisible();
  });

  test("tab bar is visible with services, dashboard, and topology tabs", async ({ page }) => {
    await page.goto("/coroot");
    const tabBar = page.getByTestId("coroot-tab-bar");
    await expect(tabBar).toBeVisible();
    // Naive UI tabs render as .ops-tabs-tab
    await expect(page.locator(".ops-tabs-tab", { hasText: "服务总览" })).toBeVisible();
    await expect(page.locator(".ops-tabs-tab", { hasText: "Dashboard" })).toBeVisible();
    await expect(page.locator(".ops-tabs-tab", { hasText: "拓扑视图" })).toBeVisible();
  });

  test("clicking Dashboard tab shows iframe", async ({ page }) => {
    await page.goto("/coroot");
    await page.locator(".ops-tabs-tab", { hasText: "Dashboard" }).click();
    const dashboardContent = page.getByTestId("tab-content-dashboard");
    await expect(dashboardContent).toBeVisible();
    const iframe = page.getByTestId("dashboard-iframe");
    await expect(iframe).toBeVisible();
    await expect(iframe).toHaveAttribute("src", /\/api\/v1\/coroot\//);
  });

  test("services tab displays service data table", async ({ page }) => {
    await page.goto("/coroot");
    const servicesContent = page.getByTestId("tab-content-services");
    await expect(servicesContent).toBeVisible();
    const table = page.locator(".ops-data-table-table");
    await expect(table).toBeVisible();
    await expect(table.getByText("nginx-frontend")).toBeVisible();
    await expect(table.getByText("api-gateway")).toBeVisible();
    await expect(table.getByText("mysql-primary")).toBeVisible();
  });

  test("AI assistant button opens drawer with quick actions", async ({ page }) => {
    await page.goto("/coroot");
    await page.getByRole("button", { name: "AI 助手" }).click();
    const drawer = page.locator(".monitor-ai-drawer");
    await expect(drawer).toBeVisible();
    await expect(page.locator(".quick-actions .action-btn")).toHaveCount(4);
    await expect(page.locator(".action-btn", { hasText: "解释当前面板" })).toBeVisible();
    await expect(page.locator(".action-btn", { hasText: "定位异常原因" })).toBeVisible();
  });

  test("AI drawer quick action sends request with monitorContext", async ({ page }) => {
    await page.goto("/coroot");
    await page.getByRole("button", { name: "AI 助手" }).click();
    await page.waitForTimeout(500);
    const requestPromise = page.waitForRequest((req) =>
      req.url().includes("/api/v1/chat/message") && req.method() === "POST",
    );
    await page.locator(".monitor-ai-drawer .action-btn", { hasText: "解释当前面板" }).click();
    const req = await requestPromise;
    const body = req.postDataJSON();
    expect(body.monitorContext).toBeDefined();
    expect(body.monitorContext.source).toBe("coroot");
  });
});

test.describe("Coroot Monitor Embed – degraded state", () => {
  test("shows not-configured warning when Coroot is unconfigured", async ({ page }) => {
    await page.route("**/api/v1/coroot/config", (route) =>
      route.fulfill({ json: { configured: false } }),
    );
    await page.route("**/api/v1/coroot/api/v1/services", (route) =>
      route.fulfill({ json: [] }),
    );
    await page.route("**/api/v1/session*", (route) =>
      route.fulfill({ json: { sessionId: "test-session" } }),
    );
    await page.goto("/coroot");
    await expect(page.getByTestId("coroot-not-configured")).toBeVisible();
  });
});
