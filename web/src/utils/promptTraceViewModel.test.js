import { describe, expect, it } from "vitest";
import { parsePromptTrace, shortHash } from "./promptTraceViewModel";

function sampleTrace(overrides = {}) {
  const {
    modelInput,
    visibleTools,
    toolSurface,
    stepContext,
    ...rest
  } = overrides;
  const defaultVisibleTools = ["browse_url", "exec_command"];
  const defaultModelInput = [
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
  ];
  const nextVisibleTools = visibleTools ?? toolSurface?.modelVisibleTools ?? defaultVisibleTools;
  return {
    schemaVersion: "aiops.trace/v2",
    kind: "runtime_model_input",
    caseId: "case-1",
    sessionId: "sess-1",
    turnId: "turn-1",
    iteration: 0,
    createdAt: "2026-05-02T05:42:25Z",
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
    toolSurface: {
      modelVisibleTools: nextVisibleTools,
      dispatchableTools: nextVisibleTools,
      hiddenReasons: {},
      ...toolSurface,
    },
    stepContext: {
      modelInput: modelInput ?? stepContext?.modelInput ?? defaultModelInput,
      ...stepContext,
    },
    ...rest,
  };
}

describe("parsePromptTrace", () => {
  it("reads provider request and raw payload refs from trace v2", () => {
    const vm = parsePromptTrace(JSON.stringify({
      schemaVersion: "aiops.trace/v2",
      sessionId: "session-1",
      turnId: "turn-1",
      iteration: 0,
      providerRequest: {
        modelInputHash: "mih",
        providerMessagesHash: "pmh",
        requestPropertiesHash: "rph",
        promptCacheKey: "cache",
      },
      toolSurface: {
        modelVisibleTools: ["exec_command"],
        dispatchableTools: ["exec_command"],
        hiddenReasons: {},
      },
      rawPayloadRefs: [{ id: "raw-request", kind: "provider_request", path: "raw/raw-request.json" }],
      stepContext: {
        modelInput: [{ id: "history-1", providerRole: "user", semanticRole: "user", content: "hello" }],
      },
    }));

    expect(vm.summary.schemaVersion).toBe("aiops.trace/v2");
    expect(vm.providerRequest.modelInputHash).toBe("mih");
    expect(vm.rawPayloadRefs).toHaveLength(1);
    expect(vm.toolSurface.summary.visibleCount).toBe(1);
    expect(vm.summary.messageCount).toBe(1);
  });

  it("reads top-level modelInput when stepContext is hash-only", () => {
    const vm = parsePromptTrace({
      schemaVersion: "aiops.trace/v2",
      sessionId: "session-step-hash",
      turnId: "turn-step-hash",
      stepContext: { hash: "step-hash", turnAssemblyHash: "assembly-hash" },
      modelInput: [{ id: "user-1", providerRole: "user", semanticRole: "user", content: "hello" }],
      toolSurface: { modelVisibleTools: [], dispatchableTools: [] },
    });

    expect(vm.layers).toHaveLength(1);
    expect(vm.layers[0].content).toBe("hello");
    expect(vm.warnings.some((item) => item.message.includes("没有 modelInput"))).toBe(false);
  });

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

  it("projects canonical control facts without exposing dynamic payload text", () => {
    const vm = parsePromptTrace(sampleTrace({
      previousPromptFingerprint: {
        absoluteSystemHash: "hash-absolute-system",
        roleProfileHash: "hash-role-profile-old",
        stableRuntimeContractHash: "hash-stable-runtime",
        stablePrefixHash: "hash-stable-prefix",
        turnStableHash: "hash-turn-stable",
        conversationHistoryHash: "hash-history",
        dynamicContextHash: "hash-dynamic-old",
        currentUserInputHash: "hash-user-input",
        modelInputHash: "hash-model-old",
      },
      promptFingerprint: {
        absoluteSystemHash: "hash-absolute-system",
        roleProfileHash: "hash-role-profile",
        stableRuntimeContractHash: "hash-stable-runtime",
        stablePrefixHash: "hash-stable-prefix",
        turnStableHash: "hash-turn-stable",
        conversationHistoryHash: "hash-history",
        dynamicContextHash: "hash-dynamic",
        currentUserInputHash: "hash-user-input",
        modelInputHash: "hash-model",
      },
      stepContext: {
        ...sampleTrace().stepContext,
        hash: "hash-step-context",
        turnAssemblyHash: "hash-turn-assembly",
        checkpointRef: "checkpoint:pending-1",
        toolPolicyHash: "hash-tool-policy",
        stepReference: {
          transition: {
            revisions: [{ kind: "approval_resumed" }],
          },
        },
      },
      toolSurface: {
        modelVisibleTools: ["inspect_service", "restart_service"],
        dispatchableTools: ["inspect_service"],
        hiddenReasons: { restart_service: "approval_required" },
      },
      controlChain: {
        schemaVersion: "aiops.canonical-rollout.v1",
        sessionId: "sess-1",
        turnId: "turn-1",
        available: true,
        headRef: { sequence: 3, eventId: "event:3", hash: "hash-event-3" },
        events: [
          {
            sequence: 3,
            kind: "approval_decided",
            eventId: "event:3",
            hash: "hash-event-3",
            owner: "approval",
            turnAssemblyHash: "hash-turn-assembly",
            stepContextHash: "hash-step-context",
            stepRevisionKind: "approval_resumed",
            sourceRefs: ["checkpoint:pending-1"],
            payloadRefs: {
              approvalId: "approval-1",
              mismatchFields: ["permission"],
              checkpointRef: "checkpoint:pending-1",
            },
            payload: { secret: "must-not-reach-view-model" },
          },
          {
            sequence: 1,
            kind: "admission",
            eventId: "event:1",
            hash: "hash-event-1",
            ownerModule: "appui.admission",
          },
        ],
      },
    }));

    expect(vm.controlFacts).toMatchObject({
      turnAssemblyHash: "hash-turn-assembly",
      stepContextHash: "hash-step-context",
      stepRevisionKind: "approval_resumed",
      checkpointRef: "checkpoint:pending-1",
      rolloutRef: {
        sequence: 3,
        eventId: "event:3",
        hash: "hash-event-3",
        ref: "hash-event-3",
      },
    });
    expect(vm.promptHashes.stable.map((item) => item.key)).toEqual([
      "absoluteSystemHash",
      "roleProfileHash",
      "stableRuntimeContractHash",
      "stablePrefixHash",
      "turnStableHash",
    ]);
    expect(vm.promptHashes.dynamic.map((item) => item.key)).toEqual([
      "conversationHistoryHash",
      "dynamicContextHash",
      "currentUserInputHash",
      "modelInputHash",
    ]);
    expect(vm.promptHashes.all.map((item) => item.layer)).toEqual(["L0", "L1", "L2", "L0-L2", "L3", "L4", "L5", "L6", "L0-L6"]);
    expect(vm.promptHashes.stable.find((item) => item.key === "absoluteSystemHash").change).toBe("unchanged");
    expect(vm.promptHashes.stable.find((item) => item.key === "roleProfileHash").change).toBe("changed");
    expect(vm.promptHashes.dynamic.find((item) => item.key === "dynamicContextHash").change).toBe("changed");
    expect(vm.toolControl).toMatchObject({
      modelVisible: ["inspect_service", "restart_service"],
      dispatchable: ["inspect_service"],
      hidden: [{ tool: "restart_service", reason: "approval_required" }],
      policyHash: "hash-tool-policy",
      diff: {
        visibleNotDispatchable: ["restart_service"],
        dispatchableNotVisible: [],
      },
    });
    expect(vm.controlChain.events.map((event) => event.sequence)).toEqual([1, 3]);
    expect(vm.controlChain.firstDivergence).toMatchObject({
      sequence: 3,
      kind: "approval_decided",
      ownerModule: "approval",
      mismatchFields: ["permission"],
    });
    expect(vm.approvalControl).toMatchObject({
      approvalId: "approval-1",
      mismatchFields: ["permission"],
      checkpointRef: "checkpoint:pending-1",
      rolloutRef: {
        sequence: 3,
        eventId: "event:3",
        hash: "hash-event-3",
        ref: "hash-event-3",
      },
    });
    expect(vm.controlChain.events[1].payloadRefs).toEqual(expect.arrayContaining([
      expect.objectContaining({ key: "mismatchFields", ref: "permission" }),
    ]));
    expect(JSON.stringify(vm.controlChain)).not.toContain("must-not-reach-view-model");
  });

  it("does not infer a replay divergence from failed tool status text", () => {
    const vm = parsePromptTrace(sampleTrace({
      controlChain: {
        schemaVersion: "aiops.canonical-rollout.v1",
        sessionId: "sess-1",
        turnId: "turn-1",
        available: true,
        events: [{
          sequence: 1,
          kind: "tool_result",
          eventId: "event:tool-result",
          hash: "hash:tool-result",
          owner: "router",
          payload: { outcome: "failed", errorClass: "tool_not_found" },
          payloadRefs: { callId: "call-1", name: "missing_tool" },
        }],
      },
    }));

    expect(vm.controlChain.firstDivergence).toBeNull();
    expect(vm.controlChain.events[0].isDivergence).toBe(false);
  });

  it("uses stable empty control-chain fallbacks when optional fields are absent", () => {
    const vm = parsePromptTrace(sampleTrace());

    expect(vm.controlFacts).toEqual({
      turnAssemblyHash: "",
      stepContextHash: "",
      stepRevisionKind: "",
      checkpointRef: "",
      rolloutRef: { sequence: null, eventId: "", hash: "", ref: "" },
    });
    expect(vm.controlChain.events).toEqual([]);
    expect(vm.controlChain.firstDivergence).toBeNull();
    expect(vm.approvalControl.mismatchFields).toEqual([]);
    expect(vm.promptHashes.all).toHaveLength(9);
    expect(vm.promptHashes.all.every((item) => item.change === "unknown")).toBe(true);
  });

  it("accepts a JSON string", () => {
    const vm = parsePromptTrace(JSON.stringify(sampleTrace()));
    expect(vm.summary.sessionId).toBe("sess-1");
    expect(vm.layers).toHaveLength(5);
  });

  it("parses special input world state from prompt input trace", () => {
    const vm = parsePromptTrace(sampleTrace({
      promptInputTrace: {
        specialInputWorldState: {
          schemaVersion: "aiops.special_input_memory.v1",
          turnId: "turn-special",
          modelSummary: "active host host-a from previous confirmed mention",
          activeExecutionScope: {
            id: "grant-host-a",
            resourceKind: "host",
            resourceId: "host-a",
            allowedActions: ["inspect", "read"],
            validationHash: "validation-hash-a",
          },
          activeRoleBindings: [{
            id: "role-primary",
            roleKey: "pg_primary",
            runtimeName: "primary",
            resourceKind: "host",
            resourceId: "host-a",
            environmentKey: "prod",
            clusterKey: "orders",
            bindingHash: "role-hash-a",
          }],
          pendingConfirmations: [{
            id: "pending-raw",
            kind: "target",
            reason: "raw_typed_requires_confirmation",
            candidateIds: ["raw-1"],
          }],
          conflicts: [{
            id: "conflict-role",
            kind: "role_binding",
            roleKey: "pg_primary",
            environmentKey: "prod",
            clusterKey: "orders",
            resourceIds: ["host-a", "host-b"],
            reasons: ["unique_role_bound_to_multiple_resources"],
          }],
          readPlan: {
            activeGrantId: "grant-host-a",
            activeResourceKind: "host",
            activeResourceId: "host-a",
            pendingConfirmationIds: ["pending-raw"],
          },
        },
      },
    }));

    expect(vm.specialInput.summary.hasActiveGrant).toBe(true);
    expect(vm.specialInput.activeGrant.resourceId).toBe("host-a");
    expect(vm.specialInput.roleBindings[0].bindingHash).toBe("role-hash-a");
    expect(vm.specialInput.pendingConfirmations[0].reason).toBe("raw_typed_requires_confirmation");
    expect(vm.specialInput.conflicts[0].resourceIds).toEqual(["host-a", "host-b"]);
    expect(vm.specialInput.readPlan.activeGrantId).toBe("grant-host-a");
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
      modelInput: sampleTrace().stepContext.modelInput.filter((item) => item.providerRole !== "user"),
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

  it("builds environment context from aiops metadata and redacts secrets", () => {
    const vm = parsePromptTrace(sampleTrace({
      metadata: {
        "aiops.target.refs": "host:10.0.0.1,service:checkout",
        "aiops.env.readOnlyReason": "target_conflict_requires_clarification token=env-reason-secret",
        "aiops.env.compactContext": [
          "EnvironmentFactsContext:",
          "ConfirmedFacts:",
          "- host_identity=10.0.0.1 source=user_explicit",
          "ConflictFacts:",
          "- target_conflict service:checkout -> host:10.0.0.2 password=env-compact-secret",
        ].join("\n"),
      },
    }));

    expect(vm.contextGovernance.environmentContext).toMatchObject({
      targetRefs: ["host:10.0.0.1", "service:checkout"],
      hasConflict: true,
    });
    expect(vm.contextGovernance.environmentContext.readOnlyReason).toContain("target_conflict_requires_clarification");
    expect(vm.contextGovernance.environmentContext.compactContext).toContain("ConflictFacts");
    expect(vm.contextGovernance.environmentContext.compactContext).not.toContain("env-compact-secret");
    expect(vm.contextGovernance.environmentContext.readOnlyReason).not.toContain("env-reason-secret");
  });

  it("builds LLM request details from common trace fields and redacts sensitive values by default", () => {
    const vm = parsePromptTrace({
      ...sampleTrace({
        prompt: {
          system: "system prompt token=sk-system-123",
          developer: "developer prompt password=dev-pass-123",
          tools: "## browse_url\nRead URL.\n\n## exec_command\nRun command.",
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
          first_delta_ms: 31000,
          stream_ms: 65000,
          delta_count: 1201,
          output_chars: 7001,
          finishReason: "stop",
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
      finishReason: "stop",
      metrics: {
        durationMs: 321,
        firstDeltaMs: 31000,
        streamMs: 65000,
        deltaCount: 1201,
        outputChars: 7001,
      },
    });
    expect(vm.summary.toolRegistryCharCount).toBeGreaterThan(0);
    expect(detail.slowCauses.map((item) => item.label)).toEqual(expect.arrayContaining(["首 token 慢", "输出过长", "流式碎片过多"]));
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

  it("keeps tool calls returned directly by an LLM request", () => {
    const vm = parsePromptTrace(sampleTrace({
      visibleTools: ["web_search"],
      llmRequests: [
        {
          id: "llm-tool-call",
          finishReason: "tool_calls",
          usage: { prompt_tokens: 9, completion_tokens: 4, total_tokens: 13 },
          toolCalls: [
            {
              id: "call-web-search",
              name: "web_search",
              arguments: "{\"query\":\"PostgreSQL timeline pgBackRest api_key=tool-secret\"}",
            },
          ],
        },
      ],
    }));

    const request = vm.agentUiSources.userRequests[0].llmRequests[0];

    expect(request.detail).toMatchObject({
      finishReason: "tool_calls",
      output: "暂无输出",
      hasOutput: false,
      tokens: "prompt 9 / completion 4 / total 13",
    });
    expect(request.toolCalls).toHaveLength(1);
    expect(request.toolCalls[0]).toMatchObject({
      id: "call-web-search",
      name: "web_search",
      arguments: expect.stringContaining("PostgreSQL timeline pgBackRest"),
      llmRequestId: "llm-tool-call",
    });
    expect(JSON.stringify(vm)).not.toContain("tool-secret");
  });

  it("parses context governance events into budget, compaction, materialization, and external references", () => {
    const vm = parsePromptTrace(sampleTrace({
      contextGovernance: [
        {
          id: "cg-budget-1",
          layer: "L4",
          kind: "context.compaction.started",
          message: "compacting context token=secret-context-token",
          budget: {
            maxContextTokens: 200000,
            autoCompactThreshold: 167000,
            blockingLimit: 177000,
          },
          referenceIds: ["ref-1", "ref-2"],
          compactedIds: ["segment-1"],
          droppedGroupIds: ["old-tool-results"],
        },
        {
          id: "cg-materialized-1",
          layer: "L5",
          kind: "tool_result.materialized",
          message: "large tool result externalized",
          toolCallId: "call-large-logs",
          toolName: "exec_command",
          materializationTier: "large",
          originalBytes: 49152,
          inlineBytes: 512,
          referenceIds: ["tool-ref-1"],
        },
      ],
    }));

    expect(vm.contextGovernance.summary).toMatchObject({
      eventCount: 2,
      budgetEventCount: 1,
      compactionEventCount: 1,
      materializationEventCount: 1,
      externalReferenceCount: 3,
      hasCompaction: true,
      hasMaterialization: true,
      hasExternalReferences: true,
    });
    expect(vm.contextGovernance.events[0]).toMatchObject({
      id: "cg-budget-1",
      layer: "L4",
      kind: "context.compaction.started",
      retryLabel: "",
      referenceIds: ["ref-1", "ref-2"],
      compactedIds: ["segment-1"],
      droppedGroupIds: ["old-tool-results"],
      hasCompaction: true,
    });
    expect(vm.contextGovernance.budgetEvents[0].budgetItems).toEqual([
      { key: "autoCompactThreshold", label: "Auto Compact", value: 167000 },
      { key: "blockingLimit", label: "Blocking Limit", value: 177000 },
      { key: "maxContextTokens", label: "Max Context", value: 200000 },
    ]);
    expect(vm.contextGovernance.materializationEvents[0]).toMatchObject({
      id: "cg-materialized-1",
      toolCallId: "call-large-logs",
      toolName: "exec_command",
      materializationTier: "large",
      originalBytes: 49152,
      inlineBytes: 512,
      hasMaterialization: true,
    });
    expect(vm.contextGovernance.externalReferences.map((item) => item.referenceId)).toEqual(["ref-1", "ref-2", "tool-ref-1"]);
    expect(JSON.stringify(vm.contextGovernance)).not.toContain("secret-context-token");
    expect(JSON.stringify(vm.contextGovernance)).toContain("[已脱敏]");
  });

  it("keeps WebLearn external knowledge separate from ToolSearch environment facts", () => {
    const vm = parsePromptTrace(sampleTrace({
      webLearnEvidence: [
        {
          id: "wl-redis-1",
          kind: "external_knowledge",
          query: "redis latency doctor token=weblearn-query-secret",
          sourceTitle: "Redis latency official docs",
          sourceURL: "https://redis.io/docs/latest/operate/oss_and_stack/management/optimization/latency/",
          sourceKind: "official_docs",
          product: "redis",
          version: "7.2",
          applicability: "matches Redis 7 latency diagnosis",
          confidence: "high",
          relevantExcerpt: "Use LATENCY DOCTOR to inspect latency events password=weblearn-pass",
        },
      ],
      toolSurfaceTrace: {
        toolSearchEvents: [
          {
            mode: "search",
            query: "redis latency",
            request: {
              environmentFacts: ["host redis version token=env-secret"],
            },
          },
        ],
      },
    }));

    expect(vm.contextGovernance.summary).toMatchObject({
      externalKnowledgeCount: 1,
      hasExternalKnowledge: true,
    });
    expect(vm.contextGovernance.externalKnowledgeEvidence[0]).toMatchObject({
      id: "wl-redis-1",
      kind: "external_knowledge",
      sourceKind: "official_docs",
      product: "redis",
      version: "7.2",
      confidence: "high",
    });
    expect(vm.contextGovernance.externalKnowledgeEvidence[0].query).toContain("[已脱敏]");
    expect(vm.contextGovernance.externalKnowledgeEvidence[0].relevantExcerpt).toContain("[已脱敏]");
    expect(vm.toolSurface.toolSearchEvents[0].environmentFacts[0]).toContain("[已脱敏]");
    expect(vm.contextGovernance.externalKnowledgeEvidence[0].relevantExcerpt).not.toContain("env-secret");
  });

  it("returns a stable empty context governance view model", () => {
    const vm = parsePromptTrace(sampleTrace());

    expect(vm.contextGovernance.emptyText).toBe("暂无上下文治理事件");
    expect(vm.contextGovernance.summary).toMatchObject({
      eventCount: 0,
      hasCompaction: false,
      hasMaterialization: false,
      hasExternalReferences: false,
      hasExternalKnowledge: false,
    });
    expect(vm.contextGovernance.events).toEqual([]);
    expect(vm.contextGovernance.budgetEvents).toEqual([]);
    expect(vm.contextGovernance.compactionEvents).toEqual([]);
    expect(vm.contextGovernance.materializationEvents).toEqual([]);
    expect(vm.contextGovernance.externalReferences).toEqual([]);
    expect(vm.contextGovernance.externalKnowledgeEvidence).toEqual([]);
  });

  it("parses tool surface trace for Prompt Trace UI and redacts sensitive reasons", () => {
    const vm = parsePromptTrace(sampleTrace({
      visibleTools: ["exec_command", "tool_search", "generic.metrics.read"],
      toolSurfaceTrace: {
        initialTools: ["exec_command", "tool_search"],
        baseRegistryCount: 2,
        deferredFamilies: [
          {
            pack: "external_metrics",
            capability: "metrics",
            source: "mcp",
            mcpServerId: "observability",
            healthStatus: "unavailable",
            unavailableReason: "connect https://user:surface-pass@metrics.example.internal/api failed token=surface-secret",
            toolCount: 4,
          },
        ],
        loadedTools: ["generic.metrics.read"],
        loadedPacks: ["generic_metrics"],
        filteredTools: [
          { toolName: "external.metrics.read", reason: "mcp_unavailable password=filtered-secret" },
        ],
        mcpHealth: {
          observability: "unavailable: https://user:health-pass@metrics.example.internal/api",
        },
        toolSearchEvents: [
          {
            mode: "search",
            query: "metrics token=query-secret",
            ranker: "bm25",
            matchCount: 1,
            rejectedCount: 3,
            matches: ["generic.metrics.read"],
            targetCompatibility: "matched",
            riskDecision: "allowed",
            matchReasons: ["bm25", "target_compatible", "risk_allowed", "environment_fact_match"],
            request: {
              intent: "rca",
              targetRefs: ["service:checkout"],
              requiredCaps: ["read"],
              forbiddenCaps: ["execute"],
              riskLevel: "low",
              environmentFacts: ["checkout service p95 latency token=env-secret"],
              mcpHealth: {
                observability: "unavailable",
              },
            },
            rejectedReasons: [
              {
                toolName: "external.metrics.read",
                reason: "mcp_unavailable password=search-reject-secret",
                mcpServerId: "observability",
                healthStatus: "unavailable",
              },
              {
                toolName: "host.logs",
                reason: "target_incompatible",
              },
              {
                toolName: "service.restart",
                reason: "risk_exceeds_request",
              },
            ],
          },
        ],
        selectedTools: ["generic.metrics.read"],
        rejectedToolReasons: [
          { toolName: "external.metrics.read", errorType: "mcp_unavailable", reason: "api_key=reject-secret" },
        ],
      },
    }));

    expect(vm.toolSurface.summary).toMatchObject({
      initialToolCount: 2,
      baseRegistryCount: 2,
      deferredFamilyCount: 1,
      loadedToolCount: 1,
      loadedPackCount: 1,
      filteredToolCount: 1,
      mcpHealthCount: 1,
      toolSearchEventCount: 1,
      selectedToolCount: 1,
      rejectedToolReasonCount: 4,
    });
    expect(vm.toolSurface.initialTools).toEqual(["exec_command", "tool_search"]);
    expect(vm.toolSurface.deferredFamilies[0]).toMatchObject({
      pack: "external_metrics",
      mcpServerId: "observability",
      healthStatus: "unavailable",
      toolCount: 4,
    });
    expect(vm.toolSurface.mcpHealth[0]).toMatchObject({
      serverId: "observability",
    });
    expect(vm.toolSurface.mcpHealth[0].status).toContain("metrics.example.internal");
    expect(vm.toolSurface.filteredTools[0]).toMatchObject({
      toolName: "external.metrics.read",
    });
    expect(vm.toolSurface.rejectedToolReasons[0]).toMatchObject({
      toolName: "external.metrics.read",
      errorType: "mcp_unavailable",
    });
    expect(vm.toolSurface.toolSearchEvents[0]).toMatchObject({
      mode: "search",
      ranker: "bm25",
      intent: "rca",
      matchCount: 1,
      rejectedCount: 3,
      matches: ["generic.metrics.read"],
      targetCompatibility: "matched",
      riskDecision: "allowed",
      targetRefs: ["service:checkout"],
      requiredCaps: ["read"],
      forbiddenCaps: ["execute"],
      riskLevel: "low",
    });
    expect(vm.toolSurface.toolSearchEvents[0].matchReasons).toContain("environment_fact_match");
    expect(vm.toolSurface.toolSearchEvents[0].environmentFacts[0]).toContain("[已脱敏]");
    expect(vm.toolSurface.toolSearchEvents[0].mcpHealth[0]).toMatchObject({
      serverId: "observability",
      status: "unavailable",
    });
    expect(vm.toolSurface.toolSearchEvents[0].rejectedReasons[0]).toMatchObject({
      toolName: "external.metrics.read",
      mcpServerId: "observability",
      healthStatus: "unavailable",
    });
    expect(vm.toolSurface.toolSearchEvents[0].rejectedReasons[0].reason).toContain("mcp_unavailable");
    expect(vm.toolSurface.toolSearchEvents[0].rejectedReasons[0].reason).toContain("[已脱敏]");
    expect(vm.toolSurface.rejectedToolReasons.some((entry) => entry.source === "tool_search" && entry.reason.includes("mcp_unavailable") && entry.reason.includes("[已脱敏]"))).toBe(true);
    expect(vm.toolSurface.rejectedToolReasons.some((entry) => entry.source === "tool_search" && entry.reason === "target_incompatible")).toBe(true);
    expect(vm.toolSurface.rejectedToolReasons.some((entry) => entry.source === "tool_search" && entry.reason === "risk_exceeds_request")).toBe(true);
    expect(JSON.stringify(vm.toolSurface)).not.toContain("surface-pass");
    expect(JSON.stringify(vm.toolSurface)).not.toContain("surface-secret");
    expect(JSON.stringify(vm.toolSurface)).not.toContain("filtered-secret");
    expect(JSON.stringify(vm.toolSurface)).not.toContain("health-pass");
    expect(JSON.stringify(vm.toolSurface)).not.toContain("query-secret");
    expect(JSON.stringify(vm.toolSurface)).not.toContain("reject-secret");
    expect(JSON.stringify(vm.toolSurface)).not.toContain("search-reject-secret");
    expect(JSON.stringify(vm.toolSurface)).not.toContain("env-secret");
    expect(JSON.stringify(vm.toolSurface)).toContain("[已脱敏]");
  });
});

describe("shortHash", () => {
  it("keeps short values as-is and shortens long values", () => {
    expect(shortHash("abc")).toBe("abc");
    expect(shortHash("1234567890abcdefghijklmnopqrstuvwxyz")).toBe("12345678...uvwxyz");
  });
});
