import httpClient from "./httpClient";

export function createHost(payload) {
  return httpClient.post("/api/v1/hosts", payload);
}

export function updateHost(hostId, payload) {
  return httpClient.put(`/api/v1/hosts/${encodeURIComponent(hostId)}`, payload);
}

export function deleteHost(hostId) {
  return httpClient.delete(`/api/v1/hosts/${encodeURIComponent(hostId)}`);
}

export function retryHostInstall(hostId, payload) {
  return httpClient.post(`/api/v1/hosts/${encodeURIComponent(hostId)}/install`, payload);
}

export function testHostSSH(hostId, payload) {
  return httpClient.post(`/api/v1/hosts/${encodeURIComponent(hostId)}/ssh/test`, payload);
}
