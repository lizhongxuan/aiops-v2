import { describe, expect, it } from "vitest";
import { parsePromptTrace, shortHash } from "./promptTraceViewModel";

function sampleTrace(overrides = {}) {
  return {
    schemaVersion: 1,
    kind: "runtime_model_input",
    caseId: "case-1",
    sessionId: "sess-1",
    turnId: "turn-1",
    iteration: 0,
    createdAt: "2026-05-02T05:42:25Z",
    visibleTools: ["browse_url", "exec_command"],
    promptFingerprint: {
      stableHash: "40a0e0f6a5e811df620c05fce37837adbd9511fac9e341872cc0e3ee60bd6e4a",
      systemHash: "system-hash",
      developerHash: "developer-hash",
      toolRegistryHash: "tools-hash",
      runtimePolicyHash: "policy-hash",
      protocolStateHash: "protocol-hash",
    },
    prompt: {
      tools: "## browse_url\nRead URL.\n\n## exec_command\nRun command.",
    },
    modelInput: [
      {
        index: 0,
        providerRole: "system",
        semanticRole: "system",
        promptLayer: "system",
        content: "# Role\nYou are an agent.",
      },
      {
        index: 1,
        providerRole: "system",
        semanticRole: "developer",
        promptLayer: "developer",
        content: "Use tools carefully.",
      },
      {
        index: 2,
        providerRole: "system",
        semanticRole: "tool",
        promptLayer: "tool_index",
        content: "browse_url\nexec_command",
      },
      {
        index: 3,
        providerRole: "system",
        semanticRole: "context",
        promptLayer: "runtime_policy",
        content: "Current mode: execute",
      },
      {
        index: 4,
        providerRole: "user",
        semanticRole: "user",
        promptLayer: "conversation",
        content: "请只回复 smoke",
      },
    ],
    ...overrides,
  };
}

describe("parsePromptTrace", () => {
  it("parses a real-shaped prompt trace into summary, layers, tools, and fingerprints", () => {
    const vm = parsePromptTrace(sampleTrace());

    expect(vm.summary.sessionId).toBe("sess-1");
    expect(vm.summary.caseId).toBe("case-1");
    expect(vm.summary.turnId).toBe("turn-1");
    expect(vm.summary.messageCount).toBe(5);
    expect(vm.summary.visibleToolCount).toBe(2);
    expect(vm.summary.hasUserMessage).toBe(true);
    expect(vm.layers.map((item) => item.title)).toEqual([
      "System",
      "Developer",
      "Tool Registry",
      "Runtime Policy",
      "Conversation / User",
    ]);
    expect(vm.layers[2].hash).toBe("tools-hash");
    expect(vm.tools.visible).toEqual(["browse_url", "exec_command"]);
    expect(vm.tools.risky).toEqual(["exec_command"]);
    expect(vm.tools.registryText).toContain("exec_command");
    expect(vm.fingerprints.find((item) => item.key === "stableHash").shortValue).toBe("40a0e0f6...bd6e4a");
  });

  it("accepts a JSON string", () => {
    const vm = parsePromptTrace(JSON.stringify(sampleTrace()));
    expect(vm.summary.sessionId).toBe("sess-1");
    expect(vm.layers).toHaveLength(5);
  });

  it("returns a warning view model for invalid JSON", () => {
    const vm = parsePromptTrace("{bad json");
    expect(vm.layers).toHaveLength(0);
    expect(vm.warnings.some((item) => item.severity === "danger")).toBe(true);
  });

  it("handles missing modelInput and promptFingerprint without throwing", () => {
    const vm = parsePromptTrace({
      sessionId: "sess-empty",
      visibleTools: ["browse_url"],
    });

    expect(vm.summary.sessionId).toBe("sess-empty");
    expect(vm.layers).toEqual([]);
    expect(vm.fingerprints.every((item) => item.missing)).toBe(true);
    expect(vm.warnings.map((item) => item.message).join("\n")).toContain("modelInput");
    expect(vm.warnings.map((item) => item.message).join("\n")).toContain("promptFingerprint");
  });

  it("warns when visible tools are empty", () => {
    const vm = parsePromptTrace(sampleTrace({ visibleTools: [] }));
    expect(vm.tools.visible).toEqual([]);
    expect(vm.warnings.some((item) => item.message.includes("visible tools"))).toBe(true);
  });

  it("warns when no user message is present", () => {
    const trace = sampleTrace({
      modelInput: sampleTrace().modelInput.filter((item) => item.providerRole !== "user"),
    });
    const vm = parsePromptTrace(trace);
    expect(vm.summary.hasUserMessage).toBe(false);
    expect(vm.warnings.some((item) => item.message.includes("没有 user message"))).toBe(true);
  });

  it("marks long prompt content", () => {
    const longContent = "x".repeat(20_001);
    const trace = sampleTrace({
      modelInput: [
        {
          index: 0,
          providerRole: "user",
          semanticRole: "user",
          promptLayer: "conversation",
          content: longContent,
        },
      ],
    });
    const vm = parsePromptTrace(trace);
    expect(vm.summary.promptCharCount).toBe(20_001);
    expect(vm.layers[0].warnings.some((item) => item.includes("content 较大"))).toBe(true);
    expect(vm.warnings.some((item) => item.message.includes("prompt 较大"))).toBe(true);
  });

  it("builds an Agent-to-UI source tree across session, user request, and LLM request", () => {
    const vm = parsePromptTrace(sampleTrace({
      caseId: "case-checkout-1",
      modelInput: [
        {
          index: 0,
          providerRole: "user",
          semanticRole: "user",
          promptLayer: "conversation",
          content: "上一轮：检查库存服务状态",
        },
        {
          index: 1,
          providerRole: "assistant",
          semanticRole: "assistant",
          promptLayer: "conversation",
          content: "上一轮已处理。",
        },
        {
          index: 2,
          providerRole: "user",
          semanticRole: "user",
          promptLayer: "conversation",
          content: "检查 checkout p95 延迟并生成图表",
        },
        {
          index: 3,
          providerRole: "assistant",
          semanticRole: "assistant",
          promptLayer: "conversation",
          content: "我会查询 Coroot 指标。",
          metadata: {
            llmRequestId: "llm-request-1",
          },
          toolCalls: [
            {
              id: "tool-call-coroot",
              type: "function",
              function: {
                name: "coroot.query_latency",
              },
            },
          ],
        },
        {
          index: 4,
          providerRole: "tool",
          semanticRole: "tool_result",
          promptLayer: "conversation",
          toolCallId: "tool-call-coroot",
          content: "checkout p95=2800ms",
        },
      ],
      toolCalls: [
        {
          id: "tool-call-coroot",
          name: "coroot.query_latency",
          llmRequestId: "llm-request-1",
        },
      ],
      artifacts: {
        "coroot-checkout-latency-chart": {
          artifact_id: "coroot-checkout-latency-chart",
          type: "coroot_chart",
          title: "Checkout p95 延迟图",
        },
      },
      agentUiArtifacts: [
        {
          artifact_id: "coroot-checkout-latency-chart",
          metadata: {
            llmRequestId: "llm-request-1",
            toolCallId: "tool-call-coroot",
            evidence_ref: "ev-coroot-latency",
            case_id: "case-checkout-1",
            redactionStatus: "redacted",
          },
        },
      ],
    }));

    expect(vm.agentUiSources.summary).toEqual({
      artifactCount: 1,
      userRequestCount: 1,
      llmRequestCount: 1,
    });
    expect(vm.agentUiSources.session).toMatchObject({
      id: "sess-1",
      caseId: "case-checkout-1",
    });
    expect(vm.agentUiSources.userRequests).toHaveLength(1);
    expect(vm.agentUiSources.userRequests[0]).toMatchObject({
      id: "turn-1",
      turnId: "turn-1",
      title: "用户请求",
      content: "检查 checkout p95 延迟并生成图表",
    });
    expect(vm.agentUiSources.userRequests[0].llmRequests).toHaveLength(1);
    expect(vm.agentUiSources.userRequests[0].llmRequests[0]).toMatchObject({
      id: "llm-request-1",
      label: "LLMRequest llm-request-1",
      toolCalls: [
        {
          id: "tool-call-coroot",
          name: "coroot.query_latency",
        },
      ],
      generatedArtifacts: [
        {
          artifactId: "coroot-checkout-latency-chart",
          type: "coroot_chart",
          title: "Checkout p95 延迟图",
          evidenceRef: "ev-coroot-latency",
          caseId: "case-checkout-1",
          redactionStatus: "redacted",
          redactionStatusLabel: "已脱敏",
          generatedBy: {
            kind: "tool_call",
            id: "tool-call-coroot",
            name: "coroot.query_latency",
            llmRequestId: "llm-request-1",
          },
        },
      ],
    });
  });

  it("builds LLM request details from common trace fields and redacts sensitive values by default", () => {
    const vm = parsePromptTrace({
      ...sampleTrace({
        prompt: {
          system: "system prompt token=sk-system-123",
          developer: "developer prompt password=dev-pass-123",
        },
        modelInput: [
          {
            index: 0,
            role: "user",
            content: "用户请求 api_key=ak-user-123",
          },
          {
            index: 1,
            role: "tool",
            toolCallId: "tool-call-search",
            content: "tool result cookie=session-cookie-123",
          },
        ],
      }),
      llmRequests: [
        {
          id: "llm-request-detail",
          request_body: {
            messages: [
              { role: "system", content: "system prompt token=sk-request-123" },
              { role: "developer", content: "developer prompt secret=dev-secret-123" },
              { role: "user", content: "用户 prompt password=user-pass-123" },
            ],
          },
          retrieval_context: ["checkout docs cookie=doc-cookie-123"],
          output: "模型输出 api key=ak-output-123",
          error: "请求失败 secret=err-secret-123",
          usage: { prompt_tokens: 12, completion_tokens: 7, total_tokens: 19 },
          duration_ms: 321,
          tool_messages: [{ content: "tool message request body={\"password\":\"body-pass-123\"}" }],
        },
      ],
    });

    const detail = vm.agentUiSources.userRequests[0].llmRequests[0].detail;

    expect(vm.agentUiSources.summary).toMatchObject({
      userRequestCount: 1,
      llmRequestCount: 1,
    });
    expect(detail).toMatchObject({
      systemPrompt: expect.stringContaining("[已脱敏]"),
      developerPrompt: expect.stringContaining("[已脱敏]"),
      userPrompt: expect.stringContaining("[已脱敏]"),
      toolMessages: expect.stringContaining("[已脱敏]"),
      retrievalContext: expect.stringContaining("[已脱敏]"),
      output: expect.stringContaining("[已脱敏]"),
      error: expect.stringContaining("[已脱敏]"),
      tokens: "prompt 12 / completion 7 / total 19",
      duration: "321 ms",
    });
    expect(JSON.stringify(vm)).not.toContain("sk-request-123");
    expect(JSON.stringify(vm)).not.toContain("dev-secret-123");
    expect(JSON.stringify(vm)).not.toContain("user-pass-123");
    expect(JSON.stringify(vm)).not.toContain("body-pass-123");
    expect(JSON.stringify(vm)).not.toContain("doc-cookie-123");
    expect(JSON.stringify(vm)).not.toContain("ak-output-123");
    expect(JSON.stringify(vm)).not.toContain("err-secret-123");
  });

  it("uses Chinese empty states when optional LLM detail fields are absent", () => {
    const vm = parsePromptTrace(sampleTrace({
      modelInput: [
        {
          index: 0,
          providerRole: "user",
          semanticRole: "user",
          promptLayer: "conversation",
          content: "只检查状态",
        },
      ],
      llmRequests: [{ id: "llm-empty" }],
    }));

    expect(vm.agentUiSources.userRequests[0].llmRequests[0].detail).toMatchObject({
      systemPrompt: "暂无 system prompt",
      developerPrompt: "暂无 developer prompt",
      toolMessages: "暂无 tool messages",
      retrievalContext: "暂无 retrieval context",
      output: "暂无输出",
      error: "暂无错误",
      tokens: "暂无 token 信息",
      duration: "暂无耗时",
    });
  });
});

describe("shortHash", () => {
  it("keeps short values as-is and shortens long values", () => {
    expect(shortHash("abc")).toBe("abc");
    expect(shortHash("1234567890abcdefghijklmnopqrstuvwxyz")).toBe("12345678...uvwxyz");
  });
});
