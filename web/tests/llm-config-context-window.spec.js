import { expect, test } from "@playwright/test";

test("LLM config saves manual context window with minimum clamp", async ({ page }) => {
  let savedPayload = null;

  await page.route("**/api/v1/llm-config", async (route) => {
    if (route.request().method() === "PUT") {
      savedPayload = route.request().postDataJSON();
      await route.fulfill({ json: { ok: true, message: "saved", maxContextTokens: savedPayload.maxContextTokens } });
      return;
    }
    await route.fulfill({
      json: {
        provider: "openai",
        model: "gpt-5.4",
        apiKeySet: true,
        apiKeyMasked: "sk-***",
        baseURL: "https://www.aicodexcn.com/v1",
        maxContextTokens: 200000,
        bifrostActive: true,
      },
    });
  });

  await page.goto("/settings/llm");
  const contextInput = page.getByTestId("llm-context-tokens-input");
  await expect(contextInput).toHaveValue("200000");

  await contextInput.fill("9000");
  await page.getByTestId("llm-save-button").click();

  await expect.poll(() => savedPayload?.maxContextTokens).toBe(10000);
  await expect(page.getByText("正在重试压缩")).toHaveCount(0);
});
