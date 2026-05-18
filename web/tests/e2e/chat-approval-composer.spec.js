// @ts-check
import { expect, test } from "@playwright/test";

import {
  createProtocolFixtureSessions,
  createProtocolFixtureState,
  installUiFixture,
  waitForFixtureStable,
} from "../helpers/uiFixtureHarness.js";

test("approval composer gives immediate feedback while approval decision is pending", async ({ page }) => {
  await installUiFixture(page, {
    name: "approval-submit-feedback",
    state: createProtocolFixtureState(),
    sessions: createProtocolFixtureSessions(),
  });
  await page.goto("/", { waitUntil: "networkidle" });
  await waitForFixtureStable(page);

  const approvalComposer = page.getByTestId("codex-approval-inline");
  await expect(approvalComposer).toContainText("等待审批");

  const submit = approvalComposer.getByRole("button", { name: "提交" });
  await expect(submit).toBeEnabled();

  await submit.click();

  await expect(approvalComposer).toContainText("已提交确认，正在继续执行");
  await expect(approvalComposer.getByRole("button", { name: "提交中" })).toBeDisabled();
});
