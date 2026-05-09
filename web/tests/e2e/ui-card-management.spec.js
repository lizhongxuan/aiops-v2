// @ts-check
import { test, expect } from "@playwright/test";

const CARDS_RESPONSE = {
  items: [
    { id: "mcp-summary", name: "摘要卡片", kind: "readonly_summary", renderer: "McpSummaryCard", status: "active", builtIn: true, version: 1, capabilities: ["kv_rows"], triggerTypes: ["mcp_tool_result"], summary: "展示结构化摘要信息" },
    { id: "mcp-control-panel", name: "控制面板卡片", kind: "action_panel", renderer: "McpControlPanelCard", status: "active", builtIn: true, version: 1, capabilities: ["action_buttons"], triggerTypes: ["user_action"], summary: "操作按钮面板" },
    { id: "custom-card-1", name: "自定义卡片", kind: "form_panel", renderer: "McpActionFormCard", status: "draft", builtIn: false, version: 1, capabilities: [], triggerTypes: [], summary: "测试卡片" },
  ],
  stats: { total: 3, active: 2, draft: 1, disabled: 0, builtIn: 2, custom: 1 },
  total: 3,
};

const PREVIEW_RESPONSE = {
  cardId: "mcp-summary",
  name: "摘要卡片",
  kind: "readonly_summary",
  renderer: "McpSummaryCard",
  mockData: { title: "摘要卡片 — 预览", kvRows: [{ key: "示例指标", value: "42" }] },
};

test.describe("UICardManagementPage", () => {
  test.beforeEach(async ({ page }) => {
    await page.route("**/api/v1/ui-cards", (route) => {
      if (route.request().method() === "GET") {
        return route.fulfill({ json: CARDS_RESPONSE });
      }
      return route.fulfill({ json: { ok: true } });
    });
    await page.route("**/api/v1/ui-cards/*/preview", (route) =>
      route.fulfill({ json: PREVIEW_RESPONSE })
    );
    await page.route("**/api/v1/ui-cards/*", (route) => {
      if (route.request().method() === "PUT") {
        return route.fulfill({ json: { ok: true } });
      }
      return route.fulfill({ json: CARDS_RESPONSE.items[0] });
    });
    await page.route("**/api/v1/session*", (route) =>
      route.fulfill({ json: { sessionId: "test-session" } })
    );
    await page.goto("/ui-cards");
  });

  test("page renders with title and stats", async ({ page }) => {
    await expect(page.getByRole("heading", { name: "UI 卡片管理" })).toBeVisible();
    await expect(page.locator(".uic-stat strong").first()).toBeVisible();
  });

  test("overview tab shows kind groups", async ({ page }) => {
    await expect(page.locator(".ops-card").first()).toBeVisible();
  });

  test("list tab shows card table", async ({ page }) => {
    await page.locator(".ops-tabs-tab", { hasText: "卡片列表" }).click();
    await expect(page.locator(".ops-data-table-table")).toBeVisible();
    await expect(page.getByText("摘要卡片").first()).toBeVisible();
  });

  test("clicking detail button shows card details", async ({ page }) => {
    await page.locator(".ops-tabs-tab", { hasText: "卡片列表" }).click();
    const table = page.locator(".ops-data-table-table");
    await table.getByRole("button", { name: "详情" }).first().click();
    await expect(page.locator(".ops-descriptions")).toBeVisible();
  });

  test("clicking edit button opens editor", async ({ page }) => {
    await page.locator(".ops-tabs-tab", { hasText: "卡片列表" }).click();
    const table = page.locator(".ops-data-table-table");
    await table.getByRole("button", { name: "编辑" }).first().click();
    await expect(page.locator(".ops-form")).toBeVisible();
  });

  test("debugger tab is accessible", async ({ page }) => {
    await page.locator(".ops-tabs-tab", { hasText: "触发调试器" }).click();
    // The card header contains "触发调试器"
    await expect(page.locator(".ops-card-header", { hasText: "触发调试器" })).toBeVisible();
  });
});
