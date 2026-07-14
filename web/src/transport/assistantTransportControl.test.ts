import { afterEach, describe, expect, it, vi } from "vitest";

import { createInitialAiopsTransportState } from "./aiopsTransportRuntime";
import {
  fetchAssistantTransportResumeState,
  parseAssistantTransportResumeState,
  postAssistantTransportCommand,
} from "./assistantTransportControl";

describe("assistantTransportControl", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("posts control commands to the assistant transport endpoint with the current state", async () => {
    const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response("aui-state:[]\n", {
        status: 200,
        headers: { "Content-Type": "text/plain" },
      }),
    );
    const state = createInitialAiopsTransportState("thread-1");
    state.sessionId = "sess-1";

    await postAssistantTransportCommand(state, {
      type: "aiops.stop",
      sessionId: "sess-1",
      reason: "user requested stop",
    });

    expect(fetchSpy).toHaveBeenCalledWith(
      "/api/v1/assistant/transport",
      expect.objectContaining({
        method: "POST",
        headers: expect.objectContaining({
          "Content-Type": "application/json",
          Accept: "text/plain",
        }),
        body: JSON.stringify({
          state,
          threadId: "thread-1",
          commands: [
            {
              type: "aiops.stop",
              sessionId: "sess-1",
              reason: "user requested stop",
            },
          ],
        }),
      }),
    );
  });

  it("parses a full resume state from assistant transport stream text", () => {
    const state = createInitialAiopsTransportState("sess-history");
    state.sessionId = "sess-history";
    state.turnOrder = ["turn-1"];

    expect(parseAssistantTransportResumeState(`aui-state:${JSON.stringify([{ type: "set", path: [], value: state }])}\n`)).toEqual(state);
  });

  it("normalizes legacy resume state missing host mission maps", () => {
    const state = createInitialAiopsTransportState("sess-legacy-history") as Partial<ReturnType<typeof createInitialAiopsTransportState>>;
    state.sessionId = "sess-legacy-history";
    state.turnOrder = ["turn-1"];
    delete state.hostMissions;
    delete state.childAgents;

    expect(parseAssistantTransportResumeState(`aui-state:${JSON.stringify([{ type: "set", path: [], value: state }])}\n`)).toMatchObject({
      sessionId: "sess-legacy-history",
      turnOrder: ["turn-1"],
      hostMissions: {},
      childAgents: {},
    });
  });

  it("drops incomplete resume states missing critical transport maps", () => {
    const staleState = createInitialAiopsTransportState("sess-stale") as Partial<ReturnType<typeof createInitialAiopsTransportState>>;
    staleState.sessionId = "sess-stale";
    delete staleState.hostMissions;
    delete staleState.childAgents;
    delete staleState.pendingApprovals;
    delete staleState.mcpSurfaces;
    delete staleState.artifacts;
    delete staleState.runtimeLiveness;

    expect(parseAssistantTransportResumeState(`aui-state:${JSON.stringify([{ type: "set", path: [], value: staleState }])}\n`)).toBeNull();
  });

  it("fetches completed history through the resume endpoint", async () => {
    const state = createInitialAiopsTransportState("sess-history");
    state.sessionId = "sess-history";
    state.turnOrder = ["turn-1"];
    const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(`aui-state:${JSON.stringify([{ type: "set", path: [], value: state }])}\n`, {
        status: 200,
        headers: { "Content-Type": "text/plain" },
      }),
    );

    await expect(fetchAssistantTransportResumeState("sess-history")).resolves.toEqual(state);
    expect(fetchSpy).toHaveBeenCalledWith(
      "/api/v1/assistant/resume",
      expect.objectContaining({
        method: "POST",
        headers: expect.objectContaining({ Accept: "text/plain" }),
      }),
    );
  });

  it("localizes transport network failures instead of surfacing raw fetch errors", async () => {
    vi.spyOn(globalThis, "fetch").mockRejectedValue(new TypeError("Failed to fetch"));
    const state = createInitialAiopsTransportState("thread-network-error");
    state.sessionId = "sess-network-error";

    await expect(
      postAssistantTransportCommand(state, {
        type: "aiops.retry",
        sessionId: "sess-network-error",
      }),
    ).rejects.toThrow("网络异常,请检查后重试");
  });

  it("localizes raw HTTP transport errors before showing them to the user", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response("network error", { status: 502 }));
    const state = createInitialAiopsTransportState("thread-http-error");
    state.sessionId = "sess-http-error";

    await expect(
      postAssistantTransportCommand(state, {
        type: "aiops.stop",
        sessionId: "sess-http-error",
      }),
    ).rejects.toThrow("网络异常,请检查后重试");
  });

  it("localizes resume network failures instead of surfacing raw fetch errors", async () => {
    vi.spyOn(globalThis, "fetch").mockRejectedValue(new TypeError("NetworkError when attempting to fetch resource."));

    await expect(fetchAssistantTransportResumeState("sess-resume-network-error")).rejects.toThrow("网络异常,请检查后重试");
  });
});
