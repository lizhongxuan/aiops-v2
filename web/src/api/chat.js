import httpClient from "./httpClient";

export function sendMessage(payload) {
  return httpClient.post("/api/v1/chat/message", payload);
}

export function stopMessage(payload = {}) {
  return httpClient.post("/api/v1/chat/stop", payload);
}
