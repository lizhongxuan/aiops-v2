import { describe, expect, it } from "vitest";

import { formatTargetButtonLabel, resolveComposerDisabledReason } from "./SessionContextBar";

describe("SessionContextBar", () => {
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
});
