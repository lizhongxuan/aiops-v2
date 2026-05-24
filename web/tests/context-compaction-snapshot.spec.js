import { expect, test } from "@playwright/test";

import { openFixturePage } from "./helpers/uiFixtureHarness.js";

test("chat shows context compaction and externalized evidence states", async ({ page }) => {
  await openFixturePage(page, "/", "context-compaction");

  await expect(page.getByText("上下文过长，已使用本地摘要继续")).toBeVisible();
  await expect(page.getByText("正在重试压缩")).toHaveCount(0);
  await expect(page.getByText("已外溢")).toHaveCount(0);
  await expect(page.getByRole("button", { name: /查看原始证据/ })).toHaveCount(0);

  const toolRow = page.getByTestId("aiops-tool-row-tool-context-spill");
  if ((await toolRow.count()) === 0) {
    await page.getByTestId("aiops-process-header").click();
  }
  await expect(toolRow).toBeVisible();
  await toolRow.click();
  await expect(page.getByText("结果较大，仅显示摘要。")).toBeVisible();
  await expect(page.getByText(/upstream timed out/)).toHaveCount(0);
  await expect(page.getByTestId("context-status-notice")).toHaveScreenshot("context-compaction-notice.png");
  await expect(page.getByTestId("aiops-process-transcript")).toHaveScreenshot("context-compaction-process.png");
});
