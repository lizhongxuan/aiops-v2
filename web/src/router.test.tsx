import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { AppShell } from "@/app/AppShell";
import { AppShellChromeProvider, useRegisterAppShellWorkspace } from "@/app/AppShellChromeContext";
import { AppRouter as RawAppRouter } from "@/router";

const routedPaths = [
  "/",
  "/protocol",
  "/incidents",
  "/incidents/incident-1",
  "/coroot",
  "/coroot/config",
  "/coroot/p/5hxbfx6p/applications",
  "/coroot/p/5hxbfx6p/logs",
  "/erp",
  "/opsgraph",
  "/opsgraph/graphs",
  "/opsgraph/graph.default",
  "/runbooks",
  "/runbooks/runbook-1",
  "/runner",
  "/runner/payment-health",
  "/postmortems/postmortem-1",
  "/terminal/host-1",
  "/settings",
  "/settings/llm",
  "/settings/runtime",
  "/settings/coroot",
  "/settings/hosts",
  "/settings/ops-manuals",
  "/settings/experience-packs",
  "/settings/agent",
  "/mcp",
  "/capabilities",
  "/approval-management",
  "/agent-ui",
  "/ui-cards",
  "/script-configs",
  "/lab",
  "/generator",
  "/debug/prompts",
];

function createTestQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: Infinity },
      mutations: { retry: false },
    },
  });
}

function AppRouter() {
  return (
    <QueryClientProvider client={createTestQueryClient()}>
      <AppShellChromeProvider>
        <RawAppRouter />
      </AppShellChromeProvider>
    </QueryClientProvider>
  );
}

describe("AppRouter", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
    HTMLElement.prototype.scrollTo = vi.fn();
    globalThis.ResizeObserver = class ResizeObserver {
      observe() {}
      unobserve() {}
      disconnect() {}
    };
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => {
      root.unmount();
    });
    container.remove();
    localStorage.clear();
    vi.restoreAllMocks();
  });

  it.each(routedPaths)("renders React shell for %s", async (path) => {
    await act(async () => {
      root.render(
        <MemoryRouter initialEntries={[path]}>
          <AppRouter />
        </MemoryRouter>,
      );
    });
    await flushRouteWork();

    if (path.startsWith("/coroot/p/")) {
      expect(container.textContent).toContain("返回 AIOps");
    } else {
      expect(container.textContent).toContain("V2");
      expect(container.textContent).toContain("AIOPS");
    }
    expect(container.textContent?.trim()).not.toBe("");
  });

  it("shows Coroot as a main navigation item", async () => {
    await act(async () => {
      root.render(
        <MemoryRouter initialEntries={["/"]}>
          <AppRouter />
        </MemoryRouter>,
      );
    });

    expect(container.textContent).toContain("Coroot");
  });

  it("moves global configuration links into the bottom settings menu", async () => {
    await act(async () => {
      root.render(
        <MemoryRouter initialEntries={["/incidents"]}>
          <AppRouter />
        </MemoryRouter>,
      );
    });
    await flushRouteWork();

    const sidebar = container.querySelector('[data-testid="app-shell-sidebar"]');
    const primaryNav = sidebar?.querySelector("nav");
    expect(primaryNav?.textContent).not.toContain("LLM 配置");
    expect(primaryNav?.textContent).not.toContain("运行时配置");

    const settingsButton = container.querySelector('[data-testid="app-shell-settings-menu-button"]') as HTMLButtonElement | null;
    expect(settingsButton).toBeTruthy();

    await act(async () => {
      settingsButton?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    const settingsMenu = container.querySelector('[data-testid="app-shell-settings-menu"]');
    expect(settingsMenu?.textContent).toContain("LLM 配置");
    expect(settingsMenu?.textContent).toContain("运行时配置");
    expect(settingsMenu?.textContent).toContain("Coroot 监控");
    expect(settingsMenu?.querySelector('a[href="/settings/llm"]')).toBeTruthy();
    expect(settingsMenu?.querySelector('a[href="/settings/runtime"]')).toBeTruthy();
    expect(settingsMenu?.querySelector('a[href="/settings/coroot"]')).toBeTruthy();
  });

  it("closes the bottom settings menu when clicking outside it", async () => {
    await act(async () => {
      root.render(
        <MemoryRouter initialEntries={["/capabilities"]}>
          <AppRouter />
        </MemoryRouter>,
      );
    });
    await flushRouteWork();

    const settingsButton = container.querySelector('[data-testid="app-shell-settings-menu-button"]') as HTMLButtonElement | null;
    expect(settingsButton).toBeTruthy();

    await act(async () => {
      settingsButton?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    expect(container.querySelector('[data-testid="app-shell-settings-menu"]')).toBeTruthy();

    await act(async () => {
      container.querySelector("main")?.dispatchEvent(new MouseEvent("pointerdown", { bubbles: true }));
    });

    expect(container.querySelector('[data-testid="app-shell-settings-menu"]')).toBeNull();
  });

  it("lets child routes register app shell workspace mode", async () => {
    function WorkspaceChild() {
      useRegisterAppShellWorkspace({
        mode: "coroot",
        sidebar: <div>返回 AIOps</div>,
        hideHeader: true,
        mainClassName: "overflow-hidden",
      });
      return <div>workspace</div>;
    }

    await act(async () => {
      root.render(
        <QueryClientProvider client={createTestQueryClient()}>
          <AppShellChromeProvider>
            <MemoryRouter initialEntries={["/workspace"]}>
              <Routes>
                <Route path="/" element={<AppShell />}>
                  <Route path="/workspace" element={<WorkspaceChild />} />
                </Route>
              </Routes>
            </MemoryRouter>
          </AppShellChromeProvider>
        </QueryClientProvider>,
      );
    });
    await flushRouteWork();

    expect(container.querySelector('[data-testid="app-shell-header"]')).toBeNull();
    expect(container.querySelector('[data-testid="app-shell-sidebar"]')?.textContent).toContain("返回 AIOps");
    expect(container.querySelector("main")?.className).toContain("overflow-hidden");
  });

  it("redirects legacy hosts route to settings hosts", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(() =>
      Promise.resolve(
        new Response(JSON.stringify({ items: [], sessions: [] }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      ),
    );

    await act(async () => {
      root.render(
        <MemoryRouter initialEntries={["/hosts"]}>
          <AppRouter />
        </MemoryRouter>,
      );
    });
    await flushRouteWork();

    expect(container.textContent).toContain("主机列表");
    expect(container.textContent).toContain("暂无主机");
  });

  it("redirects legacy experience packs route to settings ops manuals", async () => {
    await act(async () => {
      root.render(
        <MemoryRouter initialEntries={["/experience-packs"]}>
          <AppRouter />
        </MemoryRouter>,
      );
    });

    expect(container.textContent).toContain("运维手册");
    expect(container.textContent).toContain("旧入口已迁移到运维手册");
  });

  it.each(["/settings/skills", "/settings/mcp", "/settings/connectors", "/capability-center"])(
    "redirects legacy capability route %s to unified capabilities",
    async (path) => {
      await act(async () => {
        root.render(
          <MemoryRouter initialEntries={[path]}>
            <AppRouter />
          </MemoryRouter>,
        );
      });

      expect(container.textContent).toContain("能力管理");
      expect(container.textContent).toContain("Bindings");
    },
  );

  it("redirects opsgraph root to graph list before opening an editor", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({
      graphs: [{ id: "graph.default", name: "默认图谱", isDefault: true, nodeCount: 1, relationshipCount: 0 }],
    }), { status: 200, headers: { "Content-Type": "application/json" } }));

    await act(async () => {
      root.render(
        <MemoryRouter initialEntries={["/opsgraph"]}>
          <AppRouter />
        </MemoryRouter>,
      );
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    expect(container.textContent).toContain("每张图谱独立保存");
    expect(container.textContent).toContain("默认图谱");
    expect(container.textContent).not.toContain("这个图谱现在是空的");
  });

  it("can collapse the desktop navigation rail to an icon-only column", async () => {
    await act(async () => {
      root.render(
        <MemoryRouter initialEntries={["/debug/prompts"]}>
          <AppRouter />
        </MemoryRouter>,
      );
    });

    const sidebar = container.querySelector('[data-testid="app-shell-sidebar"]');
    const collapseButton = container.querySelector('[aria-label="收起侧边栏"]') as HTMLButtonElement | null;
    expect(sidebar?.getAttribute("data-collapsed")).toBe("false");
    expect(collapseButton).toBeTruthy();

    await act(async () => {
      collapseButton?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    expect(sidebar?.getAttribute("data-collapsed")).toBe("true");
    expect(sidebar?.className).toContain("w-20");
    expect(container.querySelector('[aria-label="展开侧边栏"]')).toBeTruthy();
    expect(container.querySelector('a[title="Prompt Trace"]')).toBeTruthy();
  });
});

async function flushRouteWork() {
  for (let index = 0; index < 5; index += 1) {
    await act(async () => {
      await Promise.resolve();
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
  }
}
