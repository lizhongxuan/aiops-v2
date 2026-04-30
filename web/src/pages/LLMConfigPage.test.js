import { mount, flushPromises } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";
import LLMConfigPage from "./LLMConfigPage.vue";
import { fetchLlmConfig, updateLlmConfig } from "../api/llm";

const mockFetchState = vi.hoisted(() => vi.fn());

vi.mock("../api/llm", () => ({
  fetchLlmConfig: vi.fn(),
  updateLlmConfig: vi.fn(),
}));

vi.mock("../store", () => ({
  useAppStore: () => ({
    fetchState: mockFetchState,
  }),
}));

function mountPage() {
  return mount(LLMConfigPage, {
    global: {
      stubs: {
        "n-alert": true,
        "n-card": { template: "<section><div v-if=\"$attrs.title\">{{ $attrs.title }}</div><slot name=\"header-extra\" /><slot /></section>" },
        "n-form": { template: "<form><slot /></form>" },
        "n-form-item": { template: "<label><slot /><slot name=\"feedback\" /></label>" },
        "n-select": true,
        "n-auto-complete": true,
        "n-input": true,
        "n-button": { template: "<button v-bind=\"$attrs\"><slot name=\"icon\" /><slot /></button>" },
        "n-tag": { template: "<span><slot /></span>" },
      },
    },
  });
}

describe("LLMConfigPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("does not report the model as connected when runtime is active but API key is missing", async () => {
    fetchLlmConfig.mockResolvedValue({
      provider: "openai",
      model: "gpt-5.4",
      baseURL: "https://www.aicodexcn.com/v1",
      compactModel: "gpt-5.4",
      bifrostActive: true,
      apiKeySet: false,
    });

    const wrapper = mountPage();

    await flushPromises();

    expect(wrapper.text()).toContain("未连接（缺少 API Key）");
    expect(wrapper.text()).not.toContain("✓ 已连接");
  });

  it("renders the focused LLM settings page without the removed header and fallback sections", async () => {
    fetchLlmConfig.mockResolvedValue({
      provider: "openai",
      model: "gpt-5.4",
      baseURL: "https://www.aicodexcn.com/v1",
      compactModel: "gpt-5.4",
      bifrostActive: true,
      apiKeySet: true,
      apiKeyMasked: "sk-8****bafd",
    });

    const wrapper = mountPage();
    await flushPromises();

    expect(wrapper.text()).not.toContain("LLM Configuration");
    expect(wrapper.text()).not.toContain("模型配置");
    expect(wrapper.text()).not.toContain("配置 LLM Provider");
    expect(wrapper.text()).not.toContain("Runtime 运行中");
    expect(wrapper.text()).not.toContain("Fallback 配置");
    expect(wrapper.text()).not.toContain("Fallback Provider");
    expect(wrapper.text()).not.toContain("压缩模型");
    expect(wrapper.text()).toContain("主 LLM 配置");
    expect(wrapper.text()).toContain("✓ 已配置");
  });

  it("refreshes the app snapshot after a successful save so the header model status updates immediately", async () => {
    fetchLlmConfig
      .mockResolvedValueOnce({
        provider: "openai",
        model: "gpt-5.4",
        baseURL: "https://www.aicodexcn.com/v1",
        bifrostActive: true,
        apiKeySet: true,
      })
      .mockResolvedValueOnce({
        provider: "openai",
        model: "gpt-5.4",
        baseURL: "https://www.aicodexcn.com/v1",
        bifrostActive: true,
        apiKeySet: true,
      });
    updateLlmConfig.mockResolvedValue({ ok: true, message: "配置已保存。" });
    mockFetchState.mockResolvedValue(true);

    const wrapper = mountPage();
    await flushPromises();

    await wrapper.get('[data-testid="llm-save-button"]').trigger("click");
    await flushPromises();

    expect(updateLlmConfig).toHaveBeenCalledTimes(1);
    expect(fetchLlmConfig).toHaveBeenCalledTimes(2);
    expect(mockFetchState).toHaveBeenCalledTimes(1);
  });
});
