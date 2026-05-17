import httpClient from "./httpClient";

type LooseRecord = Record<string, unknown>;

export type WorkflowRefView = {
  workflowId: string;
  workflowVersion: string;
  workflowDigest: string;
  storageUri: string;
  raw: LooseRecord;
};

export type OperationProfileView = {
  targetType: string;
  action: string;
  riskLevel: string;
  stateful: boolean;
  raw: LooseRecord;
};

export type ApplicabilityProfileView = {
  middleware: string;
  middlewareVersions: string[];
  os: string[];
  platform: string[];
  executionSurface: string[];
  topology: string[];
  internetRequired: string;
  raw: LooseRecord;
};

export type RequiredContextView = {
  requiredInputs: string[];
  requiredEvidence: string[];
  optionalEvidence: string[];
  raw: LooseRecord;
};

export type RunRecordSummaryView = {
  successCount: number;
  failureCount: number;
  recentResult: string;
  latestStatus: string;
  lastRunAt: string;
  consecutiveFailures: number;
  suppressed: boolean;
  suppressedReason: string;
  raw: LooseRecord;
};

export type ScoreBreakdownView = {
  structuralScore: number;
  keywordScore: number;
  vectorScore: number;
  runHistoryScore: number;
  penalty: number;
  finalScore: number;
  raw: LooseRecord;
};

export type OpsManualPreflightStatus = "not_run" | "passed" | "failed" | "blocked" | "not_applicable" | "unknown";

export type OpsManualPreflightEvidence = {
  name: string;
  status: string;
  value: unknown;
  note: string;
  raw: LooseRecord;
};

export type OpsManualPreflightRequest = {
  manual_id: string;
  workflow_id?: string;
  operation_frame?: Record<string, unknown>;
  parameters: Record<string, unknown>;
  requested_by?: string;
  triggered_by?: string;
};

export type OpsManualPreflightResult = {
  status: OpsManualPreflightStatus;
  ready: boolean;
  reason: string;
  manualId: string;
  workflowId: string;
  probeId: string;
  evidence: OpsManualPreflightEvidence[];
  missingPermissions: string[];
  environmentDiffs: string[];
  nextAction: string;
  checkedAt: string;
  artifactType: string;
  raw: unknown;
};

export type OpsManualParamCandidateView = {
  value: unknown;
  label: string;
  hint: string;
  source: string;
  confidence: number;
  evidence: string;
  raw: LooseRecord;
};

export type OpsManualResolvedParamView = {
  id: string;
  value: unknown;
  source: string;
  confidence: number;
  evidence: string;
  confirmedByUser: boolean;
  needsUserConfirmation: boolean;
  raw: LooseRecord;
};

export type OpsManualParamFormFieldView = {
  id: string;
  label: string;
  type: string;
  required: boolean;
  sensitive: boolean;
  uiControl: string;
  placeholder: string;
  defaultValue: unknown;
  candidates: OpsManualParamCandidateView[];
  raw: LooseRecord;
};

export type ResolveOpsManualParamsRequest = {
  request_text?: string;
  manual_id: string;
  workflow_id?: string;
  operation_frame?: Record<string, unknown>;
  known_params?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
};

export type OpsManualParamResolutionResult = {
  status: string;
  manualId: string;
  workflowId: string;
  operationFrame: Record<string, unknown>;
  resolvedParams: OpsManualResolvedParamView[];
  fields: OpsManualParamFormFieldView[];
  nextAction: string;
  artifactType: string;
  raw: unknown;
};

export type OpsManualView = {
  id: string;
  manualFamilyId: string;
  title: string;
  status: string;
  version: string;
  owner: string;
  workflowRef: WorkflowRefView;
  operation: OperationProfileView;
  applicability: ApplicabilityProfileView;
  requiredContext: RequiredContextView;
  parameterRules: LooseRecord;
  preconditions: string[];
  validation: string[];
  cannotUseWhen: string[];
  riskNotes: string[];
  documentMarkdown: string;
  searchDoc: string;
  metadata: LooseRecord;
  createdAt: string;
  updatedAt: string;
  runRecordSummary: RunRecordSummaryView;
  raw: LooseRecord;
};

export type OpsManualMatchView = {
  manualId: string;
  manualTitle: string;
  state: string;
  workflowRef: WorkflowRefView;
  reasons: string[];
  missingContext: string[];
  compatibilityGaps: string[];
  recommendedNextActions: string[];
  runRecordSummary: RunRecordSummaryView;
  manual: OpsManualView | null;
  raw: LooseRecord;
};

export type OperationFrameView = {
  intent: string;
  rawText: string;
  raw: LooseRecord;
};

export type OpsManualSearchDecision = "direct_execute" | "need_info" | "adapt" | "reference_only" | "no_match";

export type SearchOpsManualsOperationFrameView = {
  intent: string;
  objectType: string;
  operationType: string;
  operationGoal: string;
  rawText: string;
  raw: LooseRecord;
};

export type SearchManualHitView = {
  manualId: string;
  title: string;
  manualStatus: string;
  workflowStatus: string;
  boundWorkflowId: string;
  matchLevel: string;
  usableMode: OpsManualSearchDecision;
  scoreBreakdown: ScoreBreakdownView;
  preflightStatus: OpsManualPreflightStatus;
  matchedFields: string[];
  missingFields: string[];
  environmentDiffs: string[];
  blockedReasons: string[];
  recommendedAction: string;
  runRecordSummary: RunRecordSummaryView;
  manual: OpsManualView | null;
  raw: LooseRecord;
};

export type SearchOpsManualsRequest = {
  text?: string;
  operation_frame?: Record<string, unknown>;
  operationFrame?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
  limit?: number;
};

export type SearchOpsManualsResult = {
  decision: OpsManualSearchDecision;
  summary: string;
  operationFrame: SearchOpsManualsOperationFrameView;
  manuals: SearchManualHitView[];
  nextQuestions: string[];
  recommendedNextAction: string;
  searchedFields: string[];
  raw: unknown;
};

export type OpsManualCandidateView = {
  id: string;
  sourceType: string;
  sourceRefs: string[];
  proposedManual: OpsManualView;
  validationReport: string[];
  reviewStatus: string;
  reviewer: string;
  reviewNote: string;
  createdAt: string;
  updatedAt: string;
  raw: LooseRecord;
};

export type RunRecordView = {
  id: string;
  manualId: string;
  workflowId: string;
  workflowVersion: string;
  workflowDigest: string;
  dryRunStatus: string;
  executionStatus: string;
  validationStatus: string;
  rollbackStatus: string;
  failureReason: string;
  operator: string;
  startedAt: string;
  completedAt: string;
  raw: LooseRecord;
};

export type OpsManualListView = {
  items: OpsManualView[];
  nextCursor: string;
  total: number | null;
  raw: unknown;
};

export type OpsManualMatchListView = {
  items: OpsManualMatchView[];
  nextCursor: string;
  total: number | null;
  raw: unknown;
};

export type OpsManualCandidateListView = {
  items: OpsManualCandidateView[];
  nextCursor: string;
  total: number | null;
  raw: unknown;
};

export type RunRecordListView = {
  items: RunRecordView[];
  nextCursor: string;
  total: number | null;
  raw: unknown;
};

type OpsManualsHttpClient = {
  get(path: string): Promise<unknown>;
  post(path: string, body?: unknown): Promise<unknown>;
};

function isRecord(value: unknown): value is LooseRecord {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}

function asRecord(value: unknown): LooseRecord {
  return isRecord(value) ? value : {};
}

function text(value: unknown, fallback = "") {
  if (value === undefined || value === null) return fallback;
  const normalized = String(value).trim();
  return normalized || fallback;
}

function pick(source: LooseRecord, ...keys: string[]) {
  for (const key of keys) {
    const value = source[key];
    if (value !== undefined && value !== null && value !== "") return value;
  }
  return "";
}

function asArray<T = unknown>(value: unknown): T[] {
  return Array.isArray(value) ? (value as T[]) : [];
}

function stringArray(value: unknown): string[] {
  return asArray(value).map((item) => text(item)).filter(Boolean);
}

function integer(value: unknown) {
  const numeric = Number(value);
  return Number.isFinite(numeric) ? numeric : 0;
}

function bool(value: unknown, fallback = false) {
  if (typeof value === "boolean") return value;
  if (typeof value === "number") return value !== 0;
  const normalized = text(value).toLowerCase();
  if (["true", "1", "yes", "enabled"].includes(normalized)) return true;
  if (["false", "0", "no", "disabled"].includes(normalized)) return false;
  return fallback;
}

function searchDecision(value: unknown): OpsManualSearchDecision {
  const normalized = text(value).toLowerCase();
  if (normalized === "direct" || normalized === "direct_execute" || normalized === "executable") return "direct_execute";
  if (normalized === "need_info" || normalized === "need_more_info" || normalized === "missing_info") return "need_info";
  if (normalized === "adapt" || normalized === "adapt_required" || normalized === "generate_variant") return "adapt";
  if (normalized === "reference" || normalized === "reference_only") return "reference_only";
  return "no_match";
}

function preflightStatus(value: unknown): OpsManualPreflightStatus {
  const normalized = text(value).toLowerCase();
  if (["not_run", "passed", "failed", "blocked", "not_applicable", "unknown"].includes(normalized)) {
    return normalized as OpsManualPreflightStatus;
  }
  return "unknown";
}

function endpoint(path: string, params: Record<string, unknown> = {}) {
  const query = Object.entries(params)
    .filter(([, value]) => value !== undefined && value !== null && value !== "")
    .map(([key, value]) => `${encodeURIComponent(key)}=${encodeURIComponent(String(value))}`)
    .join("&");
  return query ? `${path}?${query}` : path;
}

function encodePath(value: string) {
  return encodeURIComponent(value);
}

function listTotal(source: LooseRecord) {
  const total = pick(source, "total", "totalCount", "total_count");
  const numericTotal = Number(total);
  return Number.isFinite(numericTotal) ? numericTotal : null;
}

function unwrapManual(input: unknown): LooseRecord {
  if (!isRecord(input)) return {};
  const wrapped = pick(input, "manual", "opsManual", "ops_manual", "item", "proposedManual", "proposed_manual");
  return isRecord(wrapped) ? wrapped : input;
}

export function normalizeWorkflowRef(input: unknown): WorkflowRefView {
  const source = isRecord(input) ? input : {};
  const workflowId = text(pick(source, "workflowId", "workflow_id", "id"));
  return {
    workflowId,
    workflowVersion: text(pick(source, "workflowVersion", "workflow_version", "version")),
    workflowDigest: text(pick(source, "workflowDigest", "workflow_digest", "digest")),
    storageUri: text(pick(source, "storageUri", "storage_uri")),
    raw: source,
  };
}

export function normalizeOperationProfile(input: unknown): OperationProfileView {
  const source = isRecord(input) ? input : {};
  return {
    targetType: text(pick(source, "targetType", "target_type")),
    action: text(pick(source, "action")),
    riskLevel: text(pick(source, "riskLevel", "risk_level")),
    stateful: bool(pick(source, "stateful"), false),
    raw: source,
  };
}

export function normalizeApplicability(input: unknown): ApplicabilityProfileView {
  const source = isRecord(input) ? input : {};
  return {
    middleware: text(pick(source, "middleware")),
    middlewareVersions: stringArray(pick(source, "middlewareVersions", "middleware_versions")),
    os: stringArray(pick(source, "os")),
    platform: stringArray(pick(source, "platform")),
    executionSurface: stringArray(pick(source, "executionSurface", "execution_surface")),
    topology: stringArray(pick(source, "topology")),
    internetRequired: text(pick(source, "internetRequired", "internet_required")),
    raw: source,
  };
}

export function normalizeRequiredContext(input: unknown): RequiredContextView {
  const source = isRecord(input) ? input : {};
  return {
    requiredInputs: stringArray(pick(source, "requiredInputs", "required_inputs")),
    requiredEvidence: stringArray(pick(source, "requiredEvidence", "required_evidence")),
    optionalEvidence: stringArray(pick(source, "optionalEvidence", "optional_evidence")),
    raw: source,
  };
}

export function normalizeRunRecordSummary(input: unknown): RunRecordSummaryView {
  const source = isRecord(input) ? input : {};
  return {
    successCount: integer(pick(source, "successCount", "success_count")),
    failureCount: integer(pick(source, "failureCount", "failure_count")),
    recentResult: text(pick(source, "recentResult", "recent_result")),
    latestStatus: text(pick(source, "latestStatus", "latest_status")),
    lastRunAt: text(pick(source, "lastRunAt", "last_run_at")),
    consecutiveFailures: integer(pick(source, "consecutiveFailures", "consecutive_failures")),
    suppressed: Boolean(pick(source, "suppressed")),
    suppressedReason: text(pick(source, "suppressedReason", "suppressed_reason")),
    raw: source,
  };
}

export function normalizeScoreBreakdown(input: unknown): ScoreBreakdownView {
  const source = isRecord(input) ? input : {};
  return {
    structuralScore: Number(pick(source, "structuralScore", "structural_score")) || 0,
    keywordScore: Number(pick(source, "keywordScore", "keyword_score")) || 0,
    vectorScore: Number(pick(source, "vectorScore", "vector_score")) || 0,
    runHistoryScore: Number(pick(source, "runHistoryScore", "run_history_score")) || 0,
    penalty: Number(pick(source, "penalty")) || 0,
    finalScore: Number(pick(source, "finalScore", "final_score")) || 0,
    raw: source,
  };
}

export function normalizeOpsManualPreflightEvidence(input: unknown): OpsManualPreflightEvidence {
  const source = isRecord(input) ? input : {};
  return {
    name: text(pick(source, "name")),
    status: text(pick(source, "status")),
    value: pick(source, "value"),
    note: text(pick(source, "note")),
    raw: source,
  };
}

export function normalizeOpsManualPreflightResult(input: unknown): OpsManualPreflightResult {
  const source = isRecord(input) ? input : {};
  return {
    status: preflightStatus(pick(source, "status", "preflight_status")),
    ready: bool(pick(source, "ready"), false),
    reason: text(pick(source, "reason")),
    manualId: text(pick(source, "manualId", "manual_id")),
    workflowId: text(pick(source, "workflowId", "workflow_id")),
    probeId: text(pick(source, "probeId", "probe_id")),
    evidence: asArray(pick(source, "evidence")).map(normalizeOpsManualPreflightEvidence),
    missingPermissions: stringArray(pick(source, "missingPermissions", "missing_permissions")),
    environmentDiffs: stringArray(pick(source, "environmentDiffs", "environment_diffs")),
    nextAction: text(pick(source, "nextAction", "next_action")),
    checkedAt: text(pick(source, "checkedAt", "checked_at")),
    artifactType: text(pick(source, "artifactType", "artifact_type")),
    raw: input,
  };
}

export function normalizeOpsManualParamCandidate(input: unknown): OpsManualParamCandidateView {
  const source = isRecord(input) ? input : {};
  return {
    value: pick(source, "value", "id"),
    label: text(pick(source, "label", "name")),
    hint: text(pick(source, "hint")),
    source: text(pick(source, "source")),
    confidence: Number(pick(source, "confidence")) || 0,
    evidence: text(pick(source, "evidence")),
    raw: source,
  };
}

export function normalizeOpsManualResolvedParam(input: unknown): OpsManualResolvedParamView {
  const source = isRecord(input) ? input : {};
  return {
    id: text(pick(source, "id")),
    value: pick(source, "value"),
    source: text(pick(source, "source")),
    confidence: Number(pick(source, "confidence")) || 0,
    evidence: text(pick(source, "evidence")),
    confirmedByUser: bool(pick(source, "confirmedByUser", "confirmed_by_user"), false),
    needsUserConfirmation: bool(pick(source, "needsUserConfirmation", "needs_user_confirmation"), false),
    raw: source,
  };
}

export function normalizeOpsManualParamFormField(input: unknown): OpsManualParamFormFieldView {
  const source = isRecord(input) ? input : {};
  return {
    id: text(pick(source, "id")),
    label: text(pick(source, "label", "title")),
    type: text(pick(source, "type")),
    required: bool(pick(source, "required"), false),
    sensitive: bool(pick(source, "sensitive"), false),
    uiControl: text(pick(source, "uiControl", "ui_control")),
    placeholder: text(pick(source, "placeholder")),
    defaultValue: pick(source, "default", "defaultValue", "default_value"),
    candidates: asArray(pick(source, "candidates")).map(normalizeOpsManualParamCandidate),
    raw: source,
  };
}

export function normalizeOpsManualParamResolutionResult(input: unknown): OpsManualParamResolutionResult {
  const source = isRecord(input) ? input : {};
  return {
    status: text(pick(source, "status"), "unresolved"),
    manualId: text(pick(source, "manualId", "manual_id")),
    workflowId: text(pick(source, "workflowId", "workflow_id")),
    operationFrame: isRecord(pick(source, "operationFrame", "operation_frame")) ? (pick(source, "operationFrame", "operation_frame") as LooseRecord) : {},
    resolvedParams: asArray(pick(source, "resolvedParams", "resolved_params")).map(normalizeOpsManualResolvedParam),
    fields: asArray(pick(source, "fields", "formFields", "form_fields")).map(normalizeOpsManualParamFormField),
    nextAction: text(pick(source, "nextAction", "next_action")),
    artifactType: text(pick(source, "artifactType", "artifact_type")),
    raw: input,
  };
}

export function normalizeOpsManual(input: unknown): OpsManualView {
  const source = unwrapManual(input);
  const id = text(pick(source, "id", "manualId", "manual_id"), "unknown-manual");
  return {
    id,
    manualFamilyId: text(pick(source, "manualFamilyId", "manual_family_id")),
    title: text(pick(source, "title", "name"), id),
    status: text(pick(source, "status"), "draft"),
    version: text(pick(source, "version")),
    owner: text(pick(source, "owner")),
    workflowRef: normalizeWorkflowRef(pick(source, "workflowRef", "workflow_ref")),
    operation: normalizeOperationProfile(pick(source, "operation")),
    applicability: normalizeApplicability(pick(source, "applicability")),
    requiredContext: normalizeRequiredContext(pick(source, "requiredContext", "required_context")),
    parameterRules: isRecord(pick(source, "parameterRules", "parameter_rules")) ? (pick(source, "parameterRules", "parameter_rules") as LooseRecord) : {},
    preconditions: stringArray(pick(source, "preconditions")),
    validation: stringArray(pick(source, "validation")),
    cannotUseWhen: stringArray(pick(source, "cannotUseWhen", "cannot_use_when")),
    riskNotes: stringArray(pick(source, "riskNotes", "risk_notes")),
    documentMarkdown: text(pick(source, "documentMarkdown", "document_markdown", "summary", "description")),
    searchDoc: text(pick(source, "searchDoc", "search_doc")),
    metadata: isRecord(pick(source, "metadata")) ? (pick(source, "metadata") as LooseRecord) : {},
    createdAt: text(pick(source, "createdAt", "created_at")),
    updatedAt: text(pick(source, "updatedAt", "updated_at")),
    runRecordSummary: normalizeRunRecordSummary(pick(source, "runRecordSummary", "run_record_summary")),
    raw: source,
  };
}

export function normalizeOpsManualMatch(input: unknown): OpsManualMatchView {
  const source = isRecord(input) ? input : {};
  const manualValue = pick(source, "manual", "opsManual", "ops_manual");
  const manual = isRecord(manualValue) ? normalizeOpsManual(manualValue) : null;
  const manualId = text(pick(source, "manualId", "manual_id"), manual?.id || "");
  const manualTitle = text(pick(source, "manualTitle", "manual_title", "title"), manual?.title || manualId);
  return {
    manualId,
    manualTitle,
    state: text(pick(source, "state", "decisionState", "decision_state"), "no_match"),
    workflowRef: normalizeWorkflowRef(pick(source, "workflowRef", "workflow_ref") || manual?.workflowRef),
    reasons: stringArray(pick(source, "reasons", "reason")),
    missingContext: stringArray(pick(source, "missingContext", "missing_context")),
    compatibilityGaps: stringArray(pick(source, "compatibilityGaps", "compatibility_gaps")),
    recommendedNextActions: stringArray(pick(source, "recommendedNextActions", "recommended_next_actions")),
    runRecordSummary: normalizeRunRecordSummary(pick(source, "runRecordSummary", "run_record_summary")),
    manual,
    raw: source,
  };
}

export function normalizeOperationFrame(input: unknown): OperationFrameView {
  const source = isRecord(input) ? input : {};
  return {
    intent: text(pick(source, "intent")),
    rawText: text(pick(source, "rawText", "raw_text")),
    raw: source,
  };
}

export function normalizeSearchOperationFrame(input: unknown): SearchOpsManualsOperationFrameView {
  const source = isRecord(input) ? input : {};
  return {
    intent: text(pick(source, "intent")),
    objectType: text(pick(source, "objectType", "object_type"), text(pick(asRecord(pick(source, "target")), "type"))),
    operationType: text(pick(source, "operationType", "operation_type"), text(pick(asRecord(pick(source, "operation")), "action", "type"))),
    operationGoal: text(pick(source, "operationGoal", "operation_goal")),
    rawText: text(pick(source, "rawText", "raw_text", "text")),
    raw: source,
  };
}

export function normalizeSearchManualHit(input: unknown): SearchManualHitView {
  const source = isRecord(input) ? input : {};
  const manualValue = pick(source, "manual", "opsManual", "ops_manual");
  const manual = isRecord(manualValue) ? normalizeOpsManual(manualValue) : null;
  const manualId = text(pick(source, "manualId", "manual_id"), manual?.id || "");
  const workflowRef = normalizeWorkflowRef(pick(source, "workflowRef", "workflow_ref") || manual?.workflowRef);
  return {
    manualId,
    title: text(pick(source, "title", "manualTitle", "manual_title"), manual?.title || manualId),
    manualStatus: text(pick(source, "manualStatus", "manual_status"), manual?.status || ""),
    workflowStatus: text(pick(source, "workflowStatus", "workflow_status")),
    boundWorkflowId: text(pick(source, "boundWorkflowId", "bound_workflow_id", "workflowId", "workflow_id"), workflowRef.workflowId),
    matchLevel: text(pick(source, "matchLevel", "match_level")),
    usableMode: searchDecision(pick(source, "usableMode", "usable_mode", "decision", "state")),
    scoreBreakdown: normalizeScoreBreakdown(pick(source, "scoreBreakdown", "score_breakdown")),
    preflightStatus: preflightStatus(pick(source, "preflightStatus", "preflight_status")),
    matchedFields: stringArray(pick(source, "matchedFields", "matched_fields")),
    missingFields: stringArray(pick(source, "missingFields", "missing_fields", "missingContext", "missing_context")),
    environmentDiffs: stringArray(pick(source, "environmentDiffs", "environment_diffs", "compatibilityGaps", "compatibility_gaps")),
    blockedReasons: stringArray(pick(source, "blockedReasons", "blocked_reasons")),
    recommendedAction: text(pick(source, "recommendedAction", "recommended_action")),
    runRecordSummary: normalizeRunRecordSummary(pick(source, "runRecordSummary", "run_record_summary")),
    manual,
    raw: source,
  };
}

export function normalizeOpsManualSearchResult(input: unknown): SearchOpsManualsResult {
  const source = isRecord(input) ? input : {};
  return {
    decision: searchDecision(pick(source, "decision", "state")),
    summary: text(pick(source, "summary", "message")),
    operationFrame: normalizeSearchOperationFrame(pick(source, "operationFrame", "operation_frame")),
    manuals: asArray(pick(source, "manuals", "hits", "matches", "items")).map(normalizeSearchManualHit),
    nextQuestions: stringArray(pick(source, "nextQuestions", "next_questions")),
    recommendedNextAction: text(pick(source, "recommendedNextAction", "recommended_next_action")),
    searchedFields: stringArray(pick(source, "searchedFields", "searched_fields")),
    raw: input,
  };
}

export function normalizeOpsManualCandidate(input: unknown): OpsManualCandidateView {
  const source = isRecord(input) ? input : {};
  const id = text(pick(source, "id", "candidateId", "candidate_id"), "unknown-candidate");
  return {
    id,
    sourceType: text(pick(source, "sourceType", "source_type")),
    sourceRefs: stringArray(pick(source, "sourceRefs", "source_refs")),
    proposedManual: normalizeOpsManual(pick(source, "proposedManual", "proposed_manual", "manual")),
    validationReport: stringArray(pick(source, "validationReport", "validation_report")),
    reviewStatus: text(pick(source, "reviewStatus", "review_status"), "pending"),
    reviewer: text(pick(source, "reviewer")),
    reviewNote: text(pick(source, "reviewNote", "review_note")),
    createdAt: text(pick(source, "createdAt", "created_at")),
    updatedAt: text(pick(source, "updatedAt", "updated_at")),
    raw: source,
  };
}

export function normalizeRunRecord(input: unknown): RunRecordView {
  const source = isRecord(input) ? input : {};
  const id = text(pick(source, "id", "runId", "run_id"), "unknown-run");
  return {
    id,
    manualId: text(pick(source, "manualId", "manual_id")),
    workflowId: text(pick(source, "workflowId", "workflow_id")),
    workflowVersion: text(pick(source, "workflowVersion", "workflow_version")),
    workflowDigest: text(pick(source, "workflowDigest", "workflow_digest")),
    dryRunStatus: text(pick(source, "dryRunStatus", "dry_run_status")),
    executionStatus: text(pick(source, "executionStatus", "execution_status", "status")),
    validationStatus: text(pick(source, "validationStatus", "validation_status")),
    rollbackStatus: text(pick(source, "rollbackStatus", "rollback_status")),
    failureReason: text(pick(source, "failureReason", "failure_reason")),
    operator: text(pick(source, "operator")),
    startedAt: text(pick(source, "startedAt", "started_at")),
    completedAt: text(pick(source, "completedAt", "completed_at")),
    raw: source,
  };
}

export function normalizeOpsManualList(input: unknown): OpsManualListView {
  const source = isRecord(input) ? input : {};
  return {
    items: asArray(pick(source, "items", "manuals", "opsManuals", "ops_manuals")).map(normalizeOpsManual),
    nextCursor: text(pick(source, "nextCursor", "next_cursor", "cursor")),
    total: listTotal(source),
    raw: input,
  };
}

export function normalizeOpsManualMatchList(input: unknown): OpsManualMatchListView {
  const source = isRecord(input) ? input : {};
  return {
    items: asArray(pick(source, "items", "matches", "manualMatches", "manual_matches")).map(normalizeOpsManualMatch),
    nextCursor: text(pick(source, "nextCursor", "next_cursor", "cursor")),
    total: listTotal(source),
    raw: input,
  };
}

export function normalizeOpsManualCandidateList(input: unknown): OpsManualCandidateListView {
  const source = isRecord(input) ? input : {};
  return {
    items: asArray(pick(source, "items", "candidates", "manualCandidates", "manual_candidates")).map(normalizeOpsManualCandidate),
    nextCursor: text(pick(source, "nextCursor", "next_cursor", "cursor")),
    total: listTotal(source),
    raw: input,
  };
}

export function normalizeRunRecordList(input: unknown): RunRecordListView {
  const source = isRecord(input) ? input : {};
  return {
    items: asArray(pick(source, "items", "runRecords", "run_records", "records")).map(normalizeRunRecord),
    nextCursor: text(pick(source, "nextCursor", "next_cursor", "cursor")),
    total: listTotal(source),
    raw: input,
  };
}

export function createOpsManualsApi(client: OpsManualsHttpClient = httpClient) {
  return {
    async list(params: Record<string, string | number | boolean> = {}) {
      const payload = await client.get(endpoint("/api/v1/ops-manuals", params));
      return normalizeOpsManualList(payload);
    },

    async get(id: string) {
      const payload = await client.get(`/api/v1/ops-manuals/${encodePath(id)}`);
      return normalizeOpsManual(payload);
    },

    async retrieve(payload: Record<string, unknown>) {
      const response = await client.post("/api/v1/ops-manuals/retrieve", payload);
      return normalizeOpsManualMatchList(response);
    },

    async searchOpsManuals(payload: SearchOpsManualsRequest) {
      const response = await client.post("/api/v1/ops-manuals/search", payload);
      return normalizeOpsManualSearchResult(response);
    },

    async runOpsManualPreflight(payload: OpsManualPreflightRequest) {
      const response = await client.post("/api/v1/ops-manuals/preflight", payload);
      return normalizeOpsManualPreflightResult(response);
    },

    async resolveOpsManualParams(payload: ResolveOpsManualParamsRequest) {
      const response = await client.post("/api/v1/ops-manuals/resolve-params", payload);
      return normalizeOpsManualParamResolutionResult(response);
    },

    async listCandidates(params: Record<string, string | number | boolean> = {}) {
      const payload = await client.get(endpoint("/api/v1/ops-manuals/candidates", params));
      return normalizeOpsManualCandidateList(payload);
    },

    async prepareCandidate(payload: Record<string, unknown>) {
      const response = await client.post("/api/v1/ops-manuals/candidates/prepare", payload);
      return normalizeOpsManualCandidate(response);
    },

    async confirmCandidate(id: string, payload: Record<string, unknown>) {
      const response = await client.post(`/api/v1/ops-manuals/candidates/${encodePath(id)}/confirm`, payload);
      return normalizeOpsManual(response);
    },

    async listRunRecords(manualId: string, params: Record<string, string | number | boolean> = {}) {
      const payload = await client.get(endpoint(`/api/v1/ops-manuals/${encodePath(manualId)}/run-records`, params));
      return normalizeRunRecordList(payload);
    },

    async listAllRunRecords(params: Record<string, string | number | boolean> = {}) {
      const payload = await client.get(endpoint("/api/v1/ops-manuals/run-records", params));
      return normalizeRunRecordList(payload);
    },
  };
}

export const opsManualsApi = createOpsManualsApi();

export async function searchOpsManuals(request: SearchOpsManualsRequest): Promise<SearchOpsManualsResult> {
  return opsManualsApi.searchOpsManuals(request);
}

export async function runOpsManualPreflight(request: OpsManualPreflightRequest): Promise<OpsManualPreflightResult> {
  return opsManualsApi.runOpsManualPreflight(request);
}

export async function resolveOpsManualParams(request: ResolveOpsManualParamsRequest): Promise<OpsManualParamResolutionResult> {
  return opsManualsApi.resolveOpsManualParams(request);
}
