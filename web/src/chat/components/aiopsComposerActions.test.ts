import { describe, expect, it } from "vitest";

import { createInitialAiopsTransportState } from "@/transport/aiopsTransportRuntime";

import { resolveStopDispatchTarget } from "./aiopsComposerActions";

describe("aiopsComposerActions", () => {
  it("prefers transport stop for an active transport turn even when assistant-ui reports running", () => {
    const state = {
      ...createInitialAiopsTransportState("thread-1"),
      sessionId: "sess-1",
      currentTurnId: "turn-1",
      status: "working" as const,
      runtimeLiveness: {
        activeTurns: { "turn-1": true },
        activeAgents: {},
        pendingApprovals: {},
        pendingUserInputs: {},
        activeCommandStreams: {},
      },
    };

    expect(resolveStopDispatchTarget(state, true)).toBe("transport");
  });

  it("uses transport stop as soon as the session exists, even before currentTurnId is projected", () => {
    const state = {
      ...createInitialAiopsTransportState("thread-1"),
      sessionId: "sess-1",
      status: "idle" as const,
    };

    expect(resolveStopDispatchTarget(state, true)).toBe("transport");
  });

  it("falls back to runtime cancel only when no transport session exists yet", () => {
    const state = createInitialAiopsTransportState("thread-1");

    expect(resolveStopDispatchTarget(state, true)).toBe("runtime");
  });
});
