import httpClient from "./httpClient";

export function fetchServers() {
  return httpClient.get("/api/v1/mcp/servers");
}

export function createServer(payload) {
  return httpClient.post("/api/v1/mcp/servers", payload);
}

export function updateServer(name, payload) {
  return httpClient.put(`/api/v1/mcp/servers/${encodeURIComponent(name)}`, payload);
}

export function deleteServer(name) {
  return httpClient.delete(`/api/v1/mcp/servers/${encodeURIComponent(name)}`);
}

export function runServerAction(name, action) {
  return httpClient.post(`/api/v1/mcp/servers/${encodeURIComponent(name)}/${encodeURIComponent(action)}`);
}

export function refreshServers() {
  return httpClient.post("/api/v1/mcp/servers/refresh");
}
