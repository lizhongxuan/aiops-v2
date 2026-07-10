// @ts-check
import { expect } from "@playwright/test";
import { installUiFixture } from "../helpers/uiFixtureHarness";

export const corootProjectId = "5hxbfx6p";

export async function installAiopsFixture(page) {
  await installUiFixture(page, "chat");
}

export async function mockCorootIframe(page) {
  await page.route("**/_coroot/**", (route) =>
    route.fulfill({
      status: 200,
      contentType: "text/html",
      body: `<!doctype html>
<html>
  <head><title>Coroot</title></head>
  <body>
    <main>
      <h1>Applications</h1>
      <button type="button">last hour</button>
      <table>
        <thead><tr><th>Application</th><th>Instances</th><th>Logs</th></tr></thead>
        <tbody><tr><td>aiops-host-agent</td><td>2/3</td><td>1 unique error</td></tr></tbody>
      </table>
    </main>
  </body>
</html>`,
    }),
  );
}

export async function mockConfiguredCoroot(page) {
  await page.route("**/api/v1/coroot/config", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        configured: true,
        baseUrl: "http://coroot.local/coroot",
        project: corootProjectId,
        productBasePath: "/coroot/",
        gatewayBasePath: "/_coroot/",
        entryPath: `/coroot/p/${corootProjectId}/applications`,
        iframeEntryPath: `/_coroot/p/${corootProjectId}/applications?embed=1`,
        authMode: "anonymous_readonly",
        embedMode: "readonly",
        uiGatewayEnabled: true,
      }),
    }),
  );
  await mockCorootIframe(page);
}

export async function mockUnconfiguredCoroot(page, handlers = {}) {
  let savedConfig = null;
  await page.route("**/api/v1/coroot/config", async (route) => {
    if (route.request().method() === "GET") {
      if (savedConfig) {
        await route.fulfill({ json: savedConfig });
        return;
      }
      await route.fulfill({ json: { configured: false, iframeMode: false, tokenConfigured: false } });
      return;
    }
    const payload = route.request().postDataJSON();
    handlers.onSave?.(payload);
    savedConfig = {
      configured: true,
      baseUrl: payload.baseUrl,
      project: payload.project || corootProjectId,
      entryPath: `/coroot/p/${payload.project || corootProjectId}/applications`,
      iframeEntryPath: `/_coroot/p/${payload.project || corootProjectId}/applications?embed=1`,
      authMode: payload.authMode,
      embedMode: payload.embedMode,
      uiGatewayEnabled: true,
    };
    await route.fulfill({ json: savedConfig });
  });
  await page.route("**/api/v1/coroot/test-connection", async (route) => {
    const payload = route.request().postDataJSON();
    handlers.onTest?.(payload);
    await route.fulfill({ json: { ok: true, message: "Coroot 网关已响应。", project: payload.project || corootProjectId } });
  });
  await mockCorootIframe(page);
}

export async function expectCorootWorkspace(page) {
  await expect(page.getByRole("button", { name: "返回 AIOps" })).toBeVisible();
  await expect(page.locator('[data-testid="app-shell-sidebar"]')).toContainText("Applications");
  await expect(page.locator('[data-testid="app-shell-header"]')).toHaveCount(0);
  await expect(page.locator('iframe[title="Coroot"]')).toHaveAttribute("src", /\/_coroot\/p\/5hxbfx6p\/applications\?embed=1/);
  await expect(page.frameLocator('iframe[title="Coroot"]').getByRole("heading", { name: "Applications" })).toBeVisible();
}
