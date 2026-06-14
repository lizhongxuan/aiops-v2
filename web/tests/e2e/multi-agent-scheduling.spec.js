// @ts-check
import { test, expect } from "@playwright/test";
import { openBrowserFixturePage } from "../helpers/uiFixtureHarness";

const REQUIRED_SIGNALS = [
  "agent listing loaded: synthetic.explorer",
  "delegation decision: spawn_new",
  "assignment lint: pass",
  "parallel agents requested",
  "resource lock acquired",
  "pending agent final gate: require_wait",
  "wait_agent notifications: completed",
  "continuation decision: continue_existing",
  "verification agent: PASS",
  "final synthesis: evidence checked",
];

test.describe("multi-agent scheduling fixture", () => {
  test("shows scheduling decisions, gates, notifications, verifier, and final synthesis", async ({ page }) => {
    await openBrowserFixturePage(page, "/", "multi-agent-scheduling");

    await page.getByRole("button", { name: /已处理/ }).click();
    const transcript = page.getByTestId("aiops-process-transcript");

    for (const signal of REQUIRED_SIGNALS) {
      await expect(transcript.getByText(signal)).toBeVisible();
    }
  });
});
