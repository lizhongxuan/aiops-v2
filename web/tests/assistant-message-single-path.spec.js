// @ts-check
import { expect, test } from "@playwright/test";

import { createChatFixtureSessions, openFixturePage } from "./helpers/uiFixtureHarness.js";

function singleAssistantMessageFixture() {
  const now = "2026-06-26T11:00:00.000Z";
  const turnId = "turn-assistant-message-single-path";
  const finalText = "结论：当前证据只能确认 PostgreSQL timeline 存在分叉；还需要核对 .history、restore_command 和 WAL receiver 状态后再决定修复动作。";
  const state = {
    schemaVersion: "aiops.transport.v2",
    sessionId: "assistant-message-single-path",
    threadId: "assistant-message-single-path",
    status: "idle",
    currentTurnId: turnId,
    turns: {
      [turnId]: {
        id: turnId,
        status: "completed",
        startedAt: now,
        completedAt: "2026-06-26T11:00:06.000Z",
        updatedAt: "2026-06-26T11:00:06.000Z",
        user: {
          id: "user-assistant-message-single-path",
          text: "分析 PostgreSQL timeline 分叉原因。",
          createdAt: now,
        },
        process: [
          {
            id: "assistant-commentary-1",
            kind: "assistant",
            displayKind: "assistant.message",
            phase: "commentary",
            streamState: "complete",
            status: "completed",
            text: "我先核对公开文档和当前证据边界。",
            updatedAt: "2026-06-26T11:00:01.000Z",
          },
          {
            id: "web-search-1",
            kind: "tool",
            displayKind: "web_search",
            foldGroupKind: "web_lookup",
            status: "completed",
            text: "web_search",
            inputSummary: "PostgreSQL timeline history recovery_target_timeline",
            queries: ["PostgreSQL timeline history recovery_target_timeline"],
            updatedAt: "2026-06-26T11:00:02.000Z",
          },
          {
            id: "assistant-commentary-2",
            kind: "assistant",
            displayKind: "assistant.message",
            phase: "commentary",
            streamState: "complete",
            status: "completed",
            text: "我会把完整 RCA 放到最终回答，过程区只保留短进展。",
            updatedAt: "2026-06-26T11:00:03.000Z",
          },
        ],
        final: {
          id: "final-assistant-message-single-path",
          text: finalText,
          status: "completed",
        },
      },
    },
    turnOrder: [turnId],
    pendingApprovals: {},
    mcpSurfaces: {},
    artifacts: {},
    runtimeLiveness: {
      activeTurns: {},
      activeAgents: {},
      pendingApprovals: {},
      pendingUserInputs: {},
      activeCommandStreams: {},
    },
    seq: 6,
    updatedAt: "2026-06-26T11:00:06.000Z",
  };

  return {
    name: "assistant-message-single-path",
    state,
    sessions: createChatFixtureSessions({
      activeSessionId: "assistant-message-single-path",
      sessions: [{
        id: "assistant-message-single-path",
        kind: "single_host",
        title: "Assistant message single path",
        status: "completed",
        messageCount: 1,
        preview: "分析 PostgreSQL timeline 分叉原因",
        lastActivityAt: "2026-06-26T11:00:06.000Z",
      }],
    }),
    finalText,
  };
}

test("renders commentary in process and final answer only in final area", async ({ page }) => {
  const fixture = singleAssistantMessageFixture();
  await openFixturePage(page, "/", fixture);

  await expect(page.getByTestId("aiops-final-text")).toContainText("当前证据只能确认 PostgreSQL timeline 存在分叉");
  await expect(page.getByText("旧候选答案")).toHaveCount(0);
  await expect(page.getByText("置信度：low")).toHaveCount(0);

  await page.getByTestId("aiops-process-header").click();
  const transcript = page.getByTestId("aiops-process-transcript-body");
  await expect(transcript).toContainText("我先核对公开文档和当前证据边界");
  await expect(transcript).toContainText("网页检索 1 次");
  await expect(transcript).toContainText("过程区只保留短进展");
  await expect(transcript).not.toContainText(fixture.finalText);
});
