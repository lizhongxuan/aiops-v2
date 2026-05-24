// @ts-check
import { test, expect } from "@playwright/test";

const ENVS_RESPONSE = {
  items: [
    {
      id: "lab-1",
      name: "Web + DB 双层架构",
      scenario: "web-db-2tier",
      status: "running",
      topology: {
        nodes: [
          { id: "web1", name: "web-server-1", role: "web" },
          { id: "db1", name: "db-server-1", role: "db" },
        ],
        links: [{ from: "web1", to: "db1", protocol: "tcp", port: 3306 }],
      },
      mockHostIds: ["lab-lab-1-web1", "lab-lab-1-db1"],
      updatedAt: "2025-01-15T10:00:00Z",
    },
    {
      id: "lab-2",
      name: "缓存层测试",
      scenario: "cache-layer",
      status: "stopped",
      topology: { nodes: [], links: [] },
      mockHostIds: [],
      updatedAt: "2025-01-14T08:00:00Z",
    },
  ],
  stats: { total: 2, running: 1, stopped: 1, draft: 0 },
};

test.describe("LabEnvironmentPage", () => {
  test.beforeEach(async ({ page }) => {
    await page.route("**/api/v1/lab-environments", (route) => {
      if (route.request().method() === "GET") {
        return route.fulfill({ json: ENVS_RESPONSE });
      }
      return route.fulfill({ status: 201, json: { id: "lab-new", name: "New Lab", status: "draft" } });
    });
    await page.route("**/api/v1/lab-environments/*/start", (route) =>
      route.fulfill({ json: { ...ENVS_RESPONSE.items[1], status: "running" } })
    );
    await page.route("**/api/v1/lab-environments/*/stop", (route) =>
      route.fulfill({ json: { ...ENVS_RESPONSE.items[0], status: "stopped" } })
    );
    await page.route("**/api/v1/lab-environments/*/reset", (route) =>
      route.fulfill({ json: ENVS_RESPONSE.items[0] })
    );
    await page.route("**/api/v1/lab-environments/*", (route) => {
      if (route.request().method() === "DELETE") {
        return route.fulfill({ json: { ok: "deleted" } });
      }
      return route.fulfill({ json: ENVS_RESPONSE.items[0] });
    });
    await page.route("**/api/v1/session*", (route) =>
      route.fulfill({ json: { sessionId: "test-session" } })
    );
    await page.goto("/lab");
  });

  test("page renders with title", async ({ page }) => {
    await expect(page.locator("main").getByText("实验环境管理", { exact: true })).toBeVisible();
  });

  test("shows running/stopped stats", async ({ page }) => {
    await expect(page.locator(".stat-ok")).toContainText("1");
    await expect(page.locator(".stat-warn")).toContainText("1");
  });

  test("environment list shows entries", async ({ page }) => {
    // n-data-table renders with ops- prefix
    const table = page.locator(".ops-data-table-table");
    await expect(table).toBeVisible();
    await expect(table.getByText("Web + DB 双层架构")).toBeVisible();
    await expect(table.getByText("缓存层测试")).toBeVisible();
  });

  test("running env shows stop and reset buttons", async ({ page }) => {
    const table = page.locator(".ops-data-table-table");
    const row = table.locator("tr", { hasText: "Web + DB 双层架构" });
    await expect(row.getByRole("button", { name: "停止" })).toBeVisible();
    await expect(row.getByRole("button", { name: "重置" })).toBeVisible();
  });

  test("stopped env shows start button", async ({ page }) => {
    const table = page.locator(".ops-data-table-table");
    const row = table.locator("tr", { hasText: "缓存层测试" });
    await expect(row.getByRole("button", { name: "启动" })).toBeVisible();
  });

  test("start button sends POST request", async ({ page }) => {
    const requestPromise = page.waitForRequest((req) =>
      req.url().includes("/lab-environments/lab-2/start") && req.method() === "POST"
    );
    const table = page.locator(".ops-data-table-table");
    const row = table.locator("tr", { hasText: "缓存层测试" });
    await row.getByRole("button", { name: "启动" }).click();
    await requestPromise;
  });

  test("stop button sends POST request", async ({ page }) => {
    const requestPromise = page.waitForRequest((req) =>
      req.url().includes("/lab-environments/lab-1/stop") && req.method() === "POST"
    );
    const table = page.locator(".ops-data-table-table");
    const row = table.locator("tr", { hasText: "Web + DB 双层架构" });
    await row.getByRole("button", { name: "停止" }).click();
    await requestPromise;
  });

  test("templates tab shows scenario cards", async ({ page }) => {
    await page.locator(".tab-bar button", { hasText: "场景模板" }).click();
    // Templates are now rendered as n-card inside n-grid
    await expect(page.locator(".ops-card")).toHaveCount(3);
  });

  test("new environment dialog opens", async ({ page }) => {
    await page.locator(".create-btn").click();
    await expect(page.locator(".dialog-box")).toBeVisible();
    await expect(page.locator(".dialog-box h2")).toContainText("新建实验环境");
  });

  test("create environment sends POST", async ({ page }) => {
    await page.locator(".create-btn").click();
    await page.locator(".dialog-box input").first().fill("Test Lab");
    const requestPromise = page.waitForRequest((req) =>
      req.url().includes("/api/v1/lab-environments") && req.method() === "POST"
    );
    await page.locator(".dialog-box .action-start").click();
    const req = await requestPromise;
    const body = req.postDataJSON();
    expect(body.name).toBe("Test Lab");
  });
});
