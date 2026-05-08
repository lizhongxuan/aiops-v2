import { describe, expect, it, vi } from "vitest";

import { createInitialAiopsTransportState } from "./aiopsTransportRuntime";
import { postAssistantTransportCommand } from "./assistantTransportControl";

describe("assistantTransportControl", () => {
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
});
