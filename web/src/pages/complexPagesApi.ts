import type { AiopsTransportState } from "@/transport/aiopsTransportTypes";

type JsonMap = Record<string, unknown>;

type RequestOptions = Omit<RequestInit, "body"> & { body?: unknown };

async function request<T>(
  path: string,
  options: RequestOptions = {},
): Promise<T> {
  const { body, headers, ...rest } = options;
  const hasBody =
    body !== undefined && body !== null && !(body instanceof FormData);
  const response = await fetch(path, {
    credentials: "include",
    ...rest,
    headers: {
      ...(hasBody ? { "Content-Type": "application/json" } : {}),
      ...(headers || {}),
    },
    body:
      hasBody && typeof body !== "string"
        ? JSON.stringify(body)
        : (body as BodyInit | undefined),
  });
  const text = await response.text();
  const payload = text ? JSON.parse(text) : {};
  if (!response.ok) {
    throw new Error(
      payload?.error ||
        payload?.message ||
        `Request failed with status ${response.status}`,
    );
  }
  return payload as T;
}

export type McpServerRecord = {
  name: string;
  transport?: string;
  command?: string;
  args?: string[];
  url?: string;
  env?: Record<string, string>;
  disabled?: boolean;
  status?: string;
  error?: string;
  toolCount?: number;
  resourceCount?: number;
};

export type McpHealthRecord = {
  serverId: string;
  displayName?: string;
  status: string;
  lastCheckedAt?: string;
  lastError?: string;
  availableToolCount?: number;
  disabledReason?: string;
  retryAfterSeconds?: number;
};

export type ApprovalAuditRecord = {
  id: string;
  createdAt?: string;
  sessionKind?: string;
  host?: string;
  operator?: string;
  toolName?: string;
  decision?: string;
  command?: string;
  reason?: string;
};

export type ApprovalGrantRecord = {
  id: string;
  hostId?: string;
  command?: string;
  toolName?: string;
  grantedAt?: string;
  status?: string;
};

export type IncidentRecord = {
  id: string;
  title?: string;
  name?: string;
  status?: string;
  source?: string;
  severity?: string;
  sev?: string;
  environment?: string;
  env?: string;
  businessCapability?: string;
  capability?: string;
  summary?: string;
  updatedAt?: string;
  createdAt?: string;
  entityId?: string;
  affectedServices?: string[];
  evidenceRefs?: string[];
  evidence?: JsonMap[];
  hypotheses?: JsonMap[];
  postmortem?: JsonMap;
  pendingApprovals?: ApprovalAuditRecord[];
};

export type RunbookRecord = {
  id: string;
  title?: string;
  name?: string;
  scope?: string;
  environment?: string;
  risk?: string;
  capability?: string;
  capabilities?: string[];
  updatedAt?: string;
  updated_at?: string;
  steps?: JsonMap[];
  verifications?: JsonMap[];
  proposals?: JsonMap[];
};

export function fetchMcpServers() {
  return request<{ items?: McpServerRecord[]; configPath?: string }>(
    "/api/v1/mcp/servers",
  );
}

export function saveMcpServer(name: string, payload: McpServerRecord) {
  return name
    ? request<{ items?: McpServerRecord[] }>(
        `/api/v1/mcp/servers/${encodeURIComponent(name)}`,
        { method: "PUT", body: payload },
      )
    : request<{ items?: McpServerRecord[] }>("/api/v1/mcp/servers", {
        method: "POST",
        body: payload,
      });
}

export function deleteMcpServer(name: string) {
  return request<{ items?: McpServerRecord[] }>(
    `/api/v1/mcp/servers/${encodeURIComponent(name)}`,
    { method: "DELETE" },
  );
}

export function runMcpServerAction(name: string, action: string) {
  return request<{ items?: McpServerRecord[] }>(
    `/api/v1/mcp/servers/${encodeURIComponent(name)}/${encodeURIComponent(action)}`,
    { method: "POST" },
  );
}

export function refreshMcpServers() {
  return request<{ items?: McpServerRecord[] }>("/api/v1/mcp/servers/refresh", {
    method: "POST",
  });
}

export function fetchMcpRuntimeHealth() {
  return request<{ items?: McpHealthRecord[] }>("/api/v2/runtime/mcp-health");
}

export function refreshMcpRuntimeHealth(serverId: string) {
  return request<McpHealthRecord>(
    `/api/v2/runtime/mcp-health/${encodeURIComponent(serverId)}/refresh`,
    { method: "POST" },
  );
}

export function fetchApprovalAudits(params: Record<string, string> = {}) {
  const query = new URLSearchParams(params);
  const suffix = query.toString() ? `?${query.toString()}` : "";
  return request<{
    items?: ApprovalAuditRecord[];
    audits?: ApprovalAuditRecord[];
    total?: number;
    stats?: JsonMap;
  }>(`/api/v1/approval-audits${suffix}`);
}

export function fetchApprovalGrants(hostId = "") {
  const suffix = hostId ? `?hostId=${encodeURIComponent(hostId)}` : "";
  return request<{
    items?: ApprovalGrantRecord[];
    grants?: ApprovalGrantRecord[];
  }>(`/api/v1/approval-grants${suffix}`);
}

export function updateApprovalGrant(
  grantId: string,
  action: "revoke" | "disable" | "enable",
) {
  return request<JsonMap>(
    `/api/v1/approval-grants/${encodeURIComponent(grantId)}/${action}`,
    { method: "POST" },
  );
}

export function submitApprovalDecision(approvalId: string, decision: string) {
  return request<JsonMap>(
    `/api/v1/approvals/${encodeURIComponent(approvalId)}/decision`,
    { method: "POST", body: { decision } },
  );
}

export function sendTransportCommand(
  state: AiopsTransportState,
  command: JsonMap,
) {
  return request<JsonMap>("/api/v1/assistant/transport", {
    method: "POST",
    headers: { Accept: "text/plain" },
    body: { state, commands: [command], threadId: state.threadId },
  });
}

export function listIncidents(params: Record<string, string> = {}) {
  const query = new URLSearchParams(params);
  const suffix = query.toString() ? `?${query.toString()}` : "";
  return request<{ items?: IncidentRecord[]; incidents?: IncidentRecord[] }>(
    `/api/v1/incidents${suffix}`,
  );
}

export function getIncident(incidentId: string) {
  return request<IncidentRecord>(
    `/api/v1/incidents/${encodeURIComponent(incidentId)}`,
  );
}

export function startIncidentChat(incidentId: string, sessionId = "") {
  return request<JsonMap>(
    `/api/v1/incidents/${encodeURIComponent(incidentId)}/start-chat`,
    {
      method: "POST",
      body: sessionId ? { sessionId } : {},
    },
  );
}

export function getOpsGraphNeighborhood(
  entityId: string,
  params: Record<string, string> = {},
) {
  const query = new URLSearchParams(params);
  const suffix = query.toString() ? `?${query.toString()}` : "";
  return request<{ neighbors?: JsonMap[]; items?: JsonMap[] }>(
    `/api/v1/opsgraph/entities/${encodeURIComponent(entityId)}/neighborhood${suffix}`,
  );
}

export function getOpsGraphBusinessImpact(entityId: string) {
  return request<{ capabilities?: JsonMap[]; tenants?: JsonMap[] }>(
    `/api/v1/opsgraph/entities/${encodeURIComponent(entityId)}/business-impact`,
  );
}

export function listRunbooks(params: Record<string, string> = {}) {
  const query = new URLSearchParams(params);
  const suffix = query.toString() ? `?${query.toString()}` : "";
  return request<{ items?: RunbookRecord[]; runbooks?: RunbookRecord[] }>(
    `/api/v1/runbooks${suffix}`,
  );
}

export function getRunbook(runbookId: string) {
  return request<RunbookRecord>(
    `/api/v1/runbooks/${encodeURIComponent(runbookId)}`,
  );
}

export function matchRunbooks(payload: JsonMap) {
  return request<{ items?: JsonMap[]; matches?: JsonMap[] }>(
    "/api/v1/runbooks/match",
    { method: "POST", body: payload },
  );
}

export function listRunbookInstances(params: Record<string, string> = {}) {
  const query = new URLSearchParams(params);
  const suffix = query.toString() ? `?${query.toString()}` : "";
  return request<{ items?: JsonMap[]; instances?: JsonMap[] }>(
    `/api/v1/runbooks/instances${suffix}`,
  );
}

export function compactText(value: unknown) {
  return typeof value === "string" ? value.trim() : String(value || "").trim();
}

export function asArray<T = JsonMap>(value: unknown): T[] {
  return Array.isArray(value) ? (value as T[]) : [];
}
