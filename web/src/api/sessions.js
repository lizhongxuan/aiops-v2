import httpClient from "./httpClient";

export function fetchSessions() {
  return httpClient.get("/api/v1/sessions");
}

export function createSession(kind = "single_host") {
  return httpClient.post("/api/v1/sessions", { kind });
}

export function activateSession(sessionId) {
  return httpClient.post(`/api/v1/sessions/${encodeURIComponent(sessionId)}/activate`);
}
