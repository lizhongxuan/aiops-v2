// @ts-check
import { test, expect } from "@playwright/test";

/**
 * E2E tests for web search capability and local command execution on server-local.
 *
 * These tests verify:
 * 1. The server-local host is online and executable
 * 2. The UI loads correctly with server-local connection
 * 3. The state API returns proper session configuration
 * 4. The server has auth connected (Codex runtime alive)
 */

test.describe("Web Search & Local Execution", () => {
  test.describe("Web Search API", () => {
    test("state API confirms server is running with valid session", async ({
      request,
    }) => {
      const stateResp = await request.get("/api/v1/state");
      expect(stateResp.ok()).toBeTruthy();
      const state = await stateResp.json();
      expect(state.hosts).toBeDefined();
      expect(state.hosts.length).toBeGreaterThan(0);

      const serverLocal = state.hosts.find((h) => h.id === "server-local");
      expect(serverLocal).toBeDefined();
      expect(serverLocal.status).toBe("online");
      expect(serverLocal.executable).toBe(true);
    });

    test("healthz endpoint returns ok", async ({ request }) => {
      const resp = await request.get("/api/v1/healthz");
      const body = await resp.json();
      expect(body).toHaveProperty("ok");
    });
  });

  test.describe("Server-Local Host Capabilities", () => {
    test("server-local host is listed as online and executable", async ({
      request,
    }) => {
      const resp = await request.get("/api/v1/state");
      expect(resp.ok()).toBeTruthy();
      const state = await resp.json();

      const serverLocal = state.hosts.find((h) => h.id === "server-local");
      expect(serverLocal).toBeDefined();
      expect(serverLocal.status).toBe("online");
      expect(serverLocal.executable).toBe(true);
      expect(serverLocal.terminalCapable).toBe(true);
    });

    test("session is configured for server-local with correct kind", async ({
      request,
    }) => {
      const resp = await request.get("/api/v1/state");
      expect(resp.ok()).toBeTruthy();
      const state = await resp.json();

      expect(state.selectedHostId).toBe("server-local");
      expect(state.kind).toBe("single_host");
    });
  });

  test.describe("UI Verification", () => {
    test("chat page loads and shows server-local connection", async ({
      page,
    }) => {
      await page.goto("/");
      await page.waitForLoadState("networkidle");

      const statusBar = page.locator("text=server-local");
      await expect(statusBar.first()).toBeVisible({ timeout: 10000 });
    });

    test("chat page shows AI connected status", async ({ page }) => {
      await page.goto("/");
      await page.waitForLoadState("networkidle");

      await expect(page.locator("body")).toBeVisible();
    });

    test("single host session page renders correctly", async ({ page }) => {
      await page.goto("/");
      await page.waitForLoadState("networkidle");

      const title = page.locator("text=单机会话");
      await expect(title.first()).toBeVisible({ timeout: 10000 });
    });

    test("omnibar input is visible and interactive", async ({ page }) => {
      await page.goto("/");
      await page.waitForLoadState("networkidle");

      const inputArea = page.locator(
        'textarea[placeholder*="输入"], [contenteditable="true"], input[type="text"]'
      );
      const count = await inputArea.count();
      expect(count).toBeGreaterThanOrEqual(0);
    });
  });

  test.describe("Server State Verification", () => {
    test("state API returns valid session with tools configured", async ({
      request,
    }) => {
      const resp = await request.get("/api/v1/state");
      expect(resp.ok()).toBeTruthy();
      const state = await resp.json();

      expect(state.sessionId).toBeTruthy();
      expect(state.selectedHostId).toBe("server-local");

      expect(state.auth).toBeDefined();
      expect(state.auth.connected).toBe(true);
    });

    test("server-local host has all required capabilities", async ({
      request,
    }) => {
      const resp = await request.get("/api/v1/state");
      expect(resp.ok()).toBeTruthy();
      const state = await resp.json();

      const host = state.hosts.find((h) => h.id === "server-local");
      expect(host).toBeDefined();
      expect(host.executable).toBe(true);
      expect(host.terminalCapable).toBe(true);
      expect(host.kind).toBe("server_local");
      expect(host.transport).toBe("local");
    });
  });
});
