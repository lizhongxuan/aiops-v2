import type { ComponentType } from "react";
import { describe, expect, it } from "vitest";

import type { AiopsTransportAgentUiArtifact } from "@/transport/aiopsTransportTypes";
import { agentUiCardDefinitions, AGENT_UI_ARTIFACT_TYPES } from "./agentUiCardDefinitions";
import {
  createAgentUiCardRegistry,
  defaultAgentUiCardRegistry,
  lookupAgentUiCardRenderer,
  type AgentUiCardRegistryDefinition,
} from "./agentUiCardRegistry";
import { WorkflowChangeArtifact } from "@/components/chat/WorkflowChangeArtifact";
import { WorkflowResultArtifact } from "@/components/chat/WorkflowResultArtifact";

const DummyRenderer: ComponentType<{ artifact: AiopsTransportAgentUiArtifact }> = () => null;

describe("agent UI card registry", () => {
  it("defines the tasklist artifact types", () => {
    expect(AGENT_UI_ARTIFACT_TYPES).toEqual([
      "coroot_chart",
      "trace_summary",
      "topology_slice",
      "rca_report",
      "workflow_result",
      "workflow_context",
      "workflow_edit_plan",
      "workflow_patch_preview",
      "workflow_patch_apply",
      "workflow_patch_result",
      "workflow_patch_validation",
      "workflow_conflict",
      "workflow_manual_candidate",
      "workflow_tool_timeline",
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

  it("routes the five workflow change contracts by exact type, renderer, and schema version", () => {
    const contracts = [
      ["workflow_edit_plan", "workflow.change.plan.v2", "aiops.workflow.plan.v1"],
      ["workflow_patch_preview", "workflow.change.preview.v2", "aiops.workflow.edit.v2"],
      ["workflow_patch_result", "workflow.change.receipt.v2", "aiops.workflow.edit.v2"],
      ["workflow_patch_validation", "workflow.change.validation.v2", "aiops.workflow.edit.v2"],
      ["workflow_conflict", "workflow.change.conflict.v2", "aiops.workflow.edit.v2"],
    ] as const;

    for (const [type, renderer, schemaVersion] of contracts) {
      expect(lookupAgentUiCardRenderer(defaultAgentUiCardRegistry, {
        id: `${type}:1`,
        type,
        renderer,
        schemaVersion,
        payload: {},
      })).toMatchObject({
        state: "active",
        Renderer: WorkflowChangeArtifact,
        definition: { type, renderer, schemaVersion, strictContract: true },
      });
    }
  });

  it("uses a local safe fallback when any workflow change contract coordinate is missing or unknown", () => {
    const base = {
      id: "preview:1",
      type: "workflow_patch_preview",
      renderer: "workflow.change.preview.v2",
      schemaVersion: "aiops.workflow.edit.v2",
      payload: { previewHash: "secret-hash" },
    };

    for (const artifact of [
      { ...base, renderer: "" },
      { ...base, renderer: "workflow.change.unknown.v2" },
      { ...base, schemaVersion: "" },
      { ...base, schemaVersion: "aiops.workflow.edit.v99" },
      { ...base, type: "workflow_conflict" },
    ]) {
      expect(lookupAgentUiCardRenderer(defaultAgentUiCardRegistry, artifact)).toMatchObject({
        state: "fallback_renderer",
        reason: "Workflow 变更卡片契约不受支持，已使用局部安全展示。",
      });
    }
  });

  it("keeps workflow_result on its dedicated execution result renderer", () => {
    expect(lookupAgentUiCardRenderer(defaultAgentUiCardRegistry, artifact("workflow_result"))).toMatchObject({
      state: "active",
      Renderer: WorkflowResultArtifact,
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
