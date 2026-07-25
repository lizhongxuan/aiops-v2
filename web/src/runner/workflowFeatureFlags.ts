export const WORKFLOW_ROLLOUT_SCHEMA_VERSION = "aiops.workflow.rollout.v1" as const;

export const WORKFLOW_CAPABILITIES = [
  "workflow_agent_ui",
  "workflow_mcp_read",
  "workflow_mcp_preview",
  "workflow_mcp_mutation",
  "workflow_delete",
  "workflow_publish",
  "workflow_execute",
  "workflow_domain_events",
  "workflow_plan_ir",
  "workflow_elk_layout",
  "workflow_edit_grants",
  "workflow_external_mcp",
  "workflow_mcp_apps",
] as const;

export type WorkflowCapability = (typeof WORKFLOW_CAPABILITIES)[number];
export type WorkflowAgentStage = "disabled" | "read_only" | "internal_apply" | "canary" | "general";
export type WorkflowRolloutDecision = "invalid" | "deny_list" | "allow_list" | "percentage" | "percentage_excluded";
export type WorkflowCapabilityValues = Readonly<Record<WorkflowCapability, boolean>>;

export type WorkflowCapabilitySnapshot = Readonly<{
  schemaVersion: typeof WORKFLOW_ROLLOUT_SCHEMA_VERSION;
  stage: WorkflowAgentStage;
  saltVersion: string;
  rolloutPercentage: number;
  bucketBasisPoints: number;
  decision: WorkflowRolloutDecision;
  capabilities: WorkflowCapabilityValues;
}>;

const STAGES = new Set<WorkflowAgentStage>(["disabled", "read_only", "internal_apply", "canary", "general"]);
const DECISIONS = new Set<WorkflowRolloutDecision>(["invalid", "deny_list", "allow_list", "percentage", "percentage_excluded"]);

export function parseWorkflowCapabilitySnapshot(value: unknown): WorkflowCapabilitySnapshot | undefined {
  try {
    const source = strictRecord(value, "snapshot");
    exactKeys(source, ["schemaVersion", "stage", "saltVersion", "rolloutPercentage", "bucketBasisPoints", "decision", "capabilities"], "snapshot");
    if (source.schemaVersion !== WORKFLOW_ROLLOUT_SCHEMA_VERSION) throw new TypeError("unsupported workflow rollout schema");
    const stage = oneOf(source.stage, STAGES, "snapshot.stage");
    const decision = oneOf(source.decision, DECISIONS, "snapshot.decision");
    const saltVersion = canonicalSaltVersion(source.saltVersion);
    const rolloutPercentage = boundedInteger(source.rolloutPercentage, 0, 100, "snapshot.rolloutPercentage");
    const bucketBasisPoints = boundedInteger(source.bucketBasisPoints, 0, 9_999, "snapshot.bucketBasisPoints");
    const rawCapabilities = strictRecord(source.capabilities, "snapshot.capabilities");
    exactKeys(rawCapabilities, WORKFLOW_CAPABILITIES, "snapshot.capabilities");
    const capabilities = Object.fromEntries(WORKFLOW_CAPABILITIES.map((capability) => {
      const enabled = rawCapabilities[capability];
      if (typeof enabled !== "boolean") throw new TypeError(`snapshot.capabilities.${capability} must be boolean`);
      return [capability, enabled];
    })) as Record<WorkflowCapability, boolean>;
    validateFailClosedConsistency(stage, decision, capabilities);
    return Object.freeze({
      schemaVersion: WORKFLOW_ROLLOUT_SCHEMA_VERSION,
      stage,
      saltVersion,
      rolloutPercentage,
      bucketBasisPoints,
      decision,
      capabilities: Object.freeze(capabilities),
    });
  } catch {
    return undefined;
  }
}

export function isWorkflowCapabilityEnabled(value: unknown, capability: WorkflowCapability): boolean {
  if (!WORKFLOW_CAPABILITIES.includes(capability)) return false;
  return parseWorkflowCapabilitySnapshot(value)?.capabilities[capability] === true;
}

function validateFailClosedConsistency(
  stage: WorkflowAgentStage,
  decision: WorkflowRolloutDecision,
  capabilities: Record<WorkflowCapability, boolean>,
): void {
  const disabledDecision = decision === "invalid" || decision === "deny_list" || decision === "percentage_excluded";
  if (disabledDecision && stage !== "disabled") throw new TypeError("excluded rollout decisions require disabled stage");
  if (stage === "disabled" && WORKFLOW_CAPABILITIES.some((capability) => capabilities[capability])) {
    throw new TypeError("disabled stage cannot expose workflow capabilities");
  }
  if (stage === "read_only" && (capabilities.workflow_mcp_mutation || capabilities.workflow_delete || capabilities.workflow_publish || capabilities.workflow_execute)) {
    throw new TypeError("read_only stage cannot expose mutation capabilities");
  }
  if (stage === "internal_apply" && (capabilities.workflow_delete || capabilities.workflow_publish || capabilities.workflow_execute)) {
    throw new TypeError("internal_apply stage cannot expose delete, publish, or execute");
  }
  if (stage === "canary" && (capabilities.workflow_publish || capabilities.workflow_execute)) {
    throw new TypeError("canary stage cannot expose publish or execute");
  }
  if (capabilities.workflow_mcp_mutation && (!capabilities.workflow_agent_ui || !capabilities.workflow_mcp_read || !capabilities.workflow_mcp_preview)) {
    throw new TypeError("mutation capability is missing its read/preview prerequisites");
  }
  if ((capabilities.workflow_delete || capabilities.workflow_publish || capabilities.workflow_execute) && !capabilities.workflow_mcp_mutation) {
    throw new TypeError("write capability is missing workflow mutation");
  }
  if (capabilities.workflow_plan_ir && (!capabilities.workflow_mcp_read || !capabilities.workflow_mcp_preview)) {
    throw new TypeError("Plan IR capability is missing read/preview");
  }
  if ((capabilities.workflow_elk_layout || capabilities.workflow_edit_grants) && !capabilities.workflow_mcp_mutation) {
    throw new TypeError("mutation extension is missing workflow mutation");
  }
  if (capabilities.workflow_external_mcp && !capabilities.workflow_mcp_read) {
    throw new TypeError("external MCP capability is missing workflow read");
  }
}

function strictRecord(value: unknown, path: string): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) throw new TypeError(`${path} must be an object`);
  return value as Record<string, unknown>;
}

function exactKeys(source: Record<string, unknown>, allowed: readonly string[], path: string): void {
  const keys = Object.keys(source);
  const unknown = keys.find((key) => !allowed.includes(key));
  const missing = allowed.find((key) => !Object.prototype.hasOwnProperty.call(source, key));
  if (unknown) throw new TypeError(`${path}.${unknown} is not supported`);
  if (missing) throw new TypeError(`${path}.${missing} is required`);
}

function oneOf<T extends string>(value: unknown, allowed: Set<T>, path: string): T {
  if (typeof value !== "string" || !allowed.has(value as T)) throw new TypeError(`${path} is not supported`);
  return value as T;
}

function canonicalSaltVersion(value: unknown): string {
  if (typeof value !== "string" || value.length === 0 || value.length > 64 || value !== value.trim() || !/^[A-Za-z0-9._-]+$/.test(value)) {
    throw new TypeError("snapshot.saltVersion must be canonical");
  }
  return value;
}

function boundedInteger(value: unknown, minimum: number, maximum: number, path: string): number {
  if (typeof value !== "number" || !Number.isSafeInteger(value) || value < minimum || value > maximum) {
    throw new TypeError(`${path} is outside its allowed range`);
  }
  return value;
}
