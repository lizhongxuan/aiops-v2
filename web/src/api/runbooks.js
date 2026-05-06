import httpClient from "./httpClient";
import { queryString } from "./query";

export function listRunbooks(params = {}) {
  return httpClient.get(`/api/v1/runbooks${queryString(params)}`);
}

export function getRunbook(runbookId) {
  return httpClient.get(`/api/v1/runbooks/${encodeURIComponent(runbookId)}`);
}

export function matchRunbooks(payload = {}) {
  return httpClient.post("/api/v1/runbooks/match", payload);
}

export function listRunbookInstances(params = {}) {
  return httpClient.get(`/api/v1/runbooks/instances${queryString(params)}`);
}
