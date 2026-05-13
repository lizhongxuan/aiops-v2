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
  status: string;
  reviewStatus: string;
  enabled: boolean;
  searchable: boolean;
  searchableReason: string;
  retrievalEval: RetrievalEvalView;
  workflowBinding: WorkflowBindingView;
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
  const searchability = searchableState(reviewStatus, enabled, authorizationScopes);
  return {
    id,
    title: text(pick(source, "title", "name"), id),
    summary: text(pick(source, "summary", "description")),
    version: text(pick(source, "version")),
    status: text(pick(source, "status", "state"), enabled ? "enabled" : "disabled"),
    reviewStatus,
    enabled,
    ...searchability,
    retrievalEval: normalizeRetrievalEval(pick(source, "retrievalEval", "retrieval_eval")),
    workflowBinding: normalizeWorkflowBinding(pick(source, "workflowBinding", "workflow_binding")),
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
  };
}

const experiencePacksApi = createExperiencePacksApi();

export const listExperiencePackCandidates = experiencePacksApi.listCandidates;
export const approveExperiencePackCandidate = experiencePacksApi.approveCandidate;
export const setExperiencePackEnabled = experiencePacksApi.setPackEnabled;
export const saveExperiencePackAuthorizationScopes = experiencePacksApi.saveAuthorizationScopes;
export const listExperiencePackReuseRecords = experiencePacksApi.listReuseRecords;
