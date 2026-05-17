import type { ComponentType } from "react";

import { OpsManualFallbackGuideArtifact, OpsManualMatchArtifact, OpsManualParamResolutionArtifact, OpsManualPreflightResultArtifact, OpsManualSearchResultArtifact, RunnerWorkflowGenerationArtifact } from "@/chat/components/OpsManualChatArtifacts";
import { CorootChartArtifact } from "@/components/chat/CorootChartArtifact";
import { ExperienceMatchArtifact } from "@/components/chat/ExperienceMatchArtifact";
import { RCAReportArtifact } from "@/components/chat/RCAReportArtifact";
import { TopologySliceArtifact } from "@/components/chat/TopologySliceArtifact";
import { TraceSummaryArtifact } from "@/components/chat/TraceSummaryArtifact";
import { VerificationResultArtifact } from "@/components/chat/VerificationResultArtifact";
import { WorkflowResultArtifact } from "@/components/chat/WorkflowResultArtifact";
import type { AiopsTransportAgentUiArtifact } from "@/transport/aiopsTransportTypes";
import { agentUiCardDefinitions, type AgentUiCardLifecycle } from "./agentUiCardDefinitions";

export type AgentUiCardRenderer = ComponentType<{ artifact: AiopsTransportAgentUiArtifact }>;

export type AgentUiCardRegistryDefinition = {
  type: string;
  label: string;
  lifecycle: AgentUiCardLifecycle;
  Renderer?: AgentUiCardRenderer;
};

export type AgentUiCardRegistry = {
  definitions: Map<string, AgentUiCardRegistryDefinition>;
};

export type AgentUiCardLookupResult =
  | { state: "active"; definition: AgentUiCardRegistryDefinition; Renderer: AgentUiCardRenderer; reason?: string }
  | { state: "deprecated"; definition: AgentUiCardRegistryDefinition; Renderer: AgentUiCardRenderer; reason: string }
  | { state: "disabled"; definition: AgentUiCardRegistryDefinition; reason: string }
  | { state: "missing_renderer"; definition: AgentUiCardRegistryDefinition; reason: string }
  | { state: "unsupported"; reason: string }
  | { state: "invalid_payload"; definition?: AgentUiCardRegistryDefinition; reason: string };

const DEFAULT_RENDERERS: Record<string, AgentUiCardRenderer> = {
  coroot_chart: CorootChartArtifact,
  trace_summary: TraceSummaryArtifact,
  topology_slice: TopologySliceArtifact,
  rca_report: RCAReportArtifact,
  workflow_result: WorkflowResultArtifact,
  verification_result: VerificationResultArtifact,
  experience_match: ExperienceMatchArtifact,
  ops_manual_match: OpsManualMatchArtifact,
  ops_manual_search_result: OpsManualSearchResultArtifact,
  ops_manual_param_resolution: OpsManualParamResolutionArtifact,
  ops_manual_param_form: OpsManualParamResolutionArtifact,
  ops_manual_preflight_result: OpsManualPreflightResultArtifact,
  ops_manual_fallback_guide: OpsManualFallbackGuideArtifact,
  runner_workflow_generation: RunnerWorkflowGenerationArtifact,
};

export function createAgentUiCardRegistry(definitions: AgentUiCardRegistryDefinition[]): AgentUiCardRegistry {
  return {
    definitions: new Map(definitions.map((definition) => [definition.type, definition])),
  };
}

export const defaultAgentUiCardRegistry = createAgentUiCardRegistry(
  agentUiCardDefinitions.map((definition) => ({
    ...definition,
    Renderer: DEFAULT_RENDERERS[definition.type],
  })),
);

export function lookupAgentUiCardRenderer(
  registry: AgentUiCardRegistry,
  artifact: AiopsTransportAgentUiArtifact,
): AgentUiCardLookupResult {
  const definition = registry.definitions.get(artifact.type);

  if (!isValidPayload(artifact.payload)) {
    return { state: "invalid_payload", definition, reason: "卡片 payload 必须是对象。" };
  }

  if (!definition) {
    return { state: "unsupported", reason: "未注册的卡片类型。" };
  }

  if (definition.lifecycle === "disabled") {
    return { state: "disabled", definition, reason: "卡片类型已禁用。" };
  }

  if (!definition.Renderer) {
    return { state: "missing_renderer", definition, reason: "卡片类型已注册但未配置前端渲染器。" };
  }

  if (definition.lifecycle === "deprecated") {
    return {
      state: "deprecated",
      definition,
      Renderer: definition.Renderer,
      reason: "卡片类型已废弃，将使用兼容渲染器。",
    };
  }

  return { state: "active", definition, Renderer: definition.Renderer };
}

function isValidPayload(payload: unknown): boolean {
  return payload === undefined || payload === null || (typeof payload === "object" && !Array.isArray(payload));
}
