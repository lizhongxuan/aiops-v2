import httpClient from "./httpClient";

type ArchivePayload = {
  sessionId?: string;
  turnId?: string;
  title?: string;
  summary?: string;
};

function archivePath(opsRunId: string, action: string) {
  return `/api/v1/chat/ops-runs/${encodeURIComponent(opsRunId)}/${action}`;
}

export function archiveOpsRunCase(
  opsRunId: string,
  payload: ArchivePayload = {},
) {
  return httpClient.post(archivePath(opsRunId, "archive-case"), payload);
}

export function createOpsRunRunRecord(
  opsRunId: string,
  payload: ArchivePayload = {},
) {
  return httpClient.post(archivePath(opsRunId, "run-record"), payload);
}

export function createOpsRunExperienceCandidates(
  opsRunId: string,
  payload: ArchivePayload = {},
) {
  return httpClient.post(
    archivePath(opsRunId, "experience-candidates"),
    payload,
  );
}
