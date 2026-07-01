import { expect, test } from "@playwright/test";

test("LLM provider config page shows OpenAI, DeepSeek, and Zhipu provider states", async ({ page }) => {
  await page.route("**/api/v1/llm-config", async (route) => {
    if (route.request().method() === "PUT") {
      const savedPayload = route.request().postDataJSON();
      await route.fulfill({ json: { ok: true, message: "saved", maxContextTokens: savedPayload.maxContextTokens, maxOutputTokens: savedPayload.maxOutputTokens } });
      return;
    }
    await route.fulfill({
      json: {
        provider: "openai",
        model: "gpt-5.4",
        apiKeySet: true,
        apiKeyMasked: "sk-***",
        baseURL: "",
        maxContextTokens: 200000,
        maxOutputTokens: 20000,
        reasoningEffort: "medium",
        bifrostActive: true,
      },
    });
  });

  await page.goto("/settings/llm");
  const panel = page.locator("section").first();
  await expect(page.getByTestId("llm-provider-select")).toHaveValue("openai");
  await expect(panel).toHaveScreenshot("llm-provider-config-openai.png");

  await page.getByTestId("llm-provider-select").selectOption("deepseek");
  await expect(page.getByTestId("llm-model-select")).toHaveValue("deepseek-v4-pro");
  await expect(page.getByTestId("llm-base-url-select")).toHaveValue("https://api.deepseek.com");
  await expect(page.getByTestId("llm-reasoning-effort-select")).toHaveValue("high");
  await expect(panel).toHaveScreenshot("llm-provider-config-deepseek.png");

  await page.getByTestId("llm-provider-select").selectOption("zhipu");
  await expect(page.getByTestId("llm-model-select")).toHaveValue("glm-5.2");
  await expect(page.getByTestId("llm-base-url-select")).toHaveValue("https://open.bigmodel.cn/api/paas/v4/");
  await expect(page.getByTestId("llm-reasoning-effort-select")).toHaveValue("max");
  await expect(page.getByTestId("llm-tool-stream-checkbox")).not.toBeChecked();
  await expect(panel).toHaveScreenshot("llm-provider-config-zhipu.png");
});
