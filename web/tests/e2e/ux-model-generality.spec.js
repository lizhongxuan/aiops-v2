// @ts-check
import { test, expect } from "@playwright/test";
import { openBrowserFixturePage } from "../helpers/uiFixtureHarness";

const REQUIRED_SIGNALS = [
  "task_depth=investigation",
  "required_gates=plan,evidence,verification",
  "ux_phase=waiting_approval",
  "resume_action=continue_next_step",
  "manager_synthesis=required",
  "coverage_action=continue_gathering",
  "reasoning_fallback=prompt_policy",
  "genericity_violations=0",
];

test.describe("ux model generality fixture", () => {
  test("shows depth, gate, resume, synthesis, coverage, fallback, and genericity states", async ({ page }) => {
    await openBrowserFixturePage(page, "/", "ux-model-generality");

    await expect(page.getByText("synthetic_ux_model_generality_request")).toBeVisible();
    const transcript = page.getByTestId("aiops-process-transcript");
    const header = page.getByTestId("aiops-process-header");
    if ((await header.getAttribute("aria-expanded")) !== "true") {
      await header.click();
    }

    for (const signal of REQUIRED_SIGNALS) {
      await expect(transcript.getByText(signal)).toBeVisible();
    }
  });
});
