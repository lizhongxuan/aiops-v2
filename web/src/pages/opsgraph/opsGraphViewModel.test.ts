import { describe, expect, it } from "vitest";

import {
  autoLayoutOpsGraphLeftToRight,
  buildLLMContextPreview,
  buildCanvasModel,
  nextTopologyNodeName,
  nodeTypeLabel,
  reconnectOpsGraphRelationship,
  relationshipLabel,
  summarizeClusterDeployment,
  topologyNodeMeta,
} from "./opsGraphViewModel";

describe("opsGraphViewModel", () => {
  it("lays out upstream to downstream nodes from left to right", () => {
    const layout = autoLayoutOpsGraphLeftToRight({
      id: "graph.checkout",
      name: "服务拓扑",
      nodes: [
        { id: "middleware.pg", type: "middleware", subtype: "postgres", name: "checkout-postgres", position: { x: 10, y: 10 } },
        { id: "service.checkout", type: "service", name: "checkout-api", position: { x: 10, y: 10 } },
        { id: "external.pay", type: "external", name: "payment-gateway", position: { x: 10, y: 10 } },
        { id: "middleware.redis", type: "middleware", subtype: "redis", name: "checkout-redis", position: { x: 10, y: 10 } },
        { id: "middleware.nginx", type: "middleware", subtype: "nginx", name: "edge-nginx", position: { x: 10, y: 10 } },
      ],
      edges: [
        { id: "e0", from: "middleware.nginx", type: "proxies_to", to: "service.checkout" },
        { id: "e1", from: "service.checkout", type: "depends_on", to: "middleware.pg" },
        { id: "e2", from: "service.checkout", type: "depends_on", to: "middleware.redis" },
        { id: "e3", from: "service.checkout", type: "calls", to: "external.pay" },
      ],
    });

    const position = new Map(layout.nodes.map((node) => [node.id, node.position]));
    expect(position.get("middleware.nginx")?.x).toBeLessThan(position.get("service.checkout")?.x || 0);
    expect(position.get("service.checkout")?.x).toBeLessThan(position.get("middleware.pg")?.x || 0);
    expect(position.get("middleware.pg")?.x).toBe(position.get("middleware.redis")?.x);
    expect(position.get("middleware.pg")?.x).toBe(position.get("external.pay")?.x);
    expect(position.get("middleware.nginx")).toEqual({ x: 96, y: 96 });
    expect(position.get("service.checkout")).toEqual({ x: 416, y: 96 });
    expect(position.get("external.pay")).toEqual({ x: 736, y: 96 });
  });

  it("adds arrow markers to topology edges", () => {
    const model = buildCanvasModel({
      id: "graph.default",
      name: "服务拓扑",
      nodes: [
        { id: "service.order-api", type: "service", name: "order-api" },
        { id: "middleware.pg", type: "middleware", subtype: "postgres", name: "order-postgres" },
      ],
      edges: [{ id: "e1", from: "service.order-api", type: "depends_on", to: "middleware.pg" }],
    });

    expect(model.edges[0]?.markerEnd).toMatchObject({ type: "arrowclosed" });
  });

  it("separates edges that converge on the same target so each relationship can be selected", () => {
    const model = buildCanvasModel({
      id: "graph.default",
      name: "服务拓扑",
      nodes: [
        { id: "service.checkout", type: "service", name: "checkout" },
        { id: "service.order", type: "service", name: "order" },
        { id: "middleware.redis", type: "middleware", subtype: "redis", name: "redis" },
      ],
      edges: [
        { id: "e1", from: "service.checkout", type: "depends_on", to: "middleware.redis" },
        { id: "e2", from: "service.order", type: "depends_on", to: "middleware.redis" },
      ],
    });

    expect(model.edges.map((edge) => edge.data?.laneOffset)).toEqual([-18, 18]);
    expect(model.edges.every((edge) => edge.interactionWidth === 26)).toBe(true);
  });

  it("generates duplicate node names using the next numeric suffix", () => {
    expect(nextTopologyNodeName([
      { id: "service.a", type: "service", name: "新服务" },
      { id: "service.b", type: "service", name: "新服务" },
      { id: "service.c", type: "service", name: "新服务-3" },
    ], "新服务")).toBe("新服务-4");

    expect(nextTopologyNodeName([
      { id: "middleware.a", type: "middleware", name: "新中间件" },
      { id: "middleware.b", type: "middleware", name: "新中间件-2" },
    ], "新中间件")).toBe("新中间件-3");
  });

  it("reconnects relationship endpoints without losing relationship properties", () => {
    const next = reconnectOpsGraphRelationship(
      {
        id: "e1",
        from: "service.order-api",
        type: "depends_on",
        to: "middleware.pg",
        note: "primary db",
        properties: { protocol: "postgres", port: "5432" },
      },
      { source: "service.order-api", target: "middleware.redis" },
    );

    expect(next).toEqual({
      id: "e1",
      from: "service.order-api",
      type: "depends_on",
      to: "middleware.redis",
      note: "primary db",
      properties: { protocol: "postgres", port: "5432" },
    });
  });

  it("keeps service topology nodes as top-level React Flow nodes", () => {
    const model = buildCanvasModel({
      id: "graph.default",
      name: "服务拓扑",
      nodes: [
        { id: "service.order-api", type: "service", name: "order-api", position: { x: 80, y: 100 } },
        { id: "middleware.pg", type: "middleware", subtype: "postgres", name: "order-postgres", properties: { host: "erp-db-a" }, position: { x: 360, y: 100 } },
      ],
      edges: [{ id: "e1", from: "service.order-api", type: "depends_on", to: "middleware.pg" }],
    });

    expect(model.nodes.map((node) => node.type)).toEqual(["opsgraphNode", "opsgraphNode"]);
    expect(model.nodes.every((node) => !node.parentId)).toBe(true);
    expect(model.nodes.find((node) => node.id === "middleware.pg")?.data.topologyMeta).toMatchObject({ typeLabel: "Postgres" });
    expect(model.edges[0]?.label).toBe("依赖");
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

  it("filters legacy host and k8s deployment nodes out of the service topology canvas", () => {
    const model = buildCanvasModel({
      id: "graph.default",
      name: "旧图谱",
      nodes: [
        { id: "service.api", type: "service", name: "api", position: { x: 96, y: 96 } },
        { id: "host.a", type: "host", name: "host-a", container: true, position: { x: 420, y: 240 } },
        { id: "k8s.prod", type: "k8s", name: "prod", container: true, position: { x: 690, y: 240 } },
      ],
      edges: [{ id: "e1", from: "service.api", type: "runs_on", to: "host.a" }],
    });

    expect(model.nodes.find((node) => node.id === "service.api")?.parentId).toBeUndefined();
    expect(model.nodes.find((node) => node.id === "host.a")).toBeUndefined();
    expect(model.nodes.find((node) => node.id === "k8s.prod")).toBeUndefined();
    expect(model.edges.find((edge) => edge.id === "e1")).toBeUndefined();
  });

  it("uses concise Chinese labels", () => {
    expect(nodeTypeLabel("middleware_cluster")).toBe("中间件集群");
    expect(relationshipLabel("depends_on")).toBe("依赖");
    expect(relationshipLabel("runs_on")).toBe("部署于");
    expect(relationshipLabel("publishes")).toBe("发布");
  });

  it("maps middleware subtype to topology card metadata", () => {
    const redis = topologyNodeMeta({ id: "middleware.redis", type: "middleware", subtype: "redis", name: "redis", properties: { ports: "6379/redis" } });
    expect(redis.typeLabel).toBe("Redis");
    expect(redis.tone).toBe("cache");
    expect(redis.summary).toContain("6379/redis");

    const unknown = topologyNodeMeta({ id: "middleware.custom", type: "middleware", subtype: "custom-cache", name: "custom" });
    expect(unknown.typeLabel).toBe("custom-cache");
    expect(unknown.tone).toBe("middleware");
  });

  it("builds direct upstream and downstream facts for LLM preview", () => {
    const graph = {
      id: "graph.default",
      name: "服务拓扑",
      nodes: [
        { id: "service.checkout", type: "service" as const, name: "checkout", properties: { k8sCluster: "prod-k8s", namespace: "erp", ports: "8080/http" } },
        { id: "middleware.pg", type: "middleware" as const, subtype: "postgres", name: "order-postgres" },
        { id: "external.pay", type: "external" as const, name: "pay-provider" },
      ],
      edges: [
        { id: "e1", from: "service.checkout", type: "depends_on" as const, to: "middleware.pg", properties: { protocol: "postgres", port: "5432" } },
        { id: "e2", from: "external.pay", type: "calls" as const, to: "service.checkout" },
      ],
    };

    const preview = buildLLMContextPreview(graph, "service.checkout");
    expect(preview).toContain("service.checkout");
    expect(preview).toContain("k8sCluster=prod-k8s");
    expect(preview).toContain("downstream: depends_on -> middleware.pg");
    expect(preview).toContain("upstream: external.pay -> calls");
  });
});
