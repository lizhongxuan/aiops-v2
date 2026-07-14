import { normalizeMcpUiCard } from "@/lib/mcpUiCardModel";

export type AgentUIArtifactType =
  | "coroot_chart"
  | "trace_summary"
  | "topology_slice"
  | "rca_report"
  | "workflow_result"
  | "workflow_context"
  | "workflow_edit_plan"
  | "workflow_patch_preview"
  | "workflow_patch_apply"
  | "workflow_patch_result"
  | "workflow_patch_validation"
  | "workflow_conflict"
  | "workflow_manual_candidate"
  | "workflow_tool_timeline"
  | "verification_result"
  | "experience_match"
  | "ops_manual_match"
  | "ops_manual_search_result"
  | "ops_manual_param_resolution"
  | "ops_manual_param_form"
  | "ops_manual_preflight_result"
  | "ops_manual_fallback_guide"
  | "runner_workflow_generation"
  | "unsupported";

export type AgentUIArtifactStatus =
  | "ready"
  | "running"
  | "success"
  | "warning"
  | "error"
  | "blocked"
  | "expired"
  | "unsupported";

export interface AgentUIArtifact {
  id: string;
  type: AgentUIArtifactType;
  title: string;
  summary: string;
  status: AgentUIArtifactStatus;
  severity: string;
  source: string;
  createdAt: string;
  updatedAt: string;
  originalType: string;
  caseId: string;
  evidenceRef: string;
  promptTraceId: string;
  dataRef: string;
  renderer: string;
  schemaVersion: string;
  permissionScope: string;
  redactionStatus: string;
  payload: Record<string, unknown>;
  metadata: Record<string, unknown>;
  actions: Array<Record<string, unknown>>;
  mcpCard?: Record<string, unknown>;
}

const SUPPORTED_TYPES = new Set<AgentUIArtifactType>([
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

const SUPPORTED_STATUSES = new Set<AgentUIArtifactStatus>([
  "ready",
  "running",
  "success",
  "warning",
  "error",
  "blocked",
  "expired",
  "unsupported",
]);

const DANGEROUS_KEYS = new Set([
  "html",
  "script",
  "iframe",
  "dangerouslySetInnerHTML",
  "innerHTML",
  "outerHTML",
  "onClick",
  "onLoad",
  "styleText",
]);

const MCP_CARD_KEYS = ["mcpUiCard", "mcpCard", "card", "visual"] as const;
export const ALLOWED_ACTION_INTENTS = new Set([
  "open",
  "open-case",
  "open-evidence",
  "open-prompt-trace",
  "preview",
  "validate",
  "refresh",
  "retry",
  "confirm",
  "cancel",
  "request-permission",
  "generate-runner-workflow-candidate",
  "generate_runner_workflow_candidate",
]);

function compactText(value: unknown): string {
  return typeof value === "string" ? value.trim().replace(/\s+/g, " ") : "";
}

function asRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {};
}

function sanitizeValue(value: unknown): unknown {
  if (Array.isArray(value)) {
    return value.map(sanitizeValue);
  }

  if (!value || typeof value !== "object") {
    return value;
  }

  return Object.fromEntries(
    Object.entries(value as Record<string, unknown>)
      .filter(([key]) => !DANGEROUS_KEYS.has(key))
      .map(([key, entry]) => [key, sanitizeValue(entry)]),
  );
}

function sanitizeRecord(value: unknown): Record<string, unknown> {
  return asRecord(sanitizeValue(asRecord(value)));
}

function normalizeType(value: unknown): { type: AgentUIArtifactType; originalType: string } {
  const originalType = compactText(value).toLowerCase();
  if (SUPPORTED_TYPES.has(originalType as AgentUIArtifactType)) {
    return { type: originalType as AgentUIArtifactType, originalType };
  }
  return { type: "unsupported", originalType };
}

function normalizeStatus(value: unknown, type: AgentUIArtifactType): AgentUIArtifactStatus {
  const status = compactText(value).toLowerCase();
  if (SUPPORTED_STATUSES.has(status as AgentUIArtifactStatus)) {
    return status as AgentUIArtifactStatus;
  }
  return type === "unsupported" ? "unsupported" : "ready";
}

function normalizeActions(value: unknown): Array<Record<string, unknown>> {
  return Array.isArray(value)
    ? value.map((item) => normalizeAction(item)).filter((item) => Object.keys(item).length > 0)
    : [];
}

function normalizeAction(value: unknown): Record<string, unknown> {
  const action = sanitizeRecord(value);
  const intent = compactText(action.intent);
  if (!intent || ALLOWED_ACTION_INTENTS.has(intent)) {
    return action;
  }
  return {
    ...action,
    disabled: true,
    disabledReason: `未知动作 intent：${intent}`,
  };
}

function standardAction(id: string, label: string, href: string, target: Record<string, unknown>): Record<string, unknown> {
  return {
    id,
    label,
    href,
    intent: id.replace(/^view-/, "open-"),
    target,
    mutation: false,
  };
}

function normalizeArtifactActions(
  explicitActions: unknown,
  context: { caseId: string; evidenceRef: string; promptTraceId: string },
): Array<Record<string, unknown>> {
  const actions: Array<Record<string, unknown>> = [];
  if (context.caseId) {
    actions.push(standardAction(
      "view-case",
      "查看 Case",
      `/incidents/${encodeURIComponent(context.caseId)}`,
      { kind: "case", id: context.caseId },
    ));
  }
  if (context.evidenceRef) {
    actions.push(standardAction(
      "view-evidence",
      "查看证据",
      context.caseId
        ? `/incidents/${encodeURIComponent(context.caseId)}?evidence=${encodeURIComponent(context.evidenceRef)}`
        : `#evidence-${encodeURIComponent(context.evidenceRef)}`,
      { kind: "evidence", id: context.evidenceRef, caseId: context.caseId },
    ));
  }
  if (context.promptTraceId) {
    actions.push(standardAction(
      "view-prompt-trace",
      "查看 Prompt Trace",
      `/debug/prompts?trace_id=${encodeURIComponent(context.promptTraceId)}`,
      { kind: "prompt_trace", id: context.promptTraceId },
    ));
  }

  for (const action of normalizeActions(explicitActions)) {
    const key = `${compactText(action.id)}|${compactText(action.label)}|${compactText(action.href)}`;
    const duplicate = actions.some((item) => `${compactText(item.id)}|${compactText(item.label)}|${compactText(item.href)}` === key);
    if (!duplicate) {
      actions.push(action);
    }
  }

  return actions;
}

function extractMcpCard(payload: Record<string, unknown>, title: string): Record<string, unknown> | undefined {
  for (const key of MCP_CARD_KEYS) {
    const candidate = payload[key];
    if (!candidate || typeof candidate !== "object" || Array.isArray(candidate)) continue;
    if (key === "visual") {
      return normalizeMcpUiCard({
        uiKind: "readonly_chart",
        title,
        visual: candidate,
      }) as Record<string, unknown>;
    }
    return normalizeMcpUiCard(candidate) as Record<string, unknown>;
  }
  return undefined;
}

function payloadWithoutMcpCard(payload: Record<string, unknown>): Record<string, unknown> {
  return Object.fromEntries(Object.entries(payload).filter(([key]) => !MCP_CARD_KEYS.includes(key as typeof MCP_CARD_KEYS[number])));
}

export function normalizeAgentUIArtifact(input: unknown): AgentUIArtifact {
  const source = sanitizeRecord(input);
  const { type, originalType } = normalizeType(source.type || source.kind || source.artifactType);
  const title = compactText(source.title || source.name) || (type === "unsupported" ? "不支持的 UI Artifact" : "Agent UI Artifact");
  const payload = type === "unsupported" ? {} : sanitizeRecord(source.payload || source.data || source.result);
  const metadata = sanitizeRecord(source.metadata || source.meta);
  const caseId = compactText(source.caseId || source.case_id || payload.caseId || payload.case_id || metadata.caseId || metadata.case_id);
  const evidenceRef = compactText(
    source.evidenceRef
      || source.evidence_ref
      || source.evidenceId
      || source.evidence_id
      || source.dataRef
      || source.data_ref
      || payload.evidenceRef
      || payload.evidence_ref
      || payload.evidenceId
      || payload.evidence_id
      || payload.dataRef
      || payload.data_ref
      || metadata.evidenceRef
      || metadata.evidence_ref
      || metadata.evidenceId
      || metadata.evidence_id,
  );
  const promptTraceId = compactText(
    source.promptTraceId
      || source.prompt_trace_id
      || source.promptTraceRef
      || source.prompt_trace_ref
      || payload.promptTraceId
      || payload.prompt_trace_id
      || payload.promptTraceRef
      || payload.prompt_trace_ref
      || metadata.promptTraceId
      || metadata.prompt_trace_id
      || metadata.promptTraceRef
      || metadata.prompt_trace_ref,
  );
  const mcpCard = type === "coroot_chart" ? extractMcpCard(payload, title) : undefined;

  return {
    id: compactText(source.id || source.artifactId) || `${type}-artifact`,
    type,
    title,
    summary: compactText(source.summary || source.description),
    status: normalizeStatus(source.status, type),
    severity: compactText(source.severity || source.tone),
    source: compactText(source.source || source.origin || "agent"),
    createdAt: compactText(source.createdAt || source.created_at),
    updatedAt: compactText(source.updatedAt || source.updated_at),
    originalType: type === "unsupported" ? originalType : "",
    caseId,
    evidenceRef,
    promptTraceId,
    dataRef: compactText(source.dataRef || source.data_ref || payload.dataRef || payload.data_ref),
    renderer: compactText(source.renderer || payload.renderer || metadata.renderer),
    schemaVersion: compactText(source.schemaVersion || source.schema_version || payload.schemaVersion || payload.schema_version || metadata.schemaVersion || metadata.schema_version),
    permissionScope: compactText(source.permissionScope || source.permission_scope || payload.permissionScope || payload.permission_scope || metadata.permissionScope || metadata.permission_scope),
    redactionStatus: compactText(source.redactionStatus || source.redaction_status || payload.redactionStatus || payload.redaction_status || metadata.redactionStatus || metadata.redaction_status),
    payload: mcpCard ? payloadWithoutMcpCard(payload) : payload,
    metadata,
    actions: normalizeArtifactActions(source.actions, { caseId, evidenceRef, promptTraceId }),
    ...(mcpCard ? { mcpCard } : {}),
  };
}

export function normalizeAgentUIArtifacts(inputs: unknown): AgentUIArtifact[] {
  return Array.isArray(inputs)
    ? inputs.filter(Boolean).map((item) => normalizeAgentUIArtifact(item))
    : [];
}
