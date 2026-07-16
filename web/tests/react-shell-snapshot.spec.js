import { expect, test } from "@playwright/test";

import {
  createChatFixtureSessions,
  createChatFixtureState,
  openFixturePage,
} from "./helpers/uiFixtureHarness.js";

function canonicalTranscript(process, final, artifacts = []) {
  const blocks = process.map((block) => ({
    ...block,
    type: block.kind === "assistant" ? "commentary" : block.kind,
  }));
  if (final) {
    const processStatus = final.status === "running"
      ? "running"
      : ["blocked", "needs_evidence", "approval_denied", "tool_unavailable"].includes(final.status)
        ? "blocked"
        : ["failed", "cancelled"].includes(final.status)
          ? "failed"
          : "completed";
    blocks.push({
      id: final.id,
      type: "final_answer",
      kind: "assistant",
      displayKind: "assistant.message",
      phase: "final_answer",
      streamState: final.status === "running" ? "streaming" : processStatus === "completed" ? "complete" : "incomplete",
      status: processStatus,
      text: final.text,
      finalContract: final,
    });
  }
  for (const artifact of artifacts) {
    blocks.push({
      id: artifact.id,
      type: "artifact",
      kind: "tool",
      status: artifact.status === "failed" ? "failed" : "completed",
      text: artifact.summaryZh || artifact.summary || artifact.titleZh || artifact.title || "",
      artifact,
    });
  }
  return {
    blockOrder: blocks.map((block) => block.id),
    blocksById: Object.fromEntries(blocks.map((block) => [block.id, block])),
  };
}

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
      ...canonicalTranscript([
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
      ], {
        id: "final-browser-flow",
        text: "流程验证完成：对话、命令组、对话、搜索组和后续对话按预期顺序显示。",
        status: "completed",
      }),
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

function finalContractSummaryState() {
  return {
    schemaVersion: "aiops.transport.v2",
    sessionId: "final-contract-summary-session",
    threadId: "final-contract-summary-session",
    status: "idle",
    currentTurnId: "turn-final-contract-summary",
    turns: {
      "turn-final-contract-summary": {
        id: "turn-final-contract-summary",
        status: "completed",
        startedAt: "2026-05-08T02:00:00.000Z",
        completedAt: "2026-05-08T02:00:12.000Z",
        updatedAt: "2026-05-08T02:00:12.000Z",
        user: {
          id: "user-final-contract-summary",
          text: "只读看一下主机进程和负载，不要修改任何东西",
          createdAt: "2026-05-08T02:00:00.000Z",
        },
        process: [],
        final: {
          id: "final-contract-summary",
          text: "以下是只读巡检结果：系统负载稳定，未执行任何修改。",
          status: "verified",
          schemaVersion: "aiops.harness.final.v1",
          confidence: "high",
          checkedEvidenceRefs: ["call_secret_1", "call_secret_2"],
        },
      },
    },
    turnOrder: ["turn-final-contract-summary"],
    pendingApprovals: {},
    mcpSurfaces: {},
    artifacts: {},
    hostMissions: {},
    childAgents: {},
    runtimeLiveness: {
      activeTurns: {},
      activeAgents: {},
      pendingApprovals: {},
      pendingUserInputs: {},
      activeCommandStreams: {},
    },
    seq: 2,
    updatedAt: "2026-05-08T02:00:12.000Z",
  };
}

function finalContractInternalCalibrationState() {
  return {
    schemaVersion: "aiops.transport.v2",
    sessionId: "final-contract-internal-calibration-session",
    threadId: "final-contract-internal-calibration-session",
    status: "idle",
    currentTurnId: "turn-final-contract-internal-calibration",
    turns: {
      "turn-final-contract-internal-calibration": {
        id: "turn-final-contract-internal-calibration",
        status: "completed",
        startedAt: "2026-05-08T02:00:00.000Z",
        completedAt: "2026-05-08T02:00:12.000Z",
        updatedAt: "2026-05-08T02:00:12.000Z",
        user: {
          id: "user-final-contract-internal-calibration",
          text: "你好",
          createdAt: "2026-05-08T02:00:00.000Z",
        },
        process: [],
        final: {
          id: "final-contract-internal-calibration",
          text: "你好！有什么可以帮你的吗？",
          status: "unknown",
          schemaVersion: "aiops.harness.final.v1",
          confidence: "low",
        },
      },
    },
    turnOrder: ["turn-final-contract-internal-calibration"],
    pendingApprovals: {},
    mcpSurfaces: {},
    artifacts: {},
    hostMissions: {},
    childAgents: {},
    runtimeLiveness: {
      activeTurns: {},
      activeAgents: {},
      pendingApprovals: {},
      pendingUserInputs: {},
      activeCommandStreams: {},
    },
    seq: 2,
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
        ...canonicalTranscript([], {
          id: "final-running-prelude-before-tools",
          text: runningPreludeText,
          status: "running",
        }),
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

function codexLikeProcessTranscriptState() {
  const now = "2026-06-27T10:00:00.000Z";
  const turnId = "turn-codex-like-process";
  return {
    schemaVersion: "aiops.transport.v2",
    sessionId: "browser-flow-session",
    threadId: "browser-flow-session",
    status: "idle",
    currentTurnId: turnId,
    turns: {
      [turnId]: {
        id: turnId,
        status: "completed",
        startedAt: now,
        completedAt: "2026-06-27T10:00:08.000Z",
        updatedAt: "2026-06-27T10:00:08.000Z",
        user: {
          id: "user-codex-like-process",
          text: "@server-local 查看cpu情况",
          createdAt: now,
        },
        process: [
          {
            id: "assistant-tool-search",
            kind: "assistant",
            displayKind: "assistant.message",
            phase: "commentary",
            streamState: "complete",
            status: "completed",
            text: "我会先检索可用工具并确认适合的只读检查能力，再继续获取证据。",
            commentarySource: "runtime_tool_intent",
            toolCallIds: ["call-search-tools"],
            updatedAt: "2026-06-27T10:00:01.000Z",
          },
          {
            id: "tool-search-tools",
            kind: "tool",
            displayKind: "tool_search",
            foldGroupKind: "web_lookup",
            status: "completed",
            text: "tool_search",
            inputSummary: "host CPU monitoring status check server local",
            queries: ["host CPU monitoring status check server local"],
            updatedAt: "2026-06-27T10:00:02.000Z",
          },
          {
            id: "assistant-exec-command",
            kind: "assistant",
            displayKind: "assistant.message",
            phase: "commentary",
            streamState: "complete",
            status: "completed",
            text: "我会先执行只读命令获取证据，再根据输出给出结论。",
            commentarySource: "runtime_tool_intent",
            toolCallIds: ["call-cpu"],
            updatedAt: "2026-06-27T10:00:03.000Z",
          },
          {
            id: "cmd-cpu",
            kind: "command",
            foldGroupKind: "command",
            status: "completed",
            text: "top -l 1 | head",
            command: "top -l 1 | head",
            outputPreview: "CPU usage: 10.99% user, 15.54% sys, 73.45% idle",
            updatedAt: "2026-06-27T10:00:04.000Z",
          },
        ],
        final: {
          id: "final-codex-like-process",
          text: "CPU 当前空闲约 73%，没有看到持续高负载；建议继续观察 load average 和异常进程。",
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
    seq: 10,
    updatedAt: "2026-06-27T10:00:08.000Z",
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
        ...canonicalTranscript([
          {
            id: "cmd-long-terminal-output",
            kind: "command",
            status: "completed",
            text: "ps -arc -o rss,pid,comm",
            command: "ps -arc -o rss,pid,comm",
            outputPreview,
            updatedAt: "2026-05-08T02:00:05.000Z",
          },
        ], {
          id: "final-long-terminal-output",
          text: "进程列表已获取。",
          status: "completed",
        }),
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

function contextCompactionTransportState() {
  return {
    schemaVersion: "aiops.transport.v2",
    sessionId: "context-compaction-session",
    threadId: "context-compaction-session",
    status: "working",
    currentTurnId: "turn-context-compaction",
    turns: {
      "turn-context-compaction": {
        id: "turn-context-compaction",
        status: "working",
        startedAt: "2026-05-22T08:00:00.000Z",
        updatedAt: "2026-05-22T08:00:04.000Z",
        user: {
          id: "user-context-compaction",
          text: "继续排查 nginx 超时，并保留关键摘要。",
          createdAt: "2026-05-22T08:00:00.000Z",
        },
        contextGovernance: [
          {
            id: "ctxgov-context-l4",
            layer: "L4",
            kind: "context.compaction.started",
            message: "正在压缩上下文，当前任务会继续",
            referenceIds: ["spill-1"],
            createdAt: "2026-05-22T08:00:01.000Z",
          },
          {
            id: "ctxgov-context-l5",
            layer: "L5",
            kind: "context.compaction.failed",
            message: "上下文过长，已使用本地摘要继续",
            createdAt: "2026-05-22T08:00:02.000Z",
          },
        ],
        ...canonicalTranscript([
          {
            id: "tool-context-spill",
            kind: "tool",
            displayKind: "logs_query",
            status: "completed",
            text: "logs_query nginx timeout",
            outputPreview: "Large nginx log result was externalized. Summary: 17 upstream timeout lines.",
            evidenceRefs: ["spill-1"],
            materializationTier: "large",
            originalBytes: 48213,
            inlineBytes: 920,
            externalReferences: [
              {
                id: "spill-1",
                kind: "blob",
                title: "nginx raw timeout logs",
                summary: "17 upstream timeout lines from nginx in the last 10 minutes.",
                contentType: "text/plain",
                bytes: 48213,
              },
            ],
            updatedAt: "2026-05-22T08:00:03.000Z",
          },
        ], {
          id: "final-context-compaction",
          text: "我正在整理旧上下文，关键摘要会保留在当前对话里。",
          status: "running",
        }),
      },
    },
    turnOrder: ["turn-context-compaction"],
    pendingApprovals: {},
    mcpSurfaces: {},
    artifacts: {},
    runtimeLiveness: {
      activeTurns: { "turn-context-compaction": true },
      activeAgents: {},
      pendingApprovals: {},
      pendingUserInputs: {},
      activeCommandStreams: {},
    },
    seq: 5,
    updatedAt: "2026-05-22T08:00:04.000Z",
  };
}

const rcaReportTransportState = {
  schemaVersion: "aiops.transport.v2",
  sessionId: "rca-report-session",
  threadId: "rca-report-session",
  status: "idle",
  currentTurnId: "turn-rca",
  turns: {
    "turn-rca": {
      id: "turn-rca",
      status: "completed",
      startedAt: "2026-05-15T02:00:00.000Z",
      completedAt: "2026-05-15T02:00:12.000Z",
      user: {
        id: "user-rca",
        text: "分析 checkout 服务最近 30 分钟延迟升高的根因",
        createdAt: "2026-05-15T02:00:00.000Z",
      },
      process: [
        {
          id: "tool-coroot-context",
          kind: "tool",
          displayKind: "coroot.collect_rca_context",
          status: "completed",
          text: "coroot.collect_rca_context",
          updatedAt: "2026-05-15T02:00:04.000Z",
        },
        {
          id: "tool-artifact",
          kind: "tool",
          displayKind: "rca_report",
          status: "completed",
          text: "aiops.ui_artifact_emit",
          updatedAt: "2026-05-15T02:00:08.000Z",
        },
      ],
      agentUiArtifacts: [
        {
          id: "artifact-rca-report",
          type: "rca_report",
          titleZh: "checkout 根因分析",
          summaryZh: "checkout 延迟升高最可能来自 catalog 依赖。",
          status: "ok",
          severity: "high",
          source: "coroot",
          permissionScope: "read",
          redactionStatus: "redacted",
          inlineData: {
            schemaVersion: "aiops.rca_report/v1",
            source: "coroot",
            status: "ok",
            target: { service: "checkout" },
            window: { timeRange: "30m" },
            conclusion: {
              summaryZh: "checkout 延迟升高最可能来自 catalog 依赖。",
              rootCauseEntity: "catalog",
              confidence: 0.72,
            },
            hypotheses: [
              {
                id: "hyp-1",
                titleZh: "catalog 依赖延迟",
                confidence: 0.72,
                supportingEvidenceRefs: ["ev-coroot-latency"],
                contradictingEvidenceRefs: [],
                missingEvidence: [],
              },
            ],
            sections: [
              {
                id: "propagation",
                kind: "propagation_map",
                titleZh: "传播路径",
                evidenceRefs: ["ev-coroot-latency"],
                payload: {
                  nodes: [{ id: "checkout" }, { id: "catalog" }],
                  edges: [{ source: "checkout", target: "catalog" }],
                },
              },
              {
                id: "metrics",
                kind: "timeseries_grid",
                titleZh: "关键指标",
                evidenceRefs: ["ev-coroot-latency"],
                payload: {
                  metrics: [
                    {
                      name: "latency_p99",
                      entity: "checkout->catalog",
                      valueSummary: "p99 rose to 1.8s",
                      status: "critical",
                    },
                  ],
                },
              },
            ],
            evidenceRefs: ["ev-coroot-latency"],
            rawRefs: [{ source: "coroot", uri: "coroot://project/default/checkout" }],
            limitations: [],
          },
        },
      ],
      final: {
        id: "final-rca",
        text: "RCA 初步完成，最强假设是 catalog 依赖延迟传播到 checkout。",
        status: "completed",
      },
    },
  },
  turnOrder: ["turn-rca"],
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
  seq: 1,
  updatedAt: "2026-05-15T02:00:12.000Z",
};

function artifactTransportState(sessionId, userText, artifact) {
  const artifacts = Array.isArray(artifact) ? artifact : [artifact];
  const turnId = `turn-${sessionId}`;
  return {
    schemaVersion: "aiops.transport.v2",
    sessionId,
    threadId: sessionId,
    status: "idle",
    currentTurnId: turnId,
    turns: {
      [turnId]: {
        id: turnId,
        status: "completed",
        startedAt: "2026-05-19T10:00:00.000Z",
        completedAt: "2026-05-19T10:00:08.000Z",
        updatedAt: "2026-05-19T10:00:08.000Z",
        user: {
          id: `user-${sessionId}`,
          text: userText,
          createdAt: "2026-05-19T10:00:00.000Z",
        },
        agentUiArtifacts: artifacts,
        ...canonicalTranscript([], {
          id: `final-${sessionId}`,
          text: "已完成运维手册判定。",
          status: "completed",
        }, artifacts),
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
    seq: 1,
    updatedAt: "2026-05-19T10:00:08.000Z",
  };
}

function opsManualMergedParamConfirmationState() {
  return artifactTransportState(
    "ops-manual-merged-param-confirmation",
    "按运维手册判断：对 MySQL 做备份",
    [
      {
        id: "artifact-ops-manual-mysql-search",
        type: "ops_manual_search_result",
        titleZh: "运维手册检索",
        source: "tool:search_ops_manuals",
        redactionStatus: "redacted",
        inlineData: {
          decision: "need_info",
          summary: "信息不足，不能直接使用工作流。",
          ops_manual_flow_id: "flow-mysql-search",
          operation_frame: {
            target: { type: "mysql" },
            operation: { action: "backup" },
            target_scope: { hosts: ["server-local"] },
          },
          manuals: [
            {
              manual: {
                id: "manual-mysql-backup-ssh",
                title: "MySQL SSH 备份运维手册",
              },
              bound_workflow_id: "workflow-mysql-backup-ssh",
              usable_mode: "need_info",
              matched_fields: ["object_type", "operation_type"],
              workflow_preview: {
                title: "MySQL SSH 备份工作流",
                nodes: [
                  { id: "collect", title: "检查实例", command: "docker ps --filter name=mysql" },
                  { id: "backup", title: "生成备份", command: "mysqldump --single-transaction" },
                ],
              },
            },
          ],
        },
      },
      {
        id: "artifact-ops-manual-mysql-params",
        type: "ops_manual_param_resolution",
        titleZh: "运维手册参数解析",
        source: "tool:resolve_ops_manual_params",
        redactionStatus: "redacted",
        inlineData: {
          status: "need_user_input",
          ops_manual_flow_id: "flow-mysql-search",
          manual_id: "manual-mysql-backup-ssh",
          workflow_id: "flow-mysql-search",
          resolved_params: [
            { id: "target_host", value: "server-local", source: "user_form", evidence: "context fact: target_host" },
            { id: "target_instance", value: "docker:aiops-mysql", source: "docker", evidence: "docker ps: image=mysql:8.0 ports=33306 status=Up" },
          ],
          fields: [
            {
              id: "backup_path",
              label: "备份路径",
              type: "path",
              ui_control: "text",
              required: true,
              placeholder: "例如 /data/backups",
            },
          ],
        },
      },
    ],
  );
}

function opsManualDirectActionsState() {
  return artifactTransportState(
    "ops-manual-direct-actions",
    "排查 redis-01 内存上涨",
    {
      id: "artifact-ops-manual-direct-actions",
      type: "ops_manual_search_result",
      titleZh: "运维手册检索",
      source: "tool:search_ops_manuals",
      redactionStatus: "redacted",
      inlineData: {
        decision: "direct_execute",
        summary: "找到可进入预检的 Redis 运维手册。",
        ops_manual_flow_id: "flow-redis-direct-actions",
        operation_frame: {
          target: { type: "redis", name: "redis-01" },
          operation: { action: "rca_or_repair" },
          target_scope: { hosts: ["redis-01"] },
        },
        manuals: [
          {
            manual: {
              id: "manual-redis-rca-ssh",
              title: "Redis SSH 排障运维手册",
            },
            bound_workflow_id: "workflow-redis-rca-ssh",
            usable_mode: "direct_execute",
            matched_fields: ["object_type", "operation_type", "execution_surface"],
          },
        ],
        recommended_next_action: "运行只读预检，通过后确认或审批执行。",
      },
    },
  );
}

function opsManualDynamicCandidatesState() {
  return artifactTransportState(
    "ops-manual-dynamic-candidates",
    "补齐 Redis 运维手册参数",
    {
      id: "artifact-ops-manual-dynamic-candidates",
      type: "ops_manual_param_resolution",
      titleZh: "运维手册参数解析",
      source: "tool:resolve_ops_manual_params",
      redactionStatus: "redacted",
      inlineData: {
        status: "ambiguous",
        manual_id: "manual-redis-rca-ssh",
        workflow_id: "workflow-redis-rca-ssh",
        ops_manual_flow_id: "flow-redis-dynamic-candidates",
        fields: [
          {
            id: "target_instance",
            label: "实例/服务",
            type: "resource_ref",
            ui_control: "select",
            required: true,
            candidates: [
              {
                value: "docker:redis-prod-a",
                label: "redis-prod-a | image redis:7.2 | ports 6379/tcp | health healthy",
              },
              {
                value: "k8s:redis-cache-0",
                label: "redis-cache-0 | namespace prod-cache | image redis:7.2.4 | phase Running",
              },
            ],
          },
          {
            id: "execution_surface",
            label: "访问/执行入口",
            type: "enum",
            ui_control: "select",
            required: true,
            candidates: [
              { value: "ssh:redis-01", label: "ssh redis-01 | service redis-server | port 6379" },
              { value: "kubectl:prod-cache", label: "kubectl prod-cache | deployment redis | health ready" },
            ],
          },
        ],
      },
    },
  );
}

function dataStreamForState(state) {
  return `aui-state:${JSON.stringify([{ type: "set", path: [], value: completeTransportFixtureState(state) }])}\n`;
}

function completeTransportFixtureState(state) {
  return {
    pendingApprovals: {},
    mcpSurfaces: {},
    artifacts: {},
    hostMissions: {},
    childAgents: {},
    runtimeLiveness: {
      activeTurns: {},
      activeAgents: {},
      pendingApprovals: {},
      pendingUserInputs: {},
      activeCommandStreams: {},
    },
    ...state,
  };
}

function phase0OutputFixture({ name, userText, status = "idle", turnStatus = "completed", blocks, timeline = [] }) {
  const turnId = `turn-${name}`;
  const startedAt = "2026-07-16T06:00:00.000Z";
  const completedAt = turnStatus === "completed" ? "2026-07-16T06:00:08.000Z" : undefined;
  const active = turnStatus === "working";
  const state = createChatFixtureState({
    sessionId: name,
    threadId: name,
    status,
    currentTurnId: turnId,
    cards: [
      {
        id: `user-${name}`,
        type: "UserMessageCard",
        role: "user",
        text: userText,
        createdAt: startedAt,
        updatedAt: startedAt,
      },
    ],
    runtime: {
      turn: { active, phase: active ? "thinking" : "completed" },
      codex: { status: "connected", retryAttempt: 0, retryMax: 5 },
      activity: {},
    },
    finalText: "",
    turns: {
      [turnId]: {
        id: turnId,
        status: turnStatus,
        startedAt,
        completedAt,
        updatedAt: completedAt || "2026-07-16T06:00:03.000Z",
        user: {
          id: `user-${name}`,
          text: userText,
          createdAt: startedAt,
        },
        blockOrder: blocks.map((block) => block.id),
        blocksById: Object.fromEntries(blocks.map((block) => [block.id, block])),
        timeline,
      },
    },
    turnOrder: [turnId],
    runtimeLiveness: {
      activeTurns: active ? { [turnId]: true } : {},
      activeAgents: active ? { "agent-main": true } : {},
      pendingApprovals: {},
      pendingUserInputs: {},
      activeCommandStreams: {},
    },
    updatedAt: completedAt || "2026-07-16T06:00:03.000Z",
  });
  return {
    name,
    state,
    sessions: createChatFixtureSessions({
      activeSessionId: name,
      sessions: [
        {
          id: name,
          kind: "single_host",
          title: `Phase 0 ${name}`,
          status: active ? "running" : "completed",
          messageCount: 1,
          preview: userText,
          selectedHostId: "server-local",
          lastActivityAt: completedAt || "2026-07-16T06:00:03.000Z",
        },
      ],
    }),
  };
}

function phase0RunningFixture() {
  return phase0OutputFixture({
    name: "output-contract-running",
    userText: "检查当前运行状态，但不要提前显示最终结论。",
    status: "working",
    turnStatus: "working",
    blocks: [],
    timeline: [
      {
        id: "draft-running-trace",
        type: "assistant_message",
        status: "running",
        text: "这段未分类草稿只能留在 trace 中。",
        payloadKind: "unclassified",
      },
    ],
  });
}

function phase0CompletedFixture() {
  const finalText = "检查完成：服务状态稳定，最终答案只提交一次。";
  return phase0OutputFixture({
    name: "output-contract-completed",
    userText: "完成检查后再给我最终结论。",
    blocks: [
      {
        id: "final-output-contract-completed",
        type: "final_answer",
        kind: "assistant",
        displayKind: "assistant.message",
        phase: "final_answer",
        streamState: "complete",
        status: "completed",
        text: finalText,
        finalContract: {
          id: "final-output-contract-completed",
          text: finalText,
          status: "verified",
          schemaVersion: "aiops.harness.final.v1",
          confidence: "high",
          checkedEvidenceRefs: ["evidence-output-contract"],
        },
      },
    ],
  });
}

function phase0RetryFixture() {
  const supersededDraft = "旧的 retry 草稿不应进入普通聊天。";
  return phase0OutputFixture({
    name: "output-contract-retry",
    userText: "如果模型重试，只显示最终提交的答案。",
    blocks: [
      {
        id: "commentary-output-contract-retry",
        type: "commentary",
        kind: "assistant",
        displayKind: "assistant.message",
        phase: "commentary",
        streamState: "complete",
        status: "completed",
        text: "重试后重新校验只读证据。",
      },
      {
        id: "final-output-contract-retry",
        type: "final_answer",
        kind: "assistant",
        displayKind: "assistant.message",
        phase: "final_answer",
        streamState: "complete",
        status: "completed",
        text: "重试完成：只保留这一次最终提交。",
      },
    ],
    timeline: [
      {
        id: "draft-output-contract-retry",
        type: "assistant_message",
        status: "rejected",
        text: supersededDraft,
        payloadKind: "unclassified",
      },
    ],
  });
}

function phase0ArtifactOrderingFixture() {
  return phase0OutputFixture({
    name: "output-contract-artifact-ordering",
    userText: "按真实发生顺序展示检查、证据卡和最终结论。",
    blocks: [
      {
        id: "commentary-before-artifact",
        type: "commentary",
        kind: "assistant",
        displayKind: "assistant.message",
        phase: "commentary",
        streamState: "complete",
        status: "completed",
        text: "先执行只读检查。",
      },
      {
        id: "command-before-artifact",
        type: "command",
        kind: "command",
        status: "completed",
        text: "printf artifact-order-source",
        command: "printf artifact-order-source",
        outputPreview: "artifact-order-source",
        toolCallId: "call-artifact-order",
        foldGroupId: "action-artifact-order",
        foldGroupKind: "command",
      },
      {
        id: "artifact-ordering-evidence",
        type: "artifact",
        kind: "tool",
        status: "completed",
        text: "顺序验证证据卡",
        artifact: {
          id: "artifact-ordering-evidence",
          type: "verification_result",
          titleZh: "顺序验证证据卡",
          summaryZh: "该证据卡紧跟来源工具出现。",
          status: "ok",
          source: "fixture",
          inlineData: { check: "artifact-order-source", result: "passed" },
        },
      },
      {
        id: "commentary-after-artifact",
        type: "commentary",
        kind: "assistant",
        displayKind: "assistant.message",
        phase: "commentary",
        streamState: "complete",
        status: "completed",
        text: "证据卡生成后继续整理结论。",
      },
      {
        id: "final-after-artifact",
        type: "final_answer",
        kind: "assistant",
        displayKind: "assistant.message",
        phase: "final_answer",
        streamState: "complete",
        status: "completed",
        text: "最终结论：工具、证据卡和回答顺序正确。",
      },
    ],
  });
}

function phase4TypedActionGroupFixture() {
  return phase0OutputFixture({
    name: "typed-action-group",
    userText: "用一组关联动作采集配置和 MCP 证据。",
    blocks: [
      {
        id: "commentary-typed-action",
        type: "commentary",
        kind: "assistant",
        displayKind: "assistant.message",
        phase: "commentary",
        streamState: "complete",
        commentarySource: "runtime_tool_intent",
        toolCallIds: ["call-file-action", "call-mcp-action"],
        foldGroupId: "typed-action-1",
        foldGroupKind: "tool",
        status: "completed",
        text: "采集配置文件和 MCP 资源两类只读证据。",
      },
      {
        id: "file-typed-action",
        type: "file",
        kind: "file",
        displayKind: "file.read",
        toolCallId: "call-file-action",
        foldGroupId: "typed-action-1",
        foldGroupKind: "tool",
        status: "completed",
        text: "读取服务配置",
        inputSummary: "/etc/payment-api/config.yaml",
        outputPreview: "port: 8080",
      },
      {
        id: "mcp-typed-action",
        type: "mcp",
        kind: "mcp",
        displayKind: "mcp.action",
        toolCallId: "call-mcp-action",
        foldGroupId: "typed-action-1",
        foldGroupKind: "tool",
        status: "completed",
        text: "读取 MCP 运维资源",
        inputSummary: "ops://payment-api",
        outputPreview: "resource available",
      },
      {
        id: "approval-typed-action",
        type: "approval",
        kind: "approval",
        displayKind: "approval",
        status: "rejected",
        text: "用户拒绝后续变更操作，审批记录保持独立可见。",
        approvalId: "approval-typed-action",
      },
      {
        id: "final-typed-action",
        type: "final_answer",
        kind: "assistant",
        displayKind: "assistant.message",
        phase: "final_answer",
        streamState: "complete",
        status: "completed",
        text: "只读证据采集完成；未执行被拒绝的变更。",
      },
    ],
  });
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
  await page.getByTestId("aiops-search-detail-row-toggle").first().click();

  const transcript = page.getByTestId("aiops-process-transcript-body");
  await expect(transcript).toContainText("接下来我要检查运行环境和最近任务状态。");
  await expect(transcript).toContainText("网页搜索 2 次 · 找到 1 个来源");
  await expect(transcript).toContainText("https://example.com/aiops-v2-order");
  await expect(transcript).toHaveScreenshot("process-transcript-order-alignment.png");
});

test("codex-like process transcript interleaves commentary and tools", async ({ page }) => {
  await routeShellApis(page, codexLikeProcessTranscriptState());
  await page.goto("/");
  await page.getByTestId("aiops-process-header").click();
  const transcript = page.getByTestId("aiops-process-transcript-body");
  await expect(transcript).toContainText("检索可用工具");
  await expect(transcript).toContainText("执行只读命令");
  await expect(page.getByTestId("aiops-final-text")).toContainText("CPU 当前空闲约 73%");
  await expect(transcript).toHaveScreenshot("codex-like-process-transcript.png");
});

test("assistant final markdown keeps the same layout while running and after completion", async ({ page }) => {
  let resumeState = finalMarkdownState("working");
  await routeShellApis(page, () => resumeState);

  await page.goto("/");
  const runningFinal = page.getByTestId("aiops-answer-document");
  await expect(runningFinal).toBeVisible();
  await expect(runningFinal).toHaveScreenshot("assistant-final-markdown-running.png");

  resumeState = finalMarkdownState("completed");
  await page.reload();
  const completedFinal = page.getByTestId("aiops-answer-document");
  await expect(completedFinal).toBeVisible();
  await expect(completedFinal).toHaveScreenshot("assistant-final-markdown-completed.png");
});

test("final contract summary hides raw evidence refs", async ({ page }) => {
  await routeShellApis(page, finalContractSummaryState());
  await page.goto("/");

  const summary = page.getByTestId("aiops-final-contract-summary");
  await expect(summary).toBeVisible();
  await expect(summary).toContainText("已验证");
  await expect(summary).toContainText("置信度高");
  await expect(summary).toContainText("已采集 2 条证据");
  await expect(summary).not.toContainText("call_secret_1");
  await expect(summary).not.toContainText("call_secret_2");
  await expect(summary).toHaveScreenshot("final-contract-summary-redacted-evidence.png");
});

test("final contract summary hides internal low confidence calibration", async ({ page }) => {
  await routeShellApis(page, finalContractInternalCalibrationState());
  await page.goto("/");

  await expect(page.getByTestId("aiops-final-contract-summary")).toHaveCount(0);
  const answer = page.getByTestId("aiops-answer-document");
  await expect(answer).toContainText("你好！有什么可以帮你的吗？");
  await expect(answer).not.toContainText("置信度低");
  await expect(answer).toHaveScreenshot("final-contract-internal-calibration-hidden.png");
});

test("running assistant text keeps the process header before tool blocks arrive", async ({ page }) => {
  await page.clock.setFixedTime(runningPreludeRenderedAt);
  await routeShellApis(page, runningPreludeBeforeToolsState());

  await page.goto("/");
  const transcript = page.getByTestId("aiops-process-transcript");
  await expect(page.getByTestId("aiops-process-header")).toContainText("处理中 1s");
  await expect(page.getByTestId("aiops-answer-document")).toContainText(runningPreludeText);
  await expect(page.getByText(runningPreludeText, { exact: true })).toHaveCount(1);
  await expect(page.getByTestId("aiops-process-transcript-body")).toHaveCount(0);
  await expect(transcript.locator("..")).toHaveScreenshot("assistant-running-prelude-with-process-header.png");
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
  await expect(terminalOutput).toHaveClass(/max-h-\[12rem\]/);
  await expect(terminalOutput).toHaveClass(/overflow-y-auto/);

  const sizes = await terminalOutput.evaluate((element) => ({
    clientHeight: element.clientHeight,
    scrollHeight: element.scrollHeight,
  }));
  expect(sizes.clientHeight).toBeGreaterThan(0);
  expect(sizes.clientHeight).toBeLessThanOrEqual(192);
  expect(sizes.scrollHeight).toBeGreaterThan(sizes.clientHeight);
});

test("chat renders rca report artifact", async ({ page }) => {
  await routeShellApis(page, rcaReportTransportState);

  await page.goto("/");
  await expect(page.getByTestId("rca-report-artifact")).toBeVisible();
  await expect(page).toHaveScreenshot("chat-rca-report-artifact.png", { fullPage: true });
});

test("chat shows context compaction and externalized evidence states", async ({ page }) => {
  const state = contextCompactionTransportState();
  await page.clock.setFixedTime("2026-05-22T08:00:05.000Z");
  await routeShellApis(page, state);
  await page.route("**/api/external-references/spill-1", async (route) => {
    await route.fulfill({
      json: {
        id: "spill-1",
        kind: "blob",
        contentType: "text/plain",
        summary: "17 upstream timeout lines from nginx in the last 10 minutes.",
        content: "2026-05-22T08:00:01Z upstream timed out while connecting to service-a",
        bytes: 82,
        digest: "",
        title: "nginx raw timeout logs",
      },
    });
  });

  await page.goto("/");
  await expect(page.getByText("上下文过长，已使用本地摘要继续")).toBeVisible();
  await expect(page.getByText("正在重试压缩")).toHaveCount(0);
  await expect(page.getByText("已外溢")).toHaveCount(0);
  await expect(page.getByRole("button", { name: /查看原始证据/ })).toHaveCount(0);
  const toolRow = page.getByTestId("aiops-tool-row-tool-context-spill");
  if ((await toolRow.count()) === 0) {
    await page.getByTestId("aiops-process-header").click();
  }
  await expect(toolRow).toBeVisible();
  await toolRow.click();
  await expect(page.getByText("Large nginx log result was externalized. Summary: 17 upstream timeout lines.")).toBeVisible();
  await expect(page.getByText("结果较大，仅显示摘要。")).toHaveCount(0);
  await expect(page.getByText(/upstream timed out/)).toHaveCount(0);
  await expect(page.getByTestId("context-status-notice")).toHaveScreenshot("context-compaction-notice.png");
  await expect(page.getByTestId("aiops-process-transcript")).toHaveScreenshot("context-compaction-process.png");
});

test("ops manual direct hit shows distinct use reference and skip actions", async ({ page }) => {
  await routeShellApis(page, opsManualDirectActionsState());

  await page.goto("/");
  const card = page.getByTestId("ops-manual-search-result-card");
  await expect(card).toContainText("使用该手册/Workflow");
  await expect(card).toContainText("仅参考手册");
  await expect(card).toContainText("不使用");
  await expect(card).toHaveScreenshot("ops-manual-direct-three-actions.png");

  await card.getByRole("button", { name: "仅参考手册" }).click();
  await expect(card.getByTestId("ops-manual-reference-submitted")).toContainText(
    "不进入 Workflow 预检",
  );
  await expect(card.getByTestId("ops-manual-preflight-running")).toHaveCount(0);
  await expect(card).toHaveScreenshot("ops-manual-reference-only-selected.png");
});

test("ops manual dynamic candidates keep long environment labels inside the card", async ({ page }) => {
  await routeShellApis(page, opsManualDynamicCandidatesState());

  await page.goto("/");
  const card = page.getByTestId("ops-manual-param-resolution-card");
  await expect(card).toContainText("image redis:7.2");
  await expect(card).toContainText("namespace prod-cache");
  await expect(card).toContainText("health ready");
  await expect(card).toHaveScreenshot("ops-manual-dynamic-candidates.png");
});

test("ops manual search and parameter confirmation merge into one card", async ({ page }) => {
  await routeShellApis(page, opsManualMergedParamConfirmationState());

  await page.goto("/");
  const card = page.getByTestId("ops-manual-progress-card");
  await expect(card).toBeVisible();
  await expect(page.getByTestId("ops-manual-search-result-card")).toHaveCount(0);
  await expect(page.getByTestId("ops-manual-param-resolution-card")).toHaveCount(0);
  await expect(card.getByRole("button", { name: "不使用" })).toHaveCount(1);
  await expect(card.getByText("备份路径")).toHaveCount(0);
  await expect(card.getByText("目标主机")).toBeVisible();
  await expect(card.getByText("目标实例")).toBeVisible();
  await expect(card.getByText("server-local")).toHaveCount(0);
  await expect(card.getByText("docker:aiops-mysql")).toHaveCount(0);

  await card.getByRole("button", { name: "查看详细参数" }).click();
  await expect(card.getByText("server-local")).toBeVisible();
  await expect(card.getByText("docker:aiops-mysql")).toBeVisible();
  await expect(card).toHaveScreenshot("ops-manual-merged-param-confirmation.png");
});

test.describe("phase 0 output contract snapshots", () => {
  test("running turn keeps unclassified draft out of chat", async ({ page }) => {
    await page.clock.setFixedTime("2026-07-16T06:00:02.000Z");
    await openFixturePage(page, "/", phase0RunningFixture());

    await expect(page.getByText("这段未分类草稿只能留在 trace 中。", { exact: true })).toHaveCount(0);
    await expect(page.getByTestId("aiops-answer-document")).toHaveCount(0);
    const transcript = page.getByTestId("aiops-process-transcript");
    await expect(transcript).toContainText("处理中 2s");
    await expect(transcript).toHaveScreenshot("phase0-running-no-final-draft.png");
  });

  test("completed turn shows one final without a success contract card", async ({ page }) => {
    await openFixturePage(page, "/", phase0CompletedFixture());

    await expect(page.getByTestId("aiops-final-contract-summary")).toHaveCount(0);
    const answer = page.getByTestId("aiops-answer-document");
    await expect(answer).toContainText("检查完成：服务状态稳定，最终答案只提交一次。");
    await expect(page.getByText("检查完成：服务状态稳定，最终答案只提交一次。", { exact: true })).toHaveCount(1);
    await expect(answer).toHaveScreenshot("phase0-completed-single-final.png");
  });

  test("retry keeps superseded draft trace-only", async ({ page }) => {
    await openFixturePage(page, "/", phase0RetryFixture());

    await expect(page.getByText("旧的 retry 草稿不应进入普通聊天。", { exact: true })).toHaveCount(0);
    await expect(page.getByText("重试完成：只保留这一次最终提交。", { exact: true })).toHaveCount(1);
    await page.getByTestId("aiops-process-header").click();
    const assistantTurn = page.getByTestId("aiops-process-transcript").locator("..");
    await expect(assistantTurn).toContainText("重试后重新校验只读证据。");
    await expect(assistantTurn).toHaveScreenshot("phase0-retry-superseded-draft-hidden.png");
  });

  test("artifact stays between its source tool and later final", async ({ page }) => {
    await openFixturePage(page, "/", phase0ArtifactOrderingFixture());

    const processHeaders = page.getByTestId("aiops-process-header");
    await expect(processHeaders).toHaveCount(2);
    await processHeaders.nth(0).click();
    await processHeaders.nth(1).click();

    const assistantTurn = page.getByTestId("aiops-answer-document").locator("..");
    const visibleText = await assistantTurn.innerText();
    const commentaryBefore = visibleText.indexOf("先执行只读检查。");
    const sourceTool = visibleText.indexOf("printf artifact-order-source");
    const artifact = visibleText.indexOf("顺序验证证据卡");
    const commentaryAfter = visibleText.indexOf("证据卡生成后继续整理结论。");
    const final = visibleText.indexOf("最终结论：工具、证据卡和回答顺序正确。");

    expect(commentaryBefore).toBeGreaterThanOrEqual(0);
    expect(sourceTool).toBeGreaterThan(commentaryBefore);
    expect(artifact).toBeGreaterThan(sourceTool);
    expect(commentaryAfter).toBeGreaterThan(artifact);
    expect(final).toBeGreaterThan(commentaryAfter);
    await expect(assistantTurn).toHaveScreenshot("phase0-artifact-visible-ordering.png");
  });
});

test("typed action group uses commentary as one title and keeps approval outside the fold", async ({ page }) => {
  await openFixturePage(page, "/", phase4TypedActionGroupFixture());

  await page.getByTestId("aiops-process-header").click();
  const actionToggle = page.getByTestId("aiops-merged-tool-toggle");
  await expect(actionToggle).toContainText("采集配置文件和 MCP 资源两类只读证据。");
  await expect(actionToggle).toHaveAttribute("aria-expanded", "false");
  await expect(page.getByText("采集配置文件和 MCP 资源两类只读证据。", { exact: true })).toHaveCount(1);
  await expect(page.getByTestId("aiops-process-transcript")).toContainText("用户拒绝后续变更操作，审批记录保持独立可见。");

  await actionToggle.click();
  await expect(page.getByTestId("aiops-tool-row-file-typed-action")).toBeVisible();
  await expect(page.getByTestId("aiops-tool-row-mcp-typed-action")).toBeVisible();
  const assistantTurn = page.getByTestId("aiops-answer-document").locator("..");
  await expect(assistantTurn).toHaveScreenshot("phase4-typed-action-group.png");
});
