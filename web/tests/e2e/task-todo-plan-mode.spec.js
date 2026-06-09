// @ts-check
import { test, expect } from "@playwright/test";
import { openBrowserFixturePage } from "../helpers/uiFixtureHarness";

test.describe("task todo plan mode fixture", () => {
  test("shows plan mode, task ownership, blockers, approval scope, rejection and lease", async ({ page }) => {
    await openBrowserFixturePage(page, "/protocol?promptDebug=1", "task-todo-plan-mode");

    const body = page.locator("body");
    await expect(body).toContainText("Plan Mode active");
    await expect(body).toContainText("pending_exit_approval");
    await expect(body).toContainText("owner=agent:planner");
    await expect(body).toContainText("agentId=agent-plan-7");
    await expect(body).toContainText("blockedBy=missing_user_decision");
    await expect(body).toContainText("用户要求收窄验证范围后再批准");
    await expect(body).toContainText("仍在计划模式，等待修订后重新请求批准");
    await expect(body).toContainText("allowedActions=read_metrics,read_logs,update_plan");
    await expect(body).toContainText("resourceScopes=synthetic:service:demo-api,synthetic:dashboard:latency");
    await expect(body).toContainText("riskCeiling=low");
    await expect(body).toContainText("claim-lease-synthetic-1");

    await page.screenshot({ path: "tests/screenshots/task-todo-plan-mode.png", fullPage: true });
  });
});
