// @ts-check
import { test, expect } from "@playwright/test";

const CONFIGS_RESPONSE = {
  items: [
    { id: "sc-1", scriptName: "restart-nginx", description: "重启 Nginx 服务", status: "active", approvalPolicy: "required", environmentRef: "prod", runnerProfile: "default" },
    { id: "sc-2", scriptName: "restart-nginx", description: "重启 Nginx (staging)", status: "draft", approvalPolicy: "none", environmentRef: "staging", runnerProfile: "default" },
    { id: "sc-3", scriptName: "backup-db", description: "数据库备份", status: "active", approvalPolicy: "auto", environmentRef: "prod", runnerProfile: "db-runner" },
  ],
  stats: { total: 3, active: 2, draft: 1, disabled: 0 },
};

const DRYRUN_RESPONSE = {
  configId: "sc-1",
  scriptName: "restart-nginx",
  mergedParams: { service: "nginx" },
  commandPreview: "restart-nginx --service=nginx",
  approvalPolicy: "required",
  dryRun: true,
};

test.describe("ScriptConfigPage", () => {
  test.beforeEach(async ({ page }) => {
    await page.route("**/api/v1/script-configs", (route) => {
      if (route.request().method() === "GET") {
        return route.fulfill({ json: CONFIGS_RESPONSE });
      }
      return route.fulfill({ status: 201, json: { id: "sc-new", scriptName: "new-script", status: "draft" } });
    });
    await page.route("**/api/v1/script-configs/*/dry-run", (route) =>
      route.fulfill({ json: DRYRUN_RESPONSE })
    );
    await page.route("**/api/v1/script-configs/*", (route) => {
      if (route.request().method() === "DELETE") {
        return route.fulfill({ json: { ok: "deleted" } });
      }
      if (route.request().method() === "PUT") {
        return route.fulfill({ json: { ok: true } });
      }
      return route.fulfill({ json: CONFIGS_RESPONSE.items[0] });
    });
    await page.route("**/api/v1/session*", (route) =>
      route.fulfill({ json: { sessionId: "test-session" } })
    );
    await page.goto("/script-configs");
  });

  test("page renders with title and stats", async ({ page }) => {
    await expect(page.getByRole("heading", { name: "脚本配置管理" })).toBeVisible();
  });

  test("shows script names in left panel", async ({ page }) => {
    await expect(page.locator(".script-list li")).toHaveCount(2); // restart-nginx, backup-db
  });

  test("clicking script name filters configs", async ({ page }) => {
    await page.locator(".script-list li", { hasText: "restart-nginx" }).click();
    const rows = page.locator(".data-table tbody tr");
    await expect(rows).toHaveCount(2); // sc-1 and sc-2
  });

  test("clicking detail button shows config details", async ({ page }) => {
    await page.locator("button", { hasText: "详情" }).first().click();
    await expect(page.locator(".detail-grid")).toBeVisible();
  });

  test("dry-run button triggers preview", async ({ page }) => {
    await page.locator("button", { hasText: "详情" }).first().click();
    const requestPromise = page.waitForRequest((req) =>
      req.url().includes("/dry-run") && req.method() === "POST"
    );
    await page.locator("button", { hasText: "Dry-Run" }).click();
    await requestPromise;
    await expect(page.locator(".preview-output")).toBeVisible();
  });

  test("new config button opens editor", async ({ page }) => {
    await page.locator("button", { hasText: "+ 新建配置" }).click();
    // Editor now uses n-form
    await expect(page.locator(".ops-form")).toBeVisible();
  });
});
