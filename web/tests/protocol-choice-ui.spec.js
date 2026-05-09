// @ts-check
import { test, expect } from "@playwright/test";
import {
  createProtocolFixtureSessions,
  createProtocolFixtureState,
  openFixturePage,
} from "./helpers/uiFixtureHarness";

const SCREENSHOT_DIR = "tests/screenshots";

function createProtocolChoiceFixture() {
  return {
    state: createProtocolFixtureState({
      approvals: [],
      cards: [
        {
          id: "user-choice-1",
          type: "UserMessageCard",
          role: "user",
          text: "我想知道 nginx 中间件现在应该怎么处理。",
          createdAt: "2026-04-03T12:10:00Z",
          updatedAt: "2026-04-03T12:10:00Z",
        },
        {
          id: "assistant-choice-1",
          type: "AssistantMessageCard",
          role: "assistant",
          text: "我先给你一个可执行选项。",
          createdAt: "2026-04-03T12:10:05Z",
          updatedAt: "2026-04-03T12:10:05Z",
        },
        {
          id: "choice-card-1",
          type: "ChoiceCard",
          requestId: "choice-1",
          title: "请选择处理方式",
          status: "pending",
          question: "你更希望先怎么处理 nginx 中间件？",
          isOther: true,
          noteLabel: "补充说明（可选）",
          notePlaceholder: "请输入补充说明",
          questions: [
            {
              header: "推荐方案",
              question: "你更希望先怎么处理 nginx 中间件？",
              isOther: true,
              options: [
                {
                  label: "推荐：重载并观察",
                  value: "reload_observe",
                  recommended: true,
                  description: "适合配置刚更新、希望先验证是否恢复的情况。",
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
          createdAt: "2026-04-03T12:10:10Z",
          updatedAt: "2026-04-03T12:10:10Z",
        },
      ],
      runtime: {
        turn: { active: true, phase: "waiting_input", hostId: "server-local" },
        codex: { status: "connected", retryAttempt: 0, retryMax: 5 },
        activity: {
          viewedFiles: [],
          searchedWebQueries: [],
          searchedContentQueries: [],
        },
      },
      lastActivityAt: "2026-04-03T12:10:10Z",
    }),
    sessions: createProtocolFixtureSessions({
      sessions: [
        {
          id: "workspace-1",
          kind: "workspace",
          title: "Choice workspace",
          status: "running",
          messageCount: 3,
          preview: "我想知道 nginx 中间件现在应该怎么处理。",
          selectedHostId: "server-local",
          lastActivityAt: "2026-04-03T12:10:10Z",
        },
      ],
    }),
  };
}

async function routeChoiceAnswer(page, capture) {
  await page.route("**/api/v1/choices/*/answer", async (route) => {
    const request = route.request();
    let body = null;
    try {
      body = request.postDataJSON();
    } catch {
      const raw = request.postData();
      body = raw ? JSON.parse(raw) : null;
    }

    capture.url = request.url();
    capture.method = request.method();
    capture.body = body;

    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ ok: true }),
    });
  });
}

test.describe("Protocol ChoiceCard smoke", () => {
  test("shows a pending choice card with recommended-first options, descriptions, and optional note input", async ({ page }) => {
    await openFixturePage(page, "/protocol", createProtocolChoiceFixture());

    const choiceCard = page.locator(".choice-card").first();
    await expect(choiceCard).toBeVisible();

    const optionRows = choiceCard.locator(".choice-option");
    await expect(optionRows).toHaveCount(4);
    await expect(optionRows.first()).toContainText("推荐：重载并观察");
    await expect(optionRows.first()).toContainText("适合配置刚更新、希望先验证是否恢复的情况。");
    await expect(optionRows.nth(1)).toContainText("继续采集日志");
    await expect(optionRows.nth(2)).toContainText("切换到备用节点");
    await expect(choiceCard.locator(".option-description")).toHaveCount(4);

    await expect(choiceCard.getByText("其他")).toBeVisible();
    await optionRows.filter({ hasText: "其他" }).click();

    const otherInput = choiceCard.locator(".choice-input");
    await expect(otherInput).toBeVisible();
    await otherInput.fill("先补充一下日志侧上下文，再决定是否切换节点。");

    await choiceCard.getByTestId("choice-note-toggle").click();
    const noteField = choiceCard.getByTestId("choice-note-input");
    await expect(noteField.first()).toBeVisible();
    await noteField.first().fill("请优先关注最近 10 分钟的 upstream timeout。");

    await expect(page.locator("body")).not.toContainText("choice submit failed");
  });

  test("submits choice answers to the answer endpoint and keeps the page stable", async ({ page }) => {
    const capture = { url: "", method: "", body: null };
    await routeChoiceAnswer(page, capture);
    await openFixturePage(page, "/protocol", createProtocolChoiceFixture());

    const choiceCard = page.locator(".choice-card").first();
    await expect(choiceCard).toBeVisible();

    await choiceCard.locator(".choice-option").filter({ hasText: "其他" }).click();
    await choiceCard.locator(".choice-input").fill("先补充一下日志侧上下文，再决定是否切换节点。");

    await choiceCard.getByTestId("choice-note-toggle").click();
    const noteField = choiceCard.getByTestId("choice-note-input");
    await expect(noteField.first()).toBeVisible();
    await noteField.first().fill("请优先关注最近 10 分钟的 upstream timeout。");

    await choiceCard.getByRole("button", { name: /提交/ }).click();

    await expect.poll(() => capture.url).toContain("/api/v1/choices/choice-1/answer");
    await expect.poll(() => capture.method).toBe("POST");
    await expect.poll(() => capture.body?.answers?.[0]?.value).toBe(
      "先补充一下日志侧上下文，再决定是否切换节点。",
    );
    await expect.poll(() => capture.body?.answers?.[0]?.isOther).toBe(true);
    await expect.poll(() => capture.body?.answers?.[0]?.note).toBe(
      "请优先关注最近 10 分钟的 upstream timeout。",
    );
    await expect(page.locator("body")).not.toContainText("choice submit failed");

    await page.screenshot({
      path: `${SCREENSHOT_DIR}/protocol-choice-card-smoke.png`,
      fullPage: false,
    });
  });
});
