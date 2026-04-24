import { fetchEvidenceDetail as fetchEvidenceDetailApi, fetchInvocationDetail as fetchInvocationDetailApi } from "../api/files";

/**
 * Fetch full evidence record by ID.
 * @param {string} sessionId
 * @param {string} evidenceId
 * @returns {Promise<Object>}
 */
export async function fetchEvidenceDetail(sessionId, evidenceId) {
  return fetchEvidenceDetailApi(sessionId, evidenceId);
}

/**
 * Fetch tool invocation detail by ID.
 * @param {string} sessionId
 * @param {string} invocationId
 * @returns {Promise<Object>}
 */
export async function fetchInvocationDetail(sessionId, invocationId) {
  return fetchInvocationDetailApi(sessionId, invocationId);
}
