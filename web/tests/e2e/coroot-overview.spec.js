// @ts-check
import { test, expect } from "@playwright/test";

async function setupRoutes(page) {
  await page.route("**/api/v1/coroot/config", (route) =>
    route.fulfill({ json: { configured: true, baseUrl: "https://coroot.example", lastSuccessAt: "2026-05-12T09:30:00+08:00" } }),
  );
  await page.route("**/api/v1/mcp/servers", (route) =>
    route.fulfill({ json: { items: [{ name: "coroot-rca", status: "connected", toolCount: 5, resourceCount: 2 }] } }),
  );
  await page.route("**/api/v1/coroot/evidence", (route) =>
    route.fulfill({
      json: {
        items: [
          {
            evidence_ref: "ev-coroot-latency",
            title: "checkout p95 延迟",
            summary: "p95 高于基线",
            case_id: "incident-1",
          },
        ],
      },
    }),
  );
  await page.route("**/api/v1/agent-ui-artifacts?source=coroot", (route) =>
    route.fulfill({
      json: {
        items: [
          {
            id: "coroot-checkout-latency-chart",
            type: "coroot_chart",
            title: "checkout 延迟图",
            case_id: "incident-1",
          },
        ],
      },
    }),
  );
  await page.route("**/api/v1/coroot/test-connection", (route) => route.fulfill({ json: { ok: true } }));
  await page.route("**/api/v1/session*", (route) => route.fulfill({ json: { sessionId: "test-session" } }));
}

test.describe("CorootOverviewPage", () => {
  test.beforeEach(async ({ page }) => {
    await setupRoutes(page);
    await page.goto("/coroot");
  });

  test("page focuses on Coroot observability control surfaces", async ({ page }) => {
    await expect(page.locator("main").getByText("Coroot 观测", { exact: true })).toBeVisible();
    await expect(page.locator('[data-slot="card-title"]').filter({ hasText: "Coroot 配置" })).toBeVisible();
    await expect(page.locator('[data-slot="card-title"]').filter({ hasText: "MCP 状态" })).toBeVisible();
    await expect(page.locator('[data-slot="card-title"]').filter({ hasText: "RCA Skills" })).toBeVisible();
    await expect(page.locator('[data-slot="card-title"]').filter({ hasText: "最近 Evidence" })).toBeVisible();
    await expect(page.locator('[data-slot="card-title"]').filter({ hasText: "最近发送到 AI Chat 的图表" })).toBeVisible();
    await expect(page.getByText("Dashboard")).toHaveCount(0);
  });

  test("shows Coroot MCP, evidence and artifact links", async ({ page }) => {
    await expect(page.getByText("coroot-rca")).toBeVisible();
    await expect(page.getByText("Coroot RCA 已启用")).toBeVisible();
    await expect(page.getByText("ev-coroot-latency")).toBeVisible();
    await expect(page.getByText("coroot-checkout-latency-chart")).toBeVisible();
    await expect(page.getByRole("link", { name: "查看 Case" }).first()).toHaveAttribute("href", /\/incidents\/incident-1/);
    await expect(page.getByRole("link", { name: "查看 Prompt Trace" })).toHaveAttribute("href", /artifact_id=coroot-checkout-latency-chart/);
  });

  test("test connection button reports success", async ({ page }) => {
    await page.getByRole("button", { name: /测试连接/ }).click();
    await expect(page.getByText("连接正常")).toBeVisible();
  });
});
