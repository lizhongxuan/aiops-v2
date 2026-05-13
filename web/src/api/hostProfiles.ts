import httpClient from "./httpClient";

export type HostProfileView = {
  hostId: string;
  displayName: string;
  status: string;
  os: string;
  arch: string;
  labels: Record<string, string>;
  lastHeartbeatAt: string;
  profileExpiresAt: string;
  raw: Record<string, unknown>;
};

export type HostLeaseView = {
  leaseId: string;
  hostId: string;
  status: string;
  missionId: string;
  ownerSessionId: string;
  acquiredAt: string;
  expiresAt: string;
  raw: Record<string, unknown>;
};

export type HostReportHistoryView = {
  reportId: string;
  hostId: string;
  status: string;
  reportedAt: string;
  summary: string;
  raw: Record<string, unknown>;
};

type HostProfilesHttpClient = {
  get(path: string): Promise<unknown>;
};

export type ListHostProfilesParams = {
  env?: string | null;
  role?: string | null;
  status?: string | null;
  os?: string | null;
  arch?: string | null;
  hostId?: string | null;
  limit?: number | string | null;
  cursor?: string | null;
};

export type ListHostLeasesParams = {
  hostId?: string | null;
  status?: string | null;
  missionId?: string | null;
  ownerSessionId?: string | null;
  limit?: number | string | null;
  cursor?: string | null;
};

function appendQuery(path: string, params: Record<string, unknown> = {}) {
  const queryText = Object.entries(params)
    .filter(([, value]) => value !== undefined && value !== null && value !== "")
    .map(([key, value]) => `${encodeURIComponent(key)}=${encodeURIComponent(String(value))}`)
    .join("&");
  return queryText ? `${path}?${queryText}` : path;
}

function encodePath(value: string) {
  return encodeURIComponent(value);
}

export function createHostProfilesApi(client: HostProfilesHttpClient = httpClient) {
  return {
    listHostProfiles(params: ListHostProfilesParams = {}) {
      return client.get(
        appendQuery("/api/v1/host-profiles", {
          env: params.env,
          role: params.role,
          status: params.status,
          os: params.os,
          arch: params.arch,
          host_id: params.hostId,
          limit: params.limit,
          cursor: params.cursor,
        }),
      );
    },

    getHostProfile(hostId: string) {
      return client.get(`/api/v1/host-profiles/${encodePath(hostId)}`);
    },

    listHostLeases(params: ListHostLeasesParams = {}) {
      return client.get(
        appendQuery("/api/v1/host-leases", {
          host_id: params.hostId,
          status: params.status,
          mission_id: params.missionId,
          owner_session_id: params.ownerSessionId,
          limit: params.limit,
          cursor: params.cursor,
        }),
      );
    },

    listHostReportHistory(hostId: string) {
      return client.get(`/api/v1/host-profiles/${encodePath(hostId)}/report-history`);
    },
  };
}

const hostProfilesApi = createHostProfilesApi();

export const listHostProfiles = hostProfilesApi.listHostProfiles;
export const getHostProfile = hostProfilesApi.getHostProfile;
export const listHostLeases = hostProfilesApi.listHostLeases;
export const listHostReportHistory = hostProfilesApi.listHostReportHistory;
