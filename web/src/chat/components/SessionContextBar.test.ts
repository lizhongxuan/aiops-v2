import { afterEach, describe, expect, it, vi } from "vitest";

import {
  buildTargetOptionsForTest,
  formatLlmLabel,
  formatTargetButtonLabel,
  resolveComposerDisabledReason,
  resolveHostTargetIdForTest,
  withSessionContextTimeout,
} from "./SessionContextBar";

describe("SessionContextBar", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it("does not duplicate the host prefix in the single-host selector", () => {
    expect(formatTargetButtonLabel("single_host", "server-local")).toBe("server-local");
    expect(formatTargetButtonLabel("single_host")).toBe("未选择执行目标");
  });

  it("shows only the model name in the composer LLM label", () => {
    expect(formatLlmLabel({ provider: "openai", model: "glm-4.7" })).toBe("glm-4.7");
    expect(formatLlmLabel({ provider: "openai", model: "gpt-5.4" })).toBe("gpt-5.4");
    expect(formatLlmLabel(null)).toBe("LLM 未配置");
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

  it("does not ask users to manually create a chat session", () => {
    expect(
      resolveComposerDisabledReason({
        hasActiveSession: false,
        llmConfigured: true,
      }),
    ).toBe("正在初始化会话");
    expect(
      resolveComposerDisabledReason({
        hasActiveSession: false,
        llmConfigured: true,
        sessionInitError: "会话初始化失败，请刷新重试",
      }),
    ).toBe("会话初始化失败，请刷新重试");
    expect(
      resolveComposerDisabledReason({
        hasActiveSession: false,
        llmConfigured: false,
      }),
    ).toBe("请先在设置中配置 LLM");
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

  it("does not bind an implicit local host for unselected single-host chat", () => {
    const options = buildTargetOptionsForTest([], "single_host");

    expect(resolveHostTargetIdForTest("single_host", options, "none", [])).toBeUndefined();
    expect(resolveHostTargetIdForTest("single_host", options, "host:missing", [])).toBeUndefined();
  });

  it("binds a single-host chat only when the host target is explicit", () => {
    const hosts = [{ id: "redis-01", name: "redis-01", address: "10.0.0.11", labels: {} }];
    const options = buildTargetOptionsForTest(hosts, "single_host");

    expect(resolveHostTargetIdForTest("single_host", options, "host:redis-01", hosts)).toBe("redis-01");
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
