import httpClient from "./httpClient";
import { queryString } from "./query";

export function lookupOpsGraph(payload = {}) {
  return httpClient.post("/api/v1/opsgraph/lookup", payload);
}

export function getOpsGraphNeighborhood(entityId, params = {}) {
  return httpClient.get(`/api/v1/opsgraph/entities/${encodeURIComponent(entityId)}/neighborhood${queryString(params)}`);
}

export function getOpsGraphBusinessImpact(entityId) {
  return httpClient.get(`/api/v1/opsgraph/entities/${encodeURIComponent(entityId)}/business-impact`);
}
