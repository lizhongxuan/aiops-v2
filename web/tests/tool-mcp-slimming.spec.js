// @ts-check
import { expect, test } from "@playwright/test";
import { mkdirSync } from "node:fs";
import path from "node:path";

import { createChatFixtureSessions, createChatFixtureState, installUiFixture, waitForFixtureStable } from "./helpers/uiFixtureHarness";
import { expectStableLocatorScreenshot } from "./helpers/visualSnapshot";

const screenshotDir = path.resolve(process.cwd(), "..", "output", "playwright", "tool-mcp-slimming");

function ensureScreenshotDir() {
  mkdirSync(screenshotDir, { recursive: true });
}

function toolMcpSlimmingChatFixture() {
  const now = "2026-06-12T10:00:00Z";
  const finalText = [
    "CPU 资源来自当前主机 direct host evidence：load average 1.12/1.04/0.98，CPU idle 82.4%，核心数 8。",
    "Coroot 当前 unavailable，未执行 Coroot tool，也没有用不可用 MCP 数据支撑正常结论。",
    "initial visible tools (4): exec_command, tool_search, update_plan, list_mcp_resources。",
  ].join("\n");
  const state = createChatFixtureState({
    sessionId: "tool-mcp-slimming-chat",
    threadId: "tool-mcp-slimming-chat",
    status: "idle",
    cards: [
      {
        id: "user-tool-mcp-cpu",
        type: "UserMessageCard",
        role: "user",
        text: "查看当前主机 CPU 资源信息。",
        status: "completed",
        createdAt: now,
        updatedAt: now,
      },
      {
        id: "assistant-tool-mcp-cpu",
        type: "AssistantMessageCard",
        role: "assistant",
        text: finalText,
        status: "completed",
        createdAt: "2026-06-12T10:00:12Z",
        updatedAt: "2026-06-12T10:00:12Z",
      },
    ],
    runtime: {
      turn: { active: false, phase: "completed", hostId: "server-local" },
      codex: { status: "connected", retryAttempt: 0, retryMax: 5 },
      activity: { viewedFiles: [], searchedWebQueries: [], searchedContentQueries: [] },
    },
    finalText,
  });
  const turn = state.turns[state.currentTurnId];
  turn.status = "completed";
  turn.startedAt = now;
  turn.completedAt = "2026-06-12T10:00:12Z";
  turn.updatedAt = "2026-06-12T10:00:12Z";
  turn.process = [
    {
      id: "initial-tool-surface",
      kind: "system",
      displayKind: "tool_surface",
      status: "completed",
      text: "initial visible tools (4): exec_command, tool_search, update_plan, list_mcp_resources",
      updatedAt: "2026-06-12T10:00:01Z",
    },
    {
      id: "direct-host-evidence",
      kind: "tool",
      displayKind: "exec_command",
      status: "completed",
      text: "exec_command uptime && top -l 1 -s 0 used direct host evidence for current host CPU resource query",
      outputPreview: "direct host evidence: load average 1.12/1.04/0.98; CPU idle 82.4%; ncpu=8",
      updatedAt: "2026-06-12T10:00:06Z",
    },
    {
      id: "coroot-filtered",
      kind: "system",
      displayKind: "mcp_health_gate",
      status: "completed",
      text: "coroot mcp unavailable candidate filtered; reason=mcp_unavailable; no coroot tool executed",
      outputPreview: "Coroot unavailable; fallback evidence source=exec_command",
      updatedAt: "2026-06-12T10:00:07Z",
    },
    {
      id: "historical-trend-healthy-path",
      kind: "tool",
      displayKind: "tool_search",
      status: "completed",
      text: "historical trend intent with healthy observability mcp can select coroot.metrics.query",
      outputPreview: "selected tool delta: +coroot.metrics.query when mcpHealth=healthy",
      updatedAt: "2026-06-12T10:00:08Z",
    },
  ];
  return {
    name: "tool-mcp-slimming",
    state,
    sessions: createChatFixtureSessions({
      activeSessionId: "tool-mcp-slimming-chat",
      sessions: [
        {
          id: "tool-mcp-slimming-chat",
          kind: "single_host",
          title: "Tool MCP slimming",
          status: "idle",
          messageCount: 2,
          preview: "查看当前主机 CPU 资源信息",
          selectedHostId: "server-local",
          lastActivityAt: "2026-06-12T10:00:12Z",
        },
      ],
    }),
  };
}

function toolSurfaceTraceFixture() {
  return {
    schemaVersion: 1,
    kind: "runtime_model_input",
    sessionId: "tool-mcp-slimming-chat",
    turnId: "turn-user-tool-mcp-cpu",
    iteration: 0,
    createdAt: "2026-06-12T10:00:08Z",
    visibleTools: ["exec_command", "tool_search", "update_plan", "list_mcp_resources"],
    promptFingerprint: {
      stableHash: "1111111111111111111111111111111111111111111111111111111111111111",
      developerHash: "2222222222222222222222222222222222222222222222222222222222222222",
      toolRegistryHash: "3333333333333333333333333333333333333333333333333333333333333333",
    },
    prompt: {
      tools: "exec_command\ntool_search\nupdate_plan\nlist_mcp_resources",
    },
    metadata: {
      "aiops.target.refs": "host:server-local,service:checkout",
      "aiops.env.readOnlyReason": "target_conflict_requires_clarification",
      "aiops.env.compactContext": [
        "EnvironmentFactsContext:",
        "TargetRefs:",
        "- host id=host:server-local address=server-local source=user_explicit confidence=confirmed",
        "ConflictFacts:",
        "- topology service:checkout reason=target_conflict",
      ].join("\n"),
    },
    modelInput: [
      { index: 0, providerRole: "system", semanticRole: "system", promptLayer: "system", content: "system prompt" },
      { index: 1, providerRole: "system", semanticRole: "developer", promptLayer: "developer", content: "tool slimming policy" },
      { index: 2, providerRole: "system", semanticRole: "tool", promptLayer: "tool_index", content: "exec_command\ntool_search\nupdate_plan\nlist_mcp_resources" },
      { index: 3, providerRole: "user", semanticRole: "user", promptLayer: "conversation", content: "查看当前主机 CPU 资源信息" },
    ],
    toolSurfaceTrace: {
      initialTools: ["exec_command", "tool_search", "update_plan", "list_mcp_resources"],
      baseRegistryCount: 12,
      deferredFamilies: [
        { pack: "public_web", capability: "web", source: "builtin", toolCount: 2 },
        { pack: "opsgraph", capability: "topology", source: "builtin", toolCount: 4 },
        { pack: "ops_manual_flow", capability: "runbook", source: "builtin", toolCount: 3 },
        { pack: "evidence_read", capability: "evidence", source: "builtin", toolCount: 1 },
        { pack: "coroot_observability", capability: "metrics", source: "mcp", mcpServerId: "coroot", healthStatus: "unavailable", unavailableReason: "502 bad gateway", toolCount: 6 },
      ],
      loadedTools: ["coroot.metrics.query"],
      loadedPacks: ["coroot_observability"],
      filteredTools: [{ toolName: "coroot.metrics.query", reason: "mcp_unavailable for current host CPU resource query" }],
      mcpHealth: { coroot: "unavailable: 502 bad gateway", "observability-healthy": "healthy" },
      toolSearchEvents: [
        { mode: "search", query: "current host cpu resource", matchCount: 1, matches: ["exec_command"] },
        { mode: "select", query: "historical cpu trend", matchCount: 1, matches: ["coroot.metrics.query"] },
      ],
      selectedTools: ["exec_command", "coroot.metrics.query"],
      rejectedToolReasons: [{ toolName: "coroot.metrics.query", errorType: "mcp_unavailable", reason: "unavailable data cannot support normal conclusion" }],
    },
  };
}

async function installCommonRoutes(page) {
  await page.route("**/api/v1/hosts", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        hosts: [{ id: "server-local", name: "server-local", status: "online", executable: true, terminalCapable: true }],
      }),
    }),
  );
  await page.route("**/api/v1/llm-config", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ provider: "zai", model: "glm-4.7", configured: true }),
    }),
  );
}

test.describe("Claude Code style tool MCP slimming", () => {
  test("AI Chat uses direct host evidence and does not execute unavailable Coroot", async ({ page }) => {
    ensureScreenshotDir();
    await installCommonRoutes(page);
    await installUiFixture(page, toolMcpSlimmingChatFixture());
    await page.goto("/", { waitUntil: "networkidle" });
    await waitForFixtureStable(page);

    await expect(page.getByText("查看当前主机 CPU 资源信息")).toBeVisible();
    await page.getByRole("button", { name: /已处理/ }).click();
    await expect(page.getByText("initial visible tools (4): exec_command, tool_search, update_plan, list_mcp_resources").first()).toBeVisible();
    await expect(page.getByText("direct host evidence").first()).toBeVisible();
    await expect(page.getByText("coroot mcp unavailable candidate filtered").first()).toBeVisible();
    await expect(page.getByText("未执行 Coroot tool").first()).toBeVisible();
    await expect(page.getByText("historical trend intent with healthy observability mcp can select coroot.metrics.query").first()).toBeVisible();
    await expect(page.getByText("coroot.metrics.execute")).toHaveCount(0);
    await page.screenshot({ path: path.join(screenshotDir, "ai-chat-tool-mcp-slimming.png"), fullPage: true });
  });

  test("MCP server page shows unavailable observability server without making it a normal tool", async ({ page }) => {
    ensureScreenshotDir();
    await installCommonRoutes(page);
    await page.route("**/api/v1/mcp/servers", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          configPath: "mcp-servers.json",
          items: [
            {
              name: "coroot",
              transport: "http",
              url: "",
              status: "error",
              error: "502 bad gateway",
              toolCount: 0,
              resourceCount: 0,
            },
          ],
        }),
      }),
    );
    await page.route("**/api/v2/runtime/mcp-health", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          items: [
            {
              serverId: "coroot",
              displayName: "coroot",
              status: "unhealthy",
              lastError: "502 bad gateway",
              availableToolCount: 0,
              disabledReason: "mcp_unavailable",
            },
          ],
        }),
      }),
    );
    await page.goto("/mcp", { waitUntil: "networkidle" });
    await waitForFixtureStable(page);

    await expect(page.getByText("coroot", { exact: true })).toBeVisible();
    await expect(page.getByText("502 bad gateway").first()).toBeVisible();
    await expect(page.getByText("0 / 0")).toBeVisible();
    await page.screenshot({ path: path.join(screenshotDir, "mcp-unavailable-health.png"), fullPage: true });
  });

  test("Prompt Trace shows initial tools, deferred families, MCP health, and selected observability path", async ({ page }) => {
    ensureScreenshotDir();
    const trace = toolSurfaceTraceFixture();
    await installCommonRoutes(page);
    await page.route("**/api/v1/debug/model-input-traces?**", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          selectedId: "trace-tool-mcp-slimming",
          traces: [
            {
              id: "trace-tool-mcp-slimming",
              relativePath: "tool-mcp-slimming/trace.json",
              jsonPath: "tool-mcp-slimming/trace.json",
              kind: "runtime_model_input",
              sessionId: trace.sessionId,
              turnId: trace.turnId,
              createdAt: trace.createdAt,
              visibleTools: trace.visibleTools,
              messageCount: trace.modelInput.length,
              userPromptPreview: "查看当前主机 CPU 资源信息",
              promptFingerprint: trace.promptFingerprint,
              toolSurface: {
                initialToolCount: 4,
                baseRegistryCount: 12,
                deferredFamilyCount: 5,
                loadedToolCount: 1,
                mcpHealth: { coroot: "unavailable: 502 bad gateway" },
              },
            },
          ],
        }),
      }),
    );
    await page.route("**/api/v1/debug/model-input-traces/file?**", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ content: JSON.stringify(trace, null, 2) }),
      }),
    );
    await page.goto("/debug/prompts", { waitUntil: "networkidle" });
    await waitForFixtureStable(page);

    await page.getByTestId("prompt-trace-llm-card").click();
    await expect(page.getByRole("heading", { name: "Environment Context" })).toBeVisible();
    await expect(page.getByText("host:server-local").first()).toBeVisible();
    await expect(page.getByText("target_conflict_requires_clarification").first()).toBeVisible();
    const environmentContextPanel = page
      .getByRole("heading", { name: "Environment Context" })
      .locator("xpath=ancestor::section[1]");
    await expectStableLocatorScreenshot(environmentContextPanel, "prompt-trace-environment-context.png");

    await page.getByRole("button", { name: "工具" }).click();
    await expect(page.getByText("Initial Tool Surface")).toBeVisible();
    await expect(page.getByText("exec_command").first()).toBeVisible();
    await expect(page.getByRole("heading", { name: "Deferred Families" })).toBeVisible();
    await expect(page.getByText("public_web").first()).toBeVisible();
    await expect(page.getByText("ops_manual_flow").first()).toBeVisible();
    await expect(page.getByRole("heading", { name: "MCP Health" })).toBeVisible();
    await expect(page.getByText("unavailable: 502 bad gateway").first()).toBeVisible();
    await expect(page.getByText("coroot.metrics.query").first()).toBeVisible();
    await expect(page.getByText("unavailable data cannot support normal conclusion").first()).toBeVisible();
    await page.screenshot({ path: path.join(screenshotDir, "prompt-trace-tool-surface.png"), fullPage: true });
  });
});
