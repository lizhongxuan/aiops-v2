// @ts-check
import { test, expect } from "@playwright/test";
import { corootProjectId, expectCorootWorkspace, installAiopsFixture, mockUnconfiguredCoroot } from "./coroot-helpers";

test.describe("Coroot embedded monitoring configuration", () => {
  test("redirects unconfigured entry to config and enters workspace after save", async ({ page }) => {
    let testPayload;
    let savePayload;
    await installAiopsFixture(page);
    await mockUnconfiguredCoroot(page, {
      onTest: (payload) => {
        testPayload = payload;
      },
      onSave: (payload) => {
        savePayload = payload;
      },
    });

    await page.goto("/coroot");

    await expect(page).toHaveURL(/\/coroot\/config$/);
    await expect(page.getByText("Coroot 配置")).toBeVisible();
    await page.getByLabel("Base URL").fill("http://172.18.13.11:8000/coroot");
    await page.getByLabel("Project ID").fill(corootProjectId);
    await page.getByLabel("Auth mode").selectOption("embed_trust");
    await page.getByLabel("Embed trust shared secret").fill("shared-secret");

    await page.getByRole("button", { name: "测试连接" }).click();

    await expect(page.getByText("连接可用")).toBeVisible();
    expect(testPayload).toMatchObject({
      baseUrl: "http://172.18.13.11:8000/coroot",
      project: corootProjectId,
      authMode: "embed_trust",
      embedMode: "full",
      embedTrustSecret: "shared-secret",
      uiGatewayEnabled: true,
    });

    await page.getByRole("button", { name: "保存并进入 Coroot" }).click();

    await expect(page).toHaveURL(new RegExp(`/coroot/p/${corootProjectId}/applications$`));
    expect(savePayload).toMatchObject({
      authMode: "embed_trust",
      embedMode: "full",
      embedTrustSecret: "shared-secret",
      uiGatewayEnabled: true,
    });
    await expectCorootWorkspace(page);
  });
});
