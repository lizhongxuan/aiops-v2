import { describe, expect, it } from "vitest";

import {
  CUSTOM_BASE_URL_VALUE,
  CUSTOM_MODEL_VALUE,
  defaultFormForModel,
  defaultFormForProvider,
  getBaseURLOptions,
  getModelOptions,
  getModelPreset,
  getProviderPreset,
  getReasoningOptions,
} from "@/pages/llmProviderCatalog";

describe("llmProviderCatalog", () => {
  it("defines DeepSeek and Zhipu as first-class providers with official defaults", () => {
    expect(getProviderPreset("deepseek")).toMatchObject({
      id: "deepseek",
      defaultModel: "deepseek-v4-pro",
      defaultBaseURL: "https://api.deepseek.com",
      defaultReasoning: "high",
      defaultThinkingType: "enabled",
    });
    expect(getProviderPreset("zhipu")).toMatchObject({
      id: "zhipu",
      defaultModel: "glm-5.2",
      defaultBaseURL: "https://open.bigmodel.cn/api/paas/v4/",
      defaultReasoning: "max",
      defaultThinkingType: "enabled",
      supportsToolStream: true,
    });
  });

  it("uses provider-specific reasoning option sets", () => {
    expect(getReasoningOptions("deepseek")).toEqual(["high", "max"]);
    expect(getReasoningOptions("zhipu")).toEqual(["max", "xhigh", "high", "medium", "low", "minimal", "none"]);
  });

  it("keeps custom model and custom base URL options at the end", () => {
    expect(getModelOptions("deepseek").at(-1)).toEqual({ label: "自定义", value: CUSTOM_MODEL_VALUE });
    expect(getBaseURLOptions("zhipu").at(-1)).toEqual({ label: "自定义", value: CUSTOM_BASE_URL_VALUE });
    expect(getBaseURLOptions("zhipu")).toContainEqual({ label: "GLM Coding Plan", value: "https://open.bigmodel.cn/api/coding/paas/v4" });
  });

  it("derives default form values from provider and model presets", () => {
    expect(defaultFormForProvider("deepseek")).toMatchObject({
      provider: "deepseek",
      model: "deepseek-v4-pro",
      baseURL: "https://api.deepseek.com",
      maxContextTokens: "1000000",
      maxOutputTokens: "20000",
      reasoningEffort: "high",
      thinkingType: "enabled",
      topP: "1",
    });
    expect(defaultFormForProvider("zhipu")).toMatchObject({
      provider: "zhipu",
      model: "glm-5.2",
      baseURL: "https://open.bigmodel.cn/api/paas/v4/",
      maxContextTokens: "1000000",
      maxOutputTokens: "20000",
      reasoningEffort: "max",
      thinkingType: "enabled",
      topP: "0.95",
    });
    expect(defaultFormForModel("zhipu", "glm-4.5-air")).toMatchObject({
      maxContextTokens: "128000",
      maxOutputTokens: "16000",
      topP: "0.95",
    });
    expect(getModelPreset("zhipu", "glm-5.2")?.maxOutputTokens).toBe(128000);
  });
});
