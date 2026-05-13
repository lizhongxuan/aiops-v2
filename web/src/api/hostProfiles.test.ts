import { describe, expect, it, vi } from "vitest";
import { createHostProfilesApi } from "./hostProfiles";

function createRecordingHttpClient() {
  const calls: Array<{ method: string; path: string; body?: unknown }> = [];
  return {
    calls,
    get: vi.fn((path: string) => {
      calls.push({ method: "GET", path });
      return Promise.resolve({ ok: true });
    }),
  };
}

describe("hostProfiles API", () => {
  it("routes HostProfile and HostLease reads through /api/v1 same-origin endpoints", async () => {
    const http = createRecordingHttpClient();
    const api = createHostProfilesApi(http);

    await api.listHostProfiles({ env: "prod", status: "online", limit: 20, cursor: "next/1" });
    await api.getHostProfile("host/a 1");
    await api.listHostLeases({ hostId: "host/a 1", status: "active", missionId: "mission/9" });
    await api.listHostReportHistory("host/a 1");

    expect(http.calls).toEqual([
      {
        method: "GET",
        path: "/api/v1/host-profiles?env=prod&status=online&limit=20&cursor=next%2F1",
      },
      { method: "GET", path: "/api/v1/host-profiles/host%2Fa%201" },
      {
        method: "GET",
        path: "/api/v1/host-leases?host_id=host%2Fa%201&status=active&mission_id=mission%2F9",
      },
      { method: "GET", path: "/api/v1/host-profiles/host%2Fa%201/report-history" },
    ]);
  });

  it("omits empty query values", async () => {
    const http = createRecordingHttpClient();
    const api = createHostProfilesApi(http);

    await api.listHostProfiles({ env: "", role: undefined, arch: "x86_64" });
    await api.listHostLeases({ hostId: "", status: null, ownerSessionId: "session-1" });

    expect(http.calls).toEqual([
      { method: "GET", path: "/api/v1/host-profiles?arch=x86_64" },
      { method: "GET", path: "/api/v1/host-leases?owner_session_id=session-1" },
    ]);
  });
});
