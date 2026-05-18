// @ts-check
import { test, expect } from "@playwright/test";
import { createChatFixtureSessions, createChatFixtureState, openFixturePage } from "../helpers/uiFixtureHarness";

const UI_CARDS_RESPONSE = {
  total: 2,
  stats: { total: 2, active: 2, draft: 0, deprecated: 0, disabled: 0, builtIn: 2 },
  items: [
    {
      id: "coroot-chart",
      name: "Coroot Chart",
      kind: "coroot_chart",
      renderer: "agent-ui/coroot-chart",
      status: "active",
      builtIn: true,
      version: 1,
      payloadSchema: { type: "object" },
      actionPolicy: { allowed: ["open_coroot"] },
      redactionPolicy: { dangerousKeys: ["script"] },
      placementDefaults: ["assistant_turn"],
      samplePayloads: [{ id: "sample-coroot", name: "Coroot p95", artifact: { id: "sample-1", type: "coroot_chart", titleZh: "Coroot p95 预览", payload: { chart: "p95" } } }],
    },
    {
      id: "trace-summary",
      name: "Trace Summary",
      kind: "trace_summary",
      renderer: "agent-ui/trace-summary",
      status: "active",
      builtIn: true,
      version: 1,
    },
  ],
};

const AGENT_UI_ARTIFACTS_RESPONSE = {
  total: 1,
  items: [
    {
      id: "artifact-coroot-1",
      type: "coroot_chart",
      title: "Coroot p95",
      status: "ready",
      source: "coroot",
      promptTraceId: "trace-1",
      metadata: { caseId: "case-1", promptTraceId: "trace-1" },
      payload: { chart: "p95" },
      actions: [{ id: "trace", label: "Prompt Trace", href: "/debug/prompts?trace_id=trace-1" }],
      updatedAt: "2026-05-16T10:00:00Z",
    },
  ],
};

async function routeRegistryApis(page) {
  await page.route("**/api/v1/ui-cards", (route) => route.fulfill({ json: UI_CARDS_RESPONSE }));
  await page.route("**/api/v1/ui-cards/*/preview", (route) =>
    route.fulfill({ json: { valid: true, normalizedArtifact: { titleZh: "Coroot p95 预览" } } }),
  );
  await page.route("**/api/v1/ui-cards/*/status", (route) =>
    route.fulfill({ json: { id: "coroot-chart", status: "disabled", version: 2 } }),
  );
  await page.route("**/api/v1/agent-ui-artifacts**", (route) => route.fulfill({ json: AGENT_UI_ARTIFACTS_RESPONSE }));
}

test.describe("agent ui lightweight registry", () => {
  test("UI Cards page previews built-in samples", async ({ page }) => {
    await routeRegistryApis(page);
    await openFixturePage(page, "/ui-cards", "chat");

    await expect(page.getByText("Coroot Chart").first()).toBeVisible();
    await page.getByRole("button", { name: "Preview" }).click();
    await page.getByRole("button", { name: "运行 Preview" }).click();

    await expect(page.getByText("Local Preview")).toBeVisible();
    await expect(page.getByText("Coroot p95 预览", { exact: true })).toBeVisible();
    await expect(page.getByText("API Result")).toBeVisible();
  });

  test("Chat renders unknown artifacts through the unsupported terminal renderer", async ({ page }) => {
    const state = createChatFixtureState({
      cards: [
        {
          id: "user-unknown-artifact",
          type: "UserMessageCard",
          role: "user",
          text: "展示未知 Agent UI 卡片",
          createdAt: "2026-05-16T10:00:00Z",
          updatedAt: "2026-05-16T10:00:00Z",
        },
      ],
      runtime: { turn: { active: false, phase: "idle", hostId: "server-local" }, codex: { status: "connected", retryAttempt: 0, retryMax: 5 } },
      sessionId: "unknown-artifact",
      threadId: "unknown-artifact",
    });
    state.turns[state.currentTurnId].agentUiArtifacts = [
      {
        id: "artifact-unknown-widget",
        type: "shell_widget",
        titleZh: "未知卡片",
        summaryZh: "后端返回了未注册 UI artifact。",
        payload: { script: "alert(1)" },
      },
    ];
    const sessions = createChatFixtureSessions({ activeSessionId: "unknown-artifact", sessions: [{ id: "unknown-artifact", title: "Unknown Artifact", status: "completed", messageCount: 1 }] });

    await openFixturePage(page, "/", { state, sessions });

    await expect(page.getByText("暂不支持的卡片类型")).toBeVisible();
    await expect(page.getByText("shell_widget")).toBeVisible();
    await expect(page.getByText("alert(1)")).toHaveCount(0);
    const scriptText = await page.locator("script").allTextContents();
    expect(scriptText.join("\n")).not.toContain("alert(1)");
  });

  test("Agent UI Center filters and opens artifact detail", async ({ page }) => {
    await routeRegistryApis(page);
    await openFixturePage(page, "/agent-ui", "chat");

    await expect(page.getByText("Agent UI 产物")).toBeVisible();
    await page.getByLabel("source filter").fill("coroot");
    await expect(page.getByText("Coroot p95")).toBeVisible();
    await page.getByRole("button", { name: /Coroot p95/ }).click();
    await expect(page.getByRole("dialog")).toContainText("Normalized JSON");
    await expect(page.getByRole("dialog")).toContainText("Prompt Trace");
    await expect(page.getByRole("dialog")).toContainText("trace-1");
  });
});
