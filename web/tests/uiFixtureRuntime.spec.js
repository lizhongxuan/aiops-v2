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

    expect(chat?.state).toMatchObject(createChatFixtureState());
    expect(chat?.sessions).toMatchObject(createChatFixtureSessions());
    expect(protocol?.state).toMatchObject({ kind: "workspace", sessionId: "workspace-1" });
    expect(protocol?.sessions).toMatchObject({ activeSessionId: "workspace-1" });
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
