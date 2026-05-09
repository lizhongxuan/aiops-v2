// @ts-check
import { test, expect } from "@playwright/test";
import {
  createChatFixtureSessions,
  createChatFixtureState,
  openFixturePage,
} from "../helpers/uiFixtureHarness";
import { expectStableLocatorScreenshot, expectStablePageScreenshot } from "../helpers/visualSnapshot";

function createHostsFixture() {
  const lastHeartbeat = new Date().toISOString();
  const staleHeartbeat = new Date(Date.now() - 120_000).toISOString();
  return {
    state: createChatFixtureState({
      selectedHostId: "server-local",
      hosts: [
        {
          id: "server-local",
          name: "server-local",
          status: "online",
          transport: "local",
          executable: true,
          terminalCapable: true,
        },
        {
          id: "host-online",
          name: "web-online",
          address: "10.0.2.15",
          sshUser: "root",
          sshPort: 22,
          status: "online",
          kind: "agent",
          transport: "grpc_reverse",
          agentVersion: "v0.8.1",
          lastHeartbeat,
          executable: true,
          terminalCapable: true,
        },
        {
          id: "host-installing",
          name: "manual-install",
          address: "192.168.1.42",
          sshUser: "deploy",
          sshPort: 22,
          status: "installing",
          transport: "ssh_bootstrap",
          installState: "pending_install",
        },
        {
          id: "host-offline",
          name: "offline-client",
          address: "172.16.8.9",
          sshUser: "ubuntu",
          status: "offline",
          transport: "grpc_reverse",
          agentVersion: "v0.7.4",
          lastHeartbeat: staleHeartbeat,
        },
      ],
      runtime: {
        turn: { active: false, phase: "idle", hostId: "server-local" },
        codex: { status: "connected", retryAttempt: 0, retryMax: 5 },
        activity: {},
      },
      cards: [],
      agentEventProjection: null,
      config: { codexAlive: true, model: "gpt-5.4" },
    }),
    sessions: createChatFixtureSessions({
      activeSessionId: "single-1",
      sessions: [
        { id: "single-1", kind: "single_host", title: "host online", selectedHostId: "host-online", status: "running" },
        { id: "single-2", kind: "single_host", title: "host online followup", selectedHostId: "host-online", status: "completed" },
      ],
    }),
  };
}

async function mockTerminalSessions(page) {
  await page.route("**/api/v1/terminal/sessions", (route) => {
    if (route.request().method() !== "GET") {
      return route.continue();
    }
    return route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        sessions: [
          { sessionId: "term-1", hostId: "host-online", status: "running", shell: "/bin/zsh", cwd: "~" },
        ],
      }),
    });
  });
}

test.describe("Hosts management redesign snapshot", () => {
  test("renders the simplified host inventory on desktop and mobile", async ({ page }) => {
    await mockTerminalSessions(page);

    await page.setViewportSize({ width: 1440, height: 900 });
    await openFixturePage(page, "/settings/hosts", createHostsFixture());

    await expect(page.getByRole("heading", { name: "主机管理" })).toBeVisible();
    await expect(page.locator(".hosts-page-heading")).toHaveCount(0);
    await expect(page.locator(".header-host-button")).toContainText("server-local");
    await expect(page.getByRole("button", { name: /终端/ })).toBeVisible();
    await expect(page.getByRole("button", { name: /清空上下文/ })).toHaveCount(0);
    await expect(page.locator(".header-right")).not.toContainText("清空上下文");
    await expect(page.locator(".hosts-table-shell")).toContainText("10.0.2.15 / root");
    await expect(page.locator(".hosts-table-shell")).not.toContainText("server-local");

    await expectStablePageScreenshot(page, "hosts-management-redesign.png");

    await page.setViewportSize({ width: 390, height: 820 });
    await openFixturePage(page, "/settings/hosts", createHostsFixture());
    await expect(page.getByRole("heading", { name: "主机管理" })).toBeVisible();
    await expectStableLocatorScreenshot(page.locator(".hosts-table-shell"), "hosts-management-redesign-mobile-table.png");
  });
});
