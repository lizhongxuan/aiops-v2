import type { ComponentType } from "react";

import { OpsManualFallbackGuideArtifact, OpsManualMatchArtifact, OpsManualParamResolutionArtifact, OpsManualPreflightResultArtifact, OpsManualSearchResultArtifact, RunnerWorkflowGenerationArtifact } from "@/chat/components/OpsManualChatArtifacts";
import { CorootChartArtifact } from "@/components/chat/CorootChartArtifact";
import { ExperienceMatchArtifact } from "@/components/chat/ExperienceMatchArtifact";
import { RCAReportArtifact } from "@/components/chat/RCAReportArtifact";
import { TopologySliceArtifact } from "@/components/chat/TopologySliceArtifact";
import { TraceSummaryArtifact } from "@/components/chat/TraceSummaryArtifact";
import { VerificationResultArtifact } from "@/components/chat/VerificationResultArtifact";
import { WorkflowChangeArtifact } from "@/components/chat/WorkflowChangeArtifact";
import { WorkflowResultArtifact } from "@/components/chat/WorkflowResultArtifact";
import type { AiopsTransportAgentUiArtifact } from "@/transport/aiopsTransportTypes";
import { agentUiCardDefinitions, type AgentUiCardDisplay, type AgentUiCardLifecycle } from "./agentUiCardDefinitions";

export type AgentUiCardRenderer = ComponentType<{ artifact: AiopsTransportAgentUiArtifact }>;

export type AgentUiCardRegistryDefinition = {
  type: string;
  label: string;
  lifecycle: AgentUiCardLifecycle;
  renderer?: string;
  artifactTypes?: string[];
  schemaVersion?: string;
  component?: string;
  fallback?: string;
  strictContract?: boolean;
  display?: AgentUiCardDisplay;
  Renderer?: AgentUiCardRenderer;
};

export type AgentUiCardRegistry = {
  definitions: Map<string, AgentUiCardRegistryDefinition>;
  renderers: Map<string, AgentUiCardRegistryDefinition>;
  schemas: Map<string, AgentUiCardRegistryDefinition>;
};

export type AgentUiCardLookupResult =
  | { state: "active"; definition: AgentUiCardRegistryDefinition; Renderer: AgentUiCardRenderer; reason?: string }
  | { state: "deprecated"; definition: AgentUiCardRegistryDefinition; Renderer: AgentUiCardRenderer; reason: string }
  | { state: "disabled"; definition: AgentUiCardRegistryDefinition; reason: string }
  | { state: "fallback_renderer"; definition?: AgentUiCardRegistryDefinition; Renderer: AgentUiCardRenderer; reason: string }
  | { state: "missing_renderer"; definition: AgentUiCardRegistryDefinition; reason: string }
  | { state: "unsupported"; reason: string }
  | { state: "invalid_payload"; definition?: AgentUiCardRegistryDefinition; reason: string };

const DEFAULT_RENDERERS: Record<string, AgentUiCardRenderer> = {
  coroot_chart: CorootChartArtifact,
  trace_summary: TraceSummaryArtifact,
  topology_slice: TopologySliceArtifact,
  rca_report: RCAReportArtifact,
  workflow_result: WorkflowResultArtifact,
  workflow_edit_plan: WorkflowChangeArtifact,
  workflow_patch_preview: WorkflowChangeArtifact,
  workflow_patch_result: WorkflowChangeArtifact,
  workflow_patch_validation: WorkflowChangeArtifact,
  workflow_conflict: WorkflowChangeArtifact,
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
  const definitionsByType = new Map<string, AgentUiCardRegistryDefinition>();
  const definitionsByRenderer = new Map<string, AgentUiCardRegistryDefinition>();
  const definitionsBySchema = new Map<string, AgentUiCardRegistryDefinition>();
  for (const definition of definitions) {
    definitionsByType.set(definition.type, definition);
    if (definition.renderer) {
      definitionsByRenderer.set(definition.renderer, definition);
    }
    if (definition.schemaVersion) {
      for (const artifactType of definition.artifactTypes?.length ? definition.artifactTypes : [definition.type]) {
        definitionsBySchema.set(schemaKey(artifactType, definition.schemaVersion), definition);
      }
    }
  }
  return {
    definitions: definitionsByType,
    renderers: definitionsByRenderer,
    schemas: definitionsBySchema,
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
  const rendererID = artifactRendererID(artifact);
  const schemaVersion = artifactSchemaVersion(artifact);
  const definitionByType = registry.definitions.get(artifact.type);
  const definitionByRenderer = rendererID ? registry.renderers.get(rendererID) : undefined;
  const strictDefinition = definitionByType?.strictContract
    ? definitionByType
    : definitionByRenderer?.strictContract
      ? definitionByRenderer
      : undefined;
  const definition = strictDefinition || (rendererID
    ? definitionByRenderer || registry.schemas.get(schemaKey(artifact.type, schemaVersion)) || definitionByType
    : registry.schemas.get(schemaKey(artifact.type, schemaVersion)) || definitionByType);

  if (!isValidPayload(artifact.payload)) {
    return { state: "invalid_payload", definition, reason: "卡片 payload 必须是对象。" };
  }

  if (strictDefinition && !matchesStrictContract(strictDefinition, artifact.type, rendererID, schemaVersion)) {
    return {
      state: "fallback_renderer",
      definition: definitionByType || strictDefinition,
      Renderer: SafeWorkflowContractFallbackArtifact,
      reason: "Workflow 变更卡片契约不受支持，已使用局部安全展示。",
    };
  }

  if (!definition) {
    if (rendererID) {
      return {
        state: "fallback_renderer",
        Renderer: JsonSummaryFallbackArtifact,
        reason: "未注册的 renderer，已使用 JSON 摘要安全展示。",
      };
    }
    return { state: "unsupported", reason: "未注册的卡片类型。" };
  }

  if (definition.lifecycle === "disabled") {
    return { state: "disabled", definition, reason: "卡片类型已禁用。" };
  }

  if (!definition.Renderer) {
    if (definition.fallback === "json_summary") {
      return {
        state: "fallback_renderer",
        definition,
        Renderer: JsonSummaryFallbackArtifact,
        reason: "卡片类型已注册但未配置前端渲染器，已使用 JSON 摘要安全展示。",
      };
    }
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

function JsonSummaryFallbackArtifact({ artifact }: { artifact: AiopsTransportAgentUiArtifact }) {
  const data = asRecord(artifact.payload) || asRecord(artifact.inlineData) || asRecord(artifact.metadata);
  const entries = Object.entries(data).filter(([key]) => !["html", "script", "dangerouslySetInnerHTML", "innerHTML"].includes(key));
  return (
    <div className="mt-3 rounded-lg border border-slate-100 bg-slate-50 p-3 text-xs text-slate-700" data-testid="agent-ui-json-fallback">
      {entries.length ? (
        <dl className="grid gap-2">
          {entries.slice(0, 8).map(([key, value]) => (
            <div key={key} className="grid gap-1 sm:grid-cols-[140px_minmax(0,1fr)]">
              <dt className="font-medium text-slate-500">{key}</dt>
              <dd className="min-w-0 break-words font-mono">{formatFallbackValue(value)}</dd>
            </div>
          ))}
        </dl>
      ) : (
        <div className="text-slate-500">暂无可展示的结构化数据</div>
      )}
    </div>
  );
}

function SafeWorkflowContractFallbackArtifact() {
  return (
    <section className="rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm text-amber-950" data-testid="workflow-change-contract-fallback">
      <h3 className="font-medium">暂不支持此版本的 Workflow 变更卡片</h3>
      <p className="mt-1 text-xs leading-5 text-amber-800">已隔离不兼容的数据；请刷新页面或重新生成修改预览。</p>
    </section>
  );
}

function matchesStrictContract(
  definition: AgentUiCardRegistryDefinition,
  artifactType: string,
  rendererID: string,
  schemaVersion: string,
): boolean {
  const artifactTypes = definition.artifactTypes?.length ? definition.artifactTypes : [definition.type];
  return artifactTypes.includes(artifactType)
    && Boolean(rendererID)
    && rendererID === definition.renderer
    && Boolean(schemaVersion)
    && schemaVersion === definition.schemaVersion;
}

function artifactRendererID(artifact: AiopsTransportAgentUiArtifact): string {
  return compactText(
    (artifact as { renderer?: unknown }).renderer ||
      asRecord(artifact.metadata).renderer ||
      asRecord(artifact.payload).renderer,
  );
}

function artifactSchemaVersion(artifact: AiopsTransportAgentUiArtifact): string {
  return compactText(
    (artifact as { schemaVersion?: unknown }).schemaVersion ||
      asRecord(artifact.metadata).schemaVersion ||
      asRecord(artifact.payload).schemaVersion,
  );
}

function schemaKey(type: string, schemaVersion: string): string {
  return `${compactText(type)}::${compactText(schemaVersion)}`;
}

function isValidPayload(payload: unknown): boolean {
  return payload === undefined || payload === null || (typeof payload === "object" && !Array.isArray(payload));
}

function asRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {};
}

function compactText(value: unknown): string {
  return typeof value === "string" ? value.trim().replace(/\s+/g, " ") : "";
}

function formatFallbackValue(value: unknown): string {
  if (value === undefined || value === null || value === "") {
    return "";
  }
  if (typeof value === "string" || typeof value === "number" || typeof value === "boolean") {
    return String(value);
  }
  try {
    return JSON.stringify(value);
  } catch {
    return String(value);
  }
}
