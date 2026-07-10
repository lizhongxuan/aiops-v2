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

  it("renders old raw structured evidence as readable evidence names", () => {
    const text = assistantMessageRenderedFinalText([], {
      finalText: [
        "已确认：",
        '- {"categoryCounts":{"application":25,"control-plane":14,"monitoring":10},"evidenceRefs":["ev-services"]}',
        '- {"evidenceRefs":["ev-incidents"],"incidents":[{"application":"rabbitmq-server"}]}',
        "",
        "仍缺少：",
        "- read_mcp_resource 未成功返回证据；不能当作已检查结果。",
        "- read_mcp_resource 未成功返回证据；不能当作已检查结果。",
        "",
        "下一步只读检查：",
        "1. 重新读取或替代核对 read_mcp_resource 对应的只读证据。",
      ].join("\n"),
    });

    expect(text).toContain("Coroot 服务概览已返回结构化证据。");
    expect(text).toContain("Coroot 异常事件已返回结构化证据。");
    expect(text).toContain("读取 MCP 资源 未成功返回证据；不能当作已检查结果。");
    expect(text).toContain("重新读取或替代核对 读取 MCP 资源 对应的只读证据。");
    expect(text).not.toContain('{"categoryCounts"');
    expect(text).not.toContain("read_mcp_resource");
    expect(text.match(/读取 MCP 资源 未成功返回证据/g)).toHaveLength(1);
  });

  it("hides leaked tool-reading process chatter from old final text", () => {
    const text = assistantMessageRenderedFinalText([], {
      finalText:
        "让我直接读取证据引用： greaseardereread_context_artifact with the evidence IDs:So let me try reading the evidence refs directly. I'll also try one more level of the spill chain. theringatherread_context_artifact with evidence IDs:Let me try reading the evidence refs directly. I can see from the initial summaries that there's some useful data already. Let me try one more level. theevidenceThere's useful summary data already. Let me also try to get the incidents more directly. read_context_artifact",
    });

    expect(text).toContain("工具读取过程");
    expect(text).not.toContain("read_context_artifact");
    expect(text).not.toContain("evidence IDs");
    expect(text).not.toContain("Let me try");
    expect(text).not.toContain("try reading");
  });

  it("hides repeated Coroot RCA parameter chatter from old final text", () => {
    const text = assistantMessageRenderedFinalText([], {
      finalText: "SERVICE_NAME=rabbitmq-server。让我获取RCA上下文。".repeat(8),
    });

    expect(text).toContain("工具读取过程");
    expect(text).not.toContain("SERVICE_NAME");
    expect(text).not.toContain("rabbitmq-server。让我获取RCA上下文");
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

  it("translates known tool diagnostics and removes duplicate limitations", () => {
    const summary = finalContractSummaryView({
      finalText: "还不能给最终结论。",
      finalStatus: "failed",
      finalConfidence: "medium",
      finalContract: {
        schemaVersion: "aiops.harness.final.v1",
        status: "failed",
        confidence: "medium",
        checkedEvidenceRefs: ["ev-1", "ev-2", "ev-3"],
        failedToolImpacts: [
          {
            toolName: "read_mcp_resource",
            failureClass: "tool_business_error",
            impact: "required evidence may be missing; do not use this failed tool as checked evidence",
          },
          {
            toolName: "read_mcp_resource",
            failureClass: "tool_business_error",
            impact: "required evidence may be missing; do not use this failed tool as checked evidence",
          },
        ],
        limitations: ["read_mcp_resource:tool_business_error", "read_mcp_resource:tool_business_error"],
      },
    });

    expect(summary).toMatchObject({
      failedToolImpacts: ["读取 MCP 资源：证据读取失败，不能作为已检查结果"],
      limitations: ["读取 MCP 资源：工具执行失败"],
    });
    expect(JSON.stringify(summary)).not.toContain("read_mcp_resource");
    expect(JSON.stringify(summary)).not.toContain("required evidence");
  });
});
