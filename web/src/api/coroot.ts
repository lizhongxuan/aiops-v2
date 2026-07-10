export type CorootAuthMode = "anonymous_readonly" | "embed_trust" | "session_passthrough";

export type CorootConfigResponse = {
  configured: boolean;
  baseUrl?: string;
  project?: string;
  productBasePath?: string;
  gatewayBasePath?: string;
  entryPath?: string;
  iframeEntryPath?: string;
  authMode?: CorootAuthMode;
  embedMode?: "readonly" | "full";
  uiGatewayEnabled?: boolean;
  tokenConfigured?: boolean;
  username?: string;
  passwordConfigured?: boolean;
  lastSuccessAt?: string;
};

export type CorootConfigInput = {
  baseUrl: string;
  project: string;
  authMode: CorootAuthMode;
  token?: string;
  clearToken?: boolean;
  username?: string;
  password?: string;
  clearPassword?: boolean;
  embedTrustSecret?: string;
  embedMode?: "readonly" | "full";
  uiGatewayEnabled?: boolean;
  timeout?: string;
};

export type CorootConnectionTestResult = {
  ok: boolean;
  message?: string;
  error?: string;
  latencyMs?: number;
  project?: string;
  version?: string;
  details?: unknown;
};

async function parseJsonResponse<T>(response: Response, url: string): Promise<T> {
  const text = await response.text();
  if (!response.ok) {
    const preview = text.trim().slice(0, 240) || response.statusText;
    throw new Error(`${url} failed with ${response.status}: ${preview}`);
  }
  if (!text.trim()) return {} as T;
  return JSON.parse(text) as T;
}

function jsonRequest<T>(url: string, init?: RequestInit) {
  return fetch(url, {
    credentials: "include",
    ...init,
    headers: {
      ...(init?.body ? { "Content-Type": "application/json" } : {}),
      ...init?.headers,
    },
  }).then((response) => parseJsonResponse<T>(response, url));
}

export function fetchCorootConfig(): Promise<CorootConfigResponse> {
  return jsonRequest<CorootConfigResponse>("/api/v1/coroot/config");
}

export function saveCorootConfig(input: CorootConfigInput): Promise<CorootConfigResponse> {
  return jsonRequest<CorootConfigResponse>("/api/v1/coroot/config", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function testCorootConnection(input?: Partial<CorootConfigInput>): Promise<CorootConnectionTestResult> {
  return jsonRequest<CorootConnectionTestResult>("/api/v1/coroot/test-connection", {
    method: "POST",
    body: input ? JSON.stringify(input) : undefined,
  });
}
