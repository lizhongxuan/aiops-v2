import type { ComponentType } from "react";
import { describe, expect, it } from "vitest";

import type { AiopsTransportAgentUiArtifact } from "@/transport/aiopsTransportTypes";
import { agentUiCardDefinitions, AGENT_UI_ARTIFACT_TYPES } from "./agentUiCardDefinitions";
import {
  createAgentUiCardRegistry,
  lookupAgentUiCardRenderer,
  type AgentUiCardRegistryDefinition,
} from "./agentUiCardRegistry";

const DummyRenderer: ComponentType<{ artifact: AiopsTransportAgentUiArtifact }> = () => null;

describe("agent UI card registry", () => {
  it("defines the tasklist artifact types", () => {
    expect(AGENT_UI_ARTIFACT_TYPES).toEqual([
      "coroot_chart",
      "trace_summary",
      "topology_slice",
      "rca_report",
      "workflow_result",
      "verification_result",
      "experience_match",
      "ops_manual_match",
      "ops_manual_search_result",
      "ops_manual_param_resolution",
      "ops_manual_param_form",
      "ops_manual_preflight_result",
      "ops_manual_fallback_guide",
      "runner_workflow_generation",
    ]);
    expect(agentUiCardDefinitions.map((definition) => definition.type)).toEqual(AGENT_UI_ARTIFACT_TYPES);
  });

  it("selects active and deprecated renderers through the lookup function", () => {
    const registry = createAgentUiCardRegistry([
      definition("trace_summary", "active", DummyRenderer),
      definition("workflow_result", "deprecated", DummyRenderer),
    ]);

    expect(lookupAgentUiCardRenderer(registry, artifact("trace_summary"))).toMatchObject({
      state: "active",
      Renderer: DummyRenderer,
    });
    expect(lookupAgentUiCardRenderer(registry, artifact("workflow_result"))).toMatchObject({
      state: "deprecated",
      Renderer: DummyRenderer,
      reason: "卡片类型已废弃，将使用兼容渲染器。",
    });
  });

  it("looks up renderers by renderer id and type plus schema version", () => {
    const registry = createAgentUiCardRegistry([
      {
        ...definition("observability.chart", "active", DummyRenderer),
        renderer: "observability.chart.v1",
        artifactTypes: ["observability.chart", "legacy_observability_chart"],
        schemaVersion: "observability.chart.v1",
      },
    ]);

    expect(lookupAgentUiCardRenderer(registry, {
      id: "by-renderer",
      type: "unknown_widget",
      renderer: "observability.chart.v1",
      payload: {},
    } as any)).toMatchObject({
      state: "active",
      Renderer: DummyRenderer,
    });
    expect(lookupAgentUiCardRenderer(registry, {
      id: "by-schema",
      type: "legacy_observability_chart",
      schemaVersion: "observability.chart.v1",
      payload: {},
    } as any)).toMatchObject({
      state: "active",
      Renderer: DummyRenderer,
    });
  });

  it("uses JSON fallback when a renderer id is present but unregistered", () => {
    const registry = createAgentUiCardRegistry([]);

    expect(lookupAgentUiCardRenderer(registry, {
      id: "unknown-renderer",
      type: "unknown_widget",
      renderer: "plugin.widget.v1",
      payload: { safe: true },
    } as any)).toMatchObject({
      state: "fallback_renderer",
      reason: "未注册的 renderer，已使用 JSON 摘要安全展示。",
    });
  });

  it("returns disabled, missing renderer, unsupported, and invalid-payload terminal states", () => {
    const registry = createAgentUiCardRegistry([
      definition("trace_summary", "disabled", DummyRenderer),
      definition("workflow_result", "active"),
    ]);

    expect(lookupAgentUiCardRenderer(registry, artifact("trace_summary"))).toMatchObject({
      state: "disabled",
      reason: "卡片类型已禁用。",
    });
    expect(lookupAgentUiCardRenderer(registry, artifact("workflow_result"))).toMatchObject({
      state: "missing_renderer",
      reason: "卡片类型已注册但未配置前端渲染器。",
    });
    expect(lookupAgentUiCardRenderer(registry, artifact("shell_widget"))).toMatchObject({
      state: "unsupported",
      reason: "未注册的卡片类型。",
    });
    expect(lookupAgentUiCardRenderer(registry, { id: "bad", type: "trace_summary", payload: "bad" } as any)).toMatchObject({
      state: "invalid_payload",
      reason: "卡片 payload 必须是对象。",
    });
  });
});

function definition(
  type: string,
  lifecycle: AgentUiCardRegistryDefinition["lifecycle"],
  Renderer?: ComponentType<{ artifact: AiopsTransportAgentUiArtifact }>,
): AgentUiCardRegistryDefinition {
  return {
    type,
    label: type,
    lifecycle,
    Renderer,
  };
}

function artifact(type: string): AiopsTransportAgentUiArtifact {
  return { id: type, type, payload: {} };
}
