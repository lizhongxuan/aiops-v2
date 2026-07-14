// @ts-check
import { expect, test } from "@playwright/test";

import { openFixturePage } from "../helpers/uiFixtureHarness.js";

test("renders special input short-term memory context and actions", async ({ page }) => {
  await page.setViewportSize({ width: 795, height: 863 });
  const transportRequests = [];
  await mockHostInventory(page);
  await installAssistantTransportRoute(page, transportRequests);
  await openFixturePage(page, "/", "special-input-memory");

  const contextBar = page.getByTestId("special-input-context-bar");
  await expect(contextBar).toBeVisible();
  await expect(contextBar).toContainText("host-a");
  await expect(contextBar).toContainText("prod / pg-orders");
  await expect(contextBar).toContainText("pg_primary -> host-a");
  await expect(contextBar).toContainText("pg_standby -> host-b");
  await expect(contextBar).toContainText("pg_mon -> host-c");
  await expect(contextBar).toContainText("低信任候选 1");
  await expect(contextBar).toContainText("需要确认 1");
  await expect(contextBar).toContainText("角色冲突 1");
  await expect(page.getByText("正在等待模型返回")).toHaveCount(0);

  await expect(contextBar).toHaveScreenshot("special-input-context-bar.png");

  await page.getByRole("button", { name: "确认" }).click();
  await expect.poll(() => transportRequests.length, { timeout: 5000 }).toBe(1);
  expect(transportRequests[0]?.commands?.[0]).toMatchObject({
    type: "aiops.special-input-confirm",
    sessionId: "special-input-memory",
    resourceKind: "host",
    resourceId: "1.1.1.1",
    canonicalKey: "host:1.1.1.1",
  });

  await page.getByRole("button", { name: "清除特殊输入上下文" }).click();
  await expect.poll(() => transportRequests.length, { timeout: 5000 }).toBe(2);
  expect(transportRequests[1]?.commands?.[0]).toMatchObject({
    type: "aiops.special-input-clear",
    sessionId: "special-input-memory",
    resourceKind: "host",
    resourceId: "host-a",
    canonicalKey: "host:host-a",
  });
});

test("keeps inline special token layout separate from following text and caret", async ({ page }) => {
  await page.setViewportSize({ width: 795, height: 863 });
  await mockHostInventory(page);
  await installAssistantTransportRoute(page, []);
  await openFixturePage(page, "/", "special-input-memory");

  const input = page.getByTestId("omnibar-input");
  await expect(input).toBeVisible();
  await input.fill("@local查看CPU");
  await input.evaluate((element) => {
    const textarea = /** @type {HTMLTextAreaElement} */ (element);
    textarea.focus();
    textarea.setSelectionRange("@local".length, "@local".length);
    textarea.dispatchEvent(new Event("select", { bubbles: true }));
  });

  const overlay = page.getByTestId("composer-inline-host-overlay");
  const mention = page.getByTestId("composer-inline-mention-visual").first();
  const caret = page.getByTestId("composer-inline-caret");
  await expect(overlay).toBeVisible();
  await expect(page.getByTestId("composer-inline-host-mention")).toContainText("local");
  await expect(caret).toBeVisible();

  const mentionBox = await mention.boundingBox();
  const caretBox = await caret.boundingBox();
  expect(mentionBox).not.toBeNull();
  expect(caretBox).not.toBeNull();
  expect(caretBox.x).toBeGreaterThanOrEqual(mentionBox.x + mentionBox.width - 1);

  await expect(overlay).toHaveScreenshot("special-input-composer-inline-token.png");
});

async function installAssistantTransportRoute(page, requests) {
  await page.route("**/api/v1/assistant/transport", async (route) => {
    requests.push(route.request().postDataJSON());
    return route.fulfill({
      status: 200,
      contentType: "text/plain; charset=utf-8",
      body: "aui-state:[]\n",
    });
  });
}

async function mockHostInventory(page) {
  await page.route("**/api/v1/hosts", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        items: [
          { id: "server-local", name: "server-local", address: "server-local", status: "online" },
          { id: "host-a", name: "host-a", ip: "10.0.0.11", status: "online" },
          { id: "host-b", name: "host-b", ip: "10.0.0.12", status: "online" },
          { id: "host-c", name: "host-c", ip: "10.0.0.13", status: "online" },
        ],
      }),
    }),
  );
}
