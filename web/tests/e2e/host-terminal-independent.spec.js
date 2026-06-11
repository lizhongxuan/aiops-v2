import { expect, test } from "@playwright/test";

import { createChatFixtureSessions, createChatFixtureState, installUiFixture, waitForFixtureStable } from "../helpers/uiFixtureHarness";
import { resolveUiFixturePreset } from "../../src/lib/uiFixturePresets";

test("opens independent host terminal from host list without HostOps state", async ({ page }) => {
  const hosts = [
    {
      id: "host-a",
      name: "host-a",
      address: "1.1.1.1",
      sshUser: "root",
      sshPort: 22,
      status: "online",
      installState: "installed",
      controlMode: "managed",
      executable: true,
      terminalCapable: true,
      labels: { role: "worker" },
    },
  ];
  const hostOpsFixture = resolveUiFixturePreset("host-ops-three-hosts");
  const fixture = {
    name: "host-terminal-independent",
    state: {
      ...hostOpsFixture.state,
      ...createChatFixtureState({
        sessionId: "terminal-independent",
        threadId: "terminal-independent",
        status: "idle",
        cards: hostOpsFixture.state.cards || [],
      }),
      hostMissions: hostOpsFixture.state.hostMissions,
      childAgents: hostOpsFixture.state.childAgents,
      activeHostMissionId: hostOpsFixture.state.activeHostMissionId,
      hostOpsTranscripts: hostOpsFixture.state.hostOpsTranscripts,
      hosts,
    },
    sessions: createChatFixtureSessions({ activeSessionId: "terminal-independent", sessions: [] }),
  };
  const hostOpsWrites = [];

  await installUiFixture(page, fixture);
  await installTerminalWebSocketMock(page);
  await page.route("**/api/v1/hosts", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ items: hosts }) }),
  );
  await page.route("**/api/v1/terminal/sessions", async (route) => {
    if (route.request().method() === "POST") {
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ sessionId: "term-host-a", hostId: "host-a", source: "manual_terminal", status: "running", shell: "/bin/bash", cwd: "~" }),
      });
    }
    return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ sessions: [] }) });
  });
  await page.route("**/api/v1/host-profiles**", (route) => route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ items: [] }) }));
  await page.route("**/api/v1/host-leases**", (route) => route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ items: [] }) }));
  await page.route("**/api/v1/hosts/*/reports**", (route) => route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ items: [] }) }));
  await page.route("**/api/v1/host-ops/**", (route) => {
    if (route.request().method() !== "GET") {
      hostOpsWrites.push({ method: route.request().method(), url: route.request().url() });
    }
    return route.fallback();
  });

  await page.goto("/settings/hosts", { waitUntil: "networkidle" });
  await waitForFixtureStable(page);
  await page.getByRole("button", { name: "接入配置" }).click();
  const hostRow = page.locator("tr", { hasText: "1.1.1.1" });
  await expect(hostRow).toContainText("在线");
  await hostRow.getByRole("button", { name: "终端" }).click();

  await expect(page).toHaveURL(/\/terminal\/host-a$/);
  await expect(page.getByText("Terminal · host-a")).toBeVisible();
  await expect(page.getByText("gRPC 主机客户端").first()).toBeVisible();
  await expect(page.getByTestId("terminal-xterm")).toBeVisible();
  await expect(page.getByText("HostOps Mission")).toHaveCount(0);
  await expect(page.getByText("主机 Agent 详情")).toHaveCount(0);
  await expect(page.getByText("共 3 个主机 Agent")).toHaveCount(0);

  await page.goto("/", { waitUntil: "networkidle" });
  await waitForFixtureStable(page);
  const panel = page.getByTestId("host-ops-status-panel");
  await expect(panel).toBeVisible();
  await expect(panel).toContainText("共 5 个步骤，已经完成 0 个");
  await expect(panel).toContainText("共 3 个主机 Agent");
  expect(hostOpsWrites).toEqual([]);
});

async function installTerminalWebSocketMock(page) {
  await page.addInitScript(() => {
    class TerminalFakeWebSocket {
      static CONNECTING = 0;
      static OPEN = 1;
      static CLOSED = 3;

      constructor(url) {
        this.url = url;
        this.readyState = TerminalFakeWebSocket.CONNECTING;
        this.sent = [];
        setTimeout(() => {
          this.readyState = TerminalFakeWebSocket.OPEN;
          this.onopen?.({ type: "open" });
          this.onmessage?.({ data: JSON.stringify({ type: "ready", status: "ready" }) });
          this.onmessage?.({ data: JSON.stringify({ type: "output", data: "terminal-output-ok\\r\\n" }) });
        }, 0);
      }

      send(data) {
        this.sent.push(data);
        this.onmessage?.({ data: JSON.stringify({ type: "output", data: String(data) }) });
      }

      close() {
        this.readyState = TerminalFakeWebSocket.CLOSED;
        this.onclose?.({ type: "close" });
      }

      addEventListener(type, handler) {
        this[`on${type}`] = handler;
      }

      removeEventListener(type, handler) {
        if (this[`on${type}`] === handler) this[`on${type}`] = null;
      }
    }

    window.WebSocket = TerminalFakeWebSocket;
  });
}
