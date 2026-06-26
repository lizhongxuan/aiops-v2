// @ts-check
import { expect, test } from "@playwright/test";

import { createChatFixtureSessions, openFixturePage } from "./helpers/uiFixtureHarness.js";

function foldingFixture() {
  const now = "2026-06-26T10:00:00.000Z";
  const turnId = "turn-chat-runtime-folding";
  const process = [
    {
      id: "web-search-1",
      kind: "tool",
      displayKind: "web_search",
      foldGroupKind: "web_lookup",
      status: "completed",
      text: "web_search",
      inputSummary: "postgres timeline recovery",
      queries: ["postgres timeline recovery"],
      updatedAt: "2026-06-26T10:00:01.000Z",
    },
    {
      id: "web-search-2",
      kind: "tool",
      displayKind: "web_search",
      foldGroupKind: "web_lookup",
      status: "completed",
      text: "web_search",
      inputSummary: "pgBackRest recovery target timeline",
      queries: ["pgBackRest recovery target timeline"],
      updatedAt: "2026-06-26T10:00:02.000Z",
    },
    {
      id: "web-search-3",
      kind: "tool",
      displayKind: "web_search",
      foldGroupKind: "web_lookup",
      status: "completed",
      text: "web_search",
      inputSummary: "pg_auto_failover standby timeline",
      queries: ["pg_auto_failover standby timeline"],
      updatedAt: "2026-06-26T10:00:03.000Z",
    },
    {
      id: "web-open-1",
      kind: "tool",
      displayKind: "browse_url",
      foldGroupKind: "web_lookup",
      status: "completed",
      text: "browse_url",
      inputSummary: "https://www.postgresql.org/docs/current/continuous-archiving.html",
      updatedAt: "2026-06-26T10:00:04.000Z",
    },
    {
      id: "web-open-2",
      kind: "tool",
      displayKind: "browse_url",
      foldGroupKind: "web_lookup",
      status: "completed",
      text: "browse_url",
      inputSummary: "https://pgbackrest.org/user-guide.html",
      updatedAt: "2026-06-26T10:00:05.000Z",
    },
    {
      id: "web-find-1",
      kind: "tool",
      displayKind: "browser.find",
      foldGroupKind: "web_lookup",
      status: "completed",
      text: "browser.find",
      inputSummary: "recovery_target_timeline",
      updatedAt: "2026-06-26T10:00:06.000Z",
    },
    {
      id: "web-find-2",
      kind: "tool",
      displayKind: "browser.find",
      foldGroupKind: "web_lookup",
      status: "completed",
      text: "browser.find",
      inputSummary: "timeline history",
      updatedAt: "2026-06-26T10:00:07.000Z",
    },
    {
      id: "assistant-between",
      kind: "assistant",
      displayKind: "assistant.message",
      phase: "commentary",
      streamState: "complete",
      status: "completed",
      text: "网页资料已核对，继续读取本地只读状态。",
      updatedAt: "2026-06-26T10:00:08.000Z",
    },
    {
      id: "cmd-1",
      kind: "command",
      foldGroupKind: "command",
      status: "completed",
      text: "pg_controldata",
      command: "pg_controldata $PGDATA | grep Timeline",
      outputPreview: "Latest checkpoint's TimeLineID: 4",
      updatedAt: "2026-06-26T10:00:09.000Z",
    },
    {
      id: "cmd-2",
      kind: "command",
      foldGroupKind: "command",
      status: "completed",
      text: "ls history",
      command: "ls -1 $PGDATA/pg_wal/*.history",
      outputPreview: "00000004.history",
      updatedAt: "2026-06-26T10:00:10.000Z",
    },
    {
      id: "mcp-1",
      kind: "mcp",
      displayKind: "read_mcp_resource",
      status: "completed",
      text: "read_mcp_resource ops://postgres/runbook",
      outputPreview: "MCP runbook metadata only",
      updatedAt: "2026-06-26T10:00:11.000Z",
    },
    {
      id: "approval-1",
      kind: "approval",
      displayKind: "approval.command",
      status: "completed",
      text: "等待审批：restart postgres",
      command: "systemctl restart postgresql",
      approvalId: "approval-runtime-folding",
      updatedAt: "2026-06-26T10:00:12.000Z",
    },
  ];

  const state = {
    schemaVersion: "aiops.transport.v2",
    sessionId: "chat-runtime-folding",
    threadId: "chat-runtime-folding",
    status: "idle",
    currentTurnId: turnId,
    turns: {
      [turnId]: {
        id: turnId,
        status: "completed",
        startedAt: now,
        completedAt: "2026-06-26T10:00:13.000Z",
        updatedAt: "2026-06-26T10:00:13.000Z",
        user: {
          id: "user-chat-runtime-folding",
          text: "分析 PG timeline 异常，必要时申请重启审批。",
          createdAt: now,
        },
        process,
        final: {
          id: "final-chat-runtime-folding",
          text: "结论：当前证据显示从节点 timeline 与主节点不一致；重启前需要审批。",
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
    seq: 13,
    updatedAt: "2026-06-26T10:00:13.000Z",
  };

  return {
    name: "chat-runtime-folding",
    state,
    sessions: createChatFixtureSessions({
      activeSessionId: "chat-runtime-folding",
      sessions: [{
        id: "chat-runtime-folding",
        kind: "single_host",
        title: "Chat runtime folding",
        status: "completed",
        messageCount: 1,
        preview: "分析 PG timeline 异常",
        selectedHostId: "server-local",
        lastActivityAt: "2026-06-26T10:00:13.000Z",
      }],
    }),
  };
}

test("folds only same-class web lookups and commands while preserving MCP and approval blocks", async ({ page }) => {
  await openFixturePage(page, "/", foldingFixture());
  await page.getByTestId("aiops-process-header").click();

  const transcript = page.getByTestId("aiops-process-transcript-body");
  await expect(transcript).toContainText("网页检索 7 次");
  await expect(transcript).toContainText("已运行 2 条命令");
  await expect(transcript).toContainText("read_mcp_resource ops://postgres/runbook");
  await expect(transcript).not.toContainText("等待审批：restart postgres");
  await expect(page.getByText("当前证据显示从节点 timeline 与主节点不一致")).toBeVisible();

  await expect(page.getByTestId("aiops-merged-command-icon")).toBeVisible();
  await expect(page.getByTestId("aiops-search-toggle")).toBeVisible();
  await expect(page.getByTestId("aiops-tool-row-mcp-1")).toBeVisible();
  await expect(page.getByText("已调用 1 个工具")).toHaveCount(0);
});
