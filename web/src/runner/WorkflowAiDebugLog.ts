import type {
  WorkflowAiContext,
  WorkflowAiSession,
  WorkflowAiToolLogEntry,
  WorkflowEditPlan,
  WorkflowPatch,
  WorkflowPatchResult,
  WorkflowPatchValidation,
} from "./workflowAiTypes";

export type WorkflowAiDebugLogInput = {
  session?: WorkflowAiSession;
  context?: WorkflowAiContext;
  userMessages?: string[];
  plans?: WorkflowEditPlan[];
  patches?: WorkflowPatch[];
  results?: WorkflowPatchResult[];
  validations?: WorkflowPatchValidation[];
  toolLog?: WorkflowAiToolLogEntry[];
  errors?: string[];
};

export function serializeWorkflowAiDebugLog(input: WorkflowAiDebugLogInput): string {
  const lines = [
    "Workflow AI Edit Log",
    `session_id: ${value(input.session?.id)}`,
    `workflow_id: ${value(input.context?.workflowId ?? input.session?.workflowId)}`,
    `base_revision: ${value(input.session?.baseRevision)}`,
    `active_revision: ${value(input.session?.activeRevision ?? input.context?.revision)}`,
    `selected_node: ${value(input.context?.selectedNodeId)}`,
  ];

  appendList(lines, "user_messages", input.userMessages);
  appendList(lines, "plan_ids", input.plans?.map((plan) => plan.id));
  appendList(lines, "patch_ids", input.patches?.map((patch) => patch.id));
  appendList(lines, "validation_status", input.validations?.map((validation) => (validation.valid ? "valid" : "invalid")));
  appendList(lines, "effect_status", input.results?.map((result) => result.effect?.status ?? "unknown"));
  appendList(lines, "undo_checkpoint_ids", input.results?.map((result) => result.undoCheckpoint?.id).filter(Boolean) as string[]);
  appendList(lines, "tool_names", input.toolLog?.map((entry) => entry.toolName));
  appendList(lines, "trace_ids", input.toolLog?.map((entry) => entry.traceId).filter(Boolean) as string[]);
  appendList(lines, "errors", input.errors);

  for (const entry of input.toolLog ?? []) {
    lines.push(`tool: ${entry.toolName} status=${entry.status} duration_ms=${value(entry.durationMs)} trace_id=${value(entry.traceId)}`);
    if (entry.inputSummary) lines.push(`  input: ${redactWorkflowAiLogValue(entry.inputSummary)}`);
    if (entry.outputSummary) lines.push(`  output: ${redactWorkflowAiLogValue(entry.outputSummary)}`);
    if (entry.error) lines.push(`  error: ${redactWorkflowAiLogValue(entry.error)}`);
  }

  return `${lines.join("\n")}\n`;
}

function appendList(lines: string[], label: string, values?: Array<string | undefined>) {
  const clean = (values ?? []).map((item) => redactWorkflowAiLogValue(item || "")).filter((item): item is string => Boolean(String(item || "").trim()));
  lines.push(`${label}: ${clean.length ? clean.join(", ") : "-"}`);
}

function value(input: unknown) {
  if (input === undefined || input === null || input === "") return "-";
  return redactWorkflowAiLogValue(String(input));
}

function redactWorkflowAiLogValue(input: string) {
  return String(input || "")
    .replace(/(api[_-]?key|apikey|password|passwd|token|secret)\s*[:=]\s*[^,\s]+/gi, "$1=[redacted]")
    .replace(/Bearer\s+[A-Za-z0-9._~+/=-]+/gi, "Bearer [redacted]");
}
