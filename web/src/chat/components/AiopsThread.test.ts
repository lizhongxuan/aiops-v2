import { describe, expect, it } from "vitest";

import { assistantMessageRenderedFinalText, finalContractSummaryView, isNearThreadBottom } from "./AiopsThread";

describe("AiopsThread auto-scroll helpers", () => {
  it("treats a viewport close to the bottom as sticky", () => {
    expect(isNearThreadBottom({ scrollTop: 890, clientHeight: 100, scrollHeight: 1000 })).toBe(true);
  });

  it("does not auto-stick when the user has scrolled up into history", () => {
    expect(isNearThreadBottom({ scrollTop: 200, clientHeight: 100, scrollHeight: 1000 })).toBe(false);
  });
});

describe("assistant message final text", () => {
  it("prefers transport finalText over stale assistant content", () => {
    const text = assistantMessageRenderedFinalText(
      [{ type: "text", text: "让我查看一下这台主机的基本信息。" }],
      { finalText: "" },
    );

    expect(text).toBe("");
  });
});

describe("assistant message final contract summary", () => {
  it("does not render a status-only failed summary when the final text already explains the error", () => {
    expect(
      finalContractSummaryView({
        finalText: "网络异常,请检查后重试",
        finalStatus: "failed",
        finalContract: {},
      }),
    ).toBeNull();
  });

  it("does not render internal low-confidence calibration without user-actionable details", () => {
    expect(
      finalContractSummaryView({
        finalText: "你好！有什么可以帮你的吗？",
        finalStatus: "unknown",
        finalConfidence: "low",
        finalContract: {
          schemaVersion: "aiops.harness.final.v1",
          status: "unknown",
          confidence: "low",
        },
      }),
    ).toBeNull();
  });

  it("uses structured final status instead of markdown words", () => {
    expect(
      finalContractSummaryView({
        finalText: "## 已验证\n\n看起来已经成功。",
        finalStatus: "tool_unavailable",
        finalConfidence: "low",
        finalContract: {
          schemaVersion: "aiops.harness.final.v1",
          status: "tool_unavailable",
          confidence: "low",
          failedToolImpacts: [
            {
              toolName: "exec_command",
              failureClass: "needs_host_agent",
              impact: "host agent 7072 不可用",
            },
          ],
          limitations: ["无法执行主机命令"],
        },
      }),
    ).toMatchObject({
      status: "tool_unavailable",
      statusLabel: "工具不可用",
      confidenceLabel: "置信度低",
      failedToolImpacts: ["exec_command: host agent 7072 不可用"],
      limitations: ["无法执行主机命令"],
    });
  });

  it("summarizes evidence without exposing internal evidence refs", () => {
    const summary = finalContractSummaryView({
      finalText: "已完成只读检查。",
      finalStatus: "verified",
      finalConfidence: "high",
      finalContract: {
        schemaVersion: "aiops.harness.final.v1",
        status: "verified",
        confidence: "high",
        checkedEvidenceRefs: ["call_secret_1", "call_secret_2"],
      },
    });

    expect(summary).toMatchObject({
      status: "verified",
      statusLabel: "已验证",
      confidenceLabel: "置信度高",
      evidenceLabel: "已采集 2 条证据",
    });
    expect(summary).not.toHaveProperty("checkedEvidenceRefs");
    expect(JSON.stringify(summary)).not.toContain("call_secret_1");
  });
});
