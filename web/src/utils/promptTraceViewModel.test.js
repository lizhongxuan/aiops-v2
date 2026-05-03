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
});

describe("shortHash", () => {
  it("keeps short values as-is and shortens long values", () => {
    expect(shortHash("abc")).toBe("abc");
    expect(shortHash("1234567890abcdefghijklmnopqrstuvwxyz")).toBe("12345678...uvwxyz");
  });
});
