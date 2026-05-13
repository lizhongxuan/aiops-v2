import { afterEach, describe, expect, it, vi } from "vitest";

function createMockPage() {
  return {
    addInitScript: vi.fn().mockResolvedValue(undefined),
    goto: vi.fn().mockResolvedValue(undefined),
    waitForLoadState: vi.fn().mockResolvedValue(undefined),
    waitForTimeout: vi.fn().mockResolvedValue(undefined),
  };
}

describe("uiFixtureHarness", () => {
  const originalFixtureBaseUrl = process.env.PLAYWRIGHT_FIXTURE_BASE_URL;

  afterEach(() => {
    vi.resetModules();
    if (originalFixtureBaseUrl === undefined) {
      delete process.env.PLAYWRIGHT_FIXTURE_BASE_URL;
    } else {
      process.env.PLAYWRIGHT_FIXTURE_BASE_URL = originalFixtureBaseUrl;
    }
  });

  it("builds absolute browser fixture urls from PLAYWRIGHT_FIXTURE_BASE_URL", async () => {
    process.env.PLAYWRIGHT_FIXTURE_BASE_URL = "http://127.0.0.1:4173";
    const { openBrowserFixturePage } = await import("./helpers/uiFixtureHarness.js");
    const page = createMockPage();

    await openBrowserFixturePage(page, "/protocol?promptDebug=1", "protocol");

    expect(page.goto).toHaveBeenCalledWith(
      "http://127.0.0.1:4173/protocol?promptDebug=1&fixture=protocol",
      { waitUntil: "networkidle" },
    );
    expect(page.waitForLoadState).toHaveBeenCalledWith("networkidle", { timeout: 8000 });
    expect(page.waitForTimeout).toHaveBeenCalledWith(400);
  });

  it("injects the resolved preset payload before navigating", async () => {
    process.env.PLAYWRIGHT_FIXTURE_BASE_URL = "http://127.0.0.1:4173";
    const { openBrowserFixturePage } = await import("./helpers/uiFixtureHarness.js");
    const page = createMockPage();

    await openBrowserFixturePage(page, "/", "chat");

    expect(page.addInitScript).toHaveBeenCalledTimes(1);
    const injectedPayload = page.addInitScript.mock.calls[0][1];
    expect(injectedPayload).toMatchObject({
      name: "chat",
      state: { kind: "single_host", sessionId: "single-1" },
      sessions: { activeSessionId: "single-1" },
    });
    expect(page.goto).toHaveBeenCalledWith(
      "http://127.0.0.1:4173/?fixture=chat",
      { waitUntil: "networkidle" },
    );
  });
});
