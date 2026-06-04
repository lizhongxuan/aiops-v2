import { describe, expect, it, vi } from "vitest";

import { createHostOpsApi, normalizeChildAgentTranscript } from "./hostOps";

function createRecordingHttpClient(payload: unknown = { ok: true }, error?: Error) {
  const calls: Array<{ method: string; path: string }> = [];
  return {
    calls,
    get: vi.fn((path: string) => {
      calls.push({ method: "GET", path });
      return error ? Promise.reject(error) : Promise.resolve(payload);
    }),
  };
}

describe("hostOps API", () => {
  it("loads child agent transcript through the same-origin v1 endpoint", async () => {
    const http = createRecordingHttpClient({
      childAgentId: "child/a 1",
      items: [
        {
          id: "item-1",
          type: "manager_message",
          content: "初始化主库",
          createdAt: "2026-06-04T01:00:00Z",
        },
      ],
    });
    const api = createHostOpsApi(http);

    await expect(api.getChildAgentTranscript("child/a 1")).resolves.toMatchObject({
      childAgentId: "child/a 1",
      items: [
        {
          id: "item-1",
          type: "manager_message",
          content: "初始化主库",
        },
      ],
    });
    expect(http.calls).toEqual([
      { method: "GET", path: "/api/v1/host-ops/child-agents/child%2Fa%201/transcript" },
    ]);
  });

  it("normalizes missing transcript fields without throwing", () => {
    expect(normalizeChildAgentTranscript({ childAgentId: "child-1" })).toEqual({
      childAgentId: "child-1",
      items: [],
    });
    expect(normalizeChildAgentTranscript(null)).toEqual({
      childAgentId: "",
      items: [],
    });
  });

  it("uses browser fixture transcript when available", async () => {
    const http = createRecordingHttpClient({ childAgentId: "child-1", items: [] });
    const previousFixture = (window as unknown as { __CODEX_UI_FIXTURE__?: unknown }).__CODEX_UI_FIXTURE__;
    (window as unknown as { __CODEX_UI_FIXTURE__?: unknown }).__CODEX_UI_FIXTURE__ = {
      state: {
        hostOpsTranscripts: {
          "child-1": {
            childAgentId: "child-1",
            items: [{ id: "item-1", type: "assistant_message", content: "PostgreSQL 15" }],
          },
        },
      },
    };
    const api = createHostOpsApi(http);

    await expect(api.getChildAgentTranscript("child-1")).resolves.toMatchObject({
      childAgentId: "child-1",
      items: [{ id: "item-1", content: "PostgreSQL 15" }],
    });
    expect(http.calls).toEqual([]);

    (window as unknown as { __CODEX_UI_FIXTURE__?: unknown }).__CODEX_UI_FIXTURE__ = previousFixture;
  });
});
