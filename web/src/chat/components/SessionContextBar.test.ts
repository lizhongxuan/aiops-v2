import { afterEach, describe, expect, it, vi } from "vitest";

import { formatTargetButtonLabel, resolveComposerDisabledReason, withSessionContextTimeout } from "./SessionContextBar";

describe("SessionContextBar", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it("does not duplicate the host prefix in the single-host selector", () => {
    expect(formatTargetButtonLabel("single_host", "server-local")).toBe("server-local");
    expect(formatTargetButtonLabel("single_host")).toBe("server-local");
  });

  it("disables composer while a new session is being created", () => {
    expect(
      resolveComposerDisabledReason({
        activeAction: "create",
        hasActiveSession: true,
        llmConfigured: true,
      }),
    ).toBe("正在创建会话");
  });

  it("times out session context requests so the refresh busy state can finish", async () => {
    vi.useFakeTimers();
    const pending = new Promise<string>(() => {});
    const timed = withSessionContextTimeout(pending, 25, "加载会话上下文");

    const assertion = expect(timed).rejects.toThrow("加载会话上下文 timed out after 25ms");
    await vi.advanceTimersByTimeAsync(25);
    await assertion;
  });
});
