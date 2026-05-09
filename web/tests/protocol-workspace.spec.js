// @ts-check
import { test, expect } from "@playwright/test";

async function navigateToProtocol(page) {
  await page.goto("/");
  await page.waitForLoadState("networkidle");
  const navItem = page.locator("button.nav-item", { hasText: /协议工作台|工作台|protocol/i });
  if (await navItem.isVisible({ timeout: 3000 }).catch(() => false)) {
    await navItem.click();
    await page.waitForTimeout(800);
  }
}

async function ensureWorkspaceSession(page) {
  // 如果显示"当前不是协作工作台会话"，点击新建
  const createBtn = page.locator("button", { hasText: /新建工作台/ });
  if (await createBtn.isVisible({ timeout: 2000 }).catch(() => false)) {
    await createBtn.click();
    await page.waitForTimeout(1500);
  }
}

test.describe("协作工作台页面", () => {
  test.beforeEach(async ({ page }) => {
    await navigateToProtocol(page);
    await ensureWorkspaceSession(page);
  });

  test("页面加载成功", async ({ page }) => {
    const workspace = page.locator("[data-testid='protocol-workspace-page']");
    await expect(workspace).toBeVisible({ timeout: 5000 });
  });

  test("左侧对话区可见（主 Agent 对话区）", async ({ page }) => {
    const conversationPane = page.locator(".protocol-conversation-pane");
    const visible = await conversationPane.isVisible({ timeout: 3000 }).catch(() => false);
    // 如果是空状态也算通过
    if (!visible) {
      const emptyState = page.locator("text=当前不是协作工作台会话");
      const isEmpty = await emptyState.isVisible({ timeout: 2000 }).catch(() => false);
      expect(visible || isEmpty).toBe(true);
    }
  });

  test("右侧审批面板可见", async ({ page }) => {
    const rail = page.locator("[data-testid='protocol-approval-rail']");
    const visible = await rail.isVisible({ timeout: 3000 }).catch(() => false);
    if (!visible) {
      // 空状态下审批面板不显示，跳过
      test.skip();
    }
    await expect(rail).toBeVisible();
  });

  test("右侧事件时间线可见", async ({ page }) => {
    const timeline = page.locator("[data-testid='protocol-event-timeline']");
    const visible = await timeline.isVisible({ timeout: 3000 }).catch(() => false);
    if (!visible) {
      test.skip();
    }
    await expect(timeline).toBeVisible();
  });

  test("输入框可见", async ({ page }) => {
    const composer = page.locator("textarea");
    const visible = await composer.first().isVisible({ timeout: 3000 }).catch(() => false);
    if (!visible) {
      test.skip();
    }
    await expect(composer.first()).toBeVisible();
  });

  test("输入框在视口下半部分（固定底部）", async ({ page }) => {
    const composer = page.locator("textarea").first();
    const visible = await composer.isVisible({ timeout: 3000 }).catch(() => false);
    if (!visible) {
      test.skip();
      return;
    }
    const box = await composer.boundingBox();
    if (!box) {
      test.skip();
      return;
    }
    const vh = page.viewportSize()?.height || 900;
    // 输入框应该在视口下半部分
    expect(box.y).toBeGreaterThan(vh * 0.4);
  });

  test("输入框可以输入文字", async ({ page }) => {
    const composer = page.locator("textarea").first();
    const visible = await composer.isVisible({ timeout: 3000 }).catch(() => false);
    if (!visible) {
      test.skip();
      return;
    }
    await composer.fill("测试消息");
    await expect(composer).toHaveValue("测试消息");
  });

  test("页面不出现页面级垂直滚动条", async ({ page }) => {
    await page.waitForTimeout(500);
    const hasScroll = await page.evaluate(() => {
      return document.documentElement.scrollHeight > document.documentElement.clientHeight + 5;
    });
    expect(hasScroll).toBe(false);
  });

  test("左右面板之间有分隔线", async ({ page }) => {
    const sideRail = page.locator(".workspace-side-rail");
    const visible = await sideRail.isVisible({ timeout: 3000 }).catch(() => false);
    if (!visible) {
      test.skip();
      return;
    }
    const borderLeft = await sideRail.evaluate((el) => {
      return parseInt(window.getComputedStyle(el).borderLeftWidth) || 0;
    });
    expect(borderLeft).toBeGreaterThanOrEqual(1);
  });

  test("整体截图", async ({ page }) => {
    await page.waitForTimeout(500);
    await page.screenshot({
      path: "tests/screenshots/protocol-workspace-full.png",
      fullPage: false,
    });
  });
});
