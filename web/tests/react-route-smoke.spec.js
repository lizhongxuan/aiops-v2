// @ts-check
import { expect, test } from "@playwright/test";

const routes = [
  "/",
  "/protocol",
  "/incidents",
  "/incidents/incident-1",
  "/erp",
  "/opsgraph",
  "/runbooks",
  "/runbooks/runbook-1",
  "/runner",
  "/runner/payment-health",
  "/postmortems/postmortem-1",
  "/terminal/host-1",
  "/settings",
  "/settings/llm",
  "/settings/hosts",
  "/settings/experience-packs",
  "/settings/agent",
  "/settings/skills",
  "/settings/mcp",
  "/mcp",
  "/approval-management",
  "/capability-center",
  "/ui-cards",
  "/script-configs",
  "/coroot",
  "/lab",
  "/generator",
  "/debug/prompts",
];

test.describe("React route smoke", () => {
  for (const route of routes) {
    test(`renders ${route}`, async ({ page }) => {
      await page.goto(route);

      await expect(page.locator("aside").first()).toContainText("AIOPS");
      await expect(page.locator("main")).not.toHaveText("");
    });
  }

  test("redirects legacy route aliases", async ({ page }) => {
    await page.goto("/hosts");
    await expect(page).toHaveURL(/\/settings\/hosts$/);
    await expect(page.locator("main")).toContainText("主机");

    await page.goto("/experience-packs");
    await expect(page).toHaveURL(/\/settings\/experience-packs$/);
    await expect(page.locator("main")).toContainText("经验包");
  });
});
