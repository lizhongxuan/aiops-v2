import httpClient from "./httpClient";

export function createTerminalSession(payload) {
  return httpClient.post("/api/v1/terminal/sessions", payload);
}
