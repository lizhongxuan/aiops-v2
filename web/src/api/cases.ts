import httpClient from "./httpClient";

type LooseRecord = Record<string, unknown>;

export type EvidenceView = {
  id: string;
  evidenceRef: string;
  type: string;
  title: string;
  summary: string;
  source: string;
  artifactId: string;
  traceId: string;
  createdAt: string;
  raw: LooseRecord;
};

export type HostProfileSnapshotView = {
  hostId: string;
  displayName: string;
  status: string;
  os: string;
  arch: string;
  labels: Record<string, string>;
  agentVersion: string;
  reportedAt: string;
  raw: LooseRecord;
};

export type CaseHostLeaseView = {
  leaseId: string;
  hostId: string;
  status: string;
  ownerSessionId: string;
  caseId: string;
  reason: string;
  acquiredAt: string;
  expiresAt: string;
  raw: LooseRecord;
};

export type WorkflowRunView = {
  runId: string;
  workflowId: string;
  title: string;
  status: string;
  hostIds: string[];
  startedAt: string;
  finishedAt: string;
  failedStep: string;
  rollbackResult: string;
  verificationRefs: string[];
  raw: LooseRecord;
};

export type VerificationView = {
  id: string;
  title: string;
  status: string;
  summary: string;
  before: unknown;
  after: unknown;
  createdAt: string;
  raw: LooseRecord;
};

export type ExperienceCandidateView = {
  id: string;
  packId: string;
  title: string;
  status: string;
  summary: string;
  matchReason: string;
  workflowId: string;
  risk: string;
  raw: LooseRecord;
};

export type TimelineEventView = {
  id: string;
  type: string;
  title: string;
  summary: string;
  status: string;
  actor: string;
  createdAt: string;
  raw: LooseRecord;
};

export type CaseActionView = {
  actionId: string;
  title: string;
  status: string;
  kind: string;
  raw: LooseRecord;
};

export type CaseView = {
  id: string;
  title: string;
  summary: string;
  status: string;
  severity: string;
  source: string;
  environment: string;
  service: string;
  businessCapability: string;
  createdAt: string;
  updatedAt: string;
  evidence: EvidenceView[];
  hostProfiles: HostProfileSnapshotView[];
  hostLeases: CaseHostLeaseView[];
  workflowRuns: WorkflowRunView[];
  verifications: VerificationView[];
  experienceCandidates: ExperienceCandidateView[];
  timeline: TimelineEventView[];
  pendingActions: CaseActionView[];
  raw: LooseRecord;
};

export type CaseListView = {
  items: CaseView[];
  nextCursor: string;
  total: number | null;
  raw: unknown;
};

export type ListCasesParams = {
  status?: string | null;
  source?: string | null;
  environment?: string | null;
  hostId?: string | null;
  waitingConfirmation?: boolean | string | null;
  lockConflict?: boolean | string | null;
  limit?: number | string | null;
  cursor?: string | null;
};

export type CaseDecisionPayload = {
  decision: "approved" | "rejected" | "cancelled" | string;
  comment?: string;
  reason?: string;
};

type CasesHttpClient = {
  get(path: string): Promise<unknown>;
  post(path: string, body?: unknown): Promise<unknown>;
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

function labels(value: unknown): Record<string, string> {
  if (!isRecord(value)) return {};
  return Object.fromEntries(
    Object.entries(value)
      .map(([key, labelValue]) => [key, text(labelValue)] as const)
      .filter(([, labelValue]) => labelValue),
  );
}

function stringArray(value: unknown) {
  return asArray(value).map((item) => text(item)).filter(Boolean);
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

function unwrapCase(input: unknown): LooseRecord {
  if (!isRecord(input)) return {};
  const wrapped = pick(input, "case", "item", "incident");
  return isRecord(wrapped) ? wrapped : input;
}

export function normalizeEvidence(input: unknown): EvidenceView {
  const source = isRecord(input) ? input : {};
  const evidenceRef = text(pick(source, "evidenceRef", "evidence_ref", "ref", "id"), "unknown-evidence");
  const artifactId = text(pick(source, "artifactId", "artifact_id", "agent_ui_artifact_id"));
  return {
    id: text(pick(source, "id", "evidenceId", "evidence_id"), evidenceRef),
    evidenceRef,
    type: text(pick(source, "type", "kind"), "evidence"),
    title: text(pick(source, "title", "name"), evidenceRef),
    summary: text(pick(source, "summary", "description", "detail", "message")),
    source: text(pick(source, "source", "provider"), "unknown"),
    artifactId,
    traceId: text(pick(source, "traceId", "trace_id")),
    createdAt: text(pick(source, "createdAt", "created_at", "timestamp")),
    raw: source,
  };
}

export function normalizeHostProfileSnapshot(input: unknown): HostProfileSnapshotView {
  const source = isRecord(input) ? input : {};
  const hostId = text(pick(source, "hostId", "host_id", "id"), "unknown-host");
  return {
    hostId,
    displayName: text(pick(source, "displayName", "display_name", "hostname", "name"), hostId),
    status: text(pick(source, "status", "state"), "unknown"),
    os: text(pick(source, "os", "osName", "os_name", "platform")),
    arch: text(pick(source, "arch", "architecture")),
    labels: labels(pick(source, "labels", "tags")),
    agentVersion: text(pick(source, "agentVersion", "agent_version", "clientVersion", "client_version")),
    reportedAt: text(pick(source, "reportedAt", "reported_at", "lastHeartbeatAt", "last_heartbeat_at")),
    raw: source,
  };
}

export function normalizeCaseHostLease(input: unknown): CaseHostLeaseView {
  const source = isRecord(input) ? input : {};
  return {
    leaseId: text(pick(source, "leaseId", "lease_id", "id"), "unknown-lease"),
    hostId: text(pick(source, "hostId", "host_id"), "unknown-host"),
    status: text(pick(source, "status", "state"), "unknown"),
    ownerSessionId: text(pick(source, "ownerSessionId", "owner_session_id", "sessionId", "session_id")),
    caseId: text(pick(source, "caseId", "case_id", "missionId", "mission_id")),
    reason: text(pick(source, "reason", "summary", "description")),
    acquiredAt: text(pick(source, "acquiredAt", "acquired_at", "createdAt", "created_at")),
    expiresAt: text(pick(source, "expiresAt", "expires_at")),
    raw: source,
  };
}

export function normalizeWorkflowRun(input: unknown): WorkflowRunView {
  const source = isRecord(input) ? input : {};
  const runId = text(pick(source, "runId", "run_id", "id", "instanceId", "instance_id"), "unknown-run");
  return {
    runId,
    workflowId: text(pick(source, "workflowId", "workflow_id", "workflowName", "workflow_name", "name"), runId),
    title: text(pick(source, "title", "name"), runId),
    status: text(pick(source, "status", "state"), "unknown"),
    hostIds: stringArray(pick(source, "hostIds", "host_ids", "hosts")),
    startedAt: text(pick(source, "startedAt", "started_at", "createdAt", "created_at")),
    finishedAt: text(pick(source, "finishedAt", "finished_at", "completedAt", "completed_at")),
    failedStep: text(pick(source, "failedStep", "failed_step")),
    rollbackResult: text(pick(source, "rollbackResult", "rollback_result")),
    verificationRefs: stringArray(pick(source, "verificationRefs", "verification_refs")),
    raw: source,
  };
}

export function normalizeVerification(input: unknown): VerificationView {
  const source = isRecord(input) ? input : {};
  const id = text(pick(source, "id", "verificationId", "verification_id"), "unknown-verification");
  return {
    id,
    title: text(pick(source, "title", "name"), id),
    status: text(pick(source, "status", "state", "verdict"), "unknown"),
    summary: text(pick(source, "summary", "description", "detail")),
    before: pick(source, "before", "beforeMetrics", "before_metrics"),
    after: pick(source, "after", "afterMetrics", "after_metrics"),
    createdAt: text(pick(source, "createdAt", "created_at", "timestamp")),
    raw: source,
  };
}

export function normalizeExperienceCandidate(input: unknown): ExperienceCandidateView {
  const source = isRecord(input) ? input : {};
  const packId = text(pick(source, "packId", "pack_id", "id"), "unknown-pack");
  return {
    id: text(pick(source, "id", "candidateId", "candidate_id"), packId),
    packId,
    title: text(pick(source, "title", "name"), packId),
    status: text(pick(source, "status", "state"), "candidate"),
    summary: text(pick(source, "summary", "description")),
    matchReason: text(pick(source, "matchReason", "match_reason", "reason")),
    workflowId: text(pick(source, "workflowId", "workflow_id")),
    risk: text(pick(source, "risk", "severity"), "unknown"),
    raw: source,
  };
}

export function normalizeTimelineEvent(input: unknown): TimelineEventView {
  const source = isRecord(input) ? input : {};
  const id = text(pick(source, "id", "eventId", "event_id"), "unknown-event");
  return {
    id,
    type: text(pick(source, "type", "kind"), "event"),
    title: text(pick(source, "title", "name"), id),
    summary: text(pick(source, "summary", "description", "detail", "message")),
    status: text(pick(source, "status", "state")),
    actor: text(pick(source, "actor", "createdBy", "created_by")),
    createdAt: text(pick(source, "createdAt", "created_at", "timestamp")),
    raw: source,
  };
}

export function normalizeCaseAction(input: unknown): CaseActionView {
  const source = isRecord(input) ? input : {};
  const actionId = text(pick(source, "actionId", "action_id", "id", "approvalId", "approval_id"), "unknown-action");
  return {
    actionId,
    title: text(pick(source, "title", "name", "command", "toolName", "tool_name", "reason"), actionId),
    status: text(pick(source, "status", "state", "decision"), "pending"),
    kind: text(pick(source, "kind", "type"), "approval"),
    raw: source,
  };
}

export function normalizeCase(input: unknown): CaseView {
  const source = unwrapCase(input);
  const id = text(pick(source, "caseId", "case_id", "incidentId", "incident_id", "id"), "unknown-case");
  return {
    id,
    title: text(pick(source, "title", "name"), id),
    summary: text(pick(source, "summary", "description")),
    status: text(pick(source, "status", "state"), "unknown"),
    severity: text(pick(source, "severity", "sev", "risk"), "unknown"),
    source: text(pick(source, "source", "origin"), "unknown"),
    environment: text(pick(source, "environment", "env")),
    service: text(pick(source, "service", "serviceName", "service_name", "entityId", "entity_id")),
    businessCapability: text(pick(source, "businessCapability", "business_capability", "capability")),
    createdAt: text(pick(source, "createdAt", "created_at")),
    updatedAt: text(pick(source, "updatedAt", "updated_at")),
    evidence: asArray(pick(source, "evidence", "evidences")).map(normalizeEvidence),
    hostProfiles: asArray(pick(source, "hostProfiles", "host_profiles", "hostProfileSnapshots", "host_profile_snapshots")).map(
      normalizeHostProfileSnapshot,
    ),
    hostLeases: asArray(pick(source, "hostLeases", "host_leases", "leases")).map(normalizeCaseHostLease),
    workflowRuns: asArray(pick(source, "workflowRuns", "workflow_runs", "runnerRuns", "runner_runs", "runbookInstances", "instances")).map(
      normalizeWorkflowRun,
    ),
    verifications: asArray(pick(source, "verifications", "verification")).map(normalizeVerification),
    experienceCandidates: asArray(pick(source, "experienceCandidates", "experience_candidates", "experience")).map(
      normalizeExperienceCandidate,
    ),
    timeline: asArray(pick(source, "timeline", "events")).map(normalizeTimelineEvent),
    pendingActions: asArray(pick(source, "pendingActions", "pending_actions", "pendingApprovals", "pending_approvals")).map(
      normalizeCaseAction,
    ),
    raw: source,
  };
}

export function normalizeCaseList(input: unknown): CaseListView {
  const source = isRecord(input) ? input : {};
  const items = asArray(pick(source, "items", "cases", "incidents")).map(normalizeCase);
  const total = pick(source, "total", "totalCount", "total_count");
  const numericTotal = Number(total);
  return {
    items,
    nextCursor: text(pick(source, "nextCursor", "next_cursor", "cursor")),
    total: Number.isFinite(numericTotal) ? numericTotal : null,
    raw: input,
  };
}

export function createCasesApi(client: CasesHttpClient = httpClient) {
  return {
    async listCases(params: ListCasesParams = {}) {
      const payload = await client.get(
        endpoint("/api/v1/cases", {
          status: params.status,
          source: params.source,
          environment: params.environment,
          host_id: params.hostId,
          waiting_confirmation: params.waitingConfirmation,
          lock_conflict: params.lockConflict,
          limit: params.limit,
          cursor: params.cursor,
        }),
      );
      return normalizeCaseList(payload);
    },

    async getCase(caseId: string) {
      const payload = await client.get(`/api/v1/cases/${encodePath(caseId)}`);
      return normalizeCase(payload);
    },

    confirmCaseAction(caseId: string, actionId: string, decision: CaseDecisionPayload) {
      return client.post(`/api/v1/cases/${encodePath(caseId)}/actions/${encodePath(actionId)}/decision`, decision);
    },

    closeCase(caseId: string, payload: Record<string, unknown> = {}) {
      return client.post(`/api/v1/cases/${encodePath(caseId)}/close`, payload);
    },
  };
}

const casesApi = createCasesApi();

export const listCases = casesApi.listCases;
export const getCase = casesApi.getCase;
export const confirmCaseAction = casesApi.confirmCaseAction;
export const closeCase = casesApi.closeCase;
