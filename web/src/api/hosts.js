import httpClient from "./httpClient";

export function fetchHostSessions(hostId, { limit = 8 } = {}) {
  return httpClient.get(`/api/v1/hosts/${encodeURIComponent(hostId)}/sessions?limit=${encodeURIComponent(limit)}`);
}

export function createHost(payload) {
  return httpClient.post("/api/v1/hosts", payload);
}

export function updateHost(hostId, payload) {
  return httpClient.put(`/api/v1/hosts/${encodeURIComponent(hostId)}`, payload);
}

export function deleteHost(hostId) {
  return httpClient.delete(`/api/v1/hosts/${encodeURIComponent(hostId)}`);
}

export function updateHostTags(payload) {
  return httpClient.post("/api/v1/hosts/tags", payload);
}
