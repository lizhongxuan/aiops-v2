import httpClient from "./httpClient";

export function fetchLabEnvironments() {
  return httpClient.get("/api/v1/lab-environments");
}

export function createLabEnvironment(payload) {
  return httpClient.post("/api/v1/lab-environments", payload);
}

export function startLabEnvironment(id) {
  return httpClient.post(`/api/v1/lab-environments/${encodeURIComponent(id)}/start`);
}

export function stopLabEnvironment(id) {
  return httpClient.post(`/api/v1/lab-environments/${encodeURIComponent(id)}/stop`);
}

export function resetLabEnvironment(id) {
  return httpClient.post(`/api/v1/lab-environments/${encodeURIComponent(id)}/reset`);
}

export function deleteLabEnvironment(id) {
  return httpClient.delete(`/api/v1/lab-environments/${encodeURIComponent(id)}`);
}
