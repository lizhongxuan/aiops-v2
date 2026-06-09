// @ts-check
import { test, expect } from "@playwright/test";
import { openBrowserFixturePage } from "../helpers/uiFixtureHarness";

test.describe("skills and mcp progressive discovery fixture", () => {
  test("shows skill activation, mcp instructions, artifact reference, and final evidence", async ({ page }) => {
    await openBrowserFixturePage(page, "/", "skills-mcp-progressive-discovery");

    await expect(page.getByText("synthetic_skills_mcp_progressive_request")).toBeVisible();
    await page.getByRole("button", { name: /已处理/ }).click();
    const transcript = page.getByTestId("aiops-process-transcript");
    await expect(transcript.getByText("skill_search mode=search")).toBeVisible();
    await expect(transcript.getByText("skill_read skill=synthetic.triage")).toBeVisible();
    await expect(transcript.getByText("mandatory skill activation retry")).toBeVisible();
    await expect(transcript.getByText("mcp instruction delta: added synthetic-docs")).toBeVisible();
    await expect(transcript.getByText("mcp sparse reminder")).toBeVisible();
    await expect(page.getByRole("button", { name: /mcp resource artifact: application\/pdf 已完成/ })).toBeVisible();
    await expect(page.getByRole("listitem").filter({ hasText: "final evidence: skill checked" })).toBeVisible();
  });
});
