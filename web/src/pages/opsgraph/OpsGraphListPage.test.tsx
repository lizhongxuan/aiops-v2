import { act } from "react";
import { createRoot } from "react-dom/client";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { OpsGraphListPage } from "./OpsGraphListPage";

describe("OpsGraphListPage", () => {
  let container: HTMLDivElement;
  let root: ReturnType<typeof createRoot>;

  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
    vi.restoreAllMocks();
  });

  it("renders graph list and primary actions", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({
      graphs: [{ id: "graph.default", name: "默认图谱", isDefault: true, nodeCount: 2, relationshipCount: 1, environment: "prod" }],
    }), { status: 200, headers: { "Content-Type": "application/json" } }));

    await act(async () => {
      root.render(<MemoryRouter><OpsGraphListPage /></MemoryRouter>);
    });

    expect(container.textContent).toContain("OpsGraph");
    expect(container.textContent).toContain("默认图谱");
    expect(container.textContent).toContain("新建图谱");
    expect(container.textContent).toContain("从示例开始");
  });

  it("creates a graph from the list before entering the editor", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockImplementation(async (url, init) => {
      if (init?.method === "POST") {
        const payload = JSON.parse(String(init.body));
        expect(payload.name).toBe("新建图谱");
        expect(payload.id).toMatch(/^graph\.manual-/);
        return new Response(JSON.stringify({ graph: { id: payload.id, name: payload.name, nodes: [], edges: [] } }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }
      return new Response(JSON.stringify({ graphs: [] }), { status: 200, headers: { "Content-Type": "application/json" } });
    });

    await act(async () => {
      root.render(
        <MemoryRouter initialEntries={["/opsgraph/graphs"]}>
          <Routes>
            <Route path="/opsgraph/graphs" element={<OpsGraphListPage />} />
            <Route path="/opsgraph/:graphId" element={<div>进入画布编排</div>} />
          </Routes>
        </MemoryRouter>,
      );
    });

    const createButton = Array.from(container.querySelectorAll("button")).find((button) => button.textContent?.replace(/\s+/g, "").trim() === "新建图谱");
    expect(createButton).toBeTruthy();

    await act(async () => {
      createButton?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/opsgraph/graphs",
      expect.objectContaining({ method: "POST" }),
    );
    expect(container.textContent).toContain("进入画布编排");
  });

  it("creates an example graph before entering the editor", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockImplementation(async (url, init) => {
      if (init?.method === "POST") {
        const payload = JSON.parse(String(init.body));
        expect(payload.name).toBe("示例图谱");
        expect(payload.nodes.length).toBeGreaterThanOrEqual(4);
        expect(payload.edges.length).toBeGreaterThanOrEqual(3);
        return new Response(JSON.stringify({ graph: { id: payload.id, name: payload.name, nodes: payload.nodes, edges: payload.edges } }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }
      return new Response(JSON.stringify({ graphs: [] }), { status: 200, headers: { "Content-Type": "application/json" } });
    });

    await act(async () => {
      root.render(
        <MemoryRouter initialEntries={["/opsgraph/graphs"]}>
          <Routes>
            <Route path="/opsgraph/graphs" element={<OpsGraphListPage />} />
            <Route path="/opsgraph/:graphId" element={<div>进入示例画布</div>} />
          </Routes>
        </MemoryRouter>,
      );
    });

    const exampleButton = Array.from(container.querySelectorAll("button")).find((button) => button.textContent?.replace(/\s+/g, "").trim() === "从示例开始");
    expect(exampleButton).toBeTruthy();

    await act(async () => {
      exampleButton?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/opsgraph/graphs",
      expect.objectContaining({ method: "POST" }),
    );
    expect(container.textContent).toContain("进入示例画布");
  });
});
