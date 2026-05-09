// @ts-check
import { test, expect } from "@playwright/test";

/**
 * 验证过期审批卡片在点击操作后会从右侧面板消失
 */

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

test.describe("过期审批卡片自动清除", () => {
  test.setTimeout(120000);

  test("点击过期审批的任意按钮后，卡片从面板消失", async ({ page }) => {
    await ensureWorkspaceSession(page);

    // 检查是否有审批卡片
    const approvalCard = page.locator(".approval-card");
    const hasApproval = await approvalCard.first().isVisible({ timeout: 3000 }).catch(() => false);

    if (!hasApproval) {
      // 没有审批卡片，跳过
      test.skip();
      return;
    }

    // 记录当前审批数量
    const beforeCount = await approvalCard.count();
    expect(beforeCount).toBeGreaterThan(0);

    // 点击第一个审批卡片的"同意执行"按钮
    const acceptBtn = approvalCard.first().locator(".action-btn.primary");
    const acceptVisible = await acceptBtn.isVisible({ timeout: 2000 }).catch(() => false);
    if (!acceptVisible) {
      test.skip();
      return;
    }

    await acceptBtn.click();
    await page.waitForTimeout(2000);

    // 检查 toolbar 是否显示了友好提示（过期审批）或成功提示
    const toolbar = page.locator(".toolbar-notice");
    const toolbarVisible = await toolbar.isVisible({ timeout: 3000 }).catch(() => false);

    if (toolbarVisible) {
      const toolbarText = await toolbar.textContent();
      // 不应该显示原始 "approval not found"
      expect(toolbarText).not.toContain("approval not found");
    }

    // 审批卡片数量应该减少（过期的被清除了）
    const afterCount = await approvalCard.count();
    expect(afterCount).toBeLessThan(beforeCount);

    // 如果所有审批都被清除，应该显示空状态
    if (afterCount === 0) {
      const emptyState = page.locator(".approval-empty");
      await expect(emptyState).toBeVisible({ timeout: 3000 });
    }

    await page.screenshot({
      path: "tests/screenshots/protocol-stale-approval-dismissed.png",
      fullPage: false,
    });
  });
});
