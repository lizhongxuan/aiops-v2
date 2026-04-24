import httpClient from "./httpClient";

export function fetchUiCards() {
  return httpClient.get("/api/v1/ui-cards");
}

export function updateUiCard(id, payload) {
  return httpClient.put(`/api/v1/ui-cards/${encodeURIComponent(id)}`, payload);
}

export function previewUiCard(id, payload) {
  return httpClient.post(`/api/v1/ui-cards/${encodeURIComponent(id)}/preview`, payload);
}
