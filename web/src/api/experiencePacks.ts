import httpClient from "./httpClient";

type LooseRecord = Record<string, unknown>;

export type RetrievalEvalView = {
  score: number | null;
  matchedCases: number;
  verdict: string;
  lastEvaluatedAt: string;
  raw: LooseRecord;
};

export type WorkflowBindingView = {
  workflowId: string;
  workflowName: string;
  status: string;
  version: string;
  raw: LooseRecord;
};

export type ValidationGateView = {
  status: string;
  passed: boolean;
  reasons: string[];
  validators: string[];
  raw: LooseRecord;
};

export type SkillAssetView = {
  id: string;
  name: string;
  summary: string;
  path: string;
  raw: LooseRecord;
};

export type RunnerBindingView = {
  id: string;
  workflowId: string;
  workflowName: string;
  status: string;
  version: string;
  osVariant: string;
  raw: LooseRecord;
};

export type GEPHistoryView = {
  successCount: number;
  failureCount: number;
  recentResult: string;
  raw: LooseRecord;
};

export type AdvancedRefsView = {
  geneAssetId: string;
  capsuleAssetIds: string[];
  raw: LooseRecord;
};

export type AuthorizationScopeView = {
  id: string;
  type: string;
  value: string;
  searchable: boolean;
  reason: string;
  raw: LooseRecord;
};

export type ExperiencePackView = {
  id: string;
  title: string;
  summary: string;
  version: string;
  category: string;
  usageShape: string;
  middleware: string;
  tags: string[];
  status: string;
  reviewStatus: string;
  enabled: boolean;
  searchable: boolean;
  searchableReason: string;
  skill: SkillAssetView;
  validationGate: ValidationGateView;
  history: GEPHistoryView;
  advancedRefs: AdvancedRefsView;
  retrievalEval: RetrievalEvalView;
  workflowBinding: WorkflowBindingView;
  runnerBindings: RunnerBindingView[];
  authorizationScopes: AuthorizationScopeView[];
  raw: LooseRecord;
};

export type ExperienceCandidateView = {
  id: string;
  packId: string;
  title: string;
  summary: string;
  status: string;
  matchReason: string;
  sourceCaseId: string;
  experiencePack: ExperiencePackView | null;
  raw: LooseRecord;
};

export type ReuseRecordView = {
  id: string;
  packId: string;
  caseId: string;
  result: string;
  summary: string;
  reusedBy: string;
  reusedAt: string;
  raw: LooseRecord;
};

export type ExperiencePackListView = {
  items: ExperienceCandidateView[];
  nextCursor: string;
  total: number | null;
  raw: unknown;
};

export type ReuseRecordListView = {
  items: ReuseRecordView[];
  nextCursor: string;
  total: number | null;
  raw: unknown;
};

export type ExperiencePackLibraryListView = {
  items: ExperiencePackView[];
  nextCursor: string;
  total: number | null;
  raw: unknown;
};

export type ExperienceMatchView = {
  packId: string;
  skill: SkillAssetView;
  confidence: number | null;
  compatibilityStatus: string;
  compatibilityGaps: string[];
  matchedSignals: string[];
  matchReasons: string[];
  preconditionGaps: string[];
  riskWarnings: string[];
  nextActions: string[];
  osVariant: string;
  runnerBinding: RunnerBindingView;
  history: GEPHistoryView;
  advancedRefs: AdvancedRefsView;
  raw: LooseRecord;
};

export type ExperienceMatchListView = {
  items: ExperienceMatchView[];
  total: number | null;
  raw: unknown;
};

export type ExperienceSuggestionView = {
  id: string;
  type: string;
  label: string;
  reason: string;
  sourceRefs: string[];
  raw: LooseRecord;
};

export type ExperienceSuggestionListView = {
  items: ExperienceSuggestionView[];
  raw: unknown;
};

export type RunnerCandidateView = {
  id: string;
  packId: string;
  workflowId: string;
  workflowName: string;
  status: string;
  studioDraftLink: string;
  workflow: LooseRecord;
  graph: LooseRecord;
  runnerBinding: RunnerBindingView;
  raw: LooseRecord;
};

export type ListExperiencePacksParams = {
  status?: string | null;
  category?: string | null;
  usageShape?: string | null;
  middleware?: string | null;
  tag?: string | null;
  hasRunnerBinding?: boolean | string | null;
  limit?: number | string | null;
  cursor?: string | null;
};

export type ListExperienceCandidatesParams = {
  caseId?: string | null;
  service?: string | null;
  environment?: string | null;
  limit?: number | string | null;
  cursor?: string | null;
};

export type ApproveExperienceCandidatePayload = {
  reviewer?: string;
  comment?: string;
  [key: string]: unknown;
};

export type SaveAuthorizationScopesPayload = {
  scopes: Array<Partial<AuthorizationScopeView> & Record<string, unknown>>;
};

export type RetrieveExperiencePacksPayload = Record<string, unknown>;
export type EvaluateExperiencePackSuggestionsPayload = Record<string, unknown>;
export type PrepareExperiencePackCandidatePayload = Record<string, unknown>;
export type ConfirmExperiencePackCandidatePayload = { confirmationToken?: string } & Record<string, unknown>;
export type ReviewExperiencePackPayload = Record<string, unknown>;
export type EnableExperiencePackPayload = Record<string, unknown>;
export type PauseExperiencePackPayload = Record<string, unknown>;
export type ValidationGateCheckPayload = Record<string, unknown>;
export type PrepareRunnerCandidatePayload = Record<string, unknown>;
export type ConfirmRunnerCandidatePayload = { confirmationToken?: string } & Record<string, unknown>;
export type ReviewRunnerBindingPayload = Record<string, unknown>;

export type ListReuseRecordsParams = {
  caseId?: string | null;
  limit?: number | string | null;
  cursor?: string | null;
};

type ExperiencePacksHttpClient = {
  get(path: string): Promise<unknown>;
  post(path: string, body?: unknown): Promise<unknown>;
  put(path: string, body?: unknown): Promise<unknown>;
  patch?: (path: string, body?: unknown) => Promise<unknown>;
};

function isRecord(value: unknown): value is LooseRecord {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
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

function textArray(value: unknown): string[] {
  return asArray(value).map((item) => text(item)).filter(Boolean);
}

function optionalNumber(value: unknown) {
  const numeric = Number(value);
  return Number.isFinite(numeric) ? numeric : null;
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

const RUNNER_LOCAL_DRAFTS_KEY = "runner.studio.localDrafts";

function unwrapPack(input: unknown): LooseRecord {
  if (!isRecord(input)) return {};
  const wrapped = pick(input, "pack", "experiencePack", "experience_pack", "item");
  return isRecord(wrapped) ? wrapped : input;
}

function listTotal(source: LooseRecord) {
  const total = pick(source, "total", "totalCount", "total_count");
  const numericTotal = Number(total);
  return Number.isFinite(numericTotal) ? numericTotal : null;
}

export function normalizeRetrievalEval(input: unknown): RetrievalEvalView {
  const source = isRecord(input) ? input : {};
  return {
    score: optionalNumber(pick(source, "score", "retrievalScore", "retrieval_score")),
    matchedCases: integer(pick(source, "matchedCases", "matched_cases", "matches")),
    verdict: text(pick(source, "verdict", "status")),
    lastEvaluatedAt: text(pick(source, "lastEvaluatedAt", "last_evaluated_at", "updatedAt", "updated_at")),
    raw: source,
  };
}

export function normalizeWorkflowBinding(input: unknown): WorkflowBindingView {
  const source = isRecord(input) ? input : {};
  const workflowId = text(pick(source, "workflowId", "workflow_id", "id"), "unknown-workflow");
  return {
    workflowId,
    workflowName: text(pick(source, "workflowName", "workflow_name", "name", "title"), workflowId),
    status: text(pick(source, "status", "state"), "unbound"),
    version: text(pick(source, "version", "workflowVersion", "workflow_version")),
    raw: source,
  };
}

export function normalizeSkillAsset(input: unknown): SkillAssetView {
  const source = isRecord(input) ? input : {};
  const id = text(pick(source, "id", "skillId", "skill_id", "assetId", "asset_id"));
  const name = text(pick(source, "name", "title"), id || "Skill");
  return {
    id,
    name,
    summary: text(pick(source, "summary", "description")),
    path: text(pick(source, "path", "file", "filePath", "file_path"), "skills/SKILL.md"),
    raw: source,
  };
}

export function normalizeValidationGate(input: unknown): ValidationGateView {
  const source = isRecord(input) ? input : {};
  const status = text(pick(source, "status", "state"), "unknown");
  const passed = bool(pick(source, "passed", "ok", "allowed"), status === "passed" || status === "pass" || status === "ok");
  return {
    status,
    passed,
    reasons: textArray(pick(source, "reasons", "reason", "blockedReasons", "blocked_reasons")),
    validators: textArray(pick(source, "validators", "validationItems", "validation_items", "checks")),
    raw: source,
  };
}

export function normalizeRunnerBinding(input: unknown): RunnerBindingView {
  const source = isRecord(input) ? input : {};
  const workflowId = text(pick(source, "workflowId", "workflow_id", "id"), "unknown-workflow");
  return {
    id: text(pick(source, "id", "bindingId", "binding_id"), workflowId),
    workflowId,
    workflowName: text(pick(source, "workflowName", "workflow_name", "name", "title"), workflowId),
    status: text(pick(source, "status", "state"), "unbound"),
    version: text(pick(source, "version", "workflowVersion", "workflow_version")),
    osVariant: text(pick(source, "osVariant", "os_variant", "os")),
    raw: source,
  };
}

export function normalizeGEPHistory(input: unknown): GEPHistoryView {
  const source = isRecord(input) ? input : {};
  return {
    successCount: integer(pick(source, "successCount", "success_count", "successes")),
    failureCount: integer(pick(source, "failureCount", "failure_count", "failures")),
    recentResult: text(pick(source, "recentResult", "recent_result", "lastResult", "last_result")),
    raw: source,
  };
}

export function normalizeAdvancedRefs(input: unknown): AdvancedRefsView {
  const source = isRecord(input) ? input : {};
  return {
    geneAssetId: text(pick(source, "geneAssetId", "gene_asset_id")),
    capsuleAssetIds: textArray(pick(source, "capsuleAssetIds", "capsule_asset_ids")),
    raw: source,
  };
}

export function normalizeAuthorizationScope(input: unknown): AuthorizationScopeView {
  const source = isRecord(input) ? input : {};
  const type = text(pick(source, "type", "scopeType", "scope_type", "kind"), "unknown");
  const value = text(pick(source, "value", "scopeValue", "scope_value", "name", "id"));
  const searchable = bool(pick(source, "searchable", "authorized", "enabled"), false);
  return {
    id: text(pick(source, "id", "scopeId", "scope_id"), value ? `${type}:${value}` : type),
    type,
    value,
    searchable,
    reason: text(pick(source, "reason", "description")),
    raw: source,
  };
}

function searchableState(reviewStatus: string, enabled: boolean, scopes: AuthorizationScopeView[]) {
  if (reviewStatus !== "approved") {
    return { searchable: false, searchableReason: "经验包尚未审核通过，不能被检索" };
  }
  if (!enabled) {
    return { searchable: false, searchableReason: "经验包已停用，不能被检索" };
  }
  if (!scopes.some((scope) => scope.searchable)) {
    return { searchable: false, searchableReason: "经验包尚未配置可检索授权范围" };
  }
  return { searchable: true, searchableReason: "已审核启用，且已配置可检索授权范围" };
}

export function normalizeExperiencePack(input: unknown): ExperiencePackView {
  const source = unwrapPack(input);
  const id = text(pick(source, "packId", "pack_id", "id"), "unknown-pack");
  const reviewStatus = text(pick(source, "reviewStatus", "review_status", "approvalStatus", "approval_status"), "pending");
  const enabled = bool(pick(source, "enabled", "isEnabled", "is_enabled"), text(pick(source, "status", "state")) !== "disabled");
  const authorizationScopes = asArray(pick(source, "authorizationScopes", "authorization_scopes", "scopes")).map(
    normalizeAuthorizationScope,
  );
  const runnerBindingSource = pick(source, "runnerBindings", "runner_bindings");
  const runnerBindings = asArray(runnerBindingSource).map(normalizeRunnerBinding);
  const workflowBinding = normalizeWorkflowBinding(pick(source, "workflowBinding", "workflow_binding"));
  const searchability = searchableState(reviewStatus, enabled, authorizationScopes);
  return {
    id,
    title: text(pick(source, "title", "name"), id),
    summary: text(pick(source, "summary", "description")),
    version: text(pick(source, "version")),
    category: text(pick(source, "category", "type", "experienceType", "experience_type"), "repair"),
    usageShape: text(pick(source, "usageShape", "usage_shape", "usage", "form"), "diagnostic"),
    middleware: text(pick(source, "middleware", "middlewareType", "middleware_type")),
    tags: textArray(pick(source, "tags", "middlewareTags", "middleware_tags")),
    status: text(pick(source, "status", "state"), enabled ? "enabled" : "disabled"),
    reviewStatus,
    enabled,
    ...searchability,
    skill: normalizeSkillAsset(pick(source, "skill", "skillAsset", "skill_asset")),
    validationGate: normalizeValidationGate(pick(source, "validationGate", "validation_gate")),
    history: normalizeGEPHistory(pick(source, "history", "effect", "historicalEffect", "historical_effect")),
    advancedRefs: normalizeAdvancedRefs(pick(source, "advancedRefs", "advanced_refs")),
    retrievalEval: normalizeRetrievalEval(pick(source, "retrievalEval", "retrieval_eval")),
    workflowBinding,
    runnerBindings: runnerBindings.length ? runnerBindings : (workflowBinding.workflowId !== "unknown-workflow" ? [normalizeRunnerBinding(workflowBinding.raw)] : []),
    authorizationScopes,
    raw: source,
  };
}

export function normalizeExperienceCandidate(input: unknown): ExperienceCandidateView {
  const source = isRecord(input) ? input : {};
  const packId = text(pick(source, "packId", "pack_id"), text(pick(source, "id"), "unknown-pack"));
  const embeddedPack = pick(source, "experiencePack", "experience_pack", "pack");
  return {
    id: text(pick(source, "id", "candidateId", "candidate_id"), packId),
    packId,
    title: text(pick(source, "title", "name"), packId),
    summary: text(pick(source, "summary", "description")),
    status: text(pick(source, "status", "state"), "candidate"),
    matchReason: text(pick(source, "matchReason", "match_reason", "reason")),
    sourceCaseId: text(pick(source, "sourceCaseId", "source_case_id", "caseId", "case_id")),
    experiencePack: isRecord(embeddedPack) ? normalizeExperiencePack(embeddedPack) : null,
    raw: source,
  };
}

export function normalizeReuseRecord(input: unknown): ReuseRecordView {
  const source = isRecord(input) ? input : {};
  const id = text(pick(source, "id", "reuseId", "reuse_id"), "unknown-reuse");
  return {
    id,
    packId: text(pick(source, "packId", "pack_id", "experiencePackId", "experience_pack_id"), "unknown-pack"),
    caseId: text(pick(source, "caseId", "case_id", "incidentId", "incident_id")),
    result: text(pick(source, "result", "status", "state"), "unknown"),
    summary: text(pick(source, "summary", "description")),
    reusedBy: text(pick(source, "reusedBy", "reused_by", "actor")),
    reusedAt: text(pick(source, "reusedAt", "reused_at", "createdAt", "created_at")),
    raw: source,
  };
}

export function normalizeExperiencePackList(input: unknown): ExperiencePackListView {
  const source = isRecord(input) ? input : {};
  return {
    items: asArray(pick(source, "items", "candidates", "experienceCandidates", "experience_candidates")).map(
      normalizeExperienceCandidate,
    ),
    nextCursor: text(pick(source, "nextCursor", "next_cursor", "cursor")),
    total: listTotal(source),
    raw: input,
  };
}

export function normalizeExperiencePackLibraryList(input: unknown): ExperiencePackLibraryListView {
  const source = isRecord(input) ? input : {};
  return {
    items: asArray(pick(source, "items", "packs", "experiencePacks", "experience_packs")).map(normalizeExperiencePack),
    nextCursor: text(pick(source, "nextCursor", "next_cursor", "cursor")),
    total: listTotal(source),
    raw: input,
  };
}

export function normalizeExperienceMatch(input: unknown): ExperienceMatchView {
  const source = isRecord(input) ? input : {};
  const packId = text(pick(source, "packId", "pack_id", "id"), "unknown-pack");
  const { gene: _gene, genes: _genes, capsule: _capsule, capsules: _capsules, ...safeRaw } = source;
  return {
    packId,
    skill: normalizeSkillAsset(pick(source, "skill", "skillAsset", "skill_asset")),
    confidence: optionalNumber(pick(source, "confidence", "score")),
    compatibilityStatus: text(pick(source, "compatibilityStatus", "compatibility_status")),
    compatibilityGaps: textArray(pick(source, "compatibilityGaps", "compatibility_gaps")),
    matchedSignals: textArray(pick(source, "matchedSignals", "matched_signals", "signals")),
    matchReasons: textArray(pick(source, "matchReasons", "match_reasons", "reasons", "reason")),
    preconditionGaps: textArray(pick(source, "preconditionGaps", "precondition_gaps")),
    riskWarnings: textArray(pick(source, "riskWarnings", "risk_warnings", "risks")),
    nextActions: textArray(pick(source, "nextActions", "next_actions")),
    osVariant: text(pick(source, "osVariant", "os_variant", "os")),
    runnerBinding: normalizeRunnerBinding(pick(source, "runnerBinding", "runner_binding")),
    history: normalizeGEPHistory(pick(source, "history", "historicalEffect", "historical_effect")),
    advancedRefs: normalizeAdvancedRefs(pick(source, "advancedRefs", "advanced_refs")),
    raw: safeRaw,
  };
}

export function normalizeExperienceMatchList(input: unknown): ExperienceMatchListView {
  const source = isRecord(input) ? input : {};
  return {
    items: asArray(pick(source, "items", "matches", "experienceMatches", "experience_matches")).map(normalizeExperienceMatch),
    total: listTotal(source),
    raw: input,
  };
}

export function normalizeExperienceSuggestion(input: unknown): ExperienceSuggestionView {
  const source = isRecord(input) ? input : {};
  const id = text(pick(source, "id", "suggestionId", "suggestion_id", "type"), "suggestion");
  return {
    id,
    type: text(pick(source, "type", "kind"), id),
    label: text(pick(source, "label", "title", "name"), id),
    reason: text(pick(source, "reason", "summary")),
    sourceRefs: textArray(pick(source, "sourceRefs", "source_refs", "references")),
    raw: source,
  };
}

export function normalizeExperienceSuggestionList(input: unknown): ExperienceSuggestionListView {
  const source = isRecord(input) ? input : {};
  return {
    items: asArray(pick(source, "items", "suggestions", "actions")).map(normalizeExperienceSuggestion),
    raw: input,
  };
}

export function normalizeRunnerCandidate(input: unknown): RunnerCandidateView {
  const source = isRecord(input) ? input : {};
  const workflowSource = pick(source, "workflow", "draft", "runnerWorkflow", "runner_workflow");
  const workflow = isRecord(workflowSource) ? workflowSource : {};
  const graphSource = pick(source, "graph", "workflowGraph", "workflow_graph");
  const workflowGraph = isRecord(workflow.graph) ? workflow.graph : {};
  const graph = isRecord(graphSource) ? graphSource : workflowGraph;
  const graphWorkflow = isRecord(graph.workflow) ? graph.workflow : {};
  const id = text(pick(source, "id", "candidateId", "candidate_id"), text(pick(workflow, "id", "name"), "runner-candidate"));
  const workflowId = text(
    pick(source, "workflowId", "workflow_id", "workflowName", "workflow_name"),
    text(pick(workflow, "id", "name"), text(pick(graphWorkflow, "name"), id)),
  );
  return {
    id,
    packId: text(pick(source, "packId", "pack_id")),
    workflowId,
    workflowName: text(pick(source, "workflowName", "workflow_name", "title", "name"), text(pick(workflow, "title", "display_name", "name"), workflowId)),
    status: text(pick(source, "status", "state"), text(pick(workflow, "status"), "draft")),
    studioDraftLink: text(pick(source, "studioDraftLink", "studio_draft_link", "draftLink", "draft_link"), workflowId ? `/runner/${encodeURIComponent(workflowId)}` : ""),
    workflow,
    graph,
    runnerBinding: normalizeRunnerBinding(pick(source, "runnerBinding", "runner_binding")),
    raw: source,
  };
}

function canUseLocalDraftStorage() {
  return typeof window !== "undefined" && Boolean(window.localStorage);
}

function readLocalDrafts(): LooseRecord {
  if (!canUseLocalDraftStorage()) return {};
  try {
    const parsed = JSON.parse(window.localStorage.getItem(RUNNER_LOCAL_DRAFTS_KEY) || "{}");
    return isRecord(parsed) ? parsed : {};
  } catch (_cause) {
    return {};
  }
}

function writeLocalDrafts(drafts: LooseRecord) {
  if (!canUseLocalDraftStorage()) return;
  window.localStorage.setItem(RUNNER_LOCAL_DRAFTS_KEY, JSON.stringify(drafts));
}

export function saveRunnerCandidateLocalDraft(candidate: RunnerCandidateView) {
  const workflowId = text(candidate.workflowId || candidate.id);
  if (!workflowId) return null;
  const existing = readLocalDrafts();
  const sequence =
    Object.values(existing).reduce((max, item) => {
      const record = isRecord(item) ? item : {};
      return Math.max(max, Number(record.local_sequence || 0));
    }, 0) + 1;
  const workflowGraph = isRecord(candidate.workflow.graph) ? candidate.workflow.graph : {};
  const graphSource = Object.keys(candidate.graph).length ? candidate.graph : workflowGraph;
  const graphWorkflow = isRecord(graphSource.workflow) ? graphSource.workflow : {};
  const graph = {
    version: text(graphSource.version, "v1"),
    ...graphSource,
    workflow: {
      ...graphWorkflow,
      name: text(graphWorkflow.name, workflowId),
      title: text(graphWorkflow.title, candidate.workflowName || workflowId),
    },
    nodes: asArray(pick(graphSource, "nodes")),
    edges: asArray(pick(graphSource, "edges")),
  };
  const draft = {
    ...candidate.workflow,
    id: workflowId,
    name: workflowId,
    title: candidate.workflowName || workflowId,
    status: "draft",
    local_draft: true,
    ai_generated_draft: true,
    graph,
    updated_at: new Date().toISOString(),
    local_sequence: sequence,
    experience_pack_binding: {
      ...(isRecord(candidate.workflow.experience_pack_binding) ? candidate.workflow.experience_pack_binding : {}),
      pack_id: candidate.packId,
      runner_candidate_id: candidate.id,
    },
  };
  writeLocalDrafts({ ...existing, [workflowId]: draft });
  return draft;
}

export function normalizeReuseRecordList(input: unknown): ReuseRecordListView {
  const source = isRecord(input) ? input : {};
  return {
    items: asArray(pick(source, "items", "reuseRecords", "reuse_records", "records")).map(normalizeReuseRecord),
    nextCursor: text(pick(source, "nextCursor", "next_cursor", "cursor")),
    total: listTotal(source),
    raw: input,
  };
}

export function createExperiencePacksApi(client: ExperiencePacksHttpClient = httpClient) {
  return {
    async listPacks(params: ListExperiencePacksParams = {}) {
      const payload = await client.get(
        endpoint("/api/v1/experience-packs", {
          status: params.status,
          category: params.category,
          usage_shape: params.usageShape,
          middleware: params.middleware,
          tag: params.tag,
          has_runner_binding: params.hasRunnerBinding,
          limit: params.limit,
          cursor: params.cursor,
        }),
      );
      return normalizeExperiencePackLibraryList(payload);
    },

    async getPack(packId: string) {
      return normalizeExperiencePack(await client.get(`/api/v1/experience-packs/${encodePath(packId)}`));
    },

    async getSkill(packId: string) {
      return normalizeSkillAsset(await client.get(`/api/v1/experience-packs/${encodePath(packId)}/skill`));
    },

    async listPackFiles(packId: string) {
      return client.get(`/api/v1/experience-packs/${encodePath(packId)}/files`);
    },

    async listCapsules(packId: string) {
      return client.get(`/api/v1/experience-packs/${encodePath(packId)}/capsules`);
    },

    async listEvents(packId: string) {
      return client.get(`/api/v1/experience-packs/${encodePath(packId)}/events`);
    },

    async listMemoryEvents(packId: string) {
      return client.get(`/api/v1/experience-packs/${encodePath(packId)}/memory-events`);
    },

    async listAvoidCues(packId: string) {
      return client.get(`/api/v1/experience-packs/${encodePath(packId)}/avoid-cues`);
    },

    async listCandidates(params: ListExperienceCandidatesParams = {}) {
      const payload = await client.get(
        endpoint("/api/v1/experience-packs/candidates", {
          case_id: params.caseId,
          service: params.service,
          environment: params.environment,
          limit: params.limit,
          cursor: params.cursor,
        }),
      );
      return normalizeExperiencePackList(payload);
    },

    async approveCandidate(candidateId: string, payload: ApproveExperienceCandidatePayload = {}) {
      const response = await client.post(`/api/v1/experience-packs/candidates/${encodePath(candidateId)}/approve`, payload);
      return normalizeExperiencePack(response);
    },

    async setPackEnabled(packId: string, enabled: boolean) {
      const path = `/api/v1/experience-packs/${encodePath(packId)}/enabled`;
      const payload = { enabled };
      const response = client.patch ? await client.patch(path, payload) : await client.put(path, payload);
      return normalizeExperiencePack(response);
    },

    async saveAuthorizationScopes(packId: string, payload: SaveAuthorizationScopesPayload) {
      const response = await client.put(`/api/v1/experience-packs/${encodePath(packId)}/authorization-scopes`, payload);
      return normalizeExperiencePack(response);
    },

    async listReuseRecords(packId: string, params: ListReuseRecordsParams = {}) {
      const payload = await client.get(
        endpoint(`/api/v1/experience-packs/${encodePath(packId)}/reuse-records`, {
          case_id: params.caseId,
          limit: params.limit,
          cursor: params.cursor,
        }),
      );
      return normalizeReuseRecordList(payload);
    },

    async retrieve(payload: RetrieveExperiencePacksPayload) {
      return normalizeExperienceMatchList(await client.post("/api/v1/experience-packs/retrieve", payload));
    },

    async evaluateSuggestions(payload: EvaluateExperiencePackSuggestionsPayload) {
      return normalizeExperienceSuggestionList(await client.post("/api/v1/experience-packs/suggestions/evaluate", payload));
    },

    async prepareCandidate(payload: PrepareExperiencePackCandidatePayload) {
      return client.post("/api/v1/experience-packs/candidates/prepare", payload);
    },

    async confirmCandidate(payload: ConfirmExperiencePackCandidatePayload) {
      return client.post("/api/v1/experience-packs/candidates/confirm", payload);
    },

    async reviewPack(packId: string, payload: ReviewExperiencePackPayload) {
      return normalizeExperiencePack(await client.post(`/api/v1/experience-packs/${encodePath(packId)}/review`, payload));
    },

    async enablePack(packId: string, payload: EnableExperiencePackPayload = {}) {
      return normalizeExperiencePack(await client.post(`/api/v1/experience-packs/${encodePath(packId)}/enable`, payload));
    },

    async pausePack(packId: string, payload: PauseExperiencePackPayload = {}) {
      return normalizeExperiencePack(await client.post(`/api/v1/experience-packs/${encodePath(packId)}/pause`, payload));
    },

    async getValidationGate(packId: string) {
      return normalizeValidationGate(await client.get(`/api/v1/experience-packs/${encodePath(packId)}/validation-gate`));
    },

    async checkValidationGate(packId: string, payload: ValidationGateCheckPayload) {
      return normalizeValidationGate(await client.post(`/api/v1/experience-packs/${encodePath(packId)}/validation-gate/check`, payload));
    },

    async prepareRunnerCandidate(payload: PrepareRunnerCandidatePayload) {
      return normalizeRunnerCandidate(await client.post("/api/v1/experience-packs/runner-candidates/prepare", payload));
    },

    async confirmRunnerCandidate(payload: ConfirmRunnerCandidatePayload) {
      const candidate = normalizeRunnerCandidate(await client.post("/api/v1/experience-packs/runner-candidates/confirm", payload));
      saveRunnerCandidateLocalDraft(candidate);
      return candidate;
    },

    async listRunnerBindings(packId: string) {
      const payload = await client.get(`/api/v1/experience-packs/${encodePath(packId)}/runner-bindings`);
      const source = isRecord(payload) ? payload : {};
      return asArray(pick(source, "items", "bindings", "runnerBindings", "runner_bindings")).map(normalizeRunnerBinding);
    },

    async reviewRunnerBinding(packId: string, bindingId: string, payload: ReviewRunnerBindingPayload) {
      return normalizeRunnerBinding(await client.post(`/api/v1/experience-packs/${encodePath(packId)}/runner-bindings/${encodePath(bindingId)}/review`, payload));
    },
  };
}

const experiencePacksApi = createExperiencePacksApi();

export const listExperiencePackCandidates = experiencePacksApi.listCandidates;
export const listExperiencePacks = experiencePacksApi.listPacks;
export const getExperiencePack = experiencePacksApi.getPack;
export const retrieveExperiencePacks = experiencePacksApi.retrieve;
export const evaluateExperiencePackSuggestions = experiencePacksApi.evaluateSuggestions;
export const prepareExperiencePackCandidate = experiencePacksApi.prepareCandidate;
export const confirmExperiencePackCandidate = experiencePacksApi.confirmCandidate;
export const reviewExperiencePack = experiencePacksApi.reviewPack;
export const enableExperiencePack = experiencePacksApi.enablePack;
export const pauseExperiencePack = experiencePacksApi.pausePack;
export const getExperiencePackValidationGate = experiencePacksApi.getValidationGate;
export const checkExperiencePackValidationGate = experiencePacksApi.checkValidationGate;
export const listExperiencePackRunnerBindings = experiencePacksApi.listRunnerBindings;
export const reviewExperiencePackRunnerBinding = experiencePacksApi.reviewRunnerBinding;
export const prepareRunnerCandidate = experiencePacksApi.prepareRunnerCandidate;
export const confirmRunnerCandidate = experiencePacksApi.confirmRunnerCandidate;
export const approveExperiencePackCandidate = experiencePacksApi.approveCandidate;
export const setExperiencePackEnabled = experiencePacksApi.setPackEnabled;
export const saveExperiencePackAuthorizationScopes = experiencePacksApi.saveAuthorizationScopes;
export const listExperiencePackReuseRecords = experiencePacksApi.listReuseRecords;
