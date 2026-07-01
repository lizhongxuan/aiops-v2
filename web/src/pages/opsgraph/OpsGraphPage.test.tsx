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

  it("uses topology dependency relationships for manual connections", () => {
    const nodes = [
      { id: "service.api", type: "service" as const, name: "api" },
      { id: "middleware.redis", type: "middleware" as const, name: "redis" },
      { id: "host.a", type: "host" as const, name: "host-a" },
      { id: "k8s.prod", type: "k8s" as const, name: "prod" },
    ];

    expect(relationshipTypeForManualConnection(nodes, "middleware.redis")).toBe("depends_on");
    expect(relationshipTypeForManualConnection(nodes, "host.a")).toBe("depends_on");
    expect(relationshipTypeForManualConnection(nodes, "k8s.prod")).toBe("depends_on");
    expect(relationshipTypeForManualConnection(nodes, "middleware.redis", "publishes")).toBe("publishes");
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
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
    expect(container.textContent).toContain("业务服务");
    expect(container.textContent).toContain("通用中间件");
    expect(container.textContent).toContain("Postgres");
    expect(container.textContent).toContain("外部服务");
    expect(container.textContent).not.toContain("主机");
    expect(container.textContent).not.toContain("K8s");
    expect(container.textContent).not.toContain("接口");
    expect(container.textContent).not.toContain("中间件集群");
    expect(container.textContent).not.toContain("业务能力");
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

  it("exports and imports graph YAML from app shell actions", async () => {
    const createObjectURL = vi.fn(() => "blob:opsgraph-yaml");
    const revokeObjectURL = vi.fn();
    const anchorClick = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});
    vi.stubGlobal("URL", {
      ...globalThis.URL,
      createObjectURL,
      revokeObjectURL,
    });

    const importedGraph = {
      id: "graph.default",
      name: "导入后图谱",
      nodes: [{ id: "service.imported-api", type: "service", name: "imported-api" }],
      edges: [],
    };
    const fetchMock = vi.spyOn(globalThis, "fetch").mockImplementation(async (url, init) => {
      const requestUrl = String(url);
      if (requestUrl.endsWith("/yaml") && init?.method === "GET") {
        return new Response("name: 默认图谱\nnodes: []\nedges: []\n", {
          status: 200,
          headers: { "Content-Type": "text/yaml" },
        });
      }
      if (requestUrl.endsWith("/yaml") && init?.method === "PUT") {
        expect(String(init.body)).toContain("name: 导入后图谱");
        return new Response(JSON.stringify({ graph: importedGraph }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }
      return new Response(JSON.stringify({
        graph: { id: "graph.default", name: "默认图谱", nodes: [], edges: [] },
      }), { status: 200, headers: { "Content-Type": "application/json" } });
    });

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
    const exportButton = Array.from(chrome?.querySelectorAll("button") || []).find((candidate) => candidate.textContent?.replace(/\s+/g, "").trim() === "导出YAML");
    expect(exportButton).toBeTruthy();
    await act(async () => {
      exportButton?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/opsgraph/graphs/graph.default/yaml",
      expect.objectContaining({ method: "GET" }),
    );
    expect(createObjectURL).toHaveBeenCalled();
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:opsgraph-yaml");
    expect(anchorClick).toHaveBeenCalled();

    const importInput = chrome?.querySelector('[data-testid="opsgraph-yaml-import-input"]') as HTMLInputElement | null;
    expect(importInput).toBeTruthy();
    const file = new File(["name: 导入后图谱\nnodes:\n  - id: service.imported-api\n    type: service\n    name: imported-api\nedges: []\n"], "graph.yaml", { type: "text/yaml" });
    Object.defineProperty(importInput, "files", { value: [file], configurable: true });

    await act(async () => {
      importInput?.dispatchEvent(new Event("change", { bubbles: true }));
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/opsgraph/graphs/graph.default/yaml",
      expect.objectContaining({ method: "PUT", body: expect.stringContaining("name: 导入后图谱") }),
    );
    expect(container.textContent).toContain("导入完成");
    expect(container.textContent).toContain("imported-api");
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
          properties: { environment: "prod", ports: "8080/http" },
        });
        expect(payload.container).toBeUndefined();
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

    const button = Array.from(container.querySelectorAll("button")).find((candidate) => candidate.textContent?.replace(/\s+/g, "").trim() === "业务服务");
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

  it("adds numeric suffixes when newly created node names already exist", async () => {
    const created: Array<{ id: string; type: string; subtype?: string; name: string; position: { x: number; y: number } }> = [];
    const existing = [
      { id: "service.existing-1", type: "service", name: "新服务", position: { x: 96, y: 96 } },
      { id: "service.existing-2", type: "service", name: "新服务-2", position: { x: 396, y: 96 } },
      { id: "middleware.existing-1", type: "middleware", subtype: "generic", name: "新中间件", position: { x: 96, y: 316 } },
    ];
    vi.spyOn(globalThis, "fetch").mockImplementation(async (url, init) => {
      const requestUrl = String(url);
      if (init?.method === "POST" && requestUrl.endsWith("/entities")) {
        const payload = JSON.parse(String(init.body));
        created.push(payload);
        return new Response(JSON.stringify({ node: payload }), { status: 201, headers: { "Content-Type": "application/json" } });
      }

      return new Response(JSON.stringify({
        graph: { id: "graph.default", name: "默认图谱", nodes: [...existing, ...created], edges: [] },
      }), { status: 200, headers: { "Content-Type": "application/json" } });
    });

    await act(async () => {
      root.render(<MemoryRouter><OpsGraphPage /></MemoryRouter>);
    });

    for (const label of ["业务服务", "通用中间件"]) {
      const button = Array.from(container.querySelectorAll("button")).find((candidate) => candidate.textContent?.replace(/\s+/g, "").trim() === label);
      expect(button).toBeTruthy();
      await act(async () => {
        button?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
        await new Promise((resolve) => setTimeout(resolve, 0));
      });
    }

    expect(created.map((node) => node.name)).toEqual(["新服务-3", "新中间件-2"]);
  });

  it("creates concrete middleware as middleware with subtype", async () => {
    const created: Array<{ id: string; type: string; subtype?: string; name: string; position: { x: number; y: number }; properties?: Record<string, string>; container?: boolean }> = [];
    vi.spyOn(globalThis, "fetch").mockImplementation(async (url, init) => {
      const requestUrl = String(url);
      if (init?.method === "POST" && requestUrl.endsWith("/entities")) {
        const payload = JSON.parse(String(init.body));
        created.push(payload);
        return new Response(JSON.stringify({ node: payload }), { status: 201, headers: { "Content-Type": "application/json" } });
      }

      return new Response(JSON.stringify({
        graph: { id: "graph.default", name: "默认图谱", nodes: created, edges: [] },
      }), { status: 200, headers: { "Content-Type": "application/json" } });
    });

    await act(async () => {
      root.render(<MemoryRouter><OpsGraphPage /></MemoryRouter>);
    });

    const postgres = Array.from(container.querySelectorAll("button")).find((candidate) => candidate.textContent?.replace(/\s+/g, "").trim() === "Postgres");
    await act(async () => {
      postgres?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    expect(created[0]).toMatchObject({
      type: "middleware",
      subtype: "postgres",
      name: "新Postgres",
      properties: { ports: "5432/postgres", role: "primary" },
    });
    expect(created[0].container).toBeUndefined();
  });

  it("stagers nodes created from palette clicks so they do not overlap", async () => {
    let nodes: Array<{ id: string; type: string; subtype?: string; name: string; position: { x: number; y: number } }> = [];
    vi.spyOn(globalThis, "fetch").mockImplementation(async (url, init) => {
      const requestUrl = String(url);
      if (init?.method === "POST" && requestUrl.endsWith("/entities")) {
        const payload = JSON.parse(String(init.body));
        nodes = [...nodes, { id: payload.id, type: payload.type, subtype: payload.subtype, name: payload.name, position: payload.position }];
        return new Response(JSON.stringify({ node: nodes[nodes.length - 1] }), { status: 201, headers: { "Content-Type": "application/json" } });
      }

      return new Response(JSON.stringify({
        graph: { id: "graph.default", name: "默认图谱", nodes, edges: [] },
      }), { status: 200, headers: { "Content-Type": "application/json" } });
    });

    await act(async () => {
      root.render(<MemoryRouter><OpsGraphPage /></MemoryRouter>);
    });

    for (const label of ["业务服务", "Postgres", "Redis", "外部服务"]) {
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

  it("renders subtype-aware topology node cards", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({
      graph: {
        id: "graph.default",
        name: "服务拓扑",
        nodes: [
          { id: "service.order-api", type: "service", name: "order-api", properties: { k8sCluster: "prod-k8s", ports: "8080/http" } },
          { id: "middleware.pg", type: "middleware", subtype: "postgres", name: "order-postgres", properties: { ports: "5432/postgres", role: "primary" } },
          { id: "external.sms", type: "external", name: "sms-provider", properties: { domain: "sms.example.com" } },
        ],
        edges: [{ id: "e1", from: "service.order-api", type: "depends_on", to: "middleware.pg", properties: { protocol: "postgres", port: "5432" } }],
      },
    }), { status: 200, headers: { "Content-Type": "application/json" } }));

    await act(async () => {
      root.render(<MemoryRouter><OpsGraphPage /></MemoryRouter>);
    });

    expect(container.textContent).toContain("order-api");
    expect(container.textContent).toContain("prod-k8s");
    expect(container.textContent).toContain("Postgres");
    expect(container.textContent).toContain("primary");
    expect(container.textContent).toContain("5432/postgres");
    expect(container.textContent).toContain("sms.example.com");
  });

  it("shows selected node relationship summary and LLM preview entry point", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({
      graph: {
        id: "graph.default",
        name: "服务拓扑",
        nodes: [
          { id: "service.order-api", type: "service", name: "order-api", properties: { k8sCluster: "prod-k8s", namespace: "erp", ports: "8080/http" } },
          { id: "middleware.pg", type: "middleware", subtype: "postgres", name: "order-postgres" },
        ],
        edges: [{ id: "e1", from: "service.order-api", type: "depends_on", to: "middleware.pg" }],
      },
    }), { status: 200, headers: { "Content-Type": "application/json" } }));

    await act(async () => {
      root.render(<MemoryRouter><OpsGraphPage /></MemoryRouter>);
    });

    await act(async () => {
      container.querySelector('[data-id="service.order-api"]')?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    expect(container.textContent).toContain("上游");
    expect(container.textContent).toContain("下游");
    expect(container.textContent).toContain("prod-k8s / erp");
    expect(container.textContent).toContain("LLM 上下文");
    expect(container.textContent).toContain("编辑属性");
  });

  it("saves an upstream-to-downstream layout when arranging the canvas", async () => {
    let savedLayout: any = null;
    vi.spyOn(globalThis, "fetch").mockImplementation(async (url, init) => {
      const requestUrl = String(url);
      if (init?.method === "POST" && requestUrl.endsWith("/layout")) {
        savedLayout = JSON.parse(String(init.body));
        return new Response(JSON.stringify({ ok: true }), { status: 200, headers: { "Content-Type": "application/json" } });
      }
      return new Response(JSON.stringify({
        graph: {
          id: "graph.default",
          name: "服务拓扑",
          nodes: [
            { id: "middleware.pg", type: "middleware", subtype: "postgres", name: "order-postgres", position: { x: 10, y: 10 } },
            { id: "service.order-api", type: "service", name: "order-api", position: { x: 10, y: 10 } },
            { id: "middleware.nginx", type: "middleware", subtype: "nginx", name: "edge-nginx", position: { x: 10, y: 10 } },
          ],
          edges: [
            { id: "e1", from: "middleware.nginx", type: "proxies_to", to: "service.order-api" },
            { id: "e2", from: "service.order-api", type: "depends_on", to: "middleware.pg" },
          ],
        },
      }), { status: 200, headers: { "Content-Type": "application/json" } });
    });

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
    const arrangeButton = Array.from(chrome?.querySelectorAll("button") || []).find((candidate) => candidate.textContent?.replace(/\s+/g, "").trim() === "整理布局");
    expect(arrangeButton).toBeTruthy();

    await act(async () => {
      arrangeButton?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    const positions = new Map(savedLayout?.nodes?.map((node: any) => [node.id, node.position]));
    expect(positions.get("middleware.nginx")).toEqual({ x: 96, y: 96 });
    expect(positions.get("service.order-api")).toEqual({ x: 416, y: 96 });
    expect(positions.get("middleware.pg")).toEqual({ x: 736, y: 96 });
  });

  it("hides legacy deployment nodes because deployment location is a node property", async () => {
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
    expect(container.textContent).not.toContain("host-a");
    expect(container.textContent).not.toContain("主机");
    expect(container.textContent).not.toContain("1 instances");
    expect(container.querySelector('[data-testid="opsgraph-canvas"]')?.className).toContain("h-full");
    expect(container.querySelector('[data-testid="opsgraph-canvas"]')?.className).not.toContain("h-[620px]");
  });
});
