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
  /(^|[^\p{L}\p{N}_])@coroot([^\p{L}\p{N}_]|$)/iu;

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
