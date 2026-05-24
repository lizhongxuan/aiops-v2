// @ts-check
import path from "node:path";
import { pathToFileURL } from "node:url";
import { test, expect } from "@playwright/test";

function dashboardTarget() {
  if (process.env.SELFOPT_DASHBOARD_URL) {
    return process.env.SELFOPT_DASHBOARD_URL;
  }
  if (process.env.SELFOPT_DASHBOARD_FILE) {
    return pathToFileURL(path.resolve(process.env.SELFOPT_DASHBOARD_FILE)).toString();
  }
  return "";
}

test.describe("Self Optimization dashboard", () => {
  test("renders the generated dashboard smoke surface", async ({ page }) => {
    const target = dashboardTarget();
    if (!target && process.env.SELFOPT_DASHBOARD_REQUIRED === "1") {
      throw new Error("SELFOPT_DASHBOARD_REQUIRED=1 requires SELFOPT_DASHBOARD_URL or SELFOPT_DASHBOARD_FILE.");
    }
    test.skip(!target, "Set SELFOPT_DASHBOARD_URL or SELFOPT_DASHBOARD_FILE to run the generated dashboard smoke test.");

    await page.goto(target);

    const body = page.locator("body");
    await expect(body).toContainText("Self Optimization Lab");
    await expect(body).toContainText(/overview/i);
    await expect(body).toContainText(/Run\s+\S+\s+overall\s+\d+\.\d{2}/);
    await expect(body).toContainText(/timeline/i);
    await expect(body).toContainText(/safety/i);
    await expect(body).toContainText(/Gate:\s+(pass|warn|block)/);
    await expect(body).toContainText(/real aiops tests/i);
    await expect(body).toContainText(/impact/i);
  });
});
