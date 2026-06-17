import { describe, expect, it } from "vitest";

import {
  buildCanvasModel,
  nodeTypeLabel,
  relationshipLabel,
  summarizeClusterDeployment,
} from "./opsGraphViewModel";

describe("opsGraphViewModel", () => {
  it("builds canvas groups for host, k8s, and middleware cluster nodes", () => {
    const model = buildCanvasModel({
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
    });

    expect(model.nodes.find((node) => node.id === "host.a")?.type).toBe("opsgraphGroup");
    expect(model.nodes.find((node) => node.id === "middleware.pg")?.type).toBe("opsgraphGroup");
    expect(model.edges.find((edge) => edge.id === "e1")?.animated).toBe(false);
  });

  it("summarizes middleware cluster deployment across hosts", () => {
    const summary = summarizeClusterDeployment("middleware.pg", [
      { id: "middleware.pg-0", type: "middleware_instance", name: "pg-0", parentId: "middleware.pg", properties: { role: "primary" } },
      { id: "middleware.pg-1", type: "middleware_instance", name: "pg-1", parentId: "middleware.pg", properties: { role: "replica" } },
    ], [
      { id: "r1", from: "middleware.pg-0", type: "runs_on", to: "host.a" },
      { id: "r2", from: "middleware.pg-1", type: "runs_on", to: "host.b" },
    ]);

    expect(summary.instanceCount).toBe(2);
    expect(summary.deploymentLabel).toBe("2 instances / 2 hosts");
    expect(summary.badges).toContain("跨主机");
  });

  it("places deployment children inside host or k8s containers", () => {
    const model = buildCanvasModel({
      id: "graph.default",
      name: "默认图谱",
      nodes: [
        { id: "host.a", type: "host", name: "host-a", container: true, position: { x: 420, y: 240 } },
        { id: "middleware.redis", type: "middleware", name: "redis", position: { x: 96, y: 320 } },
      ],
      edges: [
        { id: "e1", from: "middleware.redis", type: "runs_on", to: "host.a" },
      ],
    });

    const child = model.nodes.find((node) => node.id === "middleware.redis");
    expect(child?.parentId).toBe("host.a");
    expect(child?.position).toEqual({ x: 136, y: 72 });
  });

  it("preserves saved relative positions for deployment children", () => {
    const model = buildCanvasModel({
      id: "graph.default",
      name: "默认图谱",
      nodes: [
        { id: "host.a", type: "host", name: "host-a", container: true, position: { x: 420, y: 240 } },
        { id: "middleware.redis", type: "middleware", name: "redis", position: { x: 48, y: 88 } },
      ],
      edges: [
        { id: "e1", from: "middleware.redis", type: "runs_on", to: "host.a" },
      ],
    });

    expect(model.nodes.find((node) => node.id === "middleware.redis")?.position).toEqual({ x: 48, y: 88 });
  });

  it("orders deployment parents before their children for React Flow nesting", () => {
    const model = buildCanvasModel({
      id: "graph.default",
      name: "默认图谱",
      nodes: [
        { id: "middleware.redis", type: "middleware", name: "redis", position: { x: 96, y: 320 } },
        { id: "host.a", type: "host", name: "host-a", container: true, position: { x: 420, y: 240 } },
      ],
      edges: [
        { id: "e1", from: "middleware.redis", type: "runs_on", to: "host.a" },
      ],
    });

    expect(model.nodes.findIndex((node) => node.id === "host.a")).toBeLessThan(
      model.nodes.findIndex((node) => node.id === "middleware.redis"),
    );
  });

  it("separates overlapping top-level containers from legacy positions", () => {
    const model = buildCanvasModel({
      id: "graph.default",
      name: "默认图谱",
      nodes: [
        { id: "host.a", type: "host", name: "host-a", container: true, position: { x: 96, y: 316 } },
        { id: "k8s.prod", type: "k8s", name: "prod", container: true, position: { x: 256, y: 316 } },
      ],
      edges: [],
    });

    expect(model.nodes.find((node) => node.id === "k8s.prod")?.position).toEqual({ x: 396, y: 316 });
  });

  it("uses concise Chinese labels", () => {
    expect(nodeTypeLabel("middleware_cluster")).toBe("中间件集群");
    expect(relationshipLabel("depends_on")).toBe("依赖");
    expect(relationshipLabel("runs_on")).toBe("部署于");
  });
});
