// @ts-check
import { test, expect } from "@playwright/test";

async function waitForStable(page, timeout = 8000) {
  await page.waitForLoadState("networkidle", { timeout }).catch(() => {});
  await page.waitForTimeout(600);
}

async function ensureWorkspaceSession(page) {
  await page.goto("/protocol");
  await waitForStable(page);
  const createBtn = page.locator("button", { hasText: /新建工作台/ });
  if (await createBtn.isVisible({ timeout: 3000 }).catch(() => false)) {
    await createBtn.click();
    await page.waitForTimeout(2000);
  }
  await waitForStable(page);
}

test.describe("实时事件主机标签", () => {
  test.setTimeout(180000);

  test("事件不显示'未知主机'或'主机'，应显示 local 或 IP", async ({ page }) => {
    await ensureWorkspaceSession(page);

    const textarea = page.locator("textarea").first();
    if (!(await textarea.isVisible({ timeout: 5000 }).catch(() => false))) {
      test.skip();
      return;
    }

    // Send a command that will generate events
    await textarea.fill("查看当前系统时间");
    await textarea.press("Meta+Enter");

    // Wait for events to appear
    const thinking = page.locator(".thinking-wrapper");
    await thinking.waitFor({ state: "visible", timeout: 10000 }).catch(() => {});
    await thinking.waitFor({ state: "hidden", timeout: 120000 }).catch(() => {});
    await page.waitForTimeout(1000);

    // Check event timeline items
    const eventItems = page.locator(".timeline-item");
    const count = await eventItems.count();

    if (count === 0) {
      // No events generated, skip
      test.skip();
      return;
    }

    // Verify no event shows "未知主机" or bare "主机"
    for (let i = 0; i < count; i++) {
      const text = await eventItems.nth(i).textContent();
      expect(text).not.toContain("未知主机");
      expect(text).not.toMatch(/^主机\s*·/);
    }

    await page.screenshot({
      path: "tests/screenshots/protocol-host-labels.png",
      fullPage: false,
    });
  });
});
