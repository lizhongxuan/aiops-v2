// @ts-check
import { test, expect } from "@playwright/test";
import { openBrowserFixturePage } from "../helpers/uiFixtureHarness";

const REQUIRED_SIGNALS = [
  "verification_status=PARTIAL",
  "completion gate block: execution evidence missing",
  "blocker next action: rerun focused synthetic verification command",
  "destructive workaround safety signal: skip_validation high-risk blocked",
  "unexpected state gate: block_mutation",
  "approval scope summary: allowedActions=read_metrics,read_logs request_verification",
];

test.describe("verification completion safety permission fixture", () => {
  test("shows verification status, completion gate, blocker action, safety signal, unexpected state, and approval scope", async ({ page }) => {
    await openBrowserFixturePage(page, "/", "verification-completion-safety-permission");

    await expect(page.getByText("synthetic_verification_completion_safety_permission_request")).toBeVisible();
    await page.getByRole("button", { name: /等待审核|已处理/ }).click();
    const transcript = page.getByTestId("aiops-process-transcript");

    for (const signal of REQUIRED_SIGNALS) {
      await expect(transcript.getByText(signal)).toBeVisible();
    }
  });
});
