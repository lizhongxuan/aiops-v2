import { describe, expect, it } from "vitest";
import { applyAgentEvent, createAgentEventState } from "./agentEventReducer";
import { selectApprovalDock, selectProjectionActivityLines, selectRuntimeLiveness } from "./agentEventProjection";

function event(overrides = {}) {
  return {
    eventId: overrides.eventId || `${overrides.kind || "system"}-${overrides.phase || "completed"}-${overrides.seq || 1}`,
    seq: overrides.seq || 1,
    sessionId: "session-erp",
    turnId: "turn-erp",
    kind: overrides.kind || "system",
    phase: overrides.phase || "completed",
    status: overrides.status || "completed",
    visibility: overrides.visibility || "primary",
    source: overrides.source || "runtime",
    createdAt: overrides.createdAt || `2026-05-04T00:00:${String(overrides.seq || 1).padStart(2, "0")}Z`,
    payload: overrides.payload || {},
  };
}

describe("ERP SRE agent event projection", () => {
  it("keeps action proposal typed payload for proposal process rendering", () => {
    let state = createAgentEventState();
    state = applyAgentEvent(state, event({ kind: "turn", phase: "started", status: "running", seq: 1 }));
    state = applyAgentEvent(state, event({
      seq: 2,
      payload: {
        id: "proposal-1",
        displayKind: "action.proposal",
        title: "重启报表服务",
        summary: "释放报表服务占用的数据库连接",
        command: "systemctl restart erp-report.service",
        risk: "high",
        source: "runbook",
        runbookId: "order-submit-slow",
        runbookStep: "restart-report-service",
        expectedEffect: "订单提交延迟应下降",
        rollback: "失败时转人工接管",
      },
    }));

    const lines = selectProjectionActivityLines(state, "session-erp");

    expect(lines).toHaveLength(1);
    expect(lines[0]).toMatchObject({
      kind: "proposal",
      displayKind: "action.proposal",
      text: "重启报表服务",
      command: "systemctl restart erp-report.service",
      risk: "high",
      source: "runbook",
      runbookId: "order-submit-slow",
      runbookStep: "restart-report-service",
      expectedEffect: "订单提交延迟应下降",
      rollback: "失败时转人工接管",
    });
  });

  it("clears pending approval when approval is resolved", () => {
    let state = createAgentEventState();
    state = applyAgentEvent(state, event({ kind: "turn", phase: "started", status: "running", seq: 1 }));
    state = applyAgentEvent(state, event({
      eventId: "approval-requested",
      kind: "approval",
      phase: "requested",
      status: "blocked",
      seq: 2,
      payload: {
        approvalId: "approval-1",
        approvalType: "command",
        command: "systemctl restart erp-report.service",
        reason: "runbook guarded action",
        risk: "high",
      },
    }));

    expect(selectRuntimeLiveness(state, "session-erp").pendingApprovals).toEqual({ "approval-1": true });
    expect(selectApprovalDock(state, "session-erp")).toHaveLength(1);

    state = applyAgentEvent(state, event({
      eventId: "approval-resolved",
      kind: "approval",
      phase: "resolved",
      status: "completed",
      seq: 3,
      payload: {
        approvalId: "approval-1",
        approvalType: "command",
        command: "systemctl restart erp-report.service",
        decision: "approved",
      },
    }));

    expect(selectRuntimeLiveness(state, "session-erp").pendingApprovals).toEqual({});
    expect(selectApprovalDock(state, "session-erp")).toHaveLength(0);
    expect(state.projectionsBySession["session-erp"].approvals[0]).toMatchObject({
      id: "approval-1",
      status: "completed",
      decision: "approved",
    });
  });
});
