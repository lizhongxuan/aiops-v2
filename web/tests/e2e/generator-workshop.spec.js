// @ts-check
import { test, expect } from "@playwright/test";

const GENERATE_SKILL_RESPONSE = {
  draftType: "skill",
  skill: {
    id: "skill-gen-1",
    name: "list-services",
    description: "Lists all monitored services",
    source: "mcp-generated",
    category: "monitoring",
    status: "draft",
    version: "v1-draft",
  },
};

const LINT_RESPONSE = {
  issues: [
    { field: "description", level: "warning", message: "description is recommended" },
  ],
  valid: true,
};

const PUBLISH_RESPONSE = {
  published: true,
  draftType: "skill",
  skill: { id: "skill-gen-1", name: "list-services", status: "active" },
};

test.describe("GeneratorWorkshopPage", () => {
  test.beforeEach(async ({ page }) => {
    await page.route("**/api/v1/generator/generate", (route) =>
      route.fulfill({ json: GENERATE_SKILL_RESPONSE })
    );
    await page.route("**/api/v1/generator/lint", (route) =>
      route.fulfill({ json: LINT_RESPONSE })
    );
    await page.route("**/api/v1/generator/preview", (route) =>
      route.fulfill({ json: { draftType: "skill", preview: { id: "skill-gen-1", name: "list-services", status: "draft" } } })
    );
    await page.route("**/api/v1/generator/publish-draft", (route) =>
      route.fulfill({ json: PUBLISH_RESPONSE })
    );
    await page.route("**/api/v1/session*", (route) =>
      route.fulfill({ json: { sessionId: "test-session" } })
    );
    await page.goto("/generator");
  });

  test("page renders with title", async ({ page }) => {
    await expect(page.locator("main").getByText("Generator Workshop", { exact: true })).toBeVisible();
  });

  test("source selection shows three radio options", async ({ page }) => {
    await expect(page.locator(".ops-radio__label", { hasText: "MCP 工具" })).toBeVisible();
    await expect(page.locator(".ops-radio__label", { hasText: "脚本配置" })).toBeVisible();
    await expect(page.locator(".ops-radio__label", { hasText: "Coroot 服务" })).toBeVisible();
  });

  test("MCP tool source shows tool name input", async ({ page }) => {
    await expect(page.locator('input[placeholder*="list-services"]')).toBeVisible();
  });

  test("switching to script config source shows textarea", async ({ page }) => {
    // Click the radio label text (not the hidden input)
    await page.locator(".ops-radio", { hasText: "脚本配置" }).click();
    await expect(page.locator('textarea[placeholder*="scriptName"]')).toBeVisible();
  });

  test("switching to coroot source shows service type input", async ({ page }) => {
    await page.locator(".ops-radio", { hasText: "Coroot 服务" }).click();
    await expect(page.locator('input[placeholder*="web-api"]')).toBeVisible();
  });

  test("generate button creates draft and switches to preview", async ({ page }) => {
    await page.locator('input[placeholder*="list-services"]').fill("my-tool");
    await page.getByRole("button", { name: "生成草稿" }).click();
    await expect(page.locator("h2", { hasText: "草稿预览" })).toBeVisible();
    await expect(page.locator(".preview-output")).toBeVisible();
  });

  test("lint validation works after generating", async ({ page }) => {
    await page.locator('input[placeholder*="list-services"]').fill("my-tool");
    await page.getByRole("button", { name: "生成草稿" }).click();
    await page.locator(".ops-step", { hasText: "校验" }).click();
    await page.getByRole("button", { name: "运行校验" }).click();
    await expect(page.locator(".lint-status")).toBeVisible();
    await expect(page.locator(".lint-status")).toContainText("校验通过");
  });

  test("publish draft sends request", async ({ page }) => {
    await page.locator('input[placeholder*="list-services"]').fill("my-tool");
    await page.getByRole("button", { name: "生成草稿" }).click();
    await page.locator(".ops-step", { hasText: "发布" }).click();
    const requestPromise = page.waitForRequest((req) =>
      req.url().includes("/api/v1/generator/publish-draft") && req.method() === "POST"
    );
    await page.getByRole("button", { name: "确认发布" }).click();
    await requestPromise;
    await expect(page.locator(".lint-status.valid")).toContainText("发布成功");
  });
});
