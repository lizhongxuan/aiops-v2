import httpClient from "./httpClient";

export function fetchCorootConfig() {
  return httpClient.get("/api/v1/coroot/config");
}

export function fetchCorootServices() {
  return httpClient.get("/api/v1/coroot/api/v1/services");
}

export function fetchCorootJson(url) {
  return httpClient.get(url);
}
