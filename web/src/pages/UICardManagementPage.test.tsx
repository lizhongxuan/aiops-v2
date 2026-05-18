import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { AppShellChromeProvider } from "@/app/AppShellChromeContext";
import { UICardManagementPage } from "./UICardManagementPage";

const listPayload = {
  total: 2,
  stats: { total: 2, active: 1, draft: 1, deprecated: 0, disabled: 0, builtIn: 1 },
  items: [
    {
      id: "coroot-chart",
      name: "Coroot Chart",
      kind: "coroot_chart",
      renderer: "agent-ui/coroot-chart",
      status: "active",
      builtIn: true,
      version: 1,
      summary: "Coroot chart renderer",
      schemaVersion: "2026-05-16",
      payloadSchema: { type: "object", properties: { chart: { type: "object" } } },
      actionPolicy: { allowed: ["open_coroot"] },
      redactionPolicy: { dangerousKeys: ["script"] },
      placementDefaults: ["assistant_turn"],
      samplePayloads: [{ id: "sample-1", name: "示例", artifact: { id: "a1", type: "coroot_chart", payload: { chart: "p95" } } }],
    },
    {
      id: "custom-timeline",
      name: "Custom Timeline",
      kind: "timeline",
      renderer: "agent-ui/timeline",
      status: "draft",
      builtIn: false,
      version: 3,
      payloadSchema: { type: "object" },
      actionPolicy: { allowed: ["open_case"] },
      redactionPolicy: { mode: "default" },
      placementDefaults: ["drawer"],
      samplePayloads: [],
    },
  ],
};

function jsonResponse(payload: unknown) {
  return Promise.resolve(new Response(JSON.stringify(payload), { status: 200, headers: { "Content-Type": "application/json" } }));
}

function waitForEffects() {
  return act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

function setTextAreaValue(element: HTMLTextAreaElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")?.set;
  setter?.call(element, value);
  element.dispatchEvent(new Event("input", { bubbles: true }));
}

describe("UICardManagementPage", () => {
  let container: HTMLDivElement;
  let root: Root;
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
    fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.includes("/api/v1/ui-cards/coroot-chart/preview")) {
        return jsonResponse({ valid: true, definitionId: "coroot-chart", normalizedArtifact: { titleZh: "Coroot 预览" } });
      }
      if (url.includes("/api/v1/ui-cards/coroot-chart/status") && init?.method === "PUT") {
        return jsonResponse({ id: "coroot-chart", status: "disabled", version: 2 });
      }
      return jsonResponse(listPayload);
    });
    vi.stubGlobal("fetch", fetchMock);
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
    vi.unstubAllGlobals();
  });

  it("shows registry stats, definition detail, preview, and status action", async () => {
    await act(async () => {
      root.render(
        <AppShellChromeProvider>
          <UICardManagementPage />
        </AppShellChromeProvider>,
      );
    });
    await waitForEffects();

    expect(container.textContent).toContain("总计");
    expect(container.textContent).toContain("2");
    expect(container.textContent).toContain("启用");
    expect(container.textContent).toContain("草稿");
    expect(container.textContent).toContain("已废弃");
    expect(container.textContent).toContain("已禁用");
    expect(container.textContent).toContain("内置");
    expect(container.textContent).toContain("Coroot Chart");
    expect(container.textContent).toContain("coroot_chart");
    expect(container.textContent).toContain("agent-ui/coroot-chart");
    expect(container.textContent).toContain("版本 1");
    const detailTab = Array.from(container.querySelectorAll("button")).find((button) => button.textContent === "Detail") as HTMLButtonElement;
    await act(async () => detailTab.click());
    expect(container.textContent).toContain("Payload Schema");
    expect(container.textContent).toContain("Action Policy");
    expect(container.textContent).toContain("Redaction Policy");
    expect(container.textContent).toContain("assistant_turn");

    const previewTab = Array.from(container.querySelectorAll("button")).find((button) => button.textContent === "Preview") as HTMLButtonElement;
    await act(async () => previewTab.click());
    const runPreview = Array.from(container.querySelectorAll("button")).find((button) => button.textContent === "运行 Preview") as HTMLButtonElement;
    await act(async () => runPreview.click());
    await waitForEffects();
    expect(fetchMock).toHaveBeenCalledWith(expect.stringContaining("/api/v1/ui-cards/coroot-chart/preview"), expect.objectContaining({ method: "POST" }));
    expect(container.textContent).toContain("Coroot 预览");

    await act(async () => detailTab.click());
    const statusButton = Array.from(container.querySelectorAll("button")).find((button) => button.textContent === "设为禁用") as HTMLButtonElement;
    await act(async () => statusButton.click());
    await waitForEffects();
    expect(fetchMock).toHaveBeenCalledWith(expect.stringContaining("/api/v1/ui-cards/coroot-chart/status"), expect.objectContaining({ method: "PUT" }));
  });

  it("rejects invalid preview JSON locally", async () => {
    await act(async () => {
      root.render(
        <AppShellChromeProvider>
          <UICardManagementPage />
        </AppShellChromeProvider>,
      );
    });
    await waitForEffects();

    const previewTab = Array.from(container.querySelectorAll("button")).find((button) => button.textContent === "Preview") as HTMLButtonElement;
    await act(async () => previewTab.click());
    const textarea = container.querySelector("textarea") as HTMLTextAreaElement;
    await act(async () => {
      setTextAreaValue(textarea, "{broken");
    });
    const runPreview = Array.from(container.querySelectorAll("button")).find((button) => button.textContent === "运行 Preview") as HTMLButtonElement;
    const callsBefore = fetchMock.mock.calls.length;
    await act(async () => runPreview.click());
    await waitForEffects();

    expect(fetchMock.mock.calls.length).toBe(callsBefore);
    expect(container.textContent).toContain("Preview JSON 格式不正确");
  });
});
