import httpClient from "./httpClient";

export function fetchScriptConfigs() {
  return httpClient.get("/api/v1/script-configs");
}

export function createScriptConfig(payload) {
  return httpClient.post("/api/v1/script-configs", payload);
}

export function updateScriptConfig(id, payload) {
  return httpClient.put(`/api/v1/script-configs/${encodeURIComponent(id)}`, payload);
}

export function deleteScriptConfig(id) {
  return httpClient.delete(`/api/v1/script-configs/${encodeURIComponent(id)}`);
}

export function dryRunScriptConfig(id, payload) {
  return httpClient.post(`/api/v1/script-configs/${encodeURIComponent(id)}/dry-run`, payload);
}
