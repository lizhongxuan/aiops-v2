import { describe, expect, it } from "vitest";
import { buildCodexProcessTranscript } from "./codexProcessTranscript";

describe("ERP SRE process transcript", () => {
  it("maps action proposal typed items to proposal-step blocks", () => {
    const transcript = buildCodexProcessTranscript({
      turnId: "turn-proposal",
      status: "blocked",
      processItems: [{
        id: "proposal-1",
        kind: "proposal",
        displayKind: "action.proposal",
        status: "blocked",
        title: "重启报表服务",
        summary: "释放数据库连接压力",
        command: "systemctl restart erp-report.service",
        risk: "high",
        source: "runbook",
        runbookId: "order-submit-slow",
        runbookStep: "restart-report-service",
        expectedEffect: "订单提交延迟应下降",
        rollback: "失败时转人工接管",
      }],
    });

    const proposal = transcript.blocks.find((block) => block.kind === "proposal-step");
    expect(proposal).toMatchObject({
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
    expect(transcript.blocks.map((block) => block.kind)).toEqual(["header", "proposal-step"]);
  });

  it("maps fallback plans to proposal-step blocks", () => {
    const transcript = buildCodexProcessTranscript({
      turnId: "turn-fallback",
      status: "running",
      processItems: [{
        id: "fallback-1",
        kind: "proposal",
        displayKind: "fallback.plan",
        status: "running",
        title: "受控 fallback 计划",
        summary: "无高覆盖 runbook，生成只读证据后的执行提案",
        risk: "medium",
        source: "fallback",
      }],
    });

    expect(transcript.blocks.find((block) => block.kind === "proposal-step")).toMatchObject({
      displayKind: "fallback.plan",
      text: "受控 fallback 计划",
      summary: "无高覆盖 runbook，生成只读证据后的执行提案",
      risk: "medium",
      source: "fallback",
    });
  });

  it("maps verification metrics to verification-step blocks", () => {
    const transcript = buildCodexProcessTranscript({
      turnId: "turn-verify",
      status: "running",
      processItems: [{
        id: "verify-1",
        kind: "verification",
        displayKind: "verification.metric",
        status: "completed",
        title: "验证订单 SLO",
        summary: "p95 latency below threshold",
        source: "coroot",
        rawRef: "coroot:slo:order-api",
      }],
    });

    expect(transcript.blocks.find((block) => block.kind === "verification-step")).toMatchObject({
      displayKind: "verification.metric",
      text: "验证订单 SLO",
      summary: "p95 latency below threshold",
      source: "coroot",
      rawRef: "coroot:slo:order-api",
    });
  });
});
