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

  it("resolves the tool progressive discovery browser fixture", () => {
    const fixture = resolveUiFixturePreset("tool-progressive-discovery");

    expect(fixture?.name).toBe("tool-progressive-discovery");
    expect(fixture?.state?.sessionId).toBe("tool-progressive-discovery");
    expect(JSON.stringify(fixture?.state)).toContain("tool_search mode=search");
    expect(JSON.stringify(fixture?.state)).toContain("tool_unloaded recoverable error");
    expect(JSON.stringify(fixture?.state)).toContain("not_checked");
  });

  it("resolves the skills and mcp progressive discovery browser fixture", () => {
    const fixture = resolveUiFixturePreset("skills-mcp-progressive-discovery");

    expect(fixture?.name).toBe("skills-mcp-progressive-discovery");
    expect(fixture?.state?.sessionId).toBe("skills-mcp-progressive-discovery");
    expect(JSON.stringify(fixture?.state)).toContain("skill_search mode=search");
    expect(JSON.stringify(fixture?.state)).toContain("skill_read skill=synthetic.triage");
    expect(JSON.stringify(fixture?.state)).toContain("mandatory skill activation retry");
    expect(JSON.stringify(fixture?.state)).toContain("mcp instruction delta: added synthetic-docs");
    expect(JSON.stringify(fixture?.state)).toContain("mcp sparse reminder");
    expect(JSON.stringify(fixture?.state)).toContain("mcp resource artifact: application/pdf");
    expect(JSON.stringify(fixture?.state)).toContain("final evidence: skill checked");
  });

  it("resolves the task todo plan mode fixture with structured plan/task state", () => {
    const fixture = resolveUiFixturePreset("task-todo-plan-mode");
    const serialized = JSON.stringify(fixture?.state);

    expect(fixture?.name).toBe("task-todo-plan-mode");
    expect(fixture?.state?.sessionId).toBe("task-todo-plan-mode");
    expect(fixture?.state?.planModeState).toMatchObject({
      state: "active",
      planId: "plan-synthetic-task-todo-1",
    });
    expect(fixture?.state?.planArtifact).toMatchObject({
      id: "plan-synthetic-task-todo-1",
      status: "pending_exit_approval",
    });
    expect(serialized).toContain("owner=agent:planner");
    expect(serialized).toContain("agentId=agent-plan-7");
    expect(serialized).toContain("blockedBy=missing_user_decision");
    expect(serialized).toContain("用户要求收窄验证范围后再批准");
    expect(serialized).toContain("claim-lease-synthetic-1");
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
