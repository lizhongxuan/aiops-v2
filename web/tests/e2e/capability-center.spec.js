// @ts-check
import { test, expect } from "@playwright/test";

const BINDINGS_RESPONSE = {
  items: [
    { id: "bind-1", sourceType: "profile", sourceId: "main", targetType: "skill", targetId: "ops-triage", status: "active" },
  ],
  total: 1,
};

const SKILLS_RESPONSE = [
  { id: "ops-triage", name: "Ops Triage", source: "built-in", status: "active", enabled: true },
  { id: "log-analysis", name: "Log Analysis", source: "built-in", status: "active", enabled: false },
];

const MCPS_RESPONSE = [
  { id: "coroot-mcp", name: "Coroot MCP", type: "monitoring", source: "built-in", permission: "readonly" },
];

test.describe("CapabilityCenterPage", () => {
  test.beforeEach(async ({ page }) => {
    await page.route("**/api/v1/capability-bindings*", (route) =>
      route.fulfill({ json: BINDINGS_RESPONSE })
    );
    await page.route("**/api/v1/session*", (route) =>
      route.fulfill({ json: { sessionId: "test-session" } })
    );
    await page.route("**/api/v1/snapshot*", (route) =>
      route.fulfill({
        json: {
          skillCatalog: SKILLS_RESPONSE,
          mcpCatalog: MCPS_RESPONSE,
        },
      })
    );
    await page.goto("/capability-center");
  });

  test("page renders with title", async ({ page }) => {
    await expect(page.locator("main").getByText("能力中心", { exact: true })).toBeVisible();
  });

  test("shows three tabs: Skills, MCP Servers, Bindings", async ({ page }) => {
    // Naive UI n-tabs renders tabs as .ops-tabs-tab
    await expect(page.locator(".ops-tabs-tab", { hasText: "Skills" })).toBeVisible();
    await expect(page.locator(".ops-tabs-tab", { hasText: "MCP Servers" })).toBeVisible();
    await expect(page.locator(".ops-tabs-tab", { hasText: "Bindings" })).toBeVisible();
  });

  test("Skills tab is active by default", async ({ page }) => {
    const skillsTab = page.locator(".ops-tabs-tab", { hasText: "Skills" });
    await expect(skillsTab).toHaveClass(/active/);
  });

  test("switching to Bindings tab shows binding data", async ({ page }) => {
    await page.locator(".ops-tabs-tab", { hasText: "Bindings" }).click();
    await expect(page.locator('[data-slot="card-title"]').filter({ hasText: "Capability Bindings" })).toBeVisible();
  });

  test("switching to MCP Servers tab", async ({ page }) => {
    await page.locator(".ops-tabs-tab", { hasText: "MCP Servers" }).click();
    await expect(page.getByText("MCP Servers Catalog")).toBeVisible();
  });
});
