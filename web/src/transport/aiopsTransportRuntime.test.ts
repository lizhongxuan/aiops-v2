import { describe, expect, it, vi } from "vitest";

import {
  createAiopsTransportCommandActions,
  createInitialAiopsTransportState,
  markAiopsTransportCanceled,
  markAiopsTransportFailed,
  normalizeAiopsTransportState,
} from "./aiopsTransportRuntime";
import type { AiopsTransportState } from "./aiopsTransportTypes";

describe("aiopsTransportRuntime", () => {
  it("creates a fully initialized transport state for assistant-ui", () => {
    const state = createInitialAiopsTransportState("thread-1");

    expect(state).toMatchObject({
      schemaVersion: "aiops.transport.v2",
      sessionId: "",
      threadId: "thread-1",
      status: "idle",
      turns: {},
      turnOrder: [],
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
      seq: 0,
    });
    expect(state.hostMissions).toEqual({});
    expect(state.childAgents).toEqual({});
    expect(state.activeHostMissionId).toBeUndefined();
    expect(state.specialInputContext).toBeUndefined();
    expect(new Date(state.updatedAt).toString()).not.toBe("Invalid Date");
  });

  it("normalizes special input context without requiring markdown inference", () => {
    const state = normalizeAiopsTransportState({
      ...createInitialAiopsTransportState("thread-special-input"),
      specialInputContext: {
        schemaVersion: "aiops.special_input_memory.v1",
        turnId: "turn-1",
        activeGrant: {
          id: "grant-host-a",
          resourceKind: "host",
          resourceId: "host-a",
          canonicalKey: "host:host-a",
          display: "host-a",
          status: "active",
          allowedActions: ["inspect", "read", "exec_low_risk"],
        },
        candidateFacts: [
          {
            id: "fact-raw",
            kind: "host",
            resourceKind: "host",
            resourceId: "1.1.1.1",
            canonicalKey: "host:1.1.1.1",
            display: "1.1.1.1",
            trustLevel: "raw_typed",
            status: "active",
          },
        ],
        pendingConfirmations: [
          { id: "pending-target", kind: "target", reason: "active_grant_revalidate_failed" },
        ],
      },
    } satisfies Partial<AiopsTransportState>);

    expect(state.specialInputContext?.activeGrant?.resourceId).toBe("host-a");
    expect(state.specialInputContext?.candidateFacts?.[0]?.trustLevel).toBe("raw_typed");
    expect(state.specialInputContext?.pendingConfirmations?.[0]?.reason).toBe("active_grant_revalidate_failed");
  });

  it("normalizes nullable host mission arrays from runtime snapshots", () => {
    const state = normalizeAiopsTransportState({
      ...createInitialAiopsTransportState("thread-null-host-mission"),
      hostMissions: {
        "mission-1": {
          id: "mission-1",
          turnId: "turn-1",
          status: "running",
          planRequired: true,
          planAccepted: true,
          mentionedHosts: null,
          childAgentIds: null,
          planSteps: null,
        },
      },
    } as unknown as Partial<AiopsTransportState>);

    expect(state.hostMissions["mission-1"].mentionedHosts).toEqual([]);
    expect(state.hostMissions["mission-1"].childAgentIds).toEqual([]);
    expect(state.hostMissions["mission-1"].planSteps).toBeUndefined();
  });

  it("normalizes optional ops run view state", () => {
    const state = normalizeAiopsTransportState({
      ...createInitialAiopsTransportState("thread-opsrun"),
      opsRun: {
        id: "opsrun-turn-1",
        source: "chat",
        status: "working",
        title: "主机A跟主机B上PG不同步",
        routeMode: "multi_host_ops",
        targetSummary: "主机A/主机B PG 与主机C pg_mon",
        toolSurfaceSummary: "无主机执行 / HostOps",
        evidenceCount: 2,
        currentStep: "正在只读采集 PG 同步证据",
        currentStepId: "step-tool-1",
        checkpointId: "checkpoint-1",
        agentRun: {
          id: "opsrun-turn-1",
          sessionId: "sess-1",
          rootTurnId: "turn-1",
          activeTurnId: "turn-1",
          userGoal: "主机A跟主机B上PG不同步",
          normalizedGoal: "multi host database replication issue",
          routeMode: "multi_host_ops",
          profile: "manager",
          status: "running",
          targetSummary: "主机A/主机B PG 与主机C pg_mon",
          currentStep: "正在只读采集 PG 同步证据",
          currentStepId: "step-tool-1",
          checkpointId: "checkpoint-1",
          evidenceCount: 2,
          startedAt: "2026-06-23T01:00:00Z",
          updatedAt: "2026-06-23T01:00:01Z",
          steps: [
            {
              id: "step-tool-1",
              runId: "opsrun-turn-1",
              turnId: "turn-1",
              iteration: 1,
              kind: "tool_call",
              status: "completed",
              title: "读取 Coroot 指标",
              inputSummary: "service:checkout",
              outputSummary: "p95 latency high",
              toolName: "coroot.service_metrics",
              toolCallId: "call-coroot-1",
              checkpointId: "checkpoint-1",
              targetRefs: ["service:checkout"],
              evidenceRefs: ["evidence-coroot-1"],
              completedAt: "2026-06-23T01:00:01Z",
            },
          ],
        },
      },
      turns: {
        "turn-1": {
          id: "turn-1",
          status: "working",
          process: [
            {
              id: "block-tool-1",
              kind: "tool",
              status: "completed",
              text: "读取 Coroot 指标",
              source: "coroot.service_metrics",
              toolCallId: "call-coroot-1",
              checkpointId: "checkpoint-1",
            },
          ],
        },
      },
    });

    expect(state.opsRun).toMatchObject({
      id: "opsrun-turn-1",
      source: "chat",
      status: "working",
      routeMode: "multi_host_ops",
      toolSurfaceSummary: "无主机执行 / HostOps",
      evidenceCount: 2,
      currentStepId: "step-tool-1",
      checkpointId: "checkpoint-1",
      agentRun: {
        id: "opsrun-turn-1",
        steps: [
          expect.objectContaining({
            kind: "tool_call",
            toolName: "coroot.service_metrics",
            toolCallId: "call-coroot-1",
          }),
        ],
      },
    });
    expect(state.turns["turn-1"]?.process?.[0]?.toolCallId).toBe("call-coroot-1");
    expect(state.turns["turn-1"]?.process?.[0]?.checkpointId).toBe("checkpoint-1");
  });

  it("filters runtime internal completion gate text from normalized turns", () => {
    const gate = "verification completion gate: block_success_final: execution_required,missing_verification_report";
    const state = normalizeAiopsTransportState({
      ...createInitialAiopsTransportState("thread-gate"),
      turnOrder: ["turn-1"],
      turns: {
        "turn-1": {
          id: "turn-1",
          status: "completed",
          process: [
            {
              id: "gate-process",
              kind: "evidence",
              status: "completed",
              text: gate,
            },
            {
              id: "visible-process",
              kind: "system",
              status: "completed",
              text: "已识别为证据分析",
            },
          ],
          final: {
            id: "final-gate",
            text: gate,
            status: "completed",
          },
        },
      },
    });

    expect(state.turns["turn-1"]?.process).toEqual([
      expect.objectContaining({ id: "visible-process", text: "已识别为证据分析" }),
    ]);
    expect(state.turns["turn-1"]?.final).toBeUndefined();
  });

  it("redacts risky assistant final operations without hiding the RCA answer", () => {
    const risky = [
      "## 结论",
      "timeline 已经分叉。",
      "",
      "## 修复方向",
      "可以执行 rm -rf $PG_DATA/recovery/repos/archive/paf/15-1/* 清理 archive。",
      "下一步先确认 pgbackrest info 和恢复日志。",
    ].join("\n");
    const state = normalizeAiopsTransportState({
      ...createInitialAiopsTransportState("thread-risky"),
      turnOrder: ["turn-1"],
      turns: {
        "turn-1": {
          id: "turn-1",
          status: "completed",
          process: [
            {
              id: "assistant-risky",
              kind: "assistant",
              displayKind: "assistant.message",
              phase: "final_answer",
              streamState: "complete",
              status: "completed",
              text: risky,
            },
            {
              id: "visible-system",
              kind: "system",
              status: "completed",
              text: "已识别为证据分析",
            },
          ],
          final: {
            id: "final-risky",
            text: risky,
            status: "completed",
          },
        },
      },
    });

    expect(state.turns["turn-1"]?.final?.text).toContain("timeline 已经分叉");
    expect(state.turns["turn-1"]?.final?.text).toContain("下一步先确认");
    expect(state.turns["turn-1"]?.final?.text).not.toContain("涉及清空数据目录或归档清理的高风险操作");
    expect(state.turns["turn-1"]?.final?.text).not.toContain("rm -rf");
    expect(state.turns["turn-1"]?.process).toEqual([
      expect.objectContaining({
        id: "assistant-risky",
        text: expect.stringContaining("timeline 已经分叉"),
      }),
      expect.objectContaining({ id: "visible-system", text: "已识别为证据分析" }),
    ]);
  });

  it("keeps RCA cause lines that mention residual PGDATA without direct mutation advice", () => {
    const final = [
      "可能导致 B timeline 更高的具体原因：",
      "1. **B 的 `$PGDATA` 未完全清空**：步骤 4.2 要求清空，但若残留旧 `.history` 文件或 WAL 段，PostgreSQL 启动时可能识别到更高 timeline 起点并沿其分支。",
      "2. **pg_autoctl 将 B 初始化为独立主库而非 standby**：若 monitor 中有旧节点残留，可能触发 promote 产生新 timeline。",
    ].join("\n");
    const state = normalizeAiopsTransportState({
      ...createInitialAiopsTransportState("thread-residual-pgdata"),
      turnOrder: ["turn-1"],
      turns: {
        "turn-1": {
          id: "turn-1",
          status: "completed",
          final: {
            id: "final-residual-pgdata",
            text: final,
            status: "completed",
          },
        },
      },
    });

    expect(state.turns["turn-1"]?.final?.text).toContain("1. **B 的 `$PGDATA` 未完全清空**");
    expect(state.turns["turn-1"]?.final?.text).toContain("2. **pg_autoctl 将 B 初始化为独立主库");
  });

  it("sanitizes model provider timeout details from failed final text and process blocks", () => {
    const raw = `模型请求超时：约 20s 未收到模型服务响应，请检查 LLM 地址、网络连通性或代理配置: Post "https://provider.invalid/v1/chat/completions": net/http: TLS handshake timeout`;
    const state = normalizeAiopsTransportState({
      ...createInitialAiopsTransportState("thread-model-timeout"),
      turnOrder: ["turn-1"],
      turns: {
        "turn-1": {
          id: "turn-1",
          status: "failed",
          process: [
            {
              id: "runtime-error",
              kind: "system",
              status: "failed",
              text: raw,
            },
          ],
          final: {
            id: "final-model-timeout",
            text: raw,
            status: "failed",
          },
        },
      },
    });

    const finalText = state.turns["turn-1"]?.final?.text || "";
    const processText = state.turns["turn-1"]?.process?.[0]?.text || "";
    expect(finalText).toBe("模型服务连接超时，未能建立连接。上下文较大或模型服务繁忙时可能需要更长时间，请稍后重试。");
    expect(processText).toBe(finalText);
    for (const forbidden of ["provider.invalid", "chat/completions", "Post ", "TLS handshake timeout", "约 20s"]) {
      expect(finalText).not.toContain(forbidden);
      expect(processText).not.toContain(forbidden);
    }
  });

  it("builds custom AssistantTransport commands from the current state", () => {
    const send = vi.fn();
    const state = {
      ...createInitialAiopsTransportState("thread-1"),
      sessionId: "sess-1",
      currentTurnId: "turn-1",
    } satisfies AiopsTransportState;
    const actions = createAiopsTransportCommandActions(state, send);

    actions.stop("user requested stop");
    actions.retry();
    actions.approvalDecision("approval-1", "reject");
    actions.choiceAnswer("choice-1", "continue");
    actions.mcpAction("filesystem", "open", { path: "/tmp" });
    actions.mcpRefresh("filesystem");
    actions.mcpPin("filesystem", true);
    actions.specialInputClear({ resourceKind: "host", resourceId: "host-a", canonicalKey: "host:host-a" });
    actions.specialInputConfirm({ resourceKind: "host", resourceId: "1.1.1.1", canonicalKey: "host:1.1.1.1" });

    expect(send.mock.calls.map(([command]) => command)).toEqual([
      {
        type: "aiops.stop",
        sessionId: "sess-1",
        turnId: "turn-1",
        reason: "user requested stop",
      },
      { type: "aiops.retry", sessionId: "sess-1", turnId: "turn-1" },
      {
        type: "aiops.approval-decision",
        sessionId: "sess-1",
        turnId: "turn-1",
        approvalId: "approval-1",
        decision: "reject",
      },
      {
        type: "aiops.choice-answer",
        requestId: "choice-1",
        answer: "continue",
      },
      {
        type: "aiops.mcp-action",
        surfaceId: "filesystem",
        action: "open",
        params: { path: "/tmp" },
      },
      { type: "aiops.mcp-refresh", surfaceId: "filesystem" },
      { type: "aiops.mcp-pin", surfaceId: "filesystem", pinned: true },
      {
        type: "aiops.special-input-clear",
        sessionId: "sess-1",
        resourceKind: "host",
        resourceId: "host-a",
        canonicalKey: "host:host-a",
      },
      {
        type: "aiops.special-input-confirm",
        sessionId: "sess-1",
        resourceKind: "host",
        resourceId: "1.1.1.1",
        canonicalKey: "host:1.1.1.1",
      },
    ]);
  });

  it("marks local state failed or canceled without mutating the previous state", () => {
    const state = {
      ...createInitialAiopsTransportState("thread-1"),
      status: "working",
      currentTurnId: "turn-1",
      turns: {
        "turn-1": {
          id: "turn-1",
          status: "working",
          blockOrder: ["reasoning-1", "tool-1", "final-1"],
          blocksById: {
            "reasoning-1": {
              id: "reasoning-1",
              type: "reasoning",
              kind: "reasoning",
              status: "running",
              text: "正在等待模型返回",
            },
            "tool-1": {
              id: "tool-1",
              type: "tool",
              kind: "tool",
              status: "completed",
              text: "已读取日志",
            },
            "final-1": {
              id: "final-1",
              type: "final_answer",
              kind: "assistant",
              status: "running",
              text: "正在生成结论",
              finalContract: { id: "final-1", text: "正在生成结论", status: "running" },
            },
          },
        },
      },
    } satisfies AiopsTransportState;

    const failed = markAiopsTransportFailed(state, "backend unavailable");
    const canceled = markAiopsTransportCanceled(state, "user canceled");

    expect(failed).toMatchObject({
      status: "failed",
      lastError: "服务异常,请稍后重试",
      turns: {
        "turn-1": {
          status: "failed",
          blocksById: {
            "reasoning-1": {
              id: "reasoning-1",
              status: "failed",
              text: "模型调用失败",
            },
            "tool-1": {
              id: "tool-1",
              status: "completed",
              text: "已读取日志",
            },
            "final-1": {
              status: "failed",
              finalContract: { status: "failed" },
            },
          },
        },
      },
    });
    expect(canceled).toMatchObject({
      status: "canceled",
      lastError: "user canceled",
      turns: {
        "turn-1": {
          status: "canceled",
          blocksById: {
            "reasoning-1": {
              id: "reasoning-1",
              status: "rejected",
              text: "模型调用已取消",
            },
            "tool-1": {
              id: "tool-1",
              status: "completed",
              text: "已读取日志",
            },
            "final-1": {
              status: "rejected",
              finalContract: { status: "cancelled" },
            },
          },
        },
      },
    });
    expect(state.status).toBe("working");
    expect(state.turns["turn-1"]?.status).toBe("working");
    expect(state.turns["turn-1"]?.blocksById?.["reasoning-1"]?.text).toBe("正在等待模型返回");
  });
});
