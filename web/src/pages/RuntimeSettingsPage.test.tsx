import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { AppShellChromeProvider, useAppShellChrome } from "@/app/AppShellChromeContext";
import { RuntimeSettingsPage } from "@/pages/RuntimeSettingsPage";

const runtimePayload = {
  settings: {
    agentRuntime: {
      intentFrameRouting: "trace_only",
      diagnosticProtocol: true,
    },
    tooling: {
      readOnlyRetryEnabled: false,
      readOnlyRetryMaxPerCall: 1,
      readOnlyRetryMaxPerTurn: 3,
      readOnlyRetryBackoffBaseMs: 300,
      readOnlyRetryBackoffMaxMs: 2000,
    },
    workflow: {
      referenceGuardMode: "enforce",
      validationProvider: "static",
      validationImage: "python:3.12-slim",
    },
    opsManual: {
      autoRetrieval: false,
    },
    debug: {
      modelInputTrace: true,
      finalState: false,
      transportProjection: false,
      transcriptProjection: false,
    },
    publicWeb: {
      enabled: true,
    },
    updatedAt: "2026-06-30T08:00:00Z",
  },
  defaults: {},
  restartRequiredKeys: [],
};

function HeaderActionsProbe() {
  const { headerActions } = useAppShellChrome();
  return <div data-testid="header-actions">{headerActions}</div>;
}

describe("RuntimeSettingsPage", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    vi.restoreAllMocks();
    act(() => root.unmount());
    container.remove();
  });

  async function renderPage() {
    vi.spyOn(globalThis, "fetch").mockImplementation((_, init) => {
      if ((init as RequestInit | undefined)?.method === "PATCH") {
        return Promise.resolve(new Response(JSON.stringify(runtimePayload), { status: 200, headers: { "Content-Type": "application/json" } }));
      }
      return Promise.resolve(new Response(JSON.stringify(runtimePayload), { status: 200, headers: { "Content-Type": "application/json" } }));
    });

    await act(async () => {
      root.render(
        <AppShellChromeProvider>
          <HeaderActionsProbe />
          <RuntimeSettingsPage />
        </AppShellChromeProvider>,
      );
    });
    await flushReact();
  }

  it("renders runtime sections and keeps deprecated env names absent", async () => {
    await renderPage();

    expect(container.textContent).toContain("Agent Runtime");
    expect(container.textContent).toContain("Tooling");
    expect(container.textContent).toContain("Workflow");
    expect(container.textContent).toContain("Ops Manual");
    expect(container.textContent).toContain("Debug");
    expect(container.textContent).toContain("Public Web");
    expect(selectByTestId(container, "runtime-intent-frame-routing").value).toBe("trace_only");
    expect((container.querySelector('[data-testid="runtime-readonly-retry-per-turn"]') as HTMLInputElement).value).toBe("3");
    expect(container.querySelector('[data-testid="runtime-validation-image"]')).toBeNull();
    for (const deprecated of [
      "AIOPS_RUNNER_" + "DISABLED",
      "AIOPS_LLM_" + "API_KEY",
      "AIOPS_DEBUG_MODEL_INPUT_TRACE_" + "DIR",
      "AIOPS_GRPC_" + "AUTO_PORT",
    ]) {
      expect(container.textContent).not.toContain(deprecated);
    }
  });

  it("saves a partial PATCH payload and shows the effective success message", async () => {
    await renderPage();

    await changeSelect(selectByTestId(container, "runtime-validation-provider"), "docker");
    const imageInput = container.querySelector('[data-testid="runtime-validation-image"]') as HTMLInputElement | null;
    expect(imageInput).not.toBeNull();
    await changeInput(imageInput!, "python:3.13-slim");
    await changeInput(container.querySelector('[data-testid="runtime-readonly-retry-per-turn"]') as HTMLInputElement, "5");
    await changeCheckbox(container.querySelector('[data-testid="runtime-public-web-enabled"]') as HTMLInputElement, false);
    await click(container.querySelector('[data-testid="runtime-settings-save"]') as HTMLButtonElement);

    const patchCall = vi.mocked(globalThis.fetch).mock.calls.find((call) => String(call[0]).endsWith("/api/v1/runtime-settings") && (call[1] as RequestInit | undefined)?.method === "PATCH");
    expect(patchCall).toBeTruthy();
    expect(JSON.parse(String((patchCall?.[1] as RequestInit).body))).toEqual({
      tooling: { readOnlyRetryMaxPerTurn: 5 },
      workflow: {
        validationProvider: "docker",
        validationImage: "python:3.13-slim",
      },
      publicWeb: { enabled: false },
    });
    expect(container.textContent).toContain("已保存，下次请求生效");
  });
});

async function flushReact() {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

function selectByTestId(root: HTMLElement, testId: string) {
  const element = root.querySelector(`[data-testid="${testId}"]`) as HTMLSelectElement | null;
  if (!element) throw new Error(`missing select ${testId}`);
  return element;
}

async function changeSelect(select: HTMLSelectElement, value: string) {
  await act(async () => {
    select.value = value;
    select.dispatchEvent(new Event("change", { bubbles: true }));
  });
  await flushReact();
}

async function changeInput(input: HTMLInputElement, value: string) {
  await act(async () => {
    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set;
    setter?.call(input, value);
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await flushReact();
}

async function changeCheckbox(input: HTMLInputElement, checked: boolean) {
  await act(async () => {
    if (input.checked !== checked) {
      input.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    }
  });
  await flushReact();
}

async function click(button: HTMLButtonElement) {
  await act(async () => {
    button.dispatchEvent(new MouseEvent("click", { bubbles: true }));
  });
  await flushReact();
}
