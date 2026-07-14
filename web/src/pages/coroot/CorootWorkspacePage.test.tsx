import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { AppShell } from "@/app/AppShell";
import { AppShellChromeProvider } from "@/app/AppShellChromeContext";
import { CorootWorkspacePage } from "@/pages/coroot/CorootWorkspacePage";

describe("CorootWorkspacePage", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ configured: true, project: "5hxbfx6p" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
    vi.restoreAllMocks();
  });

  it("renders iframe with gateway src and hides app header in workspace mode", async () => {
    await renderCorootRoute("/coroot/p/5hxbfx6p/applications");

    const frame = iframe();
    expect(frame?.getAttribute("src")).toBe("/_coroot/p/5hxbfx6p/applications?embed=1");
    expect(container.querySelector('[data-testid="app-shell-header"]')).toBeNull();
    expect(container.querySelector('[data-testid="app-shell-sidebar"]')?.textContent).toContain("返回 AIOps");
  });

  it("updates the outer aiops route from same-origin coroot postMessage events", async () => {
    await renderCorootRoute("/coroot/p/5hxbfx6p/applications");

    await act(async () => {
      window.dispatchEvent(
        new MessageEvent("message", {
          origin: window.location.origin,
          data: {
            type: "aiops.coroot.route.v1",
            projectId: "5hxbfx6p",
            view: "applications",
            id: "aiops-host-agent",
            report: "CPU",
            query: { from: "now-1h", to: "now" },
          },
        }),
      );
    });
    await flushReact();

    expect(iframe()?.getAttribute("src")).toBe("/_coroot/p/5hxbfx6p/applications/aiops-host-agent/CPU?embed=1&from=now-1h&to=now");
  });

  async function renderCorootRoute(path: string) {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false, gcTime: Infinity },
        mutations: { retry: false },
      },
    });

    await act(async () => {
      root.render(
        <QueryClientProvider client={queryClient}>
          <AppShellChromeProvider>
            <MemoryRouter initialEntries={[path]}>
              <Routes>
                <Route path="/" element={<AppShell />}>
                  <Route path="/coroot/p/:projectId/:view?/:id?/:report?" element={<CorootWorkspacePage />} />
                </Route>
              </Routes>
            </MemoryRouter>
          </AppShellChromeProvider>
        </QueryClientProvider>,
      );
    });
    await flushReact();
  }

  function iframe() {
    return container.querySelector('iframe[title="Coroot"]');
  }
});

async function flushReact() {
  await act(async () => {
    await Promise.resolve();
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}
