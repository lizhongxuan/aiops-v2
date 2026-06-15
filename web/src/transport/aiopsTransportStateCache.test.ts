import { afterEach, describe, expect, it } from "vitest";

import { createInitialAiopsTransportState } from "./aiopsTransportRuntime";
import {
  clearCachedAiopsTransportState,
  getCachedAiopsTransportState,
  resetAiopsTransportStateCacheForTest,
  setCachedAiopsTransportState,
} from "./aiopsTransportStateCache";

describe("aiopsTransportStateCache", () => {
  afterEach(() => {
    resetAiopsTransportStateCacheForTest();
  });

  it("caches and returns a normalized state by scope", () => {
    const state = createInitialAiopsTransportState("thread-cache");
    state.sessionId = "sess-cache";
    state.turnOrder = ["turn-1"];

    setCachedAiopsTransportState("single_host", state);

    expect(getCachedAiopsTransportState("single_host")).toMatchObject({
      sessionId: "sess-cache",
      threadId: "thread-cache",
      turnOrder: ["turn-1"],
      pendingApprovals: {},
      runtimeLiveness: {
        activeTurns: {},
        activeAgents: {},
        pendingApprovals: {},
        pendingUserInputs: {},
        activeCommandStreams: {},
      },
    });
  });

  it("keeps the most recent active session per scope", () => {
    const first = createInitialAiopsTransportState("thread-first");
    first.sessionId = "sess-first";
    const second = createInitialAiopsTransportState("thread-second");
    second.sessionId = "sess-second";

    setCachedAiopsTransportState("workspace", first);
    setCachedAiopsTransportState("workspace", second);

    expect(getCachedAiopsTransportState("workspace")?.sessionId).toBe("sess-second");
  });

  it("does not cache an empty state without a session id", () => {
    const empty = createInitialAiopsTransportState("thread-empty");

    setCachedAiopsTransportState("single_host", empty);
    setCachedAiopsTransportState("single_host", null);

    expect(getCachedAiopsTransportState("single_host")).toBeNull();
  });

  it("clears only the requested cached session", () => {
    const state = createInitialAiopsTransportState("thread-clear");
    state.sessionId = "sess-clear";
    setCachedAiopsTransportState("single_host", state);

    clearCachedAiopsTransportState("single_host", "other-session");
    expect(getCachedAiopsTransportState("single_host")?.sessionId).toBe("sess-clear");

    clearCachedAiopsTransportState("single_host", "sess-clear");
    expect(getCachedAiopsTransportState("single_host")).toBeNull();
  });
});
