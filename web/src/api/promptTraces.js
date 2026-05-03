import httpClient from "./httpClient";

export function fetchPromptTraces({ limit = 100, q = "", caseId = "", trace = "" } = {}) {
  const params = new URLSearchParams();
  if (limit) params.set("limit", String(limit));
  if (q) params.set("q", q);
  if (caseId) params.set("caseId", caseId);
  if (trace) params.set("trace", trace);
  const query = params.toString();
  return httpClient.get(`/api/v1/debug/model-input-traces${query ? `?${query}` : ""}`);
}

export function fetchPromptTraceFile(path) {
  return httpClient.get(`/api/v1/debug/model-input-traces/file?path=${encodeURIComponent(path || "")}`);
}
