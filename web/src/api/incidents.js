import httpClient from "./httpClient";
import { queryString } from "./query";

export function listIncidents(params = {}) {
  return httpClient.get(`/api/v1/incidents${queryString(params)}`);
}

export function getIncident(incidentId) {
  return httpClient.get(`/api/v1/incidents/${encodeURIComponent(incidentId)}`);
}

export function createIncident(payload = {}) {
  return httpClient.post("/api/v1/incidents", payload);
}

export function updateIncident(incidentId, payload = {}) {
  return httpClient.put(`/api/v1/incidents/${encodeURIComponent(incidentId)}`, payload);
}

export function addIncidentEvidence(incidentId, payload = {}) {
  return httpClient.post(`/api/v1/incidents/${encodeURIComponent(incidentId)}/evidence`, payload);
}

export function closeIncident(incidentId, payload = {}) {
  return httpClient.post(`/api/v1/incidents/${encodeURIComponent(incidentId)}/close`, payload);
}
