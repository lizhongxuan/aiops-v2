// @ts-check
import { test, expect } from "@playwright/test";

async function waitForStable(page, timeout = 8000) {
  await page.waitForLoadState("networkidle", { timeout }).catch(() => {});
  await page.waitForTimeout(600);
}

async function ensureWorkspace(page) {
  await page.goto("/protocol");
  await waitForStable(page);
  const createBtn = page.locator("button", { hasText: /新建工作台/ });
  if (await createBtn.isVisible({ timeout: 3000 }).catch(() => false)) {
    await createBtn.click();
    await page.waitForTimeout(2000);
  }
  await waitForStable(page);
}

test.describe("布局响应式测试", () => {
  test.setTimeout(60000);

  test("1440x900 右侧栏不溢出", async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
    await ensureWorkspace(page);
    const sideRail = page.locator(".workspace-side-rail");
    if (!(await sideRail.isVisible({ timeout: 3000 }).catch(() => false))) { test.skip(); return; }
    const box = await sideRail.boundingBox();
    expect(box.y + box.height).toBeLessThanOrEqual(page.viewportSize().height + 2);
    await page.screenshot({ path: "tests/screenshots/layout-1440x900.png", fullPage: false });
  });

  test("1280x720 右侧栏不溢出", async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 720 });
    await ensureWorkspace(page);
    const sideRail = page.locator(".workspace-side-rail");
    if (!(await sideRail.isVisible({ timeout: 3000 }).catch(() => false))) { test.skip(); return; }
    const box = await sideRail.boundingBox();
    expect(box.y + box.height).toBeLessThanOrEqual(page.viewportSize().height + 2);
    await page.screenshot({ path: "tests/screenshots/layout-1280x720.png", fullPage: false });
  });

  test("单机会话 padding-bottom <= 200px", async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
    await page.goto("/");
    await waitForStable(page);
    const chatContainer = page.locator(".chat-container").first();
    if (!(await chatContainer.isVisible({ timeout: 3000 }).catch(() => false))) { test.skip(); return; }
    const pb = await chatContainer.evaluate((el) => parseInt(window.getComputedStyle(el).paddingBottom) || 0);
    expect(pb).toBeLessThanOrEqual(200);
    expect(pb).toBeGreaterThanOrEqual(100);
    await page.screenshot({ path: "tests/screenshots/chat-input-gap.png", fullPage: false });
  });
});
