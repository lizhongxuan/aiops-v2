// @ts-check
import { test, expect } from "@playwright/test";

const AUDIT_RESPONSE = {
  items: [
    { id: "audit-1", createdAt: "2025-01-15T10:00:00Z", sessionKind: "chat", host: "host-a", operator: "admin", toolName: "exec_command", decision: "approved", command: "ls -la" },
    { id: "audit-2", createdAt: "2025-01-15T11:00:00Z", sessionKind: "workspace", host: "host-b", operator: "ops", toolName: "file_write", decision: "rejected", command: "rm -rf /tmp" },
  ],
  total: 2,
  stats: { todayTotal: 12, pending: 3, autoAccepted: 5, grantedCommands: 8 },
};

const GRANTS_RESPONSE = {
  items: [
    { id: "grant-1", hostId: "host-a", command: "systemctl restart nginx", status: "active" },
    { id: "grant-2", hostId: "host-a", command: "docker ps", status: "disabled" },
  ],
};

test.describe("ApprovalManagementPage", () => {
  test.beforeEach(async ({ page }) => {
    await page.route("**/api/v1/approval-audits*", (route) =>
      route.fulfill({ json: AUDIT_RESPONSE })
    );
    await page.route("**/api/v1/approval-grants*", (route) => {
      if (route.request().method() === "POST") {
        return route.fulfill({ json: { ok: true } });
      }
      return route.fulfill({ json: GRANTS_RESPONSE });
    });
    await page.route("**/api/v1/session*", (route) =>
      route.fulfill({ json: { sessionId: "test-session" } })
    );
    await page.goto("/approval-management");
  });

  test("displays stats row with correct values", async ({ page }) => {
    await expect(page.getByTestId("stats-row")).toBeVisible();
    await expect(page.locator(".ops-statistic").filter({ hasText: "今日审批" })).toContainText("12");
    await expect(page.locator(".ops-statistic").filter({ hasText: "待处理" })).toContainText("3");
    await expect(page.locator(".ops-statistic").filter({ hasText: "自动放行" })).toContainText("5");
    await expect(page.locator(".ops-statistic").filter({ hasText: "已授权命令" })).toContainText("8");
  });

  test("renders audit table with rows", async ({ page }) => {
    await expect(page.getByTestId("audit-table")).toBeVisible();
    await expect(page.getByText("exec_command")).toBeVisible();
    await expect(page.getByText("file_write")).toBeVisible();
  });

  test("filter bar is visible with fields", async ({ page }) => {
    await expect(page.getByTestId("filter-bar")).toBeVisible();
    await expect(page.getByTestId("apply-filters-btn")).toBeVisible();
    await expect(page.getByTestId("reset-filters-btn")).toBeVisible();
  });

  test("apply filters triggers new fetch", async ({ page }) => {
    await page.getByTestId("filter-host").locator("input").fill("host-a");
    const requestPromise = page.waitForRequest((req) =>
      req.url().includes("/api/v1/approval-audits") && req.url().includes("host=")
    );
    await page.getByTestId("apply-filters-btn").click();
    const req = await requestPromise;
    expect(req.url()).toContain("host=host-a");
  });

  test("reset filters clears inputs", async ({ page }) => {
    await page.getByTestId("filter-host").locator("input").fill("host-a");
    await page.getByTestId("apply-filters-btn").click();
    await page.getByTestId("reset-filters-btn").click();
    const val = await page.getByTestId("filter-host").locator("input").inputValue();
    expect(val).toBe("");
  });

  test("audit table is visible", async ({ page }) => {
    await expect(page.getByTestId("audit-table")).toBeVisible();
  });

  test("clicking audit row opens detail drawer", async ({ page }) => {
    await page.getByText("exec_command").click();
    await expect(page.getByTestId("detail-drawer")).toBeVisible();
  });

  test("grants tab loads grant data", async ({ page }) => {
    // Tab name is "授权列表"
    await page.locator(".ops-tabs-tab", { hasText: "授权列表" }).click();
    await expect(page.getByTestId("grants-table")).toBeVisible();
    await expect(page.getByText("systemctl restart nginx")).toBeVisible();
  });

  test("revoke button sends POST request", async ({ page }) => {
    await page.locator(".ops-tabs-tab", { hasText: "授权列表" }).click();
    await page.waitForTimeout(500);
    const requestPromise = page.waitForRequest((req) =>
      req.url().includes("/approval-grants/grant-1/revoke") && req.method() === "POST"
    );
    await page.getByRole("button", { name: "撤销" }).first().click();
    await requestPromise;
  });

  test("enable button visible for disabled grants", async ({ page }) => {
    await page.locator(".ops-tabs-tab", { hasText: "授权列表" }).click();
    await page.waitForTimeout(500);
    await expect(page.getByRole("button", { name: "启用" })).toBeVisible();
  });
});
