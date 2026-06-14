import { expect, test } from "@playwright/test";

import { resolveUiFixturePreset } from "../../src/lib/uiFixturePresets";
import { openFixturePage } from "../helpers/uiFixtureHarness";

test("approves non-whitelisted host command approval inside host agent drawer", async ({ page }) => {
  const approvalDecisions = await openHostCommandApprovalDrawer(page);

  await page.getByTestId("host-subagent-approval-approve-hostcmd-approval-child-2").click();
  await expect(page.getByText("审批请求已提交")).toBeVisible();
  expect(approvalDecisions).toEqual([
    {
      url: expect.stringContaining("/api/v1/approvals/hostcmd-approval-child-2/decision"),
      body: { decision: "accept" },
    },
  ]);
});

test("rejects non-whitelisted host command approval inside host agent drawer", async ({ page }) => {
  const approvalDecisions = await openHostCommandApprovalDrawer(page);

  await page.getByTestId("host-subagent-approval-reject-hostcmd-approval-child-2").click();
  await expect(page.getByText("审批请求已提交")).toBeVisible();
  expect(approvalDecisions).toEqual([
    {
      url: expect.stringContaining("/api/v1/approvals/hostcmd-approval-child-2/decision"),
      body: { decision: "reject" },
    },
  ]);
});

async function openHostCommandApprovalDrawer(page) {
  const fixture = resolveUiFixturePreset("host-ops-command-approval");
  await page.route("**/api/v1/host-ops/child-agents/*/transcript", (route) => {
    const childAgentId = route.request().url().split("/child-agents/").at(-1)?.split("/transcript")[0] || "";
    return route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(fixture.state.hostOpsTranscripts?.[decodeURIComponent(childAgentId)] || { childAgentId, items: [] }),
    });
  });
  await openFixturePage(page, "/", fixture);
  const approvalDecisions = [];
  await page.route("**/api/v1/approvals/*/decision", async (route) => {
    let body = {};
    try {
      body = route.request().postDataJSON();
    } catch {
      body = {};
    }
    approvalDecisions.push({ url: route.request().url(), body });
    return route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ status: "executed" }),
    });
  });

  await page.getByTestId("host-subagent-status-row-child-2").click();
  const drawer = page.getByTestId("host-subagent-drawer");
  await expect(drawer).toBeVisible();
  await expect(page.getByTestId("host-subagent-tab-approval")).toHaveAttribute("aria-selected", "true");
  await expect(drawer).toContainText("审批");
  await expect(drawer).toContainText("等待执行非白名单主机命令");
  await expect(drawer).toContainText("touch /tmp/aiops-check");
  await expect(page.getByTestId("host-subagent-approval-approve-hostcmd-approval-child-2")).toBeVisible();
  await expect(page.getByTestId("host-subagent-approval-reject-hostcmd-approval-child-2")).toBeVisible();
  expect(approvalDecisions).toEqual([]);
  return approvalDecisions;
}
