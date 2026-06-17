// @ts-check
import { expect, test } from "@playwright/test";

async function installApiMocks(page, routes) {
  const mockRoutes = routes;
  await page.route("**/api/v1/state", (route) => route.fulfill({ json: {} }));
  await page.route("**/api/v1/sessions", (route) => route.fulfill({ json: { sessions: [] } }));
  await page.route("**/api/v1/hosts", (route) => route.fulfill({ json: { hosts: [] } }));
  await page.route("**/api/v1/llm-config", (route) => route.fulfill({ json: { provider: "mock", model: "mock", configured: true } }));
  await page.route("**/api/v1/opsgraph/graphs", (route) => route.fulfill({ json: mockRoutes["/api/v1/opsgraph/graphs"] || { graphs: [] } }));
  await page.route("**/api/v1/opsgraph/graphs/*/entities/*", async (route) => {
    const url = new URL(route.request().url());
    const graphKey = url.pathname.replace(/\/entities\/[^/]+$/, "");
    const nodeId = decodeURIComponent(url.pathname.split("/").at(-1) || "");
    const graphPayload = mockRoutes[graphKey] || { graph: { id: "graph.default", name: "默认图谱", nodes: [], edges: [] } };
    const next = route.request().postDataJSON();
    graphPayload.graph.nodes = graphPayload.graph.nodes.map((node) => node.id === nodeId ? { ...node, ...next, id: nodeId } : node);
    mockRoutes[graphKey] = graphPayload;
    route.fulfill({ json: { node: graphPayload.graph.nodes.find((node) => node.id === nodeId) } });
  });
  await page.route("**/api/v1/opsgraph/graphs/*/entities", async (route) => {
    const url = new URL(route.request().url());
    const graphKey = url.pathname.replace(/\/entities$/, "");
    const graphPayload = mockRoutes[graphKey] || { graph: { id: "graph.default", name: "默认图谱", nodes: [], edges: [] } };
    const node = route.request().postDataJSON();
    graphPayload.graph.nodes.push(node);
    mockRoutes[graphKey] = graphPayload;
    route.fulfill({ json: { node } });
  });
  await page.route("**/api/v1/opsgraph/graphs/*/relationships/*", async (route) => {
    const url = new URL(route.request().url());
    const graphKey = url.pathname.replace(/\/relationships\/[^/]+$/, "");
    const relationshipId = decodeURIComponent(url.pathname.split("/").at(-1) || "");
    const graphPayload = mockRoutes[graphKey] || { graph: { id: "graph.default", name: "默认图谱", nodes: [], edges: [] } };
    const next = route.request().postDataJSON();
    graphPayload.graph.edges = graphPayload.graph.edges.map((edge) => edge.id === relationshipId ? { ...edge, ...next, id: relationshipId } : edge);
    mockRoutes[graphKey] = graphPayload;
    route.fulfill({ json: { relationship: graphPayload.graph.edges.find((edge) => edge.id === relationshipId) } });
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

function populatedTopologyGraph() {
  return {
    id: "graph.default",
    name: "服务拓扑",
    nodes: [
      { id: "service.order-api", type: "service", name: "order-api", position: { x: 96, y: 128 }, properties: { k8sCluster: "prod-k8s", namespace: "erp", ports: "8080/http" } },
      { id: "middleware.pg", type: "middleware", subtype: "postgres", name: "order-postgres", position: { x: 390, y: 128 }, properties: { host: "erp-db-a", ports: "5432/postgres", role: "primary" } },
      { id: "middleware.redis", type: "middleware", subtype: "redis", name: "order-redis", position: { x: 390, y: 310 }, properties: { ports: "6379/redis" } },
      { id: "external.pay", type: "external", name: "pay-provider", position: { x: 690, y: 128 }, properties: { domain: "pay.example.com", ports: "443/https" } },
    ],
    edges: [
      { id: "e1", from: "service.order-api", type: "depends_on", to: "middleware.pg", properties: { protocol: "postgres", port: "5432" } },
      { id: "e2", from: "service.order-api", type: "depends_on", to: "middleware.redis", properties: { protocol: "redis", port: "6379" } },
      { id: "e3", from: "service.order-api", type: "calls", to: "external.pay", properties: { protocol: "https", port: "443" } },
    ],
  };
}

test.describe("OpsGraph manual authoring", () => {
  test("renders graph list and empty service topology editor", async ({ page }) => {
    await installApiMocks(page, {
      "/api/v1/opsgraph/graphs": { graphs: [{ id: "graph.default", name: "服务拓扑", isDefault: true, nodeCount: 0, relationshipCount: 0 }] },
      "/api/v1/opsgraph/graphs/graph.default": { graph: { id: "graph.default", name: "服务拓扑", nodes: [], edges: [] } },
    });

    await page.goto("/opsgraph");
    await expect(page.getByText("服务拓扑")).toBeVisible();
    await page.getByRole("link", { name: "打开" }).click();
    await expect(page.getByText("这个图谱现在是空的")).toBeVisible();
    await expect(page.getByRole("button", { name: "业务服务" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Postgres" })).toBeVisible();
    await expect(page.getByRole("button", { name: "外部服务" })).toBeVisible();
    await expect(page.getByRole("button", { name: "主机" })).toHaveCount(0);
    await expect(page.getByRole("button", { name: "K8s" })).toHaveCount(0);
    await expect(page).toHaveScreenshot("opsgraph-empty-service-topology.png", { fullPage: true });
  });

  test("renders populated service topology", async ({ page }) => {
    await installApiMocks(page, {
      "/api/v1/opsgraph/graphs/graph.default": { graph: populatedTopologyGraph() },
    });

    await page.goto("/opsgraph/graph.default");
    await expect(page.getByTestId("rf__node-service.order-api").getByText("order-api")).toBeVisible();
    await expect(page.getByTestId("rf__node-middleware.pg").getByText("Postgres", { exact: true })).toBeVisible();
    await expect(page.getByTestId("rf__node-middleware.pg").getByText("5432/postgres")).toBeVisible();
    await expect(page.getByTestId("rf__node-external.pay").getByText("pay.example.com").first()).toBeVisible();
    await expect(page).toHaveScreenshot("opsgraph-populated-service-topology.png", { fullPage: true });
  });

  test("edits service properties in modal", async ({ page }) => {
    await installApiMocks(page, {
      "/api/v1/opsgraph/graphs/graph.default": {
        graph: {
          id: "graph.default",
          name: "服务拓扑",
          nodes: [{ id: "service.order-api", type: "service", name: "order-api", position: { x: 96, y: 128 }, properties: { ports: "8080/http" } }],
          edges: [],
        },
      },
    });

    await page.goto("/opsgraph/graph.default");
    await page.locator('[data-id="service.order-api"]').click();
    await expect(page.getByTestId("opsgraph-node-summary")).toContainText("LLM 上下文");
    await page.getByRole("button", { name: "编辑属性" }).click();
    await page.locator('input[name="k8sCluster"]').fill("prod-k8s");
    await page.locator('input[name="namespace"]').fill("erp");
    await expect(page).toHaveScreenshot("opsgraph-service-property-modal.png", { fullPage: true });
    await page.getByRole("button", { name: "保存属性" }).click();
    await expect(page.getByTestId("opsgraph-node-summary").getByText("prod-k8s / erp")).toBeVisible();
  });

  test("edits relationship properties in modal", async ({ page }) => {
    await installApiMocks(page, {
      "/api/v1/opsgraph/graphs/graph.default": { graph: populatedTopologyGraph() },
    });

    await page.goto("/opsgraph/graph.default");
    await page.locator('button[title="postgres · 5432"]').click();
    await expect(page.getByRole("dialog")).toContainText("编辑关系");
    await expect(page.getByRole("dialog")).toContainText("order-api");
    await expect(page.getByRole("dialog")).toContainText("order-postgres");
    await page.locator('input[name="protocol"]').fill("mysql");
    await page.locator('input[name="port"]').fill("3306");
    await page.locator('input[name="path"]').fill("/orders");
    await page.locator('textarea[name="note"]').fill("订单服务读取订单库");
    await page.getByRole("button", { name: "保存关系" }).click();
    await expect(page.locator('button[title="mysql · 3306 · /orders"]')).toBeVisible();
  });

  test("keeps many palette-created topology nodes visible in the canvas", async ({ page }) => {
    await installApiMocks(page, {
      "/api/v1/opsgraph/graphs/graph.default": {
        graph: { id: "graph.default", name: "服务拓扑", nodes: [], edges: [] },
      },
    });

    await page.goto("/opsgraph/graph.default");
    for (const label of ["业务服务", "Postgres", "Redis", "MySQL", "RabbitMQ", "Nginx", "外部服务"]) {
      await page.getByRole("button", { name: label, exact: true }).click();
    }

    await expect(page.getByText("7 个节点")).toBeVisible();
    const canvasBox = await page.getByTestId("opsgraph-canvas").boundingBox();
    expect(canvasBox).toBeTruthy();
    const nodeBoxes = await page.locator(".react-flow__node").evaluateAll((nodes) =>
      nodes.map((node) => {
        const rect = node.getBoundingClientRect();
        return { x: rect.x, y: rect.y, width: rect.width, height: rect.height };
      }),
    );

    expect(nodeBoxes).toHaveLength(7);
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
          name: "服务拓扑",
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

  test("keeps the two-column editor inside a narrow viewport", async ({ page }) => {
    await page.setViewportSize({ width: 814, height: 964 });
    await installApiMocks(page, {
      "/api/v1/opsgraph/graphs/graph.default": {
        graph: {
          id: "graph.default",
          name: "服务拓扑",
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
          name: "服务拓扑",
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
