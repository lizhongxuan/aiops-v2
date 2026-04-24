import httpClient from "./httpClient";

export function fetchLlmConfig() {
  return httpClient.get("/api/v1/llm-config");
}

export function updateLlmConfig(payload) {
  return httpClient.put("/api/v1/llm-config", payload);
}
