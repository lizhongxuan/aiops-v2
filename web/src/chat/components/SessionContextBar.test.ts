import { afterEach, describe, expect, it, vi } from "vitest";

import { buildTargetOptionsForTest, formatTargetButtonLabel, resolveComposerDisabledReason, withSessionContextTimeout } from "./SessionContextBar";

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

  it("binds host chat metadata to environment and Coroot project labels", () => {
    const options = buildTargetOptionsForTest(
      [
        {
          id: "redis-01",
          name: "redis-01",
          labels: {
            env: "prod",
            "aiops.coroot.project": "prod-main",
            cluster: "prod-a",
          },
        },
      ],
      "single_host",
    );

    const redis = options.find((option) => option.hostId === "redis-01");
    expect(redis?.metadata["aiops.environment"]).toBe("prod");
    expect(redis?.metadata["aiops.target.environment"]).toBe("prod");
    expect(redis?.metadata["aiops.coroot.project"]).toBe("prod-main");
    expect(redis?.metadata["aiops.target.cluster"]).toBe("prod-a");
  });

  it("binds environment label groups to Coroot project for workspace chat", () => {
    const options = buildTargetOptionsForTest(
      [
        { id: "web-01", labels: { environment: "staging" } },
        { id: "web-02", labels: { environment: "staging" } },
      ],
      "workspace",
    );

    const staging = options.find((option) => option.value === "label:environment=staging");
    expect(staging?.metadata["aiops.environment"]).toBe("staging");
    expect(staging?.metadata["aiops.coroot.project"]).toBe("staging");
  });
});
