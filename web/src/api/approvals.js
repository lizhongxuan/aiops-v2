import httpClient from "./httpClient";

export function submitApprovalDecision(approvalId, decision) {
  return httpClient.post(`/api/v1/approvals/${encodeURIComponent(approvalId)}/decision`, { decision });
}

export function submitChoiceAnswer(requestId, answers) {
  return httpClient.post(`/api/v1/choices/${encodeURIComponent(requestId)}/answer`, { answers });
}
