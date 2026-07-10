// @ts-check
import { test, expect } from "@playwright/test";
import { corootProjectId, expectCorootWorkspace, installAiopsFixture, mockConfiguredCoroot } from "./coroot-helpers";

test.describe("Coroot embedded workspace", () => {
  test("uses aiops shell chrome while rendering native Coroot iframe content", async ({ page }) => {
    await installAiopsFixture(page);
    await mockConfiguredCoroot(page);
    await page.addInitScript(() => {
      window.sessionStorage.setItem("aiops.coroot.returnTo", "/incidents?status=open");
    });

    await page.goto(`/coroot/p/${corootProjectId}/applications`);

    await expectCorootWorkspace(page);
    await expect(page.locator('[data-testid="app-shell-sidebar"]')).toHaveAttribute("data-collapsed", "false");
    await page.getByRole("button", { name: "收起侧边栏" }).click();
    await expect(page.locator('[data-testid="app-shell-sidebar"]')).toHaveAttribute("data-collapsed", "true");
    await page.getByRole("button", { name: "展开侧边栏" }).click();
    await expect(page.locator('[data-testid="app-shell-sidebar"]')).toHaveAttribute("data-collapsed", "false");

    const frame = page.frameLocator('iframe[title="Coroot"]');
    await expect(frame.getByText("aiops-host-agent")).toBeVisible();
    await expect(frame.getByText("1 unique error")).toBeVisible();

    const frameElement = await page.locator('iframe[title="Coroot"]').elementHandle();
    if (!frameElement) throw new Error("Coroot iframe element was not found");
    const embeddedFrame = await frameElement.contentFrame();
    if (!embeddedFrame) throw new Error("Coroot iframe did not expose a frame");
    await embeddedFrame.evaluate(
      ({ projectId }) => {
        window.parent.postMessage(
          {
            type: "aiops.coroot.route.v1",
            projectId,
            view: "logs",
            query: { from: "now-1h", to: "now" },
          },
          window.location.origin,
        );
      },
      { projectId: corootProjectId },
    );
    await expect(page).toHaveURL(new RegExp(`/coroot/p/${corootProjectId}/logs\\?from=now-1h&to=now$`));

    await page.getByRole("button", { name: "返回 AIOps" }).click();
    await expect(page).toHaveURL(/\/incidents\?status=open$/);
  });
});
