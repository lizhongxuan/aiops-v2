import { describe, it, expect } from "vitest";
import {
  adaptTopology,
  adaptServiceDependencies,
} from "../src/lib/corootTopologyAdapter.js";

describe("corootTopologyAdapter", () => {
  describe("adaptTopology", () => {
    it("maps a full topology result to nodes and edges", () => {
      const result = {
        nodes: [
          { id: "svc-a", name: "gateway", status: "ok", kind: "service" },
          { id: "svc-b", name: "auth", status: "warning", kind: "service" },
        ],
        edges: [
          { source: "svc-a", target: "svc-b", label: "HTTP" },
        ],
      };
      const { nodes, edges } = adaptTopology(result);
      expect(nodes).toHaveLength(2);
      expect(nodes[0]).toEqual({ id: "svc-a", name: "gateway", status: "ok", kind: "service" });
      expect(nodes[1]).toEqual({ id: "svc-b", name: "auth", status: "warning", kind: "service" });
      expect(edges).toHaveLength(1);
      expect(edges[0]).toEqual({ source: "svc-a", target: "svc-b", label: "HTTP" });
    });

    it("uses defaults for missing node fields", () => {
      const result = { nodes: [{ id: "svc-1" }], edges: [] };
      const { nodes } = adaptTopology(result);
      expect(nodes[0]).toEqual({ id: "svc-1", name: "svc-1", status: "N/A", kind: "service" });
    });

    it("falls back to from/to for edge source/target", () => {
      const result = { nodes: [], edges: [{ from: "a", to: "b", protocol: "gRPC" }] };
      const { edges } = adaptTopology(result);
      expect(edges[0]).toEqual({ source: "a", target: "b", label: "gRPC" });
    });

    it("returns empty nodes and edges for null input", () => {
      const { nodes, edges } = adaptTopology(null);
      expect(nodes).toEqual([]);
      expect(edges).toEqual([]);
    });

    it("returns empty nodes and edges for undefined input", () => {
      const { nodes, edges } = adaptTopology(undefined);
      expect(nodes).toEqual([]);
      expect(edges).toEqual([]);
    });

    it("returns empty nodes and edges for empty object", () => {
      const { nodes, edges } = adaptTopology({});
      expect(nodes).toEqual([]);
      expect(edges).toEqual([]);
    });

    it("handles node with type field as kind fallback", () => {
      const result = { nodes: [{ id: "h1", name: "host-1", type: "host" }], edges: [] };
      const { nodes } = adaptTopology(result);
      expect(nodes[0].kind).toBe("host");
    });
  });

  describe("adaptServiceDependencies", () => {
    it("maps upstream and downstream correctly", () => {
      const depResult = {
        upstream: [
          { id: "svc-a", name: "frontend", status: "ok" },
        ],
        downstream: [
          { id: "svc-c", name: "database", status: "critical" },
          { id: "svc-d", name: "cache", status: "ok" },
        ],
      };
      const result = adaptServiceDependencies(depResult, "svc-b");
      expect(result.serviceID).toBe("svc-b");
      expect(result.upstream).toHaveLength(1);
      expect(result.upstream[0]).toEqual({ id: "svc-a", name: "frontend", status: "ok", kind: "service" });
      expect(result.downstream).toHaveLength(2);
      expect(result.downstream[0].id).toBe("svc-c");
      expect(result.downstream[1].id).toBe("svc-d");
    });

    it("returns empty lists for null depResult", () => {
      const result = adaptServiceDependencies(null, "svc-x");
      expect(result.serviceID).toBe("svc-x");
      expect(result.upstream).toEqual([]);
      expect(result.downstream).toEqual([]);
    });

    it("returns empty lists for undefined depResult", () => {
      const result = adaptServiceDependencies(undefined, "svc-x");
      expect(result.upstream).toEqual([]);
      expect(result.downstream).toEqual([]);
    });

    it("handles missing serviceID gracefully", () => {
      const result = adaptServiceDependencies({}, null);
      expect(result.serviceID).toBe("");
      expect(result.upstream).toEqual([]);
      expect(result.downstream).toEqual([]);
    });

    it("uses defaults for missing node fields in dependencies", () => {
      const depResult = {
        upstream: [{ id: "u1" }],
        downstream: [{}],
      };
      const result = adaptServiceDependencies(depResult, "svc-1");
      expect(result.upstream[0]).toEqual({ id: "u1", name: "u1", status: "N/A", kind: "service" });
      expect(result.downstream[0]).toEqual({ id: "", name: "N/A", status: "N/A", kind: "service" });
    });
  });
});
