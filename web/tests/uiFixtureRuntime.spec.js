import { beforeEach, describe, expect, it } from "vitest";
import { createChatFixtureSessions, createChatFixtureState, resolveUiFixturePreset } from "../src/lib/uiFixturePresets";
import { resolveUiFixtureRuntime } from "../src/lib/uiFixtureRuntime";

describe("uiFixture runtime", () => {
  beforeEach(() => {
    delete window.__CODEX_UI_FIXTURE__;
  });

  it("resolves built-in chat and protocol presets", () => {
    const chat = resolveUiFixturePreset("chat");
    const protocol = resolveUiFixturePreset("protocol");
    const opsManualForm = resolveUiFixturePreset("ops-manual-4field-form");
    const opsManualParam = resolveUiFixturePreset("ops-manual-param-auto-redis");
    const opsManualSecret = resolveUiFixturePreset("ops-manual-param-secret");

    expect(chat?.state).toMatchObject(createChatFixtureState());
    expect(chat?.sessions).toMatchObject(createChatFixtureSessions());
    expect(protocol?.state).toMatchObject({ kind: "workspace", sessionId: "workspace-1" });
    expect(protocol?.sessions).toMatchObject({ activeSessionId: "workspace-1" });
    expect(opsManualForm?.state?.sessionId).toBe("ops-manual-4field-form");
    expect(opsManualParam?.state?.sessionId).toBe("ops-manual-param-auto-redis");
    expect(opsManualSecret?.state?.sessionId).toBe("ops-manual-param-secret");
  });

  it("accepts verify query as a fixture key for manual browser checks", () => {
    window.history.pushState({}, "", "/?verify=ops-manual-4field-form");

    const runtime = resolveUiFixtureRuntime();

    expect(runtime?.state?.sessionId).toBe("ops-manual-4field-form");
  });

  it("prefers an injected browser fixture payload over query parsing", () => {
    window.__CODEX_UI_FIXTURE__ = {
      state: createChatFixtureState({ sessionId: "single-custom" }),
      sessions: createChatFixtureSessions({ activeSessionId: "single-custom" }),
    };
    const runtime = resolveUiFixtureRuntime();

    expect(runtime?.state?.sessionId).toBe("single-custom");
    expect(runtime?.sessions?.activeSessionId).toBe("single-custom");
  });
});
