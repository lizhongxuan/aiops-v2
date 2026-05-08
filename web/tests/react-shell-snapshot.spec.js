import { expect, test } from "@playwright/test";

const transportState = {
  schemaVersion: "aiops.transport.v2",
  sessionId: "browser-flow-session",
  threadId: "browser-flow-session",
  status: "idle",
  currentTurnId: "turn-browser-flow",
  turns: {
    "turn-browser-flow": {
      id: "turn-browser-flow",
      status: "completed",
      startedAt: "2026-05-08T02:00:00.000Z",
      completedAt: "2026-05-08T02:00:12.000Z",
      user: {
        id: "user-browser-flow",
        text: "测试一下 aiops-v2 对话时，工具跟对话的顺序是否正确。",
        createdAt: "2026-05-08T02:00:00.000Z",
      },
      process: [
        {
          id: "assistant-next",
          kind: "assistant",
          status: "completed",
          text: "接下来我要检查运行环境和最近任务状态。",
          updatedAt: "2026-05-08T02:00:01.000Z",
        },
        {
          id: "cmd-order-1",
          kind: "command",
          status: "completed",
          text: "pwd",
          command: "pwd",
          outputPreview: "/Users/lizhongxuan/Desktop/aiops-v2",
          updatedAt: "2026-05-08T02:00:03.000Z",
        },
        {
          id: "cmd-order-2",
          kind: "command",
          status: "completed",
          text: "git status --short",
          command: "git status --short",
          outputPreview: "",
          updatedAt: "2026-05-08T02:00:04.000Z",
        },
        {
          id: "assistant-after-commands",
          kind: "assistant",
          status: "completed",
          text: "命令结果已经拿到，我会继续核对相关页面信息。",
          updatedAt: "2026-05-08T02:00:05.000Z",
        },
        {
          id: "search-order-1",
          kind: "tool",
          displayKind: "web_search",
          status: "completed",
          text: "web_search",
          inputSummary: "aiops-v2 AssistantTransport 顺序",
          queries: ["aiops-v2 AssistantTransport 顺序"],
          updatedAt: "2026-05-08T02:00:07.000Z",
        },
        {
          id: "search-order-2",
          kind: "tool",
          displayKind: "browse_url",
          status: "completed",
          text: "browse_url",
          inputSummary: "https://example.com/aiops-v2-order",
          updatedAt: "2026-05-08T02:00:08.000Z",
        },
        {
          id: "assistant-after-search",
          kind: "assistant",
          status: "completed",
          text: "页面也确认过了，最终回答会基于上面的命令和搜索结果。",
          updatedAt: "2026-05-08T02:00:10.000Z",
        },
      ],
      final: {
        id: "final-browser-flow",
        text: "流程验证完成：对话、命令组、对话、搜索组和后续对话按预期顺序显示。",
        status: "completed",
      },
    },
  },
  turnOrder: ["turn-browser-flow"],
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
  seq: 8,
  updatedAt: "2026-05-08T02:00:12.000Z",
};

const sessionsPayload = {
  activeSessionId: "browser-flow-session",
  sessions: [
    {
      id: "browser-flow-session",
      kind: "single_host",
      title: "Browser flow",
      status: "completed",
      messageCount: 1,
      preview: "测试一下 aiops-v2 对话时，工具跟对话的顺序是否正确。",
      selectedHostId: "server-local",
      lastActivityAt: "2026-05-08T02:00:12.000Z",
    },
  ],
};

function dataStreamForState(state) {
  return `aui-state:${JSON.stringify([{ type: "set", path: [], value: state }])}\n`;
}

test("process transcript keeps narration and expanded search details aligned", async ({ page }) => {
  await page.route("**/api/v1/sessions", async (route) => {
    await route.fulfill({ json: sessionsPayload });
  });
  await page.route("**/api/v1/hosts", async (route) => {
    await route.fulfill({
      json: {
        items: [
          {
            id: "server-local",
            name: "server-local",
            status: "online",
            executable: true,
            terminalCapable: true,
          },
        ],
      },
    });
  });
  await page.route("**/api/v1/llm-config", async (route) => {
    await route.fulfill({
      json: {
        provider: "mock",
        model: "browser-flow",
        apiKeySet: true,
        bifrostActive: true,
      },
    });
  });
  await page.route("**/api/v1/assistant/resume", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "text/plain; charset=utf-8",
      body: dataStreamForState(transportState),
    });
  });

  await page.goto("/");
  await expect(page.getByTestId("aiops-process-header")).toBeVisible();
  await page.getByTestId("aiops-process-header").click();
  await page.getByTestId("aiops-search-toggle").click();

  const transcript = page.getByTestId("aiops-process-transcript-body");
  await expect(transcript).toContainText("接下来我要检查运行环境和最近任务状态。");
  await expect(transcript).toContainText("网页检索 2 项");
  await expect(transcript).toContainText("https://example.com/aiops-v2-order");
  await expect(transcript).toHaveScreenshot("process-transcript-order-alignment.png");
});
