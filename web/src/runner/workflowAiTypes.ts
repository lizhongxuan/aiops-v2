export type WorkflowAiEffectStatus = "changed" | "no_effect" | "duplicate" | "metadata_only";

export type WorkflowAiIntent = "explain" | "inspect" | "create" | "edit" | "delete" | "test" | "manual";

export type WorkflowAiPhase =
  | "idle"
  | "thinking"
  | "clarifying"
  | "plan_review"
  | "generating"
  | "preflight_ready"
  | "completed"
  | "failed"
  | "cancelled";

export type WorkflowAiPlanReply =
  | { type: "approve_plan" }
  | { type: "cancel_plan" }
  | { type: "revise_plan"; instruction: string };

export type WorkflowAiVariableSpec = {
  name: string;
  type?: string;
  required?: boolean;
  source?: string;
};

export type WorkflowAiStage =
  | "idle"
  | "context_loaded"
  | "chatting"
  | "planning"
  | "plan_review"
  | "patch_generating"
  | "patch_review"
  | "post_apply_check"
  | "budget_paused"
  | "validate_needed"
  | "manual_candidate"
  | "complete"
  | "conflict";

export type WorkflowAiSession = {
  schemaVersion?: string;
  id: string;
  workflowId?: string;
  baseRevision?: string;
  activeRevision?: string;
  sessionIntent?: "create" | "edit";
  status?: string;
  currentPlan?: WorkflowEditPlan;
  patchQueue?: WorkflowPatch[];
  undoStack?: Array<{ id: string; patchId?: string; revisionBefore?: string; revisionAfter?: string }>;
  stepBudget?: {
    maxPatchReviewsPerTurn?: number;
    usedPatchReviews?: number;
    remainingPlanItems?: number;
  };
  toolLogRef?: string;
};

export type WorkflowEditPlanItem = {
  id: string;
  title: string;
  description?: string;
  status?: string;
  goal?: string;
  environment?: string | string[];
  nodeLabel?: string;
  nodeType?: string;
  nodeAction?: string;
  scriptSummary?: string;
  validationSummary?: string;
  inputVariables?: WorkflowAiVariableSpec[];
  outputVariables?: WorkflowAiVariableSpec[];
  script?: string;
};

export type WorkflowEditPlan = {
  id: string;
  workflowId?: string;
  message?: string;
  items: WorkflowEditPlanItem[];
};

export type WorkflowPatchOperation = {
  op: string;
  nodeId?: string;
  edgeId?: string;
  fields?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
  node?: Record<string, unknown>;
  edge?: Record<string, unknown>;
};

export type WorkflowPatch = {
  id: string;
  workflowId?: string;
  baseRevision?: string;
  summary?: string;
  reason?: string;
  operations: WorkflowPatchOperation[];
};

export type WorkflowPatchValidation = {
  valid: boolean;
  errors?: string[];
  warnings?: string[];
};

export type WorkflowPatchEffect = {
  status: WorkflowAiEffectStatus;
  summary?: string;
  affectedNodes?: string[];
  affectedEdges?: string[];
  affectedVariables?: string[];
};

export type WorkflowPatchResult = {
  patchId: string;
  workflowId?: string;
  revisionBefore?: string;
  revisionAfter?: string;
  effect?: WorkflowPatchEffect;
  undoCheckpoint?: { id: string; patchId?: string; revisionBefore?: string; revisionAfter?: string };
  describe?: { summary?: string; nodeCount?: number; edgeCount?: number; nodeIds?: string[] };
};

export type WorkflowAiEvent = {
  id: string;
  workflowId?: string;
  sessionId?: string;
  planId?: string;
  planItemId?: string;
  patchId?: string;
  type: string;
  actor?: "user" | "assistant" | "tool";
  summary: string;
  visibleNodeIds?: string[];
  createdAt?: string;
};

export type WorkflowManualCandidateSummary = {
  candidateId?: string;
  manualId?: string;
  title?: string;
  reviewStatus?: string;
  workflowId?: string;
  workflowDigest?: string;
  operationType?: string;
  riskLevel?: string;
  requiredEvidence?: string[];
  cannotConditions?: string[];
  preflightSummary?: string;
  verifySummary?: string;
  rollbackSummary?: string;
  staleBinding?: boolean;
};

export type WorkflowAiToolLogEntry = {
  id: string;
  toolName: string;
  status: "pending" | "running" | "completed" | "failed";
  durationMs?: number;
  traceId?: string;
  inputSummary?: string;
  outputSummary?: string;
  error?: string;
};

export type WorkflowAiActiveStep = {
  index: number;
  total: number;
  title: string;
  goal?: string;
  environment?: string | string[];
  scriptSummary?: string;
  inputVariables?: WorkflowAiVariableSpec[];
  outputVariables?: WorkflowAiVariableSpec[];
  generatedNodeIds?: string[];
  generatedEdgeIds?: string[];
  validationSummary?: string;
};

export type WorkflowAiStepHistoryItem = WorkflowAiActiveStep & {
  status: "running" | "completed";
};

export type WorkflowAiContext = {
  workflowId?: string;
  workflowName?: string;
  revision?: string;
  selectedNodeId?: string;
  saveState?: string;
  lastModifiedLabel?: string;
  validation?: WorkflowPatchValidation;
  manualBinding?: WorkflowManualCandidateSummary | null;
};
