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

const markdownFinalText = [
  "本机资源总览如下：",
  "",
  "- CPU",
  "  - 型号：Apple M5",
  "  - 当前使用率：user 10.99%，sys 15.54%，idle 73.45%",
  "  - Load Average：2.88 / 2.84 / 2.92",
  "- 内存",
  "  - 总内存：32 GB",
  "  - 当前物理内存：31 GB 已用，552 MB 未用",
  "  - Compressor：约 7.3 GB",
  "- 磁盘",
  "  - 系统盘容量：460 Gi",
  "  - 使用率：49%",
  "",
  "数据为实时快照。",
].join("\n");

const runningPreludeText = "我先复查主机当前的 CPU、内存、磁盘和负载情况，再给你一个最新快照。";
const runningPreludeStartedAt = "2026-05-08T02:00:00.000Z";
const runningPreludeRenderedAt = "2026-05-08T02:00:01.000Z";

function finalMarkdownState(status) {
  const running = status === "working";
  return {
    schemaVersion: "aiops.transport.v2",
    sessionId: `markdown-final-${status}`,
    threadId: `markdown-final-${status}`,
    status: running ? "working" : "idle",
    currentTurnId: `turn-markdown-final-${status}`,
    turns: {
      [`turn-markdown-final-${status}`]: {
        id: `turn-markdown-final-${status}`,
        status,
        startedAt: "2026-05-08T02:00:00.000Z",
        completedAt: running ? undefined : "2026-05-08T02:00:12.000Z",
        updatedAt: "2026-05-08T02:00:12.000Z",
        user: {
          id: `user-markdown-final-${status}`,
          text: "看下本机的资源情况",
          createdAt: "2026-05-08T02:00:00.000Z",
        },
        process: [
          {
            id: `cmd-markdown-final-${status}`,
            kind: "command",
            status: running ? "running" : "completed",
            text: "top -l 1",
            command: "top -l 1",
            outputPreview: "CPU usage: 10.99% user, 15.54% sys, 73.45% idle",
            updatedAt: "2026-05-08T02:00:05.000Z",
          },
          {
            id: `assistant-markdown-final-${status}`,
            kind: "assistant",
            displayKind: "assistant.final",
            status: running ? "running" : "completed",
            text: markdownFinalText,
            updatedAt: "2026-05-08T02:00:12.000Z",
          },
        ],
        final: {
          id: `final-markdown-${status}`,
          text: markdownFinalText,
          status: running ? "running" : "completed",
        },
      },
    },
    turnOrder: [`turn-markdown-final-${status}`],
    pendingApprovals: {},
    mcpSurfaces: {},
    artifacts: {},
    runtimeLiveness: {
      activeTurns: running ? { [`turn-markdown-final-${status}`]: true } : {},
      activeAgents: {},
      pendingApprovals: {},
      pendingUserInputs: {},
      activeCommandStreams: {},
    },
    seq: running ? 4 : 8,
    updatedAt: "2026-05-08T02:00:12.000Z",
  };
}

function runningPreludeBeforeToolsState() {
  return {
    schemaVersion: "aiops.transport.v2",
    sessionId: "running-prelude-before-tools",
    threadId: "running-prelude-before-tools",
    status: "working",
    currentTurnId: "turn-running-prelude-before-tools",
    turns: {
      "turn-running-prelude-before-tools": {
        id: "turn-running-prelude-before-tools",
        status: "working",
        startedAt: runningPreludeStartedAt,
        updatedAt: runningPreludeStartedAt,
        user: {
          id: "user-running-prelude-before-tools",
          text: "再看下主机资源",
          createdAt: runningPreludeStartedAt,
        },
        process: [],
        final: {
          id: "final-running-prelude-before-tools",
          text: runningPreludeText,
          status: "running",
        },
      },
    },
    turnOrder: ["turn-running-prelude-before-tools"],
    pendingApprovals: {},
    mcpSurfaces: {},
    artifacts: {},
    runtimeLiveness: {
      activeTurns: { "turn-running-prelude-before-tools": true },
      activeAgents: {},
      pendingApprovals: {},
      pendingUserInputs: {},
      activeCommandStreams: {},
    },
    seq: 3,
    updatedAt: "2026-05-08T02:00:04.000Z",
  };
}

function longTerminalOutputState() {
  const outputPreview = [
    "RSS   PID COMM",
    ...Array.from({ length: 80 }, (_, index) => {
      const line = index + 1;
      return `${String(100000 + line).padStart(6, " ")} ${String(64000 + line).padStart(5, " ")} process-${line}`;
    }),
  ].join("\n");
  return {
    schemaVersion: "aiops.transport.v2",
    sessionId: "long-terminal-output",
    threadId: "long-terminal-output",
    status: "idle",
    currentTurnId: "turn-long-terminal-output",
    turns: {
      "turn-long-terminal-output": {
        id: "turn-long-terminal-output",
        status: "completed",
        startedAt: "2026-05-08T02:00:00.000Z",
        completedAt: "2026-05-08T02:00:12.000Z",
        user: {
          id: "user-long-terminal-output",
          text: "看下进程内存",
          createdAt: "2026-05-08T02:00:00.000Z",
        },
        process: [
          {
            id: "cmd-long-terminal-output",
            kind: "command",
            status: "completed",
            text: "ps -arc -o rss,pid,comm",
            command: "ps -arc -o rss,pid,comm",
            outputPreview,
            updatedAt: "2026-05-08T02:00:05.000Z",
          },
        ],
        final: {
          id: "final-long-terminal-output",
          text: "进程列表已获取。",
          status: "completed",
        },
      },
    },
    turnOrder: ["turn-long-terminal-output"],
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
}

function dataStreamForState(state) {
  return `aui-state:${JSON.stringify([{ type: "set", path: [], value: state }])}\n`;
}

async function routeShellApis(page, stateOrGetState) {
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
    const state = typeof stateOrGetState === "function" ? stateOrGetState() : stateOrGetState;
    await route.fulfill({
      status: 200,
      contentType: "text/plain; charset=utf-8",
      body: dataStreamForState(state),
    });
  });
}

test("process transcript keeps narration and expanded search details aligned", async ({ page }) => {
  await routeShellApis(page, transportState);
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

test("assistant final markdown keeps the same layout while running and after completion", async ({ page }) => {
  let resumeState = finalMarkdownState("working");
  await routeShellApis(page, () => resumeState);

  await page.goto("/");
  const runningFinal = page.getByTestId("aiops-final-text");
  await expect(runningFinal).toBeVisible();
  await expect(runningFinal).toHaveScreenshot("assistant-final-markdown-running.png");

  resumeState = finalMarkdownState("completed");
  await page.reload();
  const completedFinal = page.getByTestId("aiops-final-text");
  await expect(completedFinal).toBeVisible();
  await expect(completedFinal).toHaveScreenshot("assistant-final-markdown-completed.png");
});

test("running assistant text keeps the process header before tool blocks arrive", async ({ page }) => {
  await page.clock.setFixedTime(runningPreludeRenderedAt);
  await routeShellApis(page, runningPreludeBeforeToolsState());

  await page.goto("/");
  const transcript = page.getByTestId("aiops-process-transcript");
  await expect(page.getByTestId("aiops-process-header")).toContainText("处理中 1s");
  await expect(transcript).toContainText(runningPreludeText);
  await expect(page.getByTestId("aiops-process-transcript-body")).toHaveCount(0);
  await expect(transcript).toHaveScreenshot("assistant-running-prelude-with-process-header.png");
});

test("long terminal output stays inside a scrollable output box", async ({ page }) => {
  await routeShellApis(page, longTerminalOutputState());

  await page.goto("/");
  await page.getByTestId("aiops-process-header").click();
  await page.getByTestId("aiops-command-row-cmd-long-terminal-output").click();

  const terminalCard = page.getByTestId("aiops-terminal-card-cmd-long-terminal-output");
  const terminalOutput = page.getByTestId("aiops-command-output-cmd-long-terminal-output");
  await expect(terminalCard).toHaveClass(/max-h-72/);
  await expect(terminalCard).toHaveClass(/overflow-hidden/);
  await expect(terminalOutput).toHaveClass(/max-h-48/);
  await expect(terminalOutput).toHaveClass(/overflow-y-auto/);

  const sizes = await terminalOutput.evaluate((element) => ({
    clientHeight: element.clientHeight,
    scrollHeight: element.scrollHeight,
  }));
  expect(sizes.clientHeight).toBeGreaterThan(0);
  expect(sizes.clientHeight).toBeLessThanOrEqual(192);
  expect(sizes.scrollHeight).toBeGreaterThan(sizes.clientHeight);
});
