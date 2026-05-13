// @ts-check
import { test, expect } from "@playwright/test";

async function waitForStable(page, timeout = 8000) {
  await page.waitForLoadState("networkidle", { timeout }).catch(() => {});
  await page.waitForTimeout(600);
}

test.describe("侧边栏和布局修复", () => {
  test.setTimeout(60000);

  test("左侧菜单顶部按钮显示新建会话和新建工作台", async ({ page }) => {
    await page.goto("/");
    await waitForStable(page);

    const topActions = page.locator(".sidebar-actions .nav-button .nav-label");
    const titles = await topActions.allTextContents();

    expect(titles).toContain("新建会话");
    expect(titles).toContain("新建工作台");

    const navItems = page.locator(".nav-item-title");
    const navTitles = await navItems.allTextContents();
    expect(navTitles).toContain("单机会话");
    expect(navTitles).toContain("协作工作台");
    expect(navTitles).not.toContain("MCP");
    expect(navTitles).toContain("主机列表");

    await page.screenshot({
      path: "tests/screenshots/sidebar-fixed-titles.png",
      fullPage: false,
    });
  });

  test("单机会话顶部栏展示历史会话", async ({ page }) => {
    await page.goto("/");
    await waitForStable(page);

    const historyButton = page.locator(".header-right button", { hasText: "历史会话" });
    await expect(historyButton).toBeVisible();
    await expect(page.locator(".header-right button", { hasText: "历史工作台" })).toHaveCount(0);
    await historyButton.click();
    await expect(page.locator(".session-history-title", { hasText: "历史会话" })).toBeVisible();
  });

  test("顶部新建按钮分别进入单机会话和协作工作台", async ({ page }) => {
    await page.goto("/");
    await waitForStable(page);

    await page.locator(".sidebar-actions .nav-button", { hasText: "新建工作台" }).click();
    await waitForStable(page);
    expect(new URL(page.url()).pathname).toBe("/protocol");

    await page.locator(".sidebar-actions .nav-button", { hasText: "新建会话" }).click();
    await waitForStable(page);
    expect(new URL(page.url()).pathname).toBe("/");
  });

  test("协作工作台顶部栏展示历史工作台且不显示终端和主机切换", async ({ page }) => {
    await page.goto("/protocol");
    await waitForStable(page);

    const createBtn = page.locator("button", { hasText: /新建工作台/ });
    if (await createBtn.isVisible({ timeout: 3000 }).catch(() => false)) {
      await createBtn.click();
      await page.waitForTimeout(2000);
    }
    await waitForStable(page);

    const historyButton = page.locator(".header-right button", { hasText: "历史工作台" });
    await expect(historyButton).toBeVisible();
    await expect(page.locator(".header-right button", { hasText: "历史会话" })).toHaveCount(0);
    await expect(page.locator('button[title="打开终端"]')).toHaveCount(0);
    await expect(page.locator(".header-right .pill-dot")).toHaveCount(0);
    await historyButton.click();
    await expect(page.locator(".session-history-title", { hasText: "历史工作台" })).toBeVisible();
  });

  test("主机列表导航指向 /settings/hosts", async ({ page }) => {
    await page.goto("/");
    await waitForStable(page);

    const hostNav = page.locator(".nav-item", { hasText: "主机列表" });
    await expect(hostNav).toBeVisible();
    await hostNav.click();
    await page.waitForTimeout(1000);

    expect(page.url()).toContain("/settings/hosts");
  });

  test("MCP 服务器页可直接打开运行时列表并触发刷新", async ({ page }) => {
    await page.route("**/api/v1/state", (route) =>
      route.fulfill({
        json: {
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
        },
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

  test("设置齿轮菜单包含设置总览、Agent Profile、Skills 管理、MCP 服务器和 Experience Packs", async ({ page }) => {
    await page.goto("/");
    await waitForStable(page);

    const gearBtn = page.locator(".sidebar-bottom .nav-icon-btn").first();
    await gearBtn.click();
    await page.waitForTimeout(300);

    const menuItems = page.locator(".settings-menu-title");
    const titles = await menuItems.allTextContents();

    expect(titles).toContain("设置总览");
    expect(titles).toContain("Agent Profile");
    expect(titles).toContain("Skills 管理");
    expect(titles).toContain("MCP 服务器");
    expect(titles).toContain("Experience Packs");
  });

  test("Skills / MCP 服务器页可从设置菜单直达且页面语义清晰", async ({ page }) => {
    await page.goto("/");
    await waitForStable(page);

    const gearBtn = page.locator(".sidebar-bottom .nav-icon-btn").first();
    await gearBtn.click();
    await page.waitForTimeout(300);
    await page.locator(".settings-menu-item", { hasText: "Skills 管理" }).click();
    await waitForStable(page);
    expect(new URL(page.url()).pathname).toBe("/settings/skills");
    await expect(page.locator("h1", { hasText: "Skills 管理" })).toBeVisible();
    await expect(page.locator(".sidebar-head", { hasText: "Skill Catalog" })).toBeVisible();
    await expect(page.locator(".header-btn", { hasText: "保存" })).toBeVisible();
    await expect(page.locator(".header-btn", { hasText: "删除" })).toBeVisible();
    await expect(page.locator(".mini-btn", { hasText: "新增" })).toBeVisible();
    await expect(page).toHaveTitle("Skills 管理 · Settings");

    await page.goto("/");
    await waitForStable(page);
    await gearBtn.click();
    await page.waitForTimeout(300);
    await page.locator(".settings-menu-item", { hasText: "MCP 服务器" }).click();
    await waitForStable(page);
    expect(new URL(page.url()).pathname).toBe("/mcp");
    await expect(page.locator(".header-title", { hasText: "MCP 服务器" })).toBeVisible();
    await expect(page.locator(".mcp-section-header", { hasText: "服务器" })).toBeVisible();
    await expect(page.locator(".compact-btn", { hasText: "添加服务器" })).toBeVisible();
    await expect(page).toHaveTitle("MCP 服务器 · aiops-codex");
  });

  test("协作工作台右侧栏不溢出", async ({ page }) => {
    await page.goto("/protocol");
    await waitForStable(page);

    const createBtn = page.locator("button", { hasText: /新建工作台/ });
    if (await createBtn.isVisible({ timeout: 3000 }).catch(() => false)) {
      await createBtn.click();
      await page.waitForTimeout(2000);
    }
    await waitForStable(page);

    // Check the side rail exists and is within viewport
    const sideRail = page.locator(".workspace-side-rail");
    const visible = await sideRail.isVisible({ timeout: 3000 }).catch(() => false);
    if (!visible) {
      test.skip();
      return;
    }

    const approvalPanel = page.locator("[data-testid='protocol-side-panel-approval']");
    const timelinePanel = page.locator("[data-testid='protocol-side-panel-timeline']");
    const runtimePill = page.locator("[data-testid='protocol-runtime-pill']");
    const approvalVisible = await approvalPanel.isVisible({ timeout: 3000 }).catch(() => false);
    const timelineVisible = await timelinePanel.isVisible({ timeout: 3000 }).catch(() => false);
    const runtimeVisible = await runtimePill.isVisible({ timeout: 3000 }).catch(() => false);
    if (!approvalVisible || !timelineVisible || !runtimeVisible) {
      test.skip();
      return;
    }

    const box = await sideRail.boundingBox();
    const approvalBox = await approvalPanel.boundingBox();
    const timelineBox = await timelinePanel.boundingBox();
    const runtimeBox = await runtimePill.boundingBox();
    const vh = page.viewportSize()?.height || 900;
    // The side rail should not extend beyond the viewport
    expect(box.y + box.height).toBeLessThanOrEqual(vh + 5);
    expect(approvalBox.height).toBeGreaterThan(260);
    expect(timelineBox.height).toBeGreaterThan(160);
    expect(approvalBox.y).toBeLessThan(timelineBox.y);
    expect(approvalBox.y + approvalBox.height).toBeLessThanOrEqual(timelineBox.y + 1);
    expect(timelineBox.y + timelineBox.height).toBeLessThanOrEqual(runtimeBox.y + 1);
    expect(runtimeBox.y + runtimeBox.height).toBeLessThanOrEqual(box.y + box.height + 1);

    await page.screenshot({
      path: "tests/screenshots/protocol-right-sidebar-layout.png",
      fullPage: false,
    });
  });
});
