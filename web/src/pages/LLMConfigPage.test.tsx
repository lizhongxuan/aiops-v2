import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { AppShellChromeProvider, useAppShellChrome } from "@/app/AppShellChromeContext";
import { LLMConfigPage } from "@/pages/LLMConfigPage";

function HeaderActionsProbe() {
  const { headerActions } = useAppShellChrome();
  return <div data-testid="header-actions">{headerActions}</div>;
}

describe("LLMConfigPage", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    vi.restoreAllMocks();
    act(() => root.unmount());
    container.remove();
  });

  async function renderPage(initialConfig: Record<string, unknown> = {}) {
    vi.spyOn(globalThis, "fetch").mockImplementation((_, init) => {
      if ((init as RequestInit | undefined)?.method === "PUT") {
        return Promise.resolve(new Response(JSON.stringify({ ok: true }), { status: 200, headers: { "Content-Type": "application/json" } }));
      }
      return Promise.resolve(
        new Response(
          JSON.stringify({
            provider: "openai",
            model: "gpt-5.4",
            maxContextTokens: 200000,
            maxOutputTokens: 20000,
            reasoningEffort: "medium",
            bifrostActive: true,
            apiKeySet: true,
            ...initialConfig,
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        ),
      );
    });

    await act(async () => {
      root.render(
        <AppShellChromeProvider>
          <HeaderActionsProbe />
          <LLMConfigPage />
        </AppShellChromeProvider>,
      );
    });
    await flushReact();
  }

  it("switches provider-scoped defaults for DeepSeek and Zhipu", async () => {
    await renderPage();

    const provider = selectByTestId("llm-provider-select");
    expect(optionValues(provider)).toEqual(["openai", "deepseek", "zhipu", "anthropic", "ollama"]);

    await changeSelect(provider, "deepseek");
    expect(selectByTestId("llm-model-select").value).toBe("deepseek-v4-pro");
    expect(selectByTestId("llm-base-url-select").value).toBe("https://api.deepseek.com");
    expect(optionValues(selectByTestId("llm-reasoning-effort-select"))).toEqual(["high", "max"]);
    expect(selectByTestId("llm-thinking-type-select").value).toBe("enabled");
    expect(container.textContent).toContain("采样参数不会生效");
    expect(container.querySelector('[data-testid="llm-custom-model-input"]')).toBeNull();
    expect(container.querySelector('[data-testid="llm-custom-base-url-input"]')).toBeNull();

    await changeSelect(provider, "zhipu");
    expect(selectByTestId("llm-model-select").value).toBe("glm-5.2");
    expect(selectByTestId("llm-base-url-select").value).toBe("https://open.bigmodel.cn/api/paas/v4/");
    expect(optionValues(selectByTestId("llm-base-url-select"))).toContain("https://open.bigmodel.cn/api/coding/paas/v4");
    expect(optionValues(selectByTestId("llm-reasoning-effort-select"))).toEqual(["max", "xhigh", "high", "medium", "low", "minimal", "none"]);
    expect((container.querySelector('[data-testid="llm-tool-stream-checkbox"]') as HTMLInputElement | null)).not.toBeNull();

    await changeSelect(selectByTestId("llm-model-select"), "__custom_model__");
    expect(container.querySelector('[data-testid="llm-custom-model-input"]')).not.toBeNull();
    await changeSelect(selectByTestId("llm-base-url-select"), "__custom_base_url__");
    expect(container.querySelector('[data-testid="llm-custom-base-url-input"]')).not.toBeNull();
  });

  it("saves DeepSeek payload without an empty API key", async () => {
    await renderPage({ apiKeySet: false });

    await changeSelect(selectByTestId("llm-provider-select"), "deepseek");
    await click(container.querySelector('[data-testid="llm-save-button"]') as HTMLButtonElement);

    const putCall = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls.find((call) => (call[1] as RequestInit | undefined)?.method === "PUT");
    expect(putCall).toBeTruthy();
    const body = JSON.parse(String((putCall?.[1] as RequestInit).body));
    expect(body).toMatchObject({
      provider: "deepseek",
      model: "deepseek-v4-pro",
      baseURL: "https://api.deepseek.com",
      maxContextTokens: 1000000,
      maxOutputTokens: 20000,
      thinkingType: "enabled",
      reasoningEffort: "high",
      temperature: 1,
      topP: 1,
    });
    expect(body).not.toHaveProperty("apiKey");
    expect(body).not.toHaveProperty("toolStream");
  });

  function selectByTestId(testId: string) {
    const element = container.querySelector(`[data-testid="${testId}"]`) as HTMLSelectElement | null;
    if (!element) throw new Error(`missing select ${testId}`);
    return element;
  }
});

async function flushReact() {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

async function changeSelect(select: HTMLSelectElement, value: string) {
  await act(async () => {
    select.value = value;
    select.dispatchEvent(new Event("change", { bubbles: true }));
  });
  await flushReact();
}

async function click(button: HTMLButtonElement) {
  await act(async () => {
    button.dispatchEvent(new MouseEvent("click", { bubbles: true }));
  });
  await flushReact();
}

function optionValues(select: HTMLSelectElement) {
  return Array.from(select.options).map((option) => option.value);
}
