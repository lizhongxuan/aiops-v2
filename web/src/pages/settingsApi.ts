import { buildHostListViewModel } from "@/lib/hostListViewModel";

type JsonMap = Record<string, unknown>;
type RequestOptions = Omit<RequestInit, "body"> & { body?: unknown };

function parseResponseBody(text: string, contentType: string) {
  const trimmed = text.trim();
  if (!trimmed) return {};
  if (contentType.includes("application/json") || trimmed.startsWith("{") || trimmed.startsWith("[")) {
    try {
      return JSON.parse(trimmed);
    } catch {
      return { message: trimmed };
    }
  }
  return { message: trimmed };
}

function fallbackErrorMessage(path: string, status: number, statusText: string) {
  if (path.startsWith("/api/") && status >= 500) {
    return `后端 API 返回空响应（HTTP ${status} ${statusText || ""}）。开发模式下请确认 ai-server 正在监听 127.0.0.1:18080，或按 README 使用 AIOPS_HTTP_ADDR=:18080 ./scripts/start.sh 启动。`;
  }
  return `Request failed with status ${status}`;
}

async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { body, headers, ...rest } = options;
  const hasBody = body !== undefined && body !== null && !(body instanceof FormData);
  const response = await fetch(path, {
    credentials: "include",
    ...rest,
    headers: {
      ...(hasBody ? { "Content-Type": "application/json" } : {}),
      ...(headers || {}),
    },
    body: hasBody && typeof body !== "string" ? JSON.stringify(body) : (body as BodyInit | undefined),
  });
  const text = await response.text();
  const payload = parseResponseBody(text, response.headers.get("Content-Type") || "");
  if (!response.ok) {
    const message = typeof payload?.error === "string" ? payload.error : typeof payload?.message === "string" ? payload.message : "";
    throw new Error(message || fallbackErrorMessage(path, response.status, response.statusText));
  }
  return payload as T;
}

export type LlmConfigView = {
  provider?: string;
  model?: string;
  apiKeySet?: boolean;
  apiKeyMasked?: string;
  baseURL?: string;
  bifrostActive?: boolean;
};

export type LlmConfigUpdate = {
  provider: string;
  model: string;
  apiKey?: string;
  baseURL?: string;
};

export type SessionKind = "single_host" | "workspace";

export type SessionSummary = {
  id: string;
  kind?: SessionKind | string;
  title?: string;
  preview?: string;
  selectedHostId?: string;
  status?: string;
  messageCount?: number;
  lastActivityAt?: string;
};

export type SessionListResponse = {
  activeSessionId?: string;
  sessions?: SessionSummary[];
  items?: SessionSummary[];
  snapshot?: JsonMap;
};

export type HostRecord = {
  id: string;
  name?: string;
  address?: string;
  sshUser?: string;
  sshPort?: number | string;
  transport?: string;
  status?: string;
  installState?: string;
  terminalCapable?: boolean;
  executable?: boolean;
  agentVersion?: string;
  lastHeartbeat?: string;
  labels?: Record<string, string>;
};

export type SkillCatalogItem = {
  id: string;
  name: string;
  description?: string;
  source?: string;
  enabled?: boolean;
  defaultEnabled?: boolean;
  activationMode?: string;
  defaultActivationMode?: string;
};

export type McpCatalogItem = {
  id: string;
  name: string;
  type?: string;
  source?: string;
  enabled?: boolean;
  defaultEnabled?: boolean;
  permission?: string;
  requiresExplicitUserApproval?: boolean;
};

export type AgentProfileRecord = {
  id?: string;
  name?: string;
  description?: string;
  systemPrompt?: string | { content?: string };
  runtime?: JsonMap;
  skills?: SkillCatalogItem[];
  mcps?: McpCatalogItem[];
  [key: string]: unknown;
};

export function fetchStateSnapshot() {
  return request<{ hosts?: HostRecord[]; selectedHost?: HostRecord; sessionId?: string }>("/api/v1/state");
}

export function fetchSessions() {
  return request<SessionListResponse>("/api/v1/sessions");
}

export function createSession(kind: SessionKind = "single_host") {
  return request<SessionListResponse>("/api/v1/sessions", { method: "POST", body: { kind } });
}

export function activateSession(sessionId: string) {
  return request<SessionListResponse>(`/api/v1/sessions/${encodeURIComponent(sessionId)}/activate`, { method: "POST" });
}

export function selectHost(hostId: string) {
  return request<{ snapshot?: JsonMap }>("/api/v1/host/select", { method: "POST", body: { hostId } });
}

export function fetchTerminalSessions() {
  return request<{ items?: JsonMap[]; sessions?: JsonMap[] }>("/api/v1/terminal/sessions");
}

export function fetchHosts() {
  return request<{ items?: HostRecord[] }>("/api/v1/hosts");
}

export function createHost(payload: JsonMap) {
  return request<JsonMap>("/api/v1/hosts", { method: "POST", body: payload });
}

export function updateHost(hostId: string, payload: JsonMap) {
  return request<JsonMap>(`/api/v1/hosts/${encodeURIComponent(hostId)}`, { method: "PUT", body: payload });
}

export function deleteHost(hostId: string) {
  return request<JsonMap>(`/api/v1/hosts/${encodeURIComponent(hostId)}`, { method: "DELETE" });
}

export function fetchLlmConfig() {
  return request<LlmConfigView>("/api/v1/llm-config");
}

export function updateLlmConfig(payload: LlmConfigUpdate) {
  return request<{ ok?: boolean; message?: string; error?: string }>("/api/v1/llm-config", { method: "PUT", body: payload });
}

export function fetchSkillCatalog() {
  return request<{ items?: SkillCatalogItem[] }>("/api/v1/agent-skills");
}

export function saveSkillCatalogItem(item: SkillCatalogItem) {
  return request<{ item?: SkillCatalogItem; items?: SkillCatalogItem[] }>(`/api/v1/agent-skills/${encodeURIComponent(item.id)}`, {
    method: "PUT",
    body: item,
  });
}

export function deleteSkillCatalogItem(skillId: string) {
  return request<{ items?: SkillCatalogItem[] }>(`/api/v1/agent-skills/${encodeURIComponent(skillId)}`, { method: "DELETE" });
}

export function fetchMcpCatalog() {
  return request<{ items?: McpCatalogItem[] }>("/api/v1/agent-mcps");
}

export function saveMcpCatalogItem(item: McpCatalogItem) {
  return request<{ item?: McpCatalogItem; items?: McpCatalogItem[] }>(`/api/v1/agent-mcps/${encodeURIComponent(item.id)}`, {
    method: "PUT",
    body: item,
  });
}

export function deleteMcpCatalogItem(mcpId: string) {
  return request<{ items?: McpCatalogItem[] }>(`/api/v1/agent-mcps/${encodeURIComponent(mcpId)}`, { method: "DELETE" });
}

export function fetchAgentProfiles() {
  return request<{ items?: AgentProfileRecord[]; profiles?: AgentProfileRecord[]; skillCatalog?: SkillCatalogItem[]; mcpCatalog?: McpCatalogItem[] }>("/api/v1/agent-profiles");
}

export function fetchAgentProfile() {
  return request<AgentProfileRecord>("/api/v1/agent-profile");
}

export function saveAgentProfile(profile: AgentProfileRecord) {
  return request<AgentProfileRecord>("/api/v1/agent-profile", { method: "PUT", body: profile });
}

export function resetAgentProfile(profileId: string) {
  return request<AgentProfileRecord>("/api/v1/agent-profile/reset", { method: "POST", body: { profileId } });
}

export function exportAgentProfiles() {
  return request<JsonMap>("/api/v1/agent-profiles/export");
}

export function importAgentProfiles(payload: JsonMap) {
  return request<JsonMap>("/api/v1/agent-profiles/import", { method: "POST", body: payload });
}

export function buildHostsViewModel(input: Parameters<typeof buildHostListViewModel>[0]) {
  return buildHostListViewModel(input);
}
