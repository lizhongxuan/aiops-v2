// @ts-check

import {
  createChatFixtureSessions,
  createChatFixtureState,
  createProtocolFixtureSessions,
  createProtocolFixtureState,
  resolveUiFixturePreset,
} from "../../src/lib/uiFixturePresets";

export {
  createChatFixtureSessions,
  createChatFixtureState,
  createProtocolFixtureSessions,
  createProtocolFixtureState,
};

export const FIXTURE_BASE_URL = process.env.PLAYWRIGHT_FIXTURE_BASE_URL || "http://127.0.0.1:4173";

function normalizeUiFixtureInput(fixture) {
  if (typeof fixture === "string") {
    return resolveUiFixturePreset(fixture);
  }
  if (fixture && typeof fixture === "object") {
    return fixture;
  }
  return null;
}

export async function installUiFixture(page, fixture) {
  const resolved = normalizeUiFixtureInput(fixture);
  await page.addInitScript((payload) => {
    window.__CODEX_UI_FIXTURE__ = payload;
  }, resolved);
  const { state, sessions } = resolved || {};
  await page.context().addInitScript(() => {
    class FakeWebSocket {
      static CONNECTING = 0;
      static OPEN = 1;
      static CLOSING = 2;
      static CLOSED = 3;

      constructor(url) {
        this.url = url;
        this.readyState = FakeWebSocket.CONNECTING;
        setTimeout(() => {
          this.readyState = FakeWebSocket.OPEN;
          this.onopen?.({ type: "open" });
        }, 0);
      }

      send() {}

      close() {
        this.readyState = FakeWebSocket.CLOSED;
        this.onclose?.({ type: "close" });
      }

      addEventListener(type, handler) {
        this[`on${type}`] = handler;
      }

      removeEventListener(type, handler) {
        if (this[`on${type}`] === handler) {
          this[`on${type}`] = null;
        }
      }
    }

    window.WebSocket = FakeWebSocket;
  });

  await page.route("**/api/v1/state", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(state),
    }),
  );
  await page.route("**/api/v1/sessions", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(sessions),
    }),
  );
  await page.route("**/api/v1/chat/message", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ ok: true, snapshot: state }),
    }),
  );
  await page.route("**/api/v1/chat/stop", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ accepted: true }),
    }),
  );
  await page.route("**/api/v1/approvals/*/decision", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ accepted: true }),
    }),
  );
}

export async function openFixturePage(page, routePath, fixture) {
  await installUiFixture(page, fixture);
  await page.goto(`${FIXTURE_BASE_URL}${routePath}`, { waitUntil: "networkidle" });
  await waitForFixtureStable(page);
}

export async function openBrowserFixturePage(page, routePath, fixture) {
  const resolved = normalizeUiFixtureInput(fixture);
  await page.addInitScript((payload) => {
    window.__CODEX_UI_FIXTURE__ = payload;
  }, resolved);
  const fixtureKey = resolved?.name || (typeof fixture === "string" ? fixture : "");
  const hasQuery = routePath.includes("?");
  const fixtureQuery = fixtureKey ? `${hasQuery ? "&" : "?"}fixture=${encodeURIComponent(fixtureKey)}` : "";
  const url = new URL(`${routePath}${fixtureQuery}`, FIXTURE_BASE_URL).toString();
  await page.goto(url, { waitUntil: "networkidle" });
  await waitForFixtureStable(page);
}

export async function waitForFixtureStable(page, timeout = 8000) {
  await page.waitForLoadState("networkidle", { timeout }).catch(() => {});
  await page.waitForTimeout(400);
}
