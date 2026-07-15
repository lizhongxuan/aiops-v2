// @ts-check
import { expect, test } from "@playwright/test";

import {
  createChatFixtureSessions,
  createChatFixtureState,
  installUiFixture,
  waitForFixtureStable,
} from "../helpers/uiFixtureHarness.js";

test("filters @host suggestions by name/ip and sends selected mention metadata", async ({ page }) => {
  const transportRequests = [];
  await mockHostInventory(page);
  await installAssistantTransportRoute(page, transportRequests);
  await installUiFixture(page, createIdleFixture());

  await page.goto("/", { waitUntil: "networkidle" });
  await waitForFixtureStable(page);

  const input = page.getByTestId("omnibar-input");
  await expect(input).toBeVisible();
  await input.fill("@");
  await expect(page.getByTestId("host-mention-suggestion-popover")).toBeVisible();
  await expect(page.getByTestId("host-mention-suggestion-item")).toHaveCount(4);
  await page.getByTestId("host-mention-suggestion-item").first().click();
  await expect(input).toHaveValue("@host-");

  await input.fill("@host-pg");
  await expect(page.getByTestId("host-mention-suggestion-item")).toHaveCount(2);
  await expect(page.getByTestId("host-mention-suggestion-popover")).toContainText("pg-primary");
  await expect(page.getByTestId("host-mention-suggestion-popover")).toContainText("120.77.239.90");
  await expect(page.getByTestId("host-mention-suggestion-popover")).not.toContainText("redis");

  await page.getByTestId("host-mention-suggestion-item").first().click();
  await expect(input).toHaveValue("@120.77.239.90 ");

  await input.fill("@120.77.239.90 检查 PostgreSQL 状态");
  await page.getByTestId("omnibar-primary-action").click();

  await expect.poll(() => transportRequests.length, { timeout: 5000 }).toBe(1);
  const command = transportRequests[0]?.commands?.[0];
  expect(command?.type).toBe("add-message");
  const mentions = JSON.parse(command?.message?.metadata?.["aiops.hostops.mentions"] || "[]");
  expect(mentions.map((item) => item.raw)).toEqual(["@120.77.239.90"]);
});

test("highlights special AI mentions and sends explicit tool metadata without host binding", async ({ page }) => {
  const transportRequests = [];
  await mockHostInventory(page);
  await installAssistantTransportRoute(page, transportRequests);
  await installUiFixture(page, createIdleFixture());

  await page.goto("/", { waitUntil: "networkidle" });
  await waitForFixtureStable(page);

  const input = page.getByTestId("omnibar-input");
  await expect(input).toBeVisible();
  await input.fill("@coroot @ops_graph @ops_manus 分析 checkout 根因");

  await expect(page.getByTestId("composer-inline-special-mention")).toHaveCount(3);
  await expect(page.getByTestId("composer-inline-host-mention")).toHaveCount(0);
  await page.getByTestId("omnibar-primary-action").click();

  await expect.poll(() => transportRequests.length, { timeout: 5000 }).toBe(1);
  const command = transportRequests[0]?.commands?.[0];
  expect(command?.type).toBe("add-message");
  expect(command?.message?.hostId).toBeUndefined();
  expect(command?.message?.metadata?.["aiops.hostops.mentions"]).toBeUndefined();
  expect(command?.message?.metadata).toMatchObject({
    "aiops.coroot.explicitRCA": "true",
    "aiops.opsGraph.explicitMention": "true",
    "aiops.opsManuals.explicitMention": "true",
    enableTool: "search_ops_manuals",
    enableToolPack: "opsgraph,ops_manual_flow",
  });
});

test("recognizes Chinese-adjacent @Coroot and preserves special metadata on Enter submit", async ({ page }) => {
  const transportRequests = [];
  await mockHostInventory(page);
  await installAssistantTransportRoute(page, transportRequests);
  await installUiFixture(page, createIdleFixture());

  await page.goto("/", { waitUntil: "networkidle" });
  await waitForFixtureStable(page);

  const input = page.getByTestId("omnibar-input");
  await expect(input).toBeVisible();
  await input.fill("帮我看下@Coroot 情况");
  await expect(page.getByTestId("composer-inline-special-mention")).toHaveText("Coroot");

  await input.press("Enter");

  await expect.poll(() => transportRequests.length, { timeout: 5000 }).toBe(1);
  expect(transportRequests[0]?.commands).toHaveLength(1);
  const command = transportRequests[0]?.commands?.[0];
  expect(command?.type).toBe("add-message");
  expect(command?.message?.parts).toEqual([{ type: "text", text: "帮我看下@Coroot 情况" }]);
  expect(command?.message?.metadata).toMatchObject({
    "aiops.coroot.explicitRCA": "true",
    "aiops.coroot.rcaDisplayAllowed": "true",
  });
  expect(JSON.parse(command?.message?.metadata?.["aiops.input.mentions.v1"] || "{}")).toEqual({
    version: 1,
    mentions: [
      expect.objectContaining({
        kind: "capability",
        path: "capability://coroot",
        rawText: "@Coroot",
        source: "selection",
        range: { start: 4, end: 11 },
      }),
    ],
  });
});

async function mockHostInventory(page) {
  const hosts = [
    { id: "host-a", name: "pg-primary", ip: "120.77.239.90", status: "online", hostname: "ignored-hostname", labels: { role: "ignored" } },
    { id: "host-b", name: "pg-standby", address: "120.77.239.91", status: "online" },
    { id: "host-c", name: "redis", ip: "10.0.0.8", status: "online", labels: { role: "pg" } },
    ...Array.from({ length: 10 }, (_, index) => ({ id: `host-extra-${index}`, name: `node-${index}`, ip: `10.1.0.${index}`, status: "online" })),
  ];
  await page.route("**/api/v1/hosts", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ items: hosts }) }),
  );
}

async function installAssistantTransportRoute(page, requests) {
  await page.route("**/api/v1/assistant/transport", async (route) => {
    requests.push(route.request().postDataJSON());
    return route.fulfill({ status: 200, contentType: "text/plain; charset=utf-8", body: "aui-state:[]\n" });
  });
}

function createIdleFixture() {
  const state = createChatFixtureState({
    sessionId: "host-mention-fuzzy",
    threadId: "host-mention-fuzzy",
    status: "idle",
    cards: [],
    runtime: {
      turn: { active: false, phase: "completed", hostId: "workspace" },
      codex: { status: "connected", retryAttempt: 0, retryMax: 5 },
      activity: { viewedFiles: [], searchedWebQueries: [], searchedContentQueries: [] },
    },
    finalText: "",
  });
  state.status = "idle";
  state.runtimeLiveness = { activeTurns: {}, activeAgents: {}, pendingApprovals: {}, pendingUserInputs: {}, activeCommandStreams: {} };
  state.selectedHostId = "workspace";
  return {
    name: "host-mention-fuzzy",
    state,
    sessions: createChatFixtureSessions({
      activeSessionId: "host-mention-fuzzy",
      sessions: [
        {
          id: "host-mention-fuzzy",
          kind: "single_host",
          title: "Host mention fuzzy",
          status: "idle",
          messageCount: 0,
          selectedHostId: "workspace",
        },
      ],
    }),
  };
}
