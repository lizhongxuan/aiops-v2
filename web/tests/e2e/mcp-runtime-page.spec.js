// @ts-check
import { test, expect } from "@playwright/test";

function buildSnapshot() {
  return {
    sessionId: "test-session",
    kind: "single_host",
    selectedHostId: "server-local",
    auth: { connected: true, pending: false, mode: "api_key" },
    hosts: [{ id: "server-local", name: "server-local", status: "online", executable: true, terminalCapable: true }],
    cards: [],
    approvals: [],
    config: { codexAlive: true, model: "gpt-5.4" },
    runtime: {
      turn: { active: false, phase: "idle", hostId: "server-local" },
      codex: { status: "connected" },
      activity: {},
    },
  };
}

async function waitForStable(page, timeout = 8000) {
  await page.waitForLoadState("networkidle", { timeout }).catch(() => {});
  await page.waitForTimeout(400);
}

test.describe("MCP runtime page", () => {
  test("runtime list refreshes servers", async ({ page }) => {
    await page.route("**/api/v1/state", (route) =>
      route.fulfill({
        json: buildSnapshot(),
      }),
    );
    await page.route("**/api/v1/sessions*", (route) =>
      route.fulfill({
        json: {
          sessions: [{ id: "test-session", label: "test", kind: "single_host" }],
          activeSessionId: "test-session",
        },
      }),
    );
    await page.routeWebSocket("**/ws", (ws) => {
      ws.close();
    });

    let refreshCalls = 0;
    await page.route("**/api/v1/mcp/servers", async (route) => {
      if (route.request().method() === "GET") {
        await route.fulfill({
          json: {
            configPath: "/workspace/.kiro/settings/mcp.json",
            items: [
              {
                name: "coroot-rca",
                transport: "http",
                url: "http://127.0.0.1:8088/mcp",
                disabled: false,
                status: "connected",
                toolCount: 3,
                resourceCount: 1,
              },
            ],
          },
        });
        return;
      }
      await route.continue();
    });
    await page.route("**/api/v1/mcp/servers/refresh", async (route) => {
      refreshCalls += 1;
      await route.fulfill({
        json: {
          ok: true,
          items: [
            {
              name: "coroot-rca",
              transport: "http",
              url: "http://127.0.0.1:8088/mcp",
              disabled: false,
              status: "connected",
              toolCount: 4,
              resourceCount: 2,
            },
          ],
        },
      });
    });

    await page.goto("/mcp");
    await waitForStable(page);

    await expect(page.locator(".header-title", { hasText: "MCP 服务器" })).toBeVisible();
    await expect(page.locator(".runtime-list-item", { hasText: "coroot-rca" })).toBeVisible();
    await expect(page.locator(".runtime-list-item", { hasText: "connected" })).toBeVisible();

    await page.locator(".mcp-header-actions .compact-btn", { hasText: "刷新" }).click();
    await expect.poll(() => refreshCalls).toBe(1);
    await expect(page.locator(".runtime-list-item", { hasText: "4 tools" })).toBeVisible();
    await expect(page.locator(".runtime-list-item", { hasText: "2 resources" })).toBeVisible();
  });
});
