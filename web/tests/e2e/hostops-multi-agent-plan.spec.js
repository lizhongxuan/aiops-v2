import { expect, test } from "@playwright/test";

import { resolveUiFixturePreset } from "../../src/lib/uiFixturePresets";
import { openFixturePage } from "../helpers/uiFixtureHarness";

test("renders collapsible multi-agent plan and host agent list", async ({ page }) => {
  const fixture = resolveUiFixturePreset("host-ops-three-hosts");
  await page.route("**/api/v1/host-ops/child-agents/*/transcript", (route) => {
    const childAgentId = route.request().url().split("/child-agents/").at(-1)?.split("/transcript")[0] || "";
    return route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(fixture.state.hostOpsTranscripts?.[decodeURIComponent(childAgentId)] || { childAgentId, items: [] }),
    });
  });

  await openFixturePage(page, "/", fixture);

  const panel = page.getByTestId("host-ops-status-panel");
  await expect(panel).toBeVisible();
  await expect(panel).toContainText("共 5 个步骤，已经完成 0 个");
  await expect(panel).toContainText("共 3 个主机 Agent");

  await expect(page.getByTestId("task-checklist-item-confirm")).toBeVisible();
  await page.getByTestId("task-checklist-toggle").first().click();
  await expect(page.getByTestId("task-checklist-item-confirm")).toHaveCount(0);
  await expect(panel).toContainText("共 5 个步骤，已经完成 0 个");

  await expect(page.getByTestId("host-subagent-status-row-child-1")).toBeVisible();
  await page.getByTestId("task-checklist-toggle").nth(1).click();
  await expect(page.getByTestId("host-subagent-status-row-child-1")).toHaveCount(0);
  await expect(panel).toContainText("共 3 个主机 Agent");

  await page.getByTestId("task-checklist-toggle").nth(1).click();
  await page.getByTestId("host-subagent-status-row-child-1").click();
  await expect(page.getByTestId("host-subagent-drawer")).toBeVisible();
  await expect(page.getByTestId("host-subagent-tab-task")).toBeVisible();
  await expect(page.getByTestId("host-subagent-tab-conversation")).toBeVisible();
  await expect(page.getByTestId("host-subagent-tab-tools")).toBeVisible();
  await expect(page.getByTestId("host-subagent-tab-approval")).toBeVisible();
  await expect(page.getByTestId("host-subagent-tab-receipts")).toBeVisible();
});
