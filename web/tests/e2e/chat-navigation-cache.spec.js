// @ts-check
import { test, expect } from "@playwright/test";
import {
  createChatFixtureSessions,
  createChatFixtureState,
  openFixturePage,
} from "../helpers/uiFixtureHarness";

function createCachedChatFixture() {
  const state = createChatFixtureState({
    sessionId: "nav-cache-chat",
    threadId: "nav-cache-chat",
    cards: [
      {
        id: "user-nav-cache",
        type: "UserMessageCard",
        role: "user",
        text: "缓存中的历史问题：检查 api-gateway 的 5xx。",
        createdAt: "2026-07-08T10:00:00Z",
        updatedAt: "2026-07-08T10:00:00Z",
      },
      {
        id: "assistant-nav-cache",
        type: "AssistantMessageCard",
        text: "缓存中的历史回答：先看错误率，再核对最近一次发布。",
        createdAt: "2026-07-08T10:00:03Z",
        updatedAt: "2026-07-08T10:00:03Z",
      },
    ],
    runtime: {
      turn: { active: true, phase: "thinking", hostId: "web-01" },
      codex: { status: "connected", retryAttempt: 0, retryMax: 5 },
      activity: {
        viewedFiles: [],
        searchedWebQueries: [],
        searchedContentQueries: [],
      },
    },
    finalText: "缓存中的历史回答：先看错误率，再核对最近一次发布。",
    hosts: [
      {
        id: "web-01",
        name: "web-01",
        address: "10.20.30.40",
        sshUser: "root",
        sshPort: 22,
        status: "online",
        transport: "grpc_reverse",
        executable: true,
        terminalCapable: true,
      },
    ],
  });

  return {
    state,
    sessions: createChatFixtureSessions({
      activeSessionId: "nav-cache-chat",
      sessions: [
        {
          id: "nav-cache-chat",
          kind: "single_host",
          title: "导航缓存会话",
          status: "running",
          messageCount: 2,
          preview: "缓存中的历史问题：检查 api-gateway 的 5xx。",
          selectedHostId: "web-01",
          lastActivityAt: "2026-07-08T10:00:03Z",
        },
      ],
    }),
  };
}

async function mockNavigationApis(page, fixture) {
  await page.route("**/api/v1/hosts", (route) => {
    if (route.request().method() !== "GET") {
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ ok: true }),
      });
    }
    return route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ items: fixture.state.hosts }),
    });
  });
  await page.route("**/api/v1/host-profiles**", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ items: [] }),
    }),
  );
  await page.route("**/api/v1/host-leases**", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ items: [] }),
    }),
  );
  await page.route("**/api/v1/terminal/sessions", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ sessions: [] }),
    }),
  );
  await page.route("**/api/v1/llm-config", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        provider: "zai",
        model: "glm-5.1",
        configured: true,
      }),
    }),
  );
}

test.describe("chat navigation cache", () => {
  test("keeps cached transcript visible when returning from another page", async ({ page }) => {
    const fixture = createCachedChatFixture();
    let resumeRequests = 0;
    await mockNavigationApis(page, fixture);
    await page.route("**/api/v1/assistant/resume", async (route) => {
      resumeRequests += 1;
      await route.fulfill({
        status: 503,
        contentType: "text/plain",
        body: "resume unavailable during cache navigation test",
      });
    });

    await openFixturePage(page, "/", fixture);
    await expect(page.getByText("缓存中的历史问题：检查 api-gateway 的 5xx。")).toBeVisible();
    await expect(page.getByText("缓存中的历史回答：先看错误率，再核对最近一次发布。")).toBeVisible();

    await page.getByRole("link", { name: /主机列表/ }).click();
    await expect(page.locator(".hosts-table-shell")).toContainText("10.20.30.40 / root");

    await page.evaluate(() => {
      delete window.__CODEX_UI_FIXTURE__;
    });
    await page.getByRole("link", { name: /AI 对话/ }).click();

    await expect(page.getByText("缓存中的历史问题：检查 api-gateway 的 5xx。")).toBeVisible();
    await expect(page.getByText("缓存中的历史回答：先看错误率，再核对最近一次发布。")).toBeVisible();
    await expect(page.getByTestId("chat-session-restore-placeholder")).toHaveCount(0);
    await expect.poll(() => resumeRequests, { timeout: 5_000 }).toBeGreaterThan(0);
  });
});
