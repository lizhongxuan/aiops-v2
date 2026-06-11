import { describe, expect, it, vi } from "vitest";

import { createHostOpsApi, normalizeChildAgentTranscript } from "./hostOps";

function createRecordingHttpClient(payload: unknown = { ok: true }, error?: Error) {
  const calls: Array<{ method: string; path: string; body?: unknown }> = [];
  return {
    calls,
    get: vi.fn((path: string) => {
      calls.push({ method: "GET", path });
      return error ? Promise.reject(error) : Promise.resolve(payload);
    }),
    post: vi.fn((path: string, body?: unknown) => {
      calls.push({ method: "POST", path, body });
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
          content: "执行主机准备步骤",
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
          content: "执行主机准备步骤",
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

  it("submits host command approval decisions through the shared approvals endpoint", async () => {
    const http = createRecordingHttpClient({ status: "accepted" });
    const api = createHostOpsApi(http);

    await expect(api.submitApprovalDecision("hostcmd-approval/a 1", "accept")).resolves.toEqual({ status: "accepted" });
    expect(http.calls).toEqual([
      {
        method: "POST",
        path: "/api/v1/approvals/hostcmd-approval%2Fa%201/decision",
        body: { decision: "accept" },
      },
    ]);
  });

  it("uses browser fixture transcript when available", async () => {
    const http = createRecordingHttpClient({ childAgentId: "child-1", items: [] });
    const previousFixture = (window as unknown as { __CODEX_UI_FIXTURE__?: unknown }).__CODEX_UI_FIXTURE__;
    (window as unknown as { __CODEX_UI_FIXTURE__?: unknown }).__CODEX_UI_FIXTURE__ = {
      state: {
        hostOpsTranscripts: {
          "child-1": {
            childAgentId: "child-1",
            items: [{ id: "item-1", type: "assistant_message", content: "主机状态正常" }],
          },
        },
      },
    };
    const api = createHostOpsApi(http);

    await expect(api.getChildAgentTranscript("child-1")).resolves.toMatchObject({
      childAgentId: "child-1",
      items: [{ id: "item-1", content: "主机状态正常" }],
    });
    expect(http.calls).toEqual([]);

    (window as unknown as { __CODEX_UI_FIXTURE__?: unknown }).__CODEX_UI_FIXTURE__ = previousFixture;
  });
});
