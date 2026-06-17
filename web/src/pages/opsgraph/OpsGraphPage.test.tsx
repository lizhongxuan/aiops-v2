import { act } from "react";
import { createRoot } from "react-dom/client";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { AppShellChromeProvider, useAppShellChrome } from "@/app/AppShellChromeContext";

import { OpsGraphPage, relationshipTypeForManualConnection } from "./OpsGraphPage";

function ChromeProbe() {
  const { headerActions, headerDescription, headerTitle } = useAppShellChrome();
  return (
    <div data-testid="chrome-probe">
      <span>{headerTitle}</span>
      <span>{headerDescription}</span>
      <div>{headerActions}</div>
    </div>
  );
}

describe("OpsGraphPage", () => {
  let container: HTMLDivElement;
  let root: ReturnType<typeof createRoot>;

  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    globalThis.ResizeObserver = class ResizeObserver {
      observe() {}
      unobserve() {}
      disconnect() {}
    };
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  it("uses deployment relationships when connecting to host or k8s targets", () => {
    const nodes = [
      { id: "service.api", type: "service" as const, name: "api" },
      { id: "middleware.redis", type: "middleware" as const, name: "redis" },
      { id: "host.a", type: "host" as const, name: "host-a" },
      { id: "k8s.prod", type: "k8s" as const, name: "prod" },
    ];

    expect(relationshipTypeForManualConnection(nodes, "middleware.redis")).toBe("depends_on");
    expect(relationshipTypeForManualConnection(nodes, "host.a")).toBe("runs_on");
    expect(relationshipTypeForManualConnection(nodes, "k8s.prod")).toBe("runs_on");
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
    vi.restoreAllMocks();
  });

  it("renders empty manual authoring editor without external source claims", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({
      graph: { id: "graph.default", name: "默认图谱", nodes: [], edges: [] },
    }), { status: 200, headers: { "Content-Type": "application/json" } }));

    await act(async () => {
      root.render(<MemoryRouter><OpsGraphPage /></MemoryRouter>);
    });

    expect(container.textContent).toContain("这个图谱现在是空的");
    const buttonLabels = Array.from(container.querySelectorAll("button")).map((button) => button.textContent?.replace(/\s+/g, "").trim());
    expect(buttonLabels.filter((label) => label === "新建服务")).toHaveLength(0);
    expect(buttonLabels.filter((label) => label === "从示例开始")).toHaveLength(0);
    expect(buttonLabels.filter((label) => label === "复制")).toHaveLength(0);
    expect(container.querySelectorAll('[data-testid="opsgraph-empty-guide"] button')).toHaveLength(0);
    expect(container.textContent).toContain("服务");
    expect(container.textContent).toContain("中间件");
    expect(container.textContent).toContain("主机");
    expect(container.textContent).toContain("K8s");
    expect(container.textContent).not.toContain("接口");
    expect(container.textContent).not.toContain("中间件集群");
    expect(container.textContent).not.toContain("业务");
    expect(container.textContent).not.toContain("Workflow");
    expect(container.textContent).not.toContain("检查器");
    expect(container.textContent).not.toContain("服务依赖、部署位置和业务影响会在这里汇总");
    expect(container.textContent).not.toContain("保存前检查重复 ID");
    expect(container.querySelector('[data-testid="opsgraph-editor-layout"]')?.className).toContain("flex-1");
    expect(container.querySelector('[data-testid="opsgraph-editor-layout"]')?.className).toContain("min-h-0");
    expect(container.querySelector('[data-testid="opsgraph-editor-layout"]')?.className).not.toContain("_320px");
    expect(container.querySelector('[data-testid="opsgraph-editor-layout"]')?.className).not.toContain("min-h-[640px]");
    expect(container.querySelector('[data-testid="opsgraph-canvas-panel"]')?.className).toContain("min-h-0");
    expect(container.querySelector('[data-testid="opsgraph-canvas-panel"]')?.className).not.toContain("min-h-[520px]");
    expect(container.textContent).not.toContain("Coroot");
    expect(container.textContent).not.toContain("HostLease");
  });

  it("registers graph title and save action in app shell chrome", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({
      graph: { id: "graph.default", name: "默认图谱", environment: "prod", nodes: [], edges: [] },
    }), { status: 200, headers: { "Content-Type": "application/json" } }));

    await act(async () => {
      root.render(
        <MemoryRouter>
          <AppShellChromeProvider>
            <OpsGraphPage />
            <ChromeProbe />
          </AppShellChromeProvider>
        </MemoryRouter>,
      );
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    const chrome = container.querySelector('[data-testid="chrome-probe"]');
    expect(chrome?.textContent).toContain("默认图谱");
    expect(chrome?.textContent).toContain("prod · 0 节点 · 0 关系");
    expect(chrome?.textContent).toContain("返回列表");
    expect(chrome?.textContent).toContain("保存");
  });

  it("creates a default service from the palette action", async () => {
    let nodes: Array<{ id: string; type: string; name: string; position: { x: number; y: number } }> = [];
    const fetchMock = vi.spyOn(globalThis, "fetch").mockImplementation(async (url, init) => {
      const requestUrl = String(url);
      if (init?.method === "POST" && requestUrl.endsWith("/entities")) {
        const payload = JSON.parse(String(init.body));
        expect(payload).toMatchObject({
          type: "service",
          name: "新服务",
          position: { x: 96, y: 96 },
        });
        nodes = [{ id: payload.id, type: payload.type, name: payload.name, position: payload.position }];
        return new Response(JSON.stringify({ node: nodes[0] }), { status: 201, headers: { "Content-Type": "application/json" } });
      }

      return new Response(JSON.stringify({
        graph: { id: "graph.default", name: "默认图谱", nodes, edges: [] },
      }), { status: 200, headers: { "Content-Type": "application/json" } });
    });

    await act(async () => {
      root.render(<MemoryRouter><OpsGraphPage /></MemoryRouter>);
    });

    const button = Array.from(container.querySelectorAll("button")).find((candidate) => candidate.textContent?.replace(/\s+/g, "").trim() === "服务");
    expect(button).toBeTruthy();

    await act(async () => {
      button?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/opsgraph/graphs/graph.default/entities",
      expect.objectContaining({ method: "POST" }),
    );
    expect(container.textContent).toContain("新服务");
  });

  it("stagers nodes created from palette clicks so they do not overlap", async () => {
    let nodes: Array<{ id: string; type: string; name: string; position: { x: number; y: number } }> = [];
    vi.spyOn(globalThis, "fetch").mockImplementation(async (url, init) => {
      const requestUrl = String(url);
      if (init?.method === "POST" && requestUrl.endsWith("/entities")) {
        const payload = JSON.parse(String(init.body));
        nodes = [...nodes, { id: payload.id, type: payload.type, name: payload.name, position: payload.position }];
        return new Response(JSON.stringify({ node: nodes[nodes.length - 1] }), { status: 201, headers: { "Content-Type": "application/json" } });
      }

      return new Response(JSON.stringify({
        graph: { id: "graph.default", name: "默认图谱", nodes, edges: [] },
      }), { status: 200, headers: { "Content-Type": "application/json" } });
    });

    await act(async () => {
      root.render(<MemoryRouter><OpsGraphPage /></MemoryRouter>);
    });

    for (const label of ["服务", "中间件", "主机", "K8s"]) {
      const button = Array.from(container.querySelectorAll("button")).find((candidate) => candidate.textContent?.replace(/\s+/g, "").trim() === label);
      await act(async () => {
        button?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
        await new Promise((resolve) => setTimeout(resolve, 0));
      });
    }

    expect(nodes.map((node) => node.position)).toEqual([
      { x: 96, y: 96 },
      { x: 396, y: 96 },
      { x: 96, y: 316 },
      { x: 396, y: 316 },
    ]);
  });

  it("renders deployment containers and middleware cluster summaries", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({
      graph: {
        id: "graph.default",
        name: "默认图谱",
        nodes: [
          { id: "service.order-api", type: "service", name: "order-api" },
          { id: "middleware.pg", type: "middleware_cluster", name: "order-postgres", collapsed: true },
          { id: "middleware.pg-0", type: "middleware_instance", name: "pg-0", parentId: "middleware.pg", properties: { role: "primary" } },
          { id: "host.a", type: "host", name: "host-a", container: true },
        ],
        edges: [
          { id: "e1", from: "service.order-api", type: "depends_on", to: "middleware.pg" },
          { id: "e2", from: "middleware.pg", type: "contains", to: "middleware.pg-0" },
          { id: "e3", from: "middleware.pg-0", type: "runs_on", to: "host.a" },
        ],
      },
    }), { status: 200, headers: { "Content-Type": "application/json" } }));

    await act(async () => {
      root.render(<MemoryRouter><OpsGraphPage /></MemoryRouter>);
    });

    expect(container.textContent).toContain("order-api");
    expect(container.textContent).toContain("order-postgres");
    expect(container.textContent).toContain("1 instances");
    expect(container.textContent).toContain("host-a");
    expect(container.querySelector('[data-testid="opsgraph-canvas"]')?.className).toContain("h-full");
    expect(container.querySelector('[data-testid="opsgraph-canvas"]')?.className).not.toContain("h-[620px]");
  });
});
