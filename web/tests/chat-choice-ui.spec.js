// @ts-check
import { test, expect } from "@playwright/test";
import {
  createChatFixtureSessions,
  createChatFixtureState,
  openFixturePage,
} from "./helpers/uiFixtureHarness";

const SCREENSHOT_DIR = "tests/screenshots";

test.describe("Chat ChoiceCard smoke", () => {
  test("shows recommended option first, keeps per-option descriptions, and submits choice answers", async ({ page }) => {
    await openFixturePage(page, "/", {
      state: createChatFixtureState({
        cards: [
          {
            id: "user-choice-1",
            type: "UserMessageCard",
            role: "user",
            text: "我想知道现在应该怎么处理这个中间件问题。",
            createdAt: "2026-04-03T12:00:00Z",
            updatedAt: "2026-04-03T12:00:00Z",
          },
          {
            id: "choice-card-1",
            type: "ChoiceCard",
            requestId: "choice-1",
            title: "请选择处理方式",
            status: "pending",
            questions: [
              {
                header: "推荐方案",
                question: "你更希望先怎么处理 nginx 中间件？",
                isOther: true,
                options: [
                  {
                    label: "推荐：重载并观察",
                    value: "reload_observe",
                    description: "适合配置已更新、希望先验证是否恢复的情况。",
                  },
                  {
                    label: "继续采集日志",
                    value: "collect_more_logs",
                    description: "适合还需要更多证据来判断根因的时候。",
                  },
                  {
                    label: "切换到备用节点",
                    value: "failover",
                    description: "适合当前实例明显不稳定、需要快速止损的时候。",
                  },
                ],
              },
            ],
            createdAt: "2026-04-03T12:00:02Z",
            updatedAt: "2026-04-03T12:00:02Z",
          },
        ],
        runtime: {
          turn: { active: true, phase: "thinking", hostId: "web-01" },
          codex: { status: "connected", retryAttempt: 0, retryMax: 5 },
          activity: {
            viewedFiles: [],
            searchedWebQueries: [],
            searchedContentQueries: [],
          },
        },
      }),
      sessions: createChatFixtureSessions({
        sessions: [
          {
            id: "single-1",
            kind: "single_host",
            title: "Choice chat",
            status: "running",
            messageCount: 2,
            preview: "我想知道现在应该怎么处理这个中间件问题。",
            selectedHostId: "web-01",
            lastActivityAt: "2026-04-03T12:00:02Z",
          },
        ],
      }),
    });

    const choiceCard = page.locator(".choice-card");
    await expect(choiceCard).toBeVisible();

    const optionRows = choiceCard.locator(".choice-option");
    await expect(optionRows).toHaveCount(4);
    await expect(optionRows.first()).toContainText("推荐：重载并观察");
    await expect(optionRows.first()).toContainText("适合配置已更新、希望先验证是否恢复的情况。");
    await expect(optionRows.nth(1)).toContainText("继续采集日志");
    await expect(optionRows.nth(2)).toContainText("切换到备用节点");

    const optionDescriptions = choiceCard.locator(".option-description");
    await expect(optionDescriptions).toHaveCount(4);

    await expect(choiceCard.getByText("其他")).toBeVisible();
    await optionRows.last().click();

    const otherInput = choiceCard.locator(".choice-input");
    await expect(otherInput).toBeVisible();
    await otherInput.fill("先补充一下日志侧的上下文，再决定下一步。");

    await choiceCard.getByRole("button", { name: "补充说明（选填）" }).click();
    await choiceCard.locator(".choice-note-input").fill("优先避免影响现网流量。");

    let capturedRequest = null;
    await page.route("**/api/v1/choices/*/answer", async (route) => {
      capturedRequest = {
        url: route.request().url(),
        body: route.request().postDataJSON(),
      };
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ ok: true }),
      });
    });

    await choiceCard.getByRole("button", { name: /提交/ }).click();

    await expect.poll(() => capturedRequest?.url || "").toContain("/api/v1/choices/choice-1/answer");
    await expect.poll(() => capturedRequest?.body?.answers?.[0]?.isOther).toBe(true);
    await expect.poll(() => capturedRequest?.body?.answers?.[0]?.value).toBe(
      "先补充一下日志侧的上下文，再决定下一步。",
    );
    await expect.poll(() => capturedRequest?.body?.answers?.[0]?.note).toBe("优先避免影响现网流量。");

    await expect(choiceCard).toBeVisible();
    await expect(page.locator("body")).not.toContainText("choice submit failed");

    await page.screenshot({
      path: `${SCREENSHOT_DIR}/chat-choice-card-smoke.png`,
      fullPage: false,
    });
  });
});
