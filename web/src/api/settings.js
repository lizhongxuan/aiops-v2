import httpClient from "./httpClient";

export function fetchSettings() {
  return httpClient.get("/api/v1/settings");
}

export function updateSettings(payload) {
  return httpClient.post("/api/v1/settings", payload);
}
