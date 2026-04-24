import httpClient from "./httpClient";

function buildQuery(params = {}) {
  const query = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value === null || value === undefined || value === "") continue;
    query.set(key, String(value));
  }
  return query.toString();
}

export function fetchApprovalAudits(params = {}) {
  const query = buildQuery(params);
  return httpClient.get(`/api/v1/approval-audits${query ? `?${query}` : ""}`);
}

export function fetchApprovalGrants(hostId = "") {
  return httpClient.get(hostId ? `/api/v1/approval-grants?hostId=${encodeURIComponent(hostId)}` : "/api/v1/approval-grants");
}

export function revokeApprovalGrant(grantId) {
  return httpClient.post(`/api/v1/approval-grants/${encodeURIComponent(grantId)}/revoke`);
}

export function disableApprovalGrant(grantId) {
  return httpClient.post(`/api/v1/approval-grants/${encodeURIComponent(grantId)}/disable`);
}

export function enableApprovalGrant(grantId) {
  return httpClient.post(`/api/v1/approval-grants/${encodeURIComponent(grantId)}/enable`);
}
