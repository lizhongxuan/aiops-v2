import { expect, test } from "@playwright/test";

import { resolveUiFixturePreset } from "../../src/lib/uiFixturePresets";
import { openFixturePage } from "../helpers/uiFixtureHarness";

test("shows compact host ops panel above composer and opens child drawer", async ({ page }) => {
  const fixture = resolveUiFixturePreset("host-ops-three-hosts");
  await page.route("**/api/v1/host-ops/child-agents/*/transcript", (route) => {
    const childAgentId = route.request().url().split("/child-agents/").at(-1)?.split("/transcript")[0] || "";
    const transcript = fixture.state.hostOpsTranscripts?.[decodeURIComponent(childAgentId)] || {
      childAgentId,
      items: [],
    };
    return route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(transcript),
    });
  });

  await openFixturePage(page, "/", fixture);

  await expect(page.getByTestId("host-ops-status-panel")).toBeVisible();
  await expect(page.getByText("共 5 个步骤，已经完成 0 个")).toBeVisible();
  await expect(page.getByText("共 3 个主机 Agent")).toBeVisible();
  await expect(page.getByText("@1.1.1.1(@1.1.1.1)")).toBeVisible();

  await page.getByTestId("host-subagent-status-row-child-1").click();

  await expect(page.getByTestId("host-subagent-drawer")).toBeVisible();
  await expect(page.getByText("主机 Agent 详情")).toBeVisible();
  await expect(page.getByTestId("host-subagent-tab-conversation")).toBeVisible();
  await expect(page.getByTestId("host-subagent-tab-commands")).toBeVisible();
  await expect(page.getByText("@1.1.1.1 @1.1.1.1")).toBeVisible();
  await expect(page.getByText("检查主机状态")).toBeVisible();
  await expect(page.getByText("主机状态正常")).toBeVisible();
});
