// @ts-check
import { test, expect } from "@playwright/test";
import {
  createProtocolFixtureSessions,
  createProtocolFixtureState,
  openFixturePage,
} from "./helpers/uiFixtureHarness";

function createUxFixtureState({ includeApproval = false } = {}) {
  const cards = [
    {
      id: "user-ux-1",
      type: "UserMessageCard",
      role: "user",
      text: "今天这个路由和审批结果，能给我一个稳定的工作台视图吗？",
      createdAt: "2026-04-03T13:00:00Z",
      updatedAt: "2026-04-03T13:00:00Z",
    },
    {
      id: "assistant-ux-routing-1",
      type: "AssistantMessageCard",
      role: "assistant",
      text: "主 Agent 正在判断最优路径，不会生成计划或派发 worker。",
      createdAt: "2026-04-03T13:00:05Z",
      updatedAt: "2026-04-03T13:00:05Z",
    },
    {
      id: "assistant-ux-routing-2",
      type: "AssistantMessageCard",
      role: "assistant",
      text: "这是正常回答。\n```json\n{\"route\":\"direct_answer\",\"needsPlan\":false}\n```",
      createdAt: "2026-04-03T13:00:10Z",
      updatedAt: "2026-04-03T13:00:10Z",
    },
    {
      id: "assistant-ux-routing-3",
      type: "AssistantMessageCard",
      role: "assistant",
      text: "下面是路由说明 {\"route\":\"direct_answer\",\"needsPlan\":false}",
      createdAt: "2026-04-03T13:00:15Z",
      updatedAt: "2026-04-03T13:00:15Z",
    },
  ];

  if (includeApproval) {
    cards.push({
      id: "approval-expired-card",
      type: "CommandApprovalCard",
      status: "pending",
      hostId: "web-02",
      text: "需要批准 web-02 reload nginx",
      command: "systemctl reload nginx",
      approval: {
        requestId: "approval-expired-1",
        decisions: ["accept", "decline"],
      },
      createdAt: "2026-04-03T13:00:20Z",
      updatedAt: "2026-04-03T13:00:20Z",
    });
  }

  return createProtocolFixtureState({
    approvals: includeApproval ? [{ id: "approval-expired-1", status: "pending", itemId: "approval-expired-card" }] : [],
    cards,
    runtime: {
      turn: {
        active: !includeApproval,
        phase: includeApproval ? "waiting_approval" : "completed",
        hostId: "server-local",
      },
      codex: { status: "connected", retryAttempt: 0, retryMax: 5 },
      activity: {},
    },
    lastActivityAt: "2026-04-03T13:00:20Z",
  });
}

async function openProtocolUxFixture(page, { includeApproval = false } = {}) {
  await openFixturePage(page, "/protocol", {
    state: createUxFixtureState({ includeApproval }),
    sessions: createProtocolFixtureSessions({
      sessions: [
        {
          id: "workspace-1",
          kind: "workspace",
          title: "Protocol workspace",
          status: includeApproval ? "running" : "completed",
          messageCount: 4,
          preview: "协议工作台 smoke fixture",
          selectedHostId: "server-local",
          lastActivityAt: "2026-04-03T13:00:20Z",
        },
      ],
    }),
  });
}

if (process.env.VITEST) {
  const { describe: vitestDescribe, it: vitestIt } = await import("vitest");
  vitestDescribe("Protocol UX fixes", () => {
    vitestIt("is covered by Playwright smoke tests", () => {});
  });
} else {
  test.describe("Protocol UX fixes", () => {
    test.setTimeout(30000);

    test("expired approval shows a friendly toolbar message instead of raw backend text", async ({ page }) => {
      await openProtocolUxFixture(page, { includeApproval: true });
      await page.route("**/api/v1/approvals/approval-expired-1/decision", (route) =>
        route.fulfill({
          status: 404,
          contentType: "application/json",
          body: JSON.stringify({ error: "approval not found" }),
        }),
      );

      const approvalCard = page.getByTestId("protocol-approval-approval-expired-card");
      await expect(approvalCard).toBeVisible();
      await approvalCard.getByRole("button", { name: "同意执行" }).click();

      const toolbar = page.locator(".toolbar-notice");
      await expect(toolbar).toBeVisible();
      await expect(toolbar).toContainText("已过期");
      await expect(toolbar).not.toContainText("approval not found");
      await expect(approvalCard).toHaveCount(0);

      await page.screenshot({
        path: "tests/screenshots/protocol-stale-approval-friendly.png",
        fullPage: false,
      });
    });

    test("system routing messages stay out of the visible protocol正文", async ({ page }) => {
      await openProtocolUxFixture(page);

      const stream = page.locator(".protocol-turn-stream");
      const bodies = page.locator(".protocol-turn-stream .row-assistant .message-text");

      await expect(stream).toBeVisible();
      await expect(stream).not.toContainText("主 Agent 正在判断最优路径");
      await expect(stream).not.toContainText("不会生成计划或派发 worker");

      const texts = await bodies.allTextContents();
      expect(texts.join("\n")).toContain("下面是路由说明");
      for (const text of texts) {
        expect(text).not.toContain("主 Agent 正在判断最优路径");
        expect(text).not.toContain("不会生成计划或派发 worker");
      }

      await page.screenshot({
        path: "tests/screenshots/protocol-clean-messages.png",
        fullPage: false,
      });
    });

    test("raw JSON routing blocks are stripped before rendering", async ({ page }) => {
      await openProtocolUxFixture(page);

      const bodies = page.locator(".protocol-turn-stream .row-assistant .message-text");

      const texts = await bodies.allTextContents();
      expect(texts.join("\n")).toContain("下面是路由说明");
      for (const text of texts) {
        expect(text).not.toContain("\"route\"");
        expect(text).not.toContain("needsPlan");
        expect(text).not.toContain("```json");
      }

      await page.screenshot({
        path: "tests/screenshots/protocol-no-raw-json.png",
        fullPage: false,
      });
    });
  });
}
