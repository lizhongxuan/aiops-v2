// @ts-check
import { expect, test } from "@playwright/test";

import { openBrowserFixturePage } from "../helpers/uiFixtureHarness";

test.describe("web_search search/open transcript fixture", () => {
  test("keeps search and open operations as stable web_search lookup blocks", async ({ page }) => {
    await openBrowserFixturePage(page, "/", "web-search-open-transcript");

    await expect(page.getByTestId("aiops-process-header")).toBeVisible();
    await page.getByTestId("aiops-process-header").click();

    const transcript = page.getByTestId("aiops-process-transcript-body");
    await expect(transcript).toContainText("我会先搜索官方文档来源");
    await expect(transcript).toContainText("我会打开官方参数页面");
    await expect(transcript).not.toContainText("browse_url");

    const toggles = page.getByTestId("aiops-search-toggle");
    await expect(toggles).toHaveCount(2);
    await expect(toggles.nth(0)).toContainText("网页检索 1 次");
    await expect(toggles.nth(0)).toContainText("找到 2 个来源");
    await expect(toggles.nth(1)).toContainText("网页检索 1 次");
    await expect(toggles.nth(1)).toContainText("找到 1 个来源");

    await toggles.nth(0).click();
    await expect(page.getByTestId("aiops-search-details").nth(0)).toContainText("https://www.postgresql.org/docs/current/runtime-config-wal.html");
    await expect(page.getByTestId("aiops-search-details").nth(0)).toContainText("https://www.postgresql.org/docs/current/continuous-archiving.html");

    await toggles.nth(1).click();
    const openDetails = page.getByTestId("aiops-search-details").nth(1);
    await expect(openDetails).toContainText("https://www.postgresql.org/docs/current/runtime-config-wal.html");
    await expect(openDetails).not.toContainText("Full bounded page text");
    await openDetails.getByTestId("aiops-search-detail-row-toggle").first().click();
    await expect(openDetails.getByTestId("aiops-search-detail-expanded").first()).toContainText("已读取正文");
  });
});
