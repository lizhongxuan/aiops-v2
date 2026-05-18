import httpClient from "./httpClient";

type HttpClientLike = {
  get(path: string): Promise<unknown>;
  post(path: string, body?: unknown): Promise<unknown>;
};

function compactFilters(filters: Record<string, string | undefined> = {}) {
  return Object.entries(filters).filter(([, value]) => typeof value === "string" && value.trim() !== "") as Array<[string, string]>;
}

export function createAgentUiArtifactsClient(client: HttpClientLike = httpClient) {
  return {
    fetchAgentUiArtifacts(filters: Record<string, string | undefined> = {}) {
      const params = new URLSearchParams(compactFilters(filters));
      const suffix = params.toString() ? `?${params.toString()}` : "";
      return client.get(`/api/v1/agent-ui-artifacts${suffix}`);
    },

    fetchAgentUiArtifact(id: string) {
      return client.get(`/api/v1/agent-ui-artifacts/${encodeURIComponent(id)}`);
    },

    validateAgentUiArtifact(payload: Record<string, unknown>) {
      return client.post("/api/v1/agent-ui-artifacts/validate", payload);
    },
  };
}

const agentUiArtifactsClient = createAgentUiArtifactsClient();

export const fetchAgentUiArtifacts = (...args: Parameters<typeof agentUiArtifactsClient.fetchAgentUiArtifacts>) =>
  agentUiArtifactsClient.fetchAgentUiArtifacts(...args);
export const fetchAgentUiArtifact = (...args: Parameters<typeof agentUiArtifactsClient.fetchAgentUiArtifact>) =>
  agentUiArtifactsClient.fetchAgentUiArtifact(...args);
export const validateAgentUiArtifact = (...args: Parameters<typeof agentUiArtifactsClient.validateAgentUiArtifact>) =>
  agentUiArtifactsClient.validateAgentUiArtifact(...args);
