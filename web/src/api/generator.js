import httpClient from "./httpClient";

export function generateDraft(payload) {
  return httpClient.post("/api/v1/generator/generate", payload);
}

export function lintDraft(payload) {
  return httpClient.post("/api/v1/generator/lint", payload);
}

export function previewDraft(payload) {
  return httpClient.post("/api/v1/generator/preview", payload);
}

export function publishDraft(payload) {
  return httpClient.post("/api/v1/generator/publish-draft", payload);
}
