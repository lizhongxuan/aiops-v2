import httpClient from "./httpClient";

export function fetchTerminalSessions() {
  return httpClient.get("/api/v1/terminal/sessions");
}

export function createTerminalSession(payload) {
  return httpClient.post("/api/v1/terminal/sessions", payload);
}
