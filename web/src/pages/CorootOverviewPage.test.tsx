import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { AppShellChromeProvider } from "@/app/AppShellChromeContext";
import { AppRouter } from "@/router";

function jsonResponse(payload: unknown) {
  return Promise.resolve(new Response(JSON.stringify(payload), { status: 200, headers: { "Content-Type": "application/json" } }));
}

function mockFetch(input: RequestInfo | URL, init?: RequestInit) {
  const url = String(input);
  if (url.includes("/api/v1/coroot/config") && init?.method === "PUT") {
    return jsonResponse({ configured: true, baseUrl: "https://saved-coroot.example", project: "prod", tokenConfigured: true });
  }
  if (url.includes("/api/v1/coroot/config")) {
    return jsonResponse({ configured: true, baseUrl: "https://coroot.example", project: "5hxbfx6p", tokenConfigured: true, lastSuccessAt: "2026-05-12T09:30:00+08:00" });
  }
  if (url.includes("/api/v1/mcp/servers")) {
    return jsonResponse({ items: [{ name: "coroot-rca", status: "connected", toolCount: 5, resourceCount: 2 }] });
  }
  if (url.includes("/api/v1/coroot/evidence")) {
    return jsonResponse({
      items: [
        {
          evidence_ref: "ev-coroot-latency",
          title: "checkout p95 延迟",
          case_id: "incident-1",
          summary: "p95 高于基线",
        },
      ],
    });
  }
  if (url.includes("/api/v1/agent-ui-artifacts")) {
    return jsonResponse({
      items: [
        {
          id: "coroot-checkout-latency-chart",
          type: "coroot_chart",
          title: "checkout 延迟图",
          case_id: "incident-1",
        },
      ],
    });
  }
  if (url.includes("/api/v1/coroot/test-connection") && init?.method === "POST") {
    return jsonResponse({ ok: true, status: "connected" });
  }
  return jsonResponse({});
}

async function flush() {
  await act(async () => {
    for (let index = 0; index < 5; index += 1) await Promise.resolve();
  });
}

describe("CorootOverviewPage", () => {
  let container: HTMLDivElement;
  let root: Root;

  async function renderCorootRoute() {
    await act(async () => {
      root.render(
        <AppShellChromeProvider>
          <MemoryRouter initialEntries={["/coroot"]}>
            <AppRouter />
          </MemoryRouter>
        </AppShellChromeProvider>,
      );
    });
    await flush();
  }

  beforeEach(() => {
    (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
    vi.spyOn(globalThis, "fetch").mockImplementation(mockFetch as typeof fetch);
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
    vi.restoreAllMocks();
  });

  it("focuses Coroot on config, MCP, RCA skills, evidence and AI Chat artifacts", async () => {
    await renderCorootRoute();

    expect(container.textContent).toContain("Coroot 观测");
    expect(container.textContent).toContain("Coroot 配置");
    expect(container.textContent).toContain("https://coroot.example");
    expect(container.textContent).toContain("Project ID");
    expect(container.textContent).toContain("API Key");
    expect(container.textContent).toContain("MCP 状态");
    expect(container.textContent).toContain("coroot-rca");
    expect(container.textContent).toContain("RCA Skills");
    expect(container.textContent).toContain("Coroot RCA 已启用");
    expect(container.textContent).toContain("最近 Evidence");
    expect(container.textContent).toContain("ev-coroot-latency");
    expect(container.textContent).toContain("最近发送到 AI Chat 的图表");
    expect(container.textContent).toContain("coroot-checkout-latency-chart");
    expect(container.textContent).not.toContain("Dashboard");
  });

  it("tests the Coroot connection through the configured endpoint", async () => {
    await renderCorootRoute();

    const button = Array.from(container.querySelectorAll("button")).find((item) => item.textContent?.includes("测试连接"));
    expect(button).toBeTruthy();
    await act(async () => button?.dispatchEvent(new MouseEvent("click", { bubbles: true })));
    await flush();

    expect(globalThis.fetch).toHaveBeenCalledWith(
      expect.stringContaining("/api/v1/coroot/test-connection"),
      expect.objectContaining({ method: "POST" }),
    );
    expect(container.textContent).toContain("连接正常");
  });

  it("saves Coroot connection settings from the observability page", async () => {
    await renderCorootRoute();

    const baseUrl = container.querySelector<HTMLInputElement>('input[name="baseUrl"]');
    const project = container.querySelector<HTMLInputElement>('input[name="project"]');
    const token = container.querySelector<HTMLInputElement>('input[name="token"]');
    expect(baseUrl).toBeTruthy();
    expect(project).toBeTruthy();
    expect(token).toBeTruthy();

    await act(async () => {
      setInputValue(baseUrl!, "https://saved-coroot.example");
      setInputValue(project!, "prod");
      setInputValue(token!, "new-token");
    });

    const button = Array.from(container.querySelectorAll("button")).find((item) => item.textContent?.includes("保存配置"));
    expect(button).toBeTruthy();
    await act(async () => button?.dispatchEvent(new MouseEvent("click", { bubbles: true })));
    await flush();

    expect(globalThis.fetch).toHaveBeenCalledWith(
      expect.stringContaining("/api/v1/coroot/config"),
      expect.objectContaining({
        method: "PUT",
        body: expect.stringContaining("https://saved-coroot.example"),
      }),
    );
    expect(container.textContent).toContain("配置已保存");
  });
});

function setInputValue(input: HTMLInputElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set;
  setter?.call(input, value);
  input.dispatchEvent(new Event("input", { bubbles: true }));
}
