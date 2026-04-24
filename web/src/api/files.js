import httpClient from "./httpClient";

export function fetchFilePreview({ hostId = "server-local", path }) {
  return httpClient.get(`/api/v1/files/preview?hostId=${encodeURIComponent(hostId)}&path=${encodeURIComponent(path)}`);
}

export function fetchEvidenceDetail(sessionId, evidenceId) {
  return httpClient.get(`/api/sessions/${encodeURIComponent(sessionId)}/evidence/${encodeURIComponent(evidenceId)}`);
}

export function fetchInvocationDetail(sessionId, invocationId) {
  return httpClient.get(`/api/sessions/${encodeURIComponent(sessionId)}/invocations/${encodeURIComponent(invocationId)}`);
}
