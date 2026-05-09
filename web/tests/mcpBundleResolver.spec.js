import { describe, expect, it } from "vitest";
import {
  MCP_BUNDLE_PRESET_KEYS,
  buildMcpBundleCardCombos,
  buildMcpBundleSections,
  normalizeMcpBundleScope,
  resolveMcpBundlePreset,
  resolveMcpBundlePresetKey,
} from "../src/lib/mcpBundleResolver";

describe("mcpBundleResolver", () => {
  it("resolves middleware/service payloads to the monitor bundle preset", () => {
    const resolved = resolveMcpBundlePreset({
      subject: {
        type: "middleware",
        name: "redis",
        env: "prod",
      },
      summary: "redis-prod 当前存在连接抖动，但错误率仍可控",
      scope: {
        service: "redis",
        cluster: "prod-cn",
      },
    });

    expect(resolved.presetKey).toBe(MCP_BUNDLE_PRESET_KEYS.MIDDLEWARE_SERVICE_MONITOR);
    expect(resolved.bundleKind).toBe("monitor_bundle");
    expect(resolved.subjectType).toBe("middleware");
    expect(resolved.sectionConfig).toHaveLength(5);
    expect(resolved.sectionConfig[0]).toMatchObject({
      kind: "overview",
      title: "概览",
    });
    expect(resolved.cardCombos[0]).toMatchObject({
      sectionKind: "overview",
    });
    expect(resolved.sections[0]).toMatchObject({
      kind: "overview",
      title: "概览",
    });
    expect(resolved.sections[0].cards[0]).toMatchObject({
      uiKind: "readonly_summary",
      title: "当前状态",
    });
  });

  it("resolves root cause payloads to the remediation bundle preset", () => {
    const resolved = resolveMcpBundlePreset({
      rootCauseType: "upstream_timeout",
      rootCause: "upstream timeout 导致请求堆积",
      confidence: 0.88,
      recommendedActions: [
        {
          id: "action-card-1",
          uiKind: "action_panel",
          title: "扩容建议",
          summary: "先扩容再观察",
        },
      ],
    });

    expect(resolved.presetKey).toBe(MCP_BUNDLE_PRESET_KEYS.ROOT_CAUSE_REMEDIATION);
    expect(resolved.bundleKind).toBe("remediation_bundle");
    expect(resolved.rootCauseType).toBe("upstream_timeout");
    expect(resolved.sectionConfig).toHaveLength(5);
    expect(resolved.sections[0]).toMatchObject({
      kind: "root_cause",
      title: "根因",
    });
    expect(resolved.sections[2].cards[0]).toMatchObject({
      id: "action-card-1",
      uiKind: "action_panel",
      title: "扩容建议",
    });
  });

  it("parses string scope input and preserves normalized scope fields", () => {
    const scope = normalizeMcpBundleScope("service=redis env=prod middleware/service");

    expect(scope).toMatchObject({
      service: "redis",
      env: "prod",
      resourceType: "middleware",
      resourceId: "service",
    });
  });

  it("combines section config and card combos from preset defaults and explicit cards", () => {
    const presetKey = resolveMcpBundlePresetKey({
      subject: {
        type: "service",
      },
    });
    const sections = buildMcpBundleSections(
      { key: presetKey },
      {
        sections: [
          {
            kind: "alerts",
            cards: [
              {
                id: "alert-card-1",
                uiKind: "readonly_summary",
                title: "告警概览",
                summary: "当前 0 条高优先级告警",
              },
            ],
          },
        ],
      },
      {
        service: "redis",
      },
    );
    const cardCombos = buildMcpBundleCardCombos(
      { key: presetKey },
      {
        sections: [
          {
            kind: "alerts",
            cards: [
              {
                id: "alert-card-1",
                uiKind: "readonly_summary",
                title: "告警概览",
                summary: "当前 0 条高优先级告警",
              },
            ],
          },
        ],
      },
      {
        service: "redis",
      },
    );

    expect(sections).toHaveLength(5);
    expect(cardCombos).toHaveLength(5);
    expect(sections[2]).toMatchObject({
      kind: "alerts",
    });
    expect(sections[2].cards).toHaveLength(1);
    expect(sections[2].cards[0]).toMatchObject({
      id: "alert-card-1",
      title: "告警概览",
    });
  });
});

describe("Coroot toolName prefix routing", () => {
  it("routes coroot.topology toolName to COROOT_SERVICE_MONITOR", () => {
    const key = resolveMcpBundlePresetKey({ toolName: "coroot.topology" });
    expect(key).toBe(MCP_BUNDLE_PRESET_KEYS.COROOT_SERVICE_MONITOR);
  });

  it("routes coroot.host_overview toolName to COROOT_SERVICE_MONITOR", () => {
    const key = resolveMcpBundlePresetKey({ toolName: "coroot.host_overview" });
    expect(key).toBe(MCP_BUNDLE_PRESET_KEYS.COROOT_SERVICE_MONITOR);
  });

  it("routes coroot.service_overview toolName to COROOT_SERVICE_MONITOR", () => {
    const key = resolveMcpBundlePresetKey({ toolName: "coroot.service_overview" });
    expect(key).toBe(MCP_BUNDLE_PRESET_KEYS.COROOT_SERVICE_MONITOR);
  });

  it("routes coroot.alerts toolName to COROOT_SERVICE_MONITOR", () => {
    const key = resolveMcpBundlePresetKey({ toolName: "coroot.alerts" });
    expect(key).toBe(MCP_BUNDLE_PRESET_KEYS.COROOT_SERVICE_MONITOR);
  });

  it("routes coroot.rca toolName to COROOT_INCIDENT_RCA", () => {
    const key = resolveMcpBundlePresetKey({ toolName: "coroot.rca" });
    expect(key).toBe(MCP_BUNDLE_PRESET_KEYS.COROOT_INCIDENT_RCA);
  });

  it("routes coroot.rca.analyze toolName to COROOT_INCIDENT_RCA", () => {
    const key = resolveMcpBundlePresetKey({ toolName: "coroot.rca.analyze" });
    expect(key).toBe(MCP_BUNDLE_PRESET_KEYS.COROOT_INCIDENT_RCA);
  });

  it("routes coroot.metrics toolName to COROOT_SERVICE_MONITOR", () => {
    const key = resolveMcpBundlePresetKey({ toolName: "coroot.metrics" });
    expect(key).toBe(MCP_BUNDLE_PRESET_KEYS.COROOT_SERVICE_MONITOR);
  });

  it("routes coroot_host_overview bundleKind to COROOT_SERVICE_MONITOR", () => {
    const key = resolveMcpBundlePresetKey({ bundleKind: "coroot_host_overview" });
    expect(key).toBe(MCP_BUNDLE_PRESET_KEYS.COROOT_SERVICE_MONITOR);
  });

  it("routes coroot_topology bundleKind to COROOT_SERVICE_MONITOR", () => {
    const key = resolveMcpBundlePresetKey({ bundleKind: "coroot_topology" });
    expect(key).toBe(MCP_BUNDLE_PRESET_KEYS.COROOT_SERVICE_MONITOR);
  });

  it("routes via mcpServer containing coroot to COROOT_SERVICE_MONITOR", () => {
    const key = resolveMcpBundlePresetKey({ mcpServer: "coroot-mcp-server" });
    expect(key).toBe(MCP_BUNDLE_PRESET_KEYS.COROOT_SERVICE_MONITOR);
  });

  it("routes via source containing coroot to COROOT_SERVICE_MONITOR", () => {
    const key = resolveMcpBundlePresetKey({ source: "coroot" });
    expect(key).toBe(MCP_BUNDLE_PRESET_KEYS.COROOT_SERVICE_MONITOR);
  });

  it("routes coroot source with remediation signal to COROOT_INCIDENT_RCA", () => {
    const key = resolveMcpBundlePresetKey({
      source: "coroot",
      rootCauseType: "memory_leak",
    });
    expect(key).toBe(MCP_BUNDLE_PRESET_KEYS.COROOT_INCIDENT_RCA);
  });
});
