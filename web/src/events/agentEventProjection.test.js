import { describe, expect, it } from "vitest";
import { applyAgentEvent, createAgentEventState } from "./agentEventReducer";
import { selectProjectionActivityLines } from "./agentEventProjection";

function event(overrides = {}) {
  return {
    eventId: overrides.eventId || `${overrides.kind || "turn"}-${overrides.phase || "started"}-${overrides.seq || 1}`,
    seq: overrides.seq || 1,
    sessionId: "session-1",
    turnId: "turn-1",
    kind: overrides.kind || "turn",
    phase: overrides.phase || "started",
    status: overrides.status || "running",
    visibility: overrides.visibility || "secondary",
    source: "runtime",
    createdAt: overrides.createdAt || `2026-04-29T00:00:${String(overrides.seq || 1).padStart(2, "0")}Z`,
    payload: overrides.payload || {},
  };
}

describe("agent event projection UX shaping", () => {
  it("drops provisional final text once a tool call starts", () => {
    let state = createAgentEventState();
    state = applyAgentEvent(state, event({ kind: "turn", phase: "started", status: "running", seq: 1 }));
    state = applyAgentEvent(state, event({
      kind: "assistant",
      phase: "delta",
      status: "running",
      seq: 2,
      payload: { channel: "final", text: "我将先核实行情。" },
    }));
    state = applyAgentEvent(state, event({
      kind: "tool",
      phase: "started",
      status: "running",
      seq: 3,
      payload: { toolCallId: "search-1", toolName: "web_search", displayKind: "browser.search", inputSummary: "BTC price" },
    }));
    state = applyAgentEvent(state, event({
      kind: "tool",
      phase: "completed",
      status: "completed",
      seq: 4,
      payload: { toolCallId: "search-1", toolName: "web_search", displayKind: "browser.search", outputSummary: "已搜索 BTC price" },
    }));
    state = applyAgentEvent(state, event({
      kind: "assistant",
      phase: "delta",
      status: "running",
      seq: 5,
      payload: { channel: "final", text: "最终行情结论。" },
    }));

    expect(state.projectionsBySession["session-1"].finalMessages["turn-1"].text).toBe("最终行情结论。");
  });

  it("keeps provisional final text as a process summary when a tool call starts", () => {
    let state = createAgentEventState();
    state = applyAgentEvent(state, event({ kind: "turn", phase: "started", status: "running", seq: 1 }));
    state = applyAgentEvent(state, event({
      kind: "assistant",
      phase: "delta",
      status: "running",
      seq: 2,
      payload: { channel: "final", text: "我将先核实今天 A 股主要指数和成交额。" },
    }));
    state = applyAgentEvent(state, event({
      kind: "tool",
      phase: "started",
      status: "running",
      seq: 3,
      payload: { toolCallId: "search-1", toolName: "web_search", displayKind: "browser.search", inputSummary: "A股 大盘" },
    }));

    const lines = selectProjectionActivityLines(state, "session-1");

    expect(lines.map((line) => line.text)).toContain("我将先核实今天 A 股主要指数和成交额。");
    expect(state.projectionsBySession["session-1"].finalMessages["turn-1"]).toBeUndefined();
  });

  it("preserves markdown whitespace while streaming final answer deltas", () => {
    let state = createAgentEventState();
    state = applyAgentEvent(state, event({ kind: "turn", phase: "started", status: "running", seq: 1 }));
    state = applyAgentEvent(state, event({
      kind: "assistant",
      phase: "delta",
      status: "running",
      seq: 2,
      payload: { channel: "final", delta: "主机当前资源情况如下：\n\n" },
    }));
    state = applyAgentEvent(state, event({
      kind: "assistant",
      phase: "delta",
      status: "running",
      seq: 3,
      payload: { channel: "final", delta: "- **CPU**：空闲 80%\n- **内存**：压力正常" },
    }));

    expect(state.projectionsBySession["session-1"].finalMessages["turn-1"].text).toBe(
      "主机当前资源情况如下：\n\n- **CPU**：空闲 80%\n- **内存**：压力正常",
    );
  });

  it("keeps command text and output separate for command activity lines", () => {
    let state = createAgentEventState();
    state = applyAgentEvent(state, event({ kind: "turn", phase: "started", status: "running", seq: 1 }));
    state = applyAgentEvent(state, event({
      kind: "tool",
      phase: "completed",
      status: "completed",
      seq: 2,
      payload: {
        toolCallId: "tool-1",
        toolName: "exec_command",
        displayKind: "host.command",
        inputSummary: "df -h",
        outputSummary: "Filesystem Size Used Avail Capacity Mounted on",
        outputPreview: "Filesystem      Size   Used  Avail Capacity Mounted on\n/dev/disk3s1s1   460Gi   12Gi  239Gi     5% /",
      },
    }));

    const lines = selectProjectionActivityLines(state, "session-1");

    expect(lines).toHaveLength(1);
    expect(lines[0]).toMatchObject({
      kind: "command",
      text: "已运行 df -h",
      command: "df -h",
      output: "Filesystem      Size   Used  Avail Capacity Mounted on\n/dev/disk3s1s1   460Gi   12Gi  239Gi     5% /",
      outputPreview: "Filesystem      Size   Used  Avail Capacity Mounted on\n/dev/disk3s1s1   460Gi   12Gi  239Gi     5% /",
      displayKind: "host.command",
    });
  });

  it("keeps completed search rows distinct by showing the searched query", () => {
    let state = createAgentEventState();
    state = applyAgentEvent(state, event({ kind: "turn", phase: "started", status: "running", seq: 1 }));
    state = applyAgentEvent(state, event({
      eventId: "tool-search-1",
      kind: "tool",
      phase: "completed",
      status: "completed",
      seq: 2,
      payload: {
        toolCallId: "search-1",
        toolName: "web_search",
        displayKind: "browser.search",
        inputSummary: "2026-04-29 A股 大盘 上证指数 深证成指 创业板指 成交额",
        outputSummary: "已搜索网页",
      },
    }));
    state = applyAgentEvent(state, event({
      eventId: "tool-search-2",
      kind: "tool",
      phase: "completed",
      status: "completed",
      seq: 3,
      payload: {
        toolCallId: "search-2",
        toolName: "web_search",
        displayKind: "browser.search",
        inputSummary: "2026-04-29 A股 涨跌家数 成交额",
        outputSummary: "已搜索网页",
      },
    }));

    const lines = selectProjectionActivityLines(state, "session-1");

    expect(lines).toHaveLength(2);
    expect(lines.map((line) => line.text)).toEqual(["已搜索网页", "已搜索网页"]);
    expect(lines.map((line) => line.inputSummary)).toEqual([
      "2026-04-29 A股 大盘 上证指数 深证成指 创业板指 成交额",
      "2026-04-29 A股 涨跌家数 成交额",
    ]);
    expect(lines[0].queries).toEqual([
      "2026-04-29 A股 大盘 上证指数 深证成指 创业板指 成交额",
    ]);
  });

  it("does not expose raw tool names as command titles", () => {
    let state = createAgentEventState();
    state = applyAgentEvent(state, event({ kind: "turn", phase: "started", status: "running", seq: 1 }));
    state = applyAgentEvent(state, event({
      kind: "tool",
      phase: "completed",
      status: "completed",
      seq: 2,
      payload: {
        toolCallId: "tool-raw",
        toolName: "exec_command",
        displayKind: "host.command",
        outputSummary: "ok",
      },
    }));

    const lines = selectProjectionActivityLines(state, "session-1");

    expect(JSON.stringify(lines)).not.toContain("exec_command");
    expect(lines[0]).toMatchObject({
      kind: "command",
      text: "已运行命令",
      command: "",
    });
  });

  it("does not expose raw JSON tool output in activity lines", () => {
    const state = {
      projectionsBySession: {
        "session-1": {
          sessionId: "session-1",
          currentTurnId: "turn-1",
          timeline: [
            {
              id: "search-1",
              kind: "tool",
              turnId: "turn-1",
              toolCallId: "search-1",
              displayKind: "browser.search",
              title: "网页搜索已完成",
              summary: "{\"content\":\"On April 29, 2026, a long search result...\",\"query\":\"BTC price\"}",
              status: "completed",
              updatedAt: "2026-04-29T00:00:01Z",
            },
          ],
        },
      },
    };

    const lines = selectProjectionActivityLines(state, "session-1");

    expect(lines).toHaveLength(1);
    expect(lines[0].text).toBe("已搜索网页");
    expect(lines[0].text).not.toContain("{");
    expect(lines[0].text).not.toContain("content");
  });

  it("clears blocked approvals when a turn fails", () => {
    let state = createAgentEventState();
    state = applyAgentEvent(state, event({ kind: "turn", phase: "started", status: "running", seq: 1 }));
    state = applyAgentEvent(state, event({
      kind: "approval",
      phase: "requested",
      status: "blocked",
      seq: 2,
      payload: {
        approvalId: "approval-1",
        approvalType: "command",
        title: "exec_command",
        command: "bash -lc free -h",
      },
    }));
    state = applyAgentEvent(state, event({
      kind: "turn",
      phase: "failed",
      status: "failed",
      seq: 3,
      payload: { error: "command failed" },
    }));

    const projection = state.projectionsBySession["session-1"];

    expect(projection.status).toBe("failed");
    expect(projection.runtimeLiveness.pendingApprovals).toEqual({});
    expect(projection.approvals[0].status).toBe("failed");
  });
});
