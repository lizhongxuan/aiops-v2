// @ts-check
import { expect, test } from "@playwright/test";

import { createChatFixtureSessions, openFixturePage } from "./helpers/uiFixtureHarness.js";

function singleAssistantMessageFixture() {
  const now = "2026-06-27T11:00:00.000Z";
  const turnId = "turn-assistant-message-single-path";
  const finalText = "CPU 当前负载正常，未发现持续高负载。";
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
        completedAt: "2026-06-27T11:00:06.000Z",
        updatedAt: "2026-06-27T11:00:06.000Z",
        user: {
          id: "user-assistant-message-single-path",
          text: "@server-local 查看cpu情况",
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
            text: "我会先检索可用工具并确认适合的只读检查能力，再继续获取证据。",
            commentarySource: "runtime_tool_intent",
            toolCallIds: ["call-search-tools"],
            updatedAt: "2026-06-27T11:00:01.000Z",
          },
          {
            id: "tool-search-1",
            kind: "tool",
            displayKind: "tool_search",
            foldGroupKind: "web_lookup",
            status: "completed",
            text: "tool_search",
            inputSummary: "host CPU monitoring status check server local",
            queries: ["host CPU monitoring status check server local"],
            updatedAt: "2026-06-27T11:00:02.000Z",
          },
          {
            id: "assistant-commentary-2",
            kind: "assistant",
            displayKind: "assistant.message",
            phase: "commentary",
            streamState: "complete",
            status: "completed",
            text: "我会先执行只读命令获取证据，再根据输出给出结论。",
            commentarySource: "runtime_tool_intent",
            toolCallIds: ["call-cpu"],
            updatedAt: "2026-06-27T11:00:03.000Z",
          },
          {
            id: "cmd-cpu",
            kind: "command",
            foldGroupKind: "command",
            status: "completed",
            text: "top -l 1 | head",
            command: "top -l 1 | head",
            outputPreview: "CPU usage: 10% user, 15% sys, 75% idle",
            updatedAt: "2026-06-27T11:00:04.000Z",
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
        preview: "@server-local 查看cpu情况",
        lastActivityAt: "2026-06-27T11:00:06.000Z",
      }],
    }),
    finalText,
  };
}

test("renders commentary in process and final answer only in final area", async ({ page }) => {
  const fixture = singleAssistantMessageFixture();
  await openFixturePage(page, "/", fixture);

  await expect(page.getByTestId("aiops-final-text")).toContainText(fixture.finalText);
  await expect(page.getByText("旧候选答案")).toHaveCount(0);
  await expect(page.getByText("置信度：low")).toHaveCount(0);

  await page.getByTestId("aiops-process-header").click();
  const transcript = page.getByTestId("aiops-process-transcript-body");
  await expect(transcript).toContainText("检索可用工具");
  await expect(transcript).toContainText("执行只读命令");
  await expect(transcript).toContainText("已运行 top -l 1 | head");
  await expect(transcript).not.toContainText(fixture.finalText);
});
