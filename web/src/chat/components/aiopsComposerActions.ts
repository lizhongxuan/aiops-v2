import { isAiopsTransportRunning } from "@/transport/aiopsTransportConverter";
import type { AiopsTransportState } from "@/transport/aiopsTransportTypes";

export type StopDispatchTarget = "transport" | "runtime";

export type OpsManualParamFormSubmitInput = {
  artifactId?: string;
  manualId?: string;
  workflowId?: string;
  params: Record<string, string>;
};

export type OpsManualParamFormSubmit = {
  text: string;
  metadata: Record<string, string>;
};

const EXPLICIT_COROOT_MENTION_PATTERN =
  /(^|[^A-Za-z0-9_])@coroot([^A-Za-z0-9_]|$)/iu;
const EXPLICIT_OPS_GRAPH_MENTION_PATTERN =
  /(^|[^A-Za-z0-9_])@ops_graph([^A-Za-z0-9_]|$)/iu;
const EXPLICIT_OPS_MANUAL_MENTION_PATTERN =
  /(^|[^A-Za-z0-9_])@(ops_manuals|ops_manus)([^A-Za-z0-9_]|$)/iu;

export function resolveStopDispatchTarget(
  state: AiopsTransportState,
  threadIsRunning: boolean,
): StopDispatchTarget {
  if (isAiopsTransportRunning(state) && Boolean(state.currentTurnId)) {
    return "transport";
  }
  if (threadIsRunning && Boolean(state.sessionId)) {
    return "transport";
  }
  return threadIsRunning ? "runtime" : "transport";
}

export function buildOpsManualParamFormSubmit(
  input: OpsManualParamFormSubmitInput,
): OpsManualParamFormSubmit {
  const params = Object.fromEntries(
    Object.entries(input.params)
      .map(([key, value]) => [key.trim(), String(value || "").trim()])
      .filter(([key, value]) => key && value),
  );
  const paramText = Object.entries(params)
    .map(([key, value]) => `${key}=${value}`)
    .join("；");
  return {
    text: paramText
      ? `已提交运维手册参数：${paramText}`
      : "已提交运维手册参数。",
    metadata: {
      opsManualAction: "submit_ops_manual_param_form",
      ...(input.artifactId ? { sourceArtifactId: input.artifactId } : {}),
      ...(input.manualId ? { opsManualManualId: input.manualId } : {}),
      ...(input.workflowId ? { opsManualWorkflowId: input.workflowId } : {}),
      opsManualParamsJson: JSON.stringify(params),
    },
  };
}

export function buildCorootMentionMetadata(
  text: string,
): Record<string, string> {
  if (!EXPLICIT_COROOT_MENTION_PATTERN.test(text)) {
    return {};
  }
  return {
    "aiops.coroot.explicitRCA": "true",
    "aiops.coroot.rcaDisplayAllowed": "true",
  };
}

export function buildAiopsSpecialMentionMetadata(
  text: string,
): Record<string, string> {
  const metadata: Record<string, string> = { ...buildCorootMentionMetadata(text) };
  const enabledPacks: string[] = [];
  const enabledTools: string[] = [];
  if (EXPLICIT_OPS_GRAPH_MENTION_PATTERN.test(text)) {
    enabledPacks.push("opsgraph");
    metadata["aiops.opsGraph.explicitMention"] = "true";
  }
  if (EXPLICIT_OPS_MANUAL_MENTION_PATTERN.test(text)) {
    enabledPacks.push("ops_manual_flow");
    enabledTools.push("search_ops_manuals");
    metadata["aiops.opsManuals.explicitMention"] = "true";
  }
  if (enabledPacks.length) {
    metadata.enableToolPack = enabledPacks.join(",");
  }
  if (enabledTools.length) {
    metadata.enableTool = enabledTools.join(",");
  }
  return metadata;
}
