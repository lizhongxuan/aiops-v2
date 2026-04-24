import httpClient from "./httpClient";

export function fetchState() {
  return httpClient.get("/api/v1/state");
}

export function selectHost(hostId) {
  return httpClient.post("/api/v1/host/select", { hostId });
}

export function resetThread() {
  return httpClient.post("/api/v1/thread/reset");
}
