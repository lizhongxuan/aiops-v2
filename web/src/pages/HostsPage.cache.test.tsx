import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { AppShellChromeProvider } from "@/app/AppShellChromeContext";
import { HostsPage } from "@/pages/HostsPage";
import { queryKeys } from "@/queries/queryKeys";

function createTestQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: Infinity },
      mutations: { retry: false },
    },
  });
}

function renderHostsPage(queryClient = createTestQueryClient()) {
  const container = document.createElement("div");
  document.body.appendChild(container);
  const root = createRoot(container);
  act(() => {
    root.render(
      <QueryClientProvider client={queryClient}>
        <AppShellChromeProvider>
          <MemoryRouter initialEntries={["/settings/hosts"]}>
            <HostsPage />
          </MemoryRouter>
        </AppShellChromeProvider>
      </QueryClientProvider>,
    );
  });
  return { container, root, queryClient };
}

function warmHostsPayload() {
  return {
    items: [
      {
        id: "host-warm",
        address: "10.0.0.9",
        sshUser: "root",
        status: "online",
        agentStatus: "online",
        sshStatus: "ok",
        labels: {},
      },
    ],
  };
}

function seedWarmData(
  queryClient: QueryClient,
  options: { stale?: boolean } = {},
) {
  const setOptions = options.stale
    ? { updatedAt: Date.now() - 60_000 }
    : undefined;
  queryClient.setQueryData(
    queryKeys.hosts.list(),
    warmHostsPayload(),
    setOptions,
  );
  queryClient.setQueryData(
    queryKeys.sessions.list(),
    { sessions: [] },
    setOptions,
  );
  queryClient.setQueryData(
    queryKeys.terminalSessions.list(),
    { sessions: [] },
    setOptions,
  );
}

describe("HostsPage cache behavior", () => {
  let fetchSpy: ReturnType<typeof vi.spyOn>;
  let root: Root | undefined;

  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    fetchSpy = vi.spyOn(globalThis, "fetch");
  });

  afterEach(() => {
    act(() => {
      root?.unmount();
    });
    document.body.innerHTML = "";
    fetchSpy.mockRestore();
  });

  it("shows cold loading when no cache exists", () => {
    fetchSpy.mockReturnValue(new Promise<Response>(() => {}));

    const rendered = renderHostsPage();
    root = rendered.root;

    expect(rendered.container.textContent).toContain("加载主机列表");
  });

  it("keeps the cached table visible while refreshing stale data in the background", () => {
    fetchSpy.mockReturnValue(new Promise<Response>(() => {}));
    const queryClient = createTestQueryClient();
    seedWarmData(queryClient, { stale: true });

    const rendered = renderHostsPage(queryClient);
    root = rendered.root;

    expect(rendered.container.textContent).toContain("10.0.0.9 / root");
    expect(rendered.container.textContent).toContain("正在后台刷新");
    expect(rendered.container.textContent).not.toContain("加载主机列表");
  });

  it("keeps cached hosts visible when a background refresh fails", async () => {
    fetchSpy.mockRejectedValue(new Error("hosts unavailable"));
    const queryClient = createTestQueryClient();
    seedWarmData(queryClient, { stale: true });

    const rendered = renderHostsPage(queryClient);
    root = rendered.root;

    await act(async () => {
      await flushMicrotasks();
    });

    expect(rendered.container.textContent).toContain("10.0.0.9 / root");
    expect(rendered.container.textContent).toContain("hosts unavailable");
    expect(rendered.container.textContent).not.toContain("加载主机列表");
  });

  it("removes a deleted host from cached data before background validation", async () => {
    fetchSpy.mockImplementation((input, init) => {
      if (String(input).includes("/api/v1/hosts/host-warm") && init?.method === "DELETE") {
        return Promise.resolve(
          new Response(JSON.stringify({ ok: true }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }
      return new Promise<Response>(() => {});
    });
    const queryClient = createTestQueryClient();
    seedWarmData(queryClient);

    const rendered = renderHostsPage(queryClient);
    root = rendered.root;
    const deleteButton = rendered.container.querySelector(
      '[aria-label="删除主机 host-warm"]',
    ) as HTMLButtonElement | null;
    expect(deleteButton).toBeTruthy();

    await act(async () => {
      deleteButton?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
      await flushMicrotasks();
    });
    const confirmButton = Array.from(
      document.body.querySelectorAll("button"),
    ).find((button) => button.textContent?.trim() === "删除");
    expect(confirmButton).toBeTruthy();

    await act(async () => {
      confirmButton?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
      await flushMicrotasks();
    });

    expect(rendered.container.textContent).not.toContain("10.0.0.9 / root");
    expect(document.body.textContent).toContain("主机已删除");
  });
});

async function flushMicrotasks() {
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
  await new Promise((resolve) => setTimeout(resolve, 0));
}
