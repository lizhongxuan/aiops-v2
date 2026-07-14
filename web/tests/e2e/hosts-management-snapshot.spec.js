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
          osRelease: "Ubuntu 24.04 LTS",
          arch: "amd64",
          kernelVersion: "6.8.0-31-generic",
          cpuCores: 8,
          memoryBytes: 34359738368,
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
          installRunId: "run-install-001",
          installWorkflowId: "builtin.host-agent-install/v1",
          installStep: "probe_platform",
          sshCredentialRef: "secret://ops/manual-install-key",
          agentVersion: "v0.1.0",
        },
        {
          id: "host-failed",
          name: "failed-install",
          address: "10.0.9.10",
          sshUser: "deploy",
          sshPort: 22,
          status: "install_failed",
          transport: "ssh_bootstrap",
          installState: "failed",
          installRunId: "run-install-002",
          installWorkflowId: "builtin.host-agent-install/v1",
          installStep: "verify_heartbeat",
          lastError: "heartbeat timeout",
          sshCredentialRef: "secret://ops/failed-install-key",
          agentVersion: "v0.1.0",
        },
        {
          id: "host-unsupported",
          name: "unsupported-install",
          address: "10.0.9.11",
          sshUser: "admin",
          sshPort: 22,
          status: "install_failed",
          transport: "ssh_bootstrap",
          installState: "unsupported_platform",
          lastError: "freebsd/amd64 is not supported",
          sshCredentialRef: "secret://ops/unsupported-install-key",
          agentVersion: "v0.1.0",
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

async function mockHostsPageApis(page, fixture) {
  await page.route("**/api/v1/hosts", (route) => {
    if (route.request().method() !== "GET") {
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ ok: true }) });
    }
    return route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ items: fixture.state.hosts }),
    });
  });
  await page.route("**/api/v1/hosts/*/install", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ ok: true, installRunId: "run-retry-001" }) }),
  );
  await page.route("**/api/v1/host-profiles**", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ items: [] }) }),
  );
  await page.route("**/api/v1/host-leases**", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ items: [] }) }),
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

test.describe("Hosts management redesign snapshot", () => {
  test("renders the simplified host inventory on desktop and mobile", async ({ page }) => {
    const fixture = createHostsFixture();
    await mockHostsPageApis(page, fixture);
    await mockTerminalSessions(page);

    await page.setViewportSize({ width: 1440, height: 900 });
    await openFixturePage(page, "/settings/hosts", fixture);

    await expect(page.getByText("主机列表").first()).toBeVisible();
    await expect(page.locator(".hosts-page-heading")).toHaveCount(0);
    await expect(page.getByRole("button", { name: "终端" }).first()).toBeVisible();
    await expect(page.getByRole("button", { name: /清空上下文/ })).toHaveCount(0);
    await expect(page.locator(".hosts-table-shell")).toContainText("10.0.2.15 / root");
    await expect(page.locator(".hosts-table-shell")).toContainText("Ubuntu 24.04 LTS");
    await expect(page.locator(".hosts-table-shell")).toContainText("6.8.0-31-generic");
    await expect(page.locator(".hosts-table-shell")).toContainText("8 核 / 32 GiB");
    await expect(page.locator(".hosts-table-shell")).toContainText("不支持的平台");
    await expect(page.locator(".hosts-table-shell")).not.toContainText("server-local");

    await expectStablePageScreenshot(page, "hosts-management-redesign.png");

    await page.getByRole("button", { name: /查看 10\.0\.9\.10 .*错误详情/ }).click();
    await expect(page.getByTestId("host-error-dialog")).toContainText("heartbeat timeout");
    await page.getByRole("button", { name: "关闭" }).click();

    await page.setViewportSize({ width: 390, height: 820 });
    await openFixturePage(page, "/settings/hosts", fixture);
    await expectStableLocatorScreenshot(page.locator(".hosts-table-shell"), "hosts-management-redesign-mobile-table.png");
  });

  test("keeps cached hosts visible while refreshing after route navigation", async ({ page }) => {
    test.setTimeout(45_000);
    const fixture = createHostsFixture();
    let hostsRequestCount = 0;
    await page.route("**/api/v1/hosts", async (route) => {
      if (route.request().method() !== "GET") {
        return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ ok: true }) });
      }
      hostsRequestCount += 1;
      if (hostsRequestCount >= 2) {
        await new Promise((resolve) => setTimeout(resolve, 2_000));
      }
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ items: fixture.state.hosts }),
      });
    });
    await page.route("**/api/v1/host-profiles**", (route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ items: [] }) }),
    );
    await page.route("**/api/v1/host-leases**", (route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ items: [] }) }),
    );
    await page.route("**/api/v1/llm-config", (route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ provider: "zai", model: "glm-5.1", configured: true }) }),
    );
    await mockTerminalSessions(page);

    await page.setViewportSize({ width: 1440, height: 900 });
    await openFixturePage(page, "/settings/hosts", fixture);
    await expect(page.locator(".hosts-table-shell")).toContainText("10.0.2.15 / root");

    await page.getByRole("link", { name: /AI 对话/ }).click();
    await expect(page.getByRole("textbox", { name: "输入你的问题或任务" })).toBeVisible();
    await page.evaluate(() => {
      const nextNow = Date.now() + 31_000;
      Date.now = () => nextNow;
    });

    await page.getByRole("link", { name: /主机列表/ }).click();

    await expect(page.locator(".hosts-table-shell")).toContainText("10.0.2.15 / root");
    await expect(page.locator(".hosts-table-shell")).toContainText("Ubuntu 24.04 LTS");
    await expect(page.getByTestId("hosts-background-refresh")).toBeVisible();
    await expect(page.getByText("加载主机列表")).toHaveCount(0);
  });
});
