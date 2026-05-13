import { describe, expect, it } from "vitest";

import {
  normalizeAgentUIArtifact,
  normalizeAgentUIArtifacts,
  type AgentUIArtifact,
} from "./agentUiArtifacts";
import {
  agentUiArtifactFixtures,
  enterpriseAssistantDebugCases,
  experiencePackFixtures,
  hostLeaseFixtures,
  hostProfileFixtures,
} from "../data/enterpriseAssistantFixtures";

describe("normalizeAgentUIArtifact", () => {
  it("normalizes supported artifact types with stable base fields", () => {
    const artifact = normalizeAgentUIArtifact({
      id: "trace-slow-button",
      type: "trace_summary",
      title: "慢按钮链路摘要",
      summary: "提交按钮 p95 延迟升高到 2.8s。",
      status: "warning",
      severity: "high",
      createdAt: "2026-05-12T09:10:00+08:00",
      source: "agent",
      payload: {
        traceId: "trace-001",
        spans: [{ name: "POST /checkout", durationMs: 2800 }],
      },
      actions: [{ id: "open-trace", label: "打开 Trace", intent: "open" }],
    });

    expect(artifact).toMatchObject({
      id: "trace-slow-button",
      type: "trace_summary",
      title: "慢按钮链路摘要",
      summary: "提交按钮 p95 延迟升高到 2.8s。",
      status: "warning",
      severity: "high",
      source: "agent",
    });
    expect(artifact.payload).toEqual({
      traceId: "trace-001",
      spans: [{ name: "POST /checkout", durationMs: 2800 }],
    });
    expect(artifact.actions).toEqual([{ id: "open-trace", label: "打开 Trace", intent: "open" }]);
  });

  it("returns an unsupported safe artifact for unknown or invalid types", () => {
    const artifact = normalizeAgentUIArtifact({
      id: "raw-001",
      type: "shell_widget",
      title: "未知部件",
      summary: "后端返回了未登记的 UI artifact。",
      payload: { command: "rm -rf /tmp/nope" },
    });

    expect(artifact).toMatchObject({
      id: "raw-001",
      type: "unsupported",
      status: "unsupported",
      title: "未知部件",
      summary: "后端返回了未登记的 UI artifact。",
    });
    expect(artifact.originalType).toBe("shell_widget");
    expect(artifact.payload).toEqual({});
  });

  it("drops dangerous fields from nested payload, metadata, and actions", () => {
    const artifact = normalizeAgentUIArtifact({
      type: "workflow_result",
      title: "修复结果",
      html: "<img onerror=alert(1)>",
      payload: {
        step: "reload",
        innerHTML: "<b>unsafe</b>",
        nested: {
          script: "alert(1)",
          ok: true,
          items: [{ dangerouslySetInnerHTML: { __html: "bad" }, label: "保留" }],
        },
      },
      metadata: { owner: "main-agent", script: "steal()" },
      actions: [{ label: "查看", html: "<iframe />", params: { innerHTML: "bad", tab: "result" } }],
    });

    expect(JSON.stringify(artifact)).not.toContain("unsafe");
    expect(JSON.stringify(artifact)).not.toContain("steal");
    expect(JSON.stringify(artifact)).not.toContain("dangerouslySetInnerHTML");
    expect(artifact.payload).toEqual({
      step: "reload",
      nested: { ok: true, items: [{ label: "保留" }] },
    });
    expect(artifact.metadata).toEqual({ owner: "main-agent" });
    expect(artifact.actions[0]).toEqual({ label: "查看", params: { tab: "result" } });
  });

  it("preserves a normalized MCP card for Coroot chart artifacts", () => {
    const artifact = normalizeAgentUIArtifact({
      id: "coroot-nginx-latency",
      type: "coroot_chart",
      title: "Coroot 延迟趋势",
      payload: {
        metric: "request_latency",
        mcpUiCard: {
          id: "latency-card",
          uiKind: "readonly_chart",
          title: "web-checkout 延迟",
          visual: {
            kind: "timeseries",
            series: [{ name: "p95", data: [{ timestamp: 1778551200, value: 2.8 }] }],
          },
          placement: "side_panel",
        },
      },
    });

    expect(artifact.type).toBe("coroot_chart");
    expect(artifact.mcpCard).toMatchObject({
      id: "latency-card",
      uiKind: "readonly_chart",
      placement: "side_panel",
      title: "web-checkout 延迟",
      visual: { kind: "timeseries" },
    });
    expect(artifact.payload).toEqual({ metric: "request_latency" });
  });

  it("adds standard actions for case, evidence, and prompt trace links", () => {
    const artifact = normalizeAgentUIArtifact({
      id: "trace-linked-actions",
      type: "trace_summary",
      title: "慢请求 Trace",
      caseId: "case-debug-4",
      evidenceRef: "ev-trace-2",
      promptTraceId: "prompt-trace-2",
    });

    expect(artifact.caseId).toBe("case-debug-4");
    expect(artifact.evidenceRef).toBe("ev-trace-2");
    expect(artifact.promptTraceId).toBe("prompt-trace-2");
    expect(artifact.actions).toEqual([
      expect.objectContaining({ id: "view-case", label: "查看 Case", href: "/incidents/case-debug-4" }),
      expect.objectContaining({ id: "view-evidence", label: "查看证据", href: "/incidents/case-debug-4?evidence=ev-trace-2" }),
      expect.objectContaining({ id: "view-prompt-trace", label: "查看 Prompt Trace", href: "/debug/prompts?trace_id=prompt-trace-2" }),
    ]);
  });

  it("keeps Coroot state metadata while dropping dangerous HTML fields", () => {
    const artifact = normalizeAgentUIArtifact({
      id: "coroot-state-metadata",
      type: "coroot_chart",
      title: "Coroot 指标",
      status: "blocked",
      permissionScope: "restricted",
      redactionStatus: "redacted",
      html: "<img src=x onerror=alert(1)>",
      payload: {
        metric: "p95",
        script: "alert(1)",
        mcpUiCard: {
          uiKind: "readonly_chart",
          title: "web-checkout p95",
          error: "coroot unavailable",
          visual: {
            kind: "timeseries",
            series: [],
          },
        },
      },
    });

    expect(artifact).toMatchObject({
      status: "blocked",
      permissionScope: "restricted",
      redactionStatus: "redacted",
    });
    expect(artifact.mcpCard).toMatchObject({
      uiKind: "readonly_chart",
      error: "coroot unavailable",
      visual: { kind: "timeseries", series: [] },
    });
    expect(JSON.stringify(artifact)).not.toContain("onerror");
    expect(JSON.stringify(artifact)).not.toContain("alert(1)");
  });

  it("normalizes artifact arrays and skips nullish entries safely", () => {
    const artifacts = normalizeAgentUIArtifacts([
      null,
      { id: "a", type: "verification_result", title: "验证通过" },
      undefined,
      { id: "b", type: "experience_match", title: "经验包命中" },
    ]);

    expect(artifacts.map((item) => item.id)).toEqual(["a", "b"]);
    expect(artifacts.map((item) => item.type)).toEqual(["verification_result", "experience_match"]);
  });
});

describe("enterprise assistant fixtures", () => {
  it("keeps all artifact fixtures consumable by the normalize API", () => {
    const normalized = normalizeAgentUIArtifacts(agentUiArtifactFixtures);

    expect(normalized).toHaveLength(agentUiArtifactFixtures.length);
    expect(normalized.map((item) => item.type)).toEqual([
      "coroot_chart",
      "trace_summary",
      "workflow_result",
      "verification_result",
      "experience_match",
      "topology_slice",
    ]);
    expect(normalized.every((item: AgentUIArtifact) => item.title && item.summary)).toBe(true);
    expect(normalized[0].mcpCard?.uiKind).toBe("readonly_chart");
  });

  it("covers the requested debug, host, lease, and experience-pack states", () => {
    expect(enterpriseAssistantDebugCases.map((item) => item.id)).toEqual([
      "slow-button-debug-case",
      "pg-remediation-case",
    ]);
    expect(hostProfileFixtures.map((item) => item.state)).toEqual([
      "healthy",
      "offline",
      "expired",
      "label_conflict",
    ]);
    expect(hostLeaseFixtures.map((item) => item.state)).toEqual([
      "active",
      "conflict",
      "expired",
      "denied",
    ]);
    expect(experiencePackFixtures.map((item) => item.state)).toEqual([
      "candidate",
      "enabled_unauthorized",
      "authorized",
      "disabled",
    ]);
  });
});
