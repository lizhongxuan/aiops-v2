// @ts-check
import { test, expect } from "@playwright/test";
import { openBrowserFixturePage } from "../helpers/uiFixtureHarness";

test.describe("browser fixture entry", () => {
  test("loads chat fixture state and sessions without route mocks", async ({ page }) => {
    await openBrowserFixturePage(page, "/", "chat");

    await expect(page.locator("body")).toContainText("nginx 中间件的状态");
    await expect(page.locator("body")).toContainText("收集 nginx 错误日志");
  });

  test("loads protocol fixture state and sessions without route mocks", async ({ page }) => {
    await openBrowserFixturePage(page, "/protocol?promptDebug=1", "protocol");

    await expect(page.locator("body")).toContainText("复杂运维 AI Chat");
    await expect(page.locator("body")).toContainText("nginx 巡检计划");
    await expect(page.locator("body")).toContainText("等待审批");
  });

  test("loads completed context compaction fixture with composer enabled after reload", async ({ page }) => {
    await openBrowserFixturePage(page, "/", "context-compaction");

    await expect(page.getByText("上下文过长，已使用本地摘要继续")).toBeVisible();
    await expect(page.getByText("正在重试压缩")).toHaveCount(0);
    await expect(page.getByText("LLM 未配置")).toHaveCount(0);
    await expect(page.getByText("请先创建会话")).toHaveCount(0);
    await expect(page.getByRole("button", { name: "停止生成" })).toHaveCount(0);
    await expect(page.getByTestId("omnibar-input")).toBeEnabled();
    await expect(page.getByTestId("omnibar-primary-action")).toHaveAttribute("aria-label", "send message");

    await page.reload();

    await expect(page.getByText("上下文过长，已使用本地摘要继续")).toBeVisible();
    await expect(page.getByText("LLM 未配置")).toHaveCount(0);
    await expect(page.getByText("请先创建会话")).toHaveCount(0);
    await expect(page.getByRole("button", { name: "停止生成" })).toHaveCount(0);
    await expect(page.getByTestId("omnibar-input")).toBeEnabled();
    await expect(page.getByTestId("omnibar-primary-action")).toHaveAttribute("aria-label", "send message");
  });

  test("loads ops manual preflight fixture without route mocks", async ({ page }) => {
    await openBrowserFixturePage(page, "/", "ops-manual-preflight");

    const searchCard = page.getByTestId("ops-manual-search-result-card");
    await expect(searchCard).toContainText("运行预检");
    await expect(searchCard.getByTestId("ops-manual-merged-preflight")).toContainText("预检通过");
    await expect(searchCard.getByTestId("ops-manual-merged-preflight")).toContainText("确认执行");
    await expect(page.getByTestId("ops-manual-preflight-result-card")).toHaveCount(0);
    await expect(page.getByText(/命中\s*\d+\s*%/)).toHaveCount(0);
    await expect(page.getByText("立即执行")).toHaveCount(0);
  });

  test("loads ops manual 4-field form fixture without duplicate inline prompt", async ({ page }) => {
    await openBrowserFixturePage(page, "/", "ops-manual-4field-form");

    const card = page.getByTestId("ops-manual-search-result-card");
    await expect(card).not.toContainText("手册缺上下文");
    await expect(card).not.toContainText("信息不足，不能直接使用工作流。");
    await expect(card).toContainText("Redis SSH 排障运维手册");
    await expect(card).not.toContainText("请在底部补充");
    await expect(card).not.toContainText("打开补充表单");
    await expect(page.getByTestId("ops-manual-context-prompt")).toHaveCount(0);
    await expect(page.getByTestId("ops-manual-candidate-match-detail")).toHaveCount(0);
    await page.getByTestId("ops-manual-candidate-toggle").click();
    await expect(page.getByTestId("ops-manual-candidate-match-detail")).toContainText("命中依据");
    await expect(page.getByTestId("ops-manual-candidate-match-detail")).toContainText("对象类型");
    await expect(page.getByTestId("ops-manual-candidate-match-detail")).toContainText("操作类型");
    await expect(card.getByRole("button", { name: "不使用" })).toBeVisible();
    await expect(card.getByRole("button", { name: "查看工作流" })).toBeVisible();
    await expect(card.getByRole("button", { name: "查看手册" })).toBeVisible();

    await card.getByRole("button", { name: "查看工作流" }).click();
    await expect(page.getByRole("dialog")).toContainText("工作流只读预览");
    await expect(page.getByRole("dialog")).toContainText("Redis SSH 排障工作流");
    await expect(page.getByRole("dialog")).toContainText("采集只读指标");
    await expect(page.getByRole("dialog")).toContainText("redis-cli INFO memory");
    await page.getByRole("button", { name: "2. 判断内存压力" }).click();
    await expect(page.getByRole("dialog")).toContainText("compare used_memory_rss maxmemory");
    await page.keyboard.press("Escape");

    await card.getByRole("button", { name: "查看手册" }).click();
    await expect(page.getByRole("dialog")).toContainText("运维手册只读预览");
    await expect(page.getByRole("dialog")).toContainText("Redis SSH 排障运维手册");
    await expect(page.getByRole("dialog")).toContainText("用于 Redis SSH 场景的只读排障和恢复前验证");
    await page.keyboard.press("Escape");

    await expect(page.getByTestId("ops-manual-context-composer")).toHaveCount(0);
    await expect(page.getByTestId("omnibar-input")).toBeVisible();
  });

  test("does not offer chat-to-manual generation from a normal completed AI chat operation", async ({ page }) => {
    await openBrowserFixturePage(page, "/", "ops-manual-generate-from-chat");

    await expect(page.getByText("本次验证状态：已验证")).toBeVisible();
    await expect(page.getByTestId("omnibar-input")).toBeVisible();
    await expect(page.getByText("本次对话可沉淀为运维手册")).toHaveCount(0);
    await expect(page.getByTestId("aiops-generate-ops-manual-from-chat")).toHaveCount(0);
    await expect(page.getByTestId("ops-manual-generation-confirmation")).toHaveCount(0);
  });
});
