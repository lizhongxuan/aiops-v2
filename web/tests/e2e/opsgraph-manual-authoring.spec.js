// @ts-check
import { expect, test } from "@playwright/test";

async function installApiMocks(page, routes) {
  const mockRoutes = routes;
  await page.route("**/api/v1/state", (route) => route.fulfill({ json: {} }));
  await page.route("**/api/v1/sessions", (route) => route.fulfill({ json: { sessions: [] } }));
  await page.route("**/api/v1/hosts", (route) => route.fulfill({ json: { hosts: [] } }));
  await page.route("**/api/v1/llm-config", (route) => route.fulfill({ json: { provider: "mock", model: "mock", configured: true } }));
  await page.route("**/api/v1/opsgraph/graphs", (route) => route.fulfill({ json: mockRoutes["/api/v1/opsgraph/graphs"] || { graphs: [] } }));
  await page.route("**/api/v1/opsgraph/graphs/*/entities", async (route) => {
    const url = new URL(route.request().url());
    const graphKey = url.pathname.replace(/\/entities$/, "");
    const graphPayload = mockRoutes[graphKey] || { graph: { id: "graph.default", name: "默认图谱", nodes: [], edges: [] } };
    const node = route.request().postDataJSON();
    graphPayload.graph.nodes.push(node);
    mockRoutes[graphKey] = graphPayload;
    route.fulfill({ json: { node } });
  });
  await page.route("**/api/v1/opsgraph/graphs/*/relationships", async (route) => {
    const url = new URL(route.request().url());
    const graphKey = url.pathname.replace(/\/relationships$/, "");
    const graphPayload = mockRoutes[graphKey] || { graph: { id: "graph.default", name: "默认图谱", nodes: [], edges: [] } };
    const relationship = route.request().postDataJSON();
    graphPayload.graph.edges.push(relationship);
    mockRoutes[graphKey] = graphPayload;
    route.fulfill({ json: { relationship } });
  });
  await page.route("**/api/v1/opsgraph/graphs/*/layout", async (route) => {
    const url = new URL(route.request().url());
    const graphKey = url.pathname.replace(/\/layout$/, "");
    const graphPayload = mockRoutes[graphKey] || { graph: { id: "graph.default", name: "默认图谱", nodes: [], edges: [] } };
    const layout = route.request().postDataJSON();
    mockRoutes.__layoutRequests?.push(layout);
    const positions = new Map((layout.nodes || []).map((node) => [node.id, node.position]));
    graphPayload.graph.nodes = graphPayload.graph.nodes.map((node) => (
      positions.has(node.id) ? { ...node, position: positions.get(node.id) } : node
    ));
    graphPayload.graph.viewport = layout.viewport;
    mockRoutes[graphKey] = graphPayload;
    route.fulfill({ json: { graph: graphPayload.graph } });
  });
  await page.route("**/api/v1/opsgraph/graphs/*", (route) => {
    const url = new URL(route.request().url());
    const key = url.pathname;
    route.fulfill({ json: mockRoutes[key] || { graph: { id: "graph.default", name: "默认图谱", nodes: [], edges: [] } } });
  });
}

test.describe("OpsGraph manual authoring", () => {
  test("renders graph list and empty manual editor", async ({ page }) => {
    await installApiMocks(page, {
      "/api/v1/opsgraph/graphs": { graphs: [{ id: "graph.default", name: "默认图谱", isDefault: true, nodeCount: 0, relationshipCount: 0 }] },
      "/api/v1/opsgraph/graphs/graph.default": { graph: { id: "graph.default", name: "默认图谱", nodes: [], edges: [] } },
    });

    await page.goto("/opsgraph");
    await expect(page.getByText("默认图谱")).toBeVisible();
    await expect(page.getByText("每张图谱独立保存")).toBeVisible();

    await page.getByRole("link", { name: "打开" }).click();
    await expect(page.getByText("这个图谱现在是空的")).toBeVisible();
    await expect(page.getByRole("link", { name: "返回列表" })).toBeVisible();
    await expect(page.getByRole("button", { name: "保存" })).toBeVisible();
    await expect(page.getByRole("button", { name: "服务" })).toBeVisible();
    await expect(page.getByRole("button", { name: "接口" })).toHaveCount(0);
    await expect(page.getByRole("button", { name: "中间件集群" })).toHaveCount(0);
    await expect(page.getByRole("button", { name: "业务" })).toHaveCount(0);
    await expect(page.getByRole("button", { name: "Workflow" })).toHaveCount(0);
    const layoutBox = await page.getByTestId("opsgraph-editor-layout").boundingBox();
    const viewport = page.viewportSize();
    expect(layoutBox?.height).toBeGreaterThan(500);
    expect(viewport && layoutBox ? viewport.height - (layoutBox.y + layoutBox.height) : 999).toBeLessThanOrEqual(28);
    await expect(page).toHaveScreenshot("opsgraph-empty-manual-editor.png", { fullPage: true });
  });

  test("renders middleware cluster deployment distribution", async ({ page }) => {
    await installApiMocks(page, {
      "/api/v1/opsgraph/graphs/graph.default": {
        graph: {
          id: "graph.default",
          name: "默认图谱",
          nodes: [
            { id: "service.order-api", type: "service", name: "order-api", position: { x: 80, y: 120 } },
            { id: "middleware.pg", type: "middleware_cluster", name: "order-postgres", collapsed: true, position: { x: 360, y: 120 } },
            { id: "middleware.pg-0", type: "middleware_instance", name: "pg-0", parentId: "middleware.pg", properties: { role: "primary" } },
            { id: "middleware.pg-1", type: "middleware_instance", name: "pg-1", parentId: "middleware.pg", properties: { role: "replica" } },
            { id: "host.a", type: "host", name: "host-a", container: true, position: { x: 320, y: 320 } },
            { id: "host.b", type: "host", name: "host-b", container: true, position: { x: 560, y: 320 } },
          ],
          edges: [
            { id: "e1", from: "service.order-api", type: "depends_on", to: "middleware.pg" },
            { id: "e2", from: "middleware.pg", type: "contains", to: "middleware.pg-0" },
            { id: "e3", from: "middleware.pg", type: "contains", to: "middleware.pg-1" },
            { id: "e4", from: "middleware.pg-0", type: "runs_on", to: "host.a" },
            { id: "e5", from: "middleware.pg-1", type: "runs_on", to: "host.b" },
          ],
        },
      },
    });

    await page.goto("/opsgraph/graph.default");
    const clusterNode = page.getByTestId("rf__node-middleware.pg");
    await expect(clusterNode.getByText("order-postgres")).toBeVisible();
    await expect(clusterNode.getByText("跨主机")).toBeVisible();
    await expect(clusterNode.getByText("2 instances / 2 hosts")).toBeVisible();
    await expect(page).toHaveScreenshot("opsgraph-cluster-deployment.png", { fullPage: true });
  });

  test("keeps many palette-created nodes visible in the canvas", async ({ page }) => {
    await installApiMocks(page, {
      "/api/v1/opsgraph/graphs/graph.default": {
        graph: { id: "graph.default", name: "默认图谱", nodes: [], edges: [] },
      },
    });

    await page.goto("/opsgraph/graph.default");
    for (const label of ["服务", "服务", "中间件", "中间件", "中间件", "主机", "主机", "K8s"]) {
      await page.getByRole("button", { name: label, exact: true }).click();
    }

    await expect(page.getByText("8 个节点")).toBeVisible();
    const canvasBox = await page.getByTestId("opsgraph-canvas").boundingBox();
    expect(canvasBox).toBeTruthy();
    const nodeBoxes = await page.locator(".react-flow__node").evaluateAll((nodes) =>
      nodes.map((node) => {
        const rect = node.getBoundingClientRect();
        return { x: rect.x, y: rect.y, width: rect.width, height: rect.height };
      }),
    );

    expect(nodeBoxes).toHaveLength(8);
    for (const box of nodeBoxes) {
      expect(box.x).toBeGreaterThanOrEqual((canvasBox?.x || 0) - 1);
      expect(box.y).toBeGreaterThanOrEqual((canvasBox?.y || 0) - 1);
      expect(box.x + box.width).toBeLessThanOrEqual((canvasBox?.x || 0) + (canvasBox?.width || 0) + 1);
      expect(box.y + box.height).toBeLessThanOrEqual((canvasBox?.y || 0) + (canvasBox?.height || 0) + 1);
    }
  });

  test("keeps the palette and canvas side by side on desktop widths", async ({ page }) => {
    await page.setViewportSize({ width: 1242, height: 964 });
    await installApiMocks(page, {
      "/api/v1/opsgraph/graphs/graph.default": {
        graph: {
          id: "graph.default",
          name: "默认图谱",
          nodes: [{ id: "service.api", type: "service", name: "api", position: { x: 96, y: 96 } }],
          edges: [],
        },
      },
    });

    await page.goto("/opsgraph/graph.default");
    const asideBox = await page.locator('[data-testid="opsgraph-editor-layout"] > aside').boundingBox();
    const canvasBox = await page.getByTestId("opsgraph-canvas").boundingBox();

    expect(asideBox).toBeTruthy();
    expect(canvasBox).toBeTruthy();
    expect(canvasBox?.x).toBeGreaterThan((asideBox?.x || 0) + (asideBox?.width || 0));
    expect(canvasBox?.height).toBeGreaterThan(700);
    expect(Math.abs((canvasBox?.height || 0) - (asideBox?.height || 0))).toBeLessThan(30);
  });

  test("keeps the palette and canvas side by side below the large breakpoint", async ({ page }) => {
    await page.setViewportSize({ width: 969, height: 964 });
    await installApiMocks(page, {
      "/api/v1/opsgraph/graphs/graph.default": {
        graph: {
          id: "graph.default",
          name: "默认图谱",
          nodes: [{ id: "service.api", type: "service", name: "api", position: { x: 96, y: 96 } }],
          edges: [],
        },
      },
    });

    await page.goto("/opsgraph/graph.default");
    const asideBox = await page.locator('[data-testid="opsgraph-editor-layout"] > aside').boundingBox();
    const canvasBox = await page.getByTestId("opsgraph-canvas").boundingBox();

    expect(asideBox).toBeTruthy();
    expect(canvasBox).toBeTruthy();
    expect(canvasBox?.x).toBeGreaterThan((asideBox?.x || 0) + (asideBox?.width || 0));
    expect(canvasBox?.height).toBeGreaterThan(700);
  });

  test("keeps the two-column editor inside a narrow viewport", async ({ page }) => {
    await page.setViewportSize({ width: 814, height: 964 });
    await installApiMocks(page, {
      "/api/v1/opsgraph/graphs/graph.default": {
        graph: {
          id: "graph.default",
          name: "默认图谱",
          nodes: [{ id: "service.api", type: "service", name: "api", position: { x: 96, y: 96 } }],
          edges: [],
        },
      },
    });

    await page.goto("/opsgraph/graph.default");
    const layoutBox = await page.getByTestId("opsgraph-editor-layout").boundingBox();
    const asideBox = await page.locator('[data-testid="opsgraph-editor-layout"] > aside').boundingBox();
    const canvasBox = await page.getByTestId("opsgraph-canvas").boundingBox();

    expect(layoutBox).toBeTruthy();
    expect(asideBox).toBeTruthy();
    expect(canvasBox).toBeTruthy();
    expect(canvasBox?.x).toBeGreaterThan((asideBox?.x || 0) + (asideBox?.width || 0));
    expect(layoutBox?.width).toBeLessThanOrEqual(814);
  });

  test("allows dragging a canvas node and persists layout", async ({ page }) => {
    const layoutRequests = [];
    await installApiMocks(page, {
      __layoutRequests: layoutRequests,
      "/api/v1/opsgraph/graphs/graph.default": {
        graph: {
          id: "graph.default",
          name: "默认图谱",
          nodes: [{ id: "service.api", type: "service", name: "api", position: { x: 96, y: 96 } }],
          edges: [],
        },
      },
    });

    await page.goto("/opsgraph/graph.default");
    const node = page.locator('[data-id="service.api"]');
    const before = await node.boundingBox();
    expect(before).toBeTruthy();

    await page.mouse.move((before?.x || 0) + 40, (before?.y || 0) + 20);
    await page.mouse.down();
    await page.mouse.move((before?.x || 0) + 180, (before?.y || 0) + 80, { steps: 8 });
    await page.mouse.up();

    await expect.poll(async () => layoutRequests.length).toBeGreaterThan(0);
    const after = await node.boundingBox();
    expect(after?.x).toBeGreaterThan((before?.x || 0) + 40);
    expect(after?.y).toBeGreaterThan((before?.y || 0) + 20);
  });
});
