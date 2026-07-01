import httpClient, { HttpClientError } from "./httpClient";
import { queryString } from "./query";

export function listOpsGraphs() {
  return httpClient.get("/api/v1/opsgraph/graphs");
}

export function createOpsGraph(payload = {}) {
  return httpClient.post("/api/v1/opsgraph/graphs", payload);
}

export function getOpsGraph(graphId) {
  return httpClient.get(`/api/v1/opsgraph/graphs/${encodeURIComponent(graphId)}`);
}

export function updateOpsGraph(graphId, payload = {}) {
  return httpClient.put(`/api/v1/opsgraph/graphs/${encodeURIComponent(graphId)}`, payload);
}

export function exportOpsGraphYaml(graphId) {
  return httpClient.get(`/api/v1/opsgraph/graphs/${encodeURIComponent(graphId)}/yaml`, {
    headers: { Accept: "text/yaml" },
  });
}

export async function importOpsGraphYaml(graphId, yamlText) {
  const url = `/api/v1/opsgraph/graphs/${encodeURIComponent(graphId)}/yaml`;
  const response = await fetch(url, {
    method: "PUT",
    credentials: "include",
    headers: {
      Accept: "application/json",
      "Content-Type": "text/yaml",
    },
    body: yamlText,
  });
  const rawText = typeof response.text === "function" ? await response.text() : "";
  let payload = {};
  if (rawText) {
    try {
      payload = JSON.parse(rawText);
    } catch (error) {
      if (response.ok) {
        throw new HttpClientError("Invalid JSON response", {
          status: response.status,
          url,
          code: "invalid_json",
          cause: error,
        });
      }
      payload = { error: rawText };
    }
  }
  if (!response.ok) {
    throw new HttpClientError(payload?.error || payload?.message || `Request failed with status ${response.status}`, {
      status: response.status,
      url,
      payload,
    });
  }
  return payload;
}

export function duplicateOpsGraph(graphId) {
  return httpClient.post(`/api/v1/opsgraph/graphs/${encodeURIComponent(graphId)}/duplicate`, {});
}

export function deleteOpsGraph(graphId) {
  return httpClient.delete(`/api/v1/opsgraph/graphs/${encodeURIComponent(graphId)}`);
}

export function createOpsGraphNode(graphId, payload = {}) {
  return httpClient.post(`/api/v1/opsgraph/graphs/${encodeURIComponent(graphId)}/entities`, payload);
}

export function updateOpsGraphNode(graphId, nodeId, payload = {}) {
  return httpClient.put(`/api/v1/opsgraph/graphs/${encodeURIComponent(graphId)}/entities/${encodeURIComponent(nodeId)}`, payload);
}

export function deleteOpsGraphNode(graphId, nodeId, params = {}) {
  return httpClient.delete(`/api/v1/opsgraph/graphs/${encodeURIComponent(graphId)}/entities/${encodeURIComponent(nodeId)}${queryString(params)}`);
}

export function createOpsGraphRelationship(graphId, payload = {}) {
  return httpClient.post(`/api/v1/opsgraph/graphs/${encodeURIComponent(graphId)}/relationships`, payload);
}

export function updateOpsGraphRelationship(graphId, relationshipId, payload = {}) {
  return httpClient.put(`/api/v1/opsgraph/graphs/${encodeURIComponent(graphId)}/relationships/${encodeURIComponent(relationshipId)}`, payload);
}

export function deleteOpsGraphRelationship(graphId, relationshipId) {
  return httpClient.delete(`/api/v1/opsgraph/graphs/${encodeURIComponent(graphId)}/relationships/${encodeURIComponent(relationshipId)}`);
}

export function saveOpsGraphLayout(graphId, payload = {}) {
  return httpClient.post(`/api/v1/opsgraph/graphs/${encodeURIComponent(graphId)}/layout`, payload);
}

export function validateOpsGraph(graphId) {
  return httpClient.get(`/api/v1/opsgraph/graphs/${encodeURIComponent(graphId)}/validate`);
}

export function lookupOpsGraph(payload = {}) {
  const { graphId, ...body } = payload;
  const path = graphId ? `/api/v1/opsgraph/graphs/${encodeURIComponent(graphId)}/lookup` : "/api/v1/opsgraph/lookup";
  return httpClient.post(path, body);
}

export function getOpsGraphNeighborhood(entityId, params = {}) {
  const { graphId, ...query } = params;
  const prefix = graphId ? `/api/v1/opsgraph/graphs/${encodeURIComponent(graphId)}` : "/api/v1/opsgraph";
  return httpClient.get(`${prefix}/entities/${encodeURIComponent(entityId)}/neighborhood${queryString(query)}`);
}

export function getOpsGraphBusinessImpact(entityId, params = {}) {
  const { graphId } = params;
  const prefix = graphId ? `/api/v1/opsgraph/graphs/${encodeURIComponent(graphId)}` : "/api/v1/opsgraph";
  return httpClient.get(`${prefix}/entities/${encodeURIComponent(entityId)}/business-impact`);
}
