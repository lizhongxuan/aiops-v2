import { afterEach, describe, expect, it, vi } from "vitest";
import { listIncidents, getIncident, createIncident, updateIncident, addIncidentEvidence, closeIncident } from "../src/api/incidents";
import {
  listOpsGraphs,
  createOpsGraph,
  getOpsGraph,
  createOpsGraphNode,
  createOpsGraphRelationship,
  saveOpsGraphLayout,
  validateOpsGraph,
  lookupOpsGraph,
  getOpsGraphNeighborhood,
  getOpsGraphBusinessImpact,
} from "../src/api/opsgraph";
import { listRunbooks, getRunbook, matchRunbooks, listRunbookInstances } from "../src/api/runbooks";
import { getERPHealthSummary, getERPBusinessMetrics, getERPTenantImpact } from "../src/api/erp";
import { getRecentDeployments, getRecentConfigChanges } from "../src/api/changes";

function mockFetch() {
  const calls = [];
  vi.stubGlobal("fetch", vi.fn(async (url, init = {}) => {
    calls.push({ url, init });
    return {
      ok: true,
      status: 200,
      headers: { get: () => "application/json" },
      text: async () => JSON.stringify({ ok: true }),
    };
  }));
  return calls;
}

function bodyOf(call) {
  return call?.init?.body ? JSON.parse(call.init.body) : undefined;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("ERP SRE API clients", () => {
  it("uses standard incident paths, methods, and payloads", async () => {
    const calls = mockFetch();

    await listIncidents({ status: "open" });
    await getIncident("inc-1");
    await createIncident({ title: "Order issue" });
    await updateIncident("inc-1", { severity: "sev2" });
    await addIncidentEvidence("inc-1", { source: "coroot" });
    await closeIncident("inc-1", { rootCause: "db pressure" });

    expect(calls.map((call) => [call.url, call.init.method])).toEqual([
      ["/api/v1/incidents?status=open", "GET"],
      ["/api/v1/incidents/inc-1", "GET"],
      ["/api/v1/incidents", "POST"],
      ["/api/v1/incidents/inc-1", "PUT"],
      ["/api/v1/incidents/inc-1/evidence", "POST"],
      ["/api/v1/incidents/inc-1/close", "POST"],
    ]);
    expect(bodyOf(calls[2])).toEqual({ title: "Order issue" });
    expect(bodyOf(calls[4])).toEqual({ source: "coroot" });
  });

  it("uses standard opsgraph, runbook, ERP, and changes paths", async () => {
    const calls = mockFetch();

    await lookupOpsGraph({ query: "order" });
    await getOpsGraphNeighborhood("service.order-api", { depth: 2 });
    await getOpsGraphBusinessImpact("capability.order.submit");
    await listRunbooks();
    await getRunbook("order-submit-slow");
    await matchRunbooks({ service: "order-api" });
    await listRunbookInstances({ status: "running" });
    await getERPHealthSummary({ environment: "prod" });
    await getERPBusinessMetrics({ service: "order-api" });
    await getERPTenantImpact({ capability: "订单提交" });
    await getRecentDeployments({ service: "order-api" });
    await getRecentConfigChanges({ service: "order-api" });

    expect(calls.map((call) => [call.url, call.init.method])).toEqual([
      ["/api/v1/opsgraph/lookup", "POST"],
      ["/api/v1/opsgraph/entities/service.order-api/neighborhood?depth=2", "GET"],
      ["/api/v1/opsgraph/entities/capability.order.submit/business-impact", "GET"],
      ["/api/v1/runbooks", "GET"],
      ["/api/v1/runbooks/order-submit-slow", "GET"],
      ["/api/v1/runbooks/match", "POST"],
      ["/api/v1/runbooks/instances?status=running", "GET"],
      ["/api/v1/erp/health?environment=prod", "GET"],
      ["/api/v1/erp/business-metrics?service=order-api", "GET"],
      ["/api/v1/erp/tenant-impact?capability=%E8%AE%A2%E5%8D%95%E6%8F%90%E4%BA%A4", "GET"],
      ["/api/v1/changes/deployments?service=order-api", "GET"],
      ["/api/v1/changes/config?service=order-api", "GET"],
    ]);
  });

  it("uses manual opsgraph graph, node, relationship, layout, and read paths", async () => {
    const calls = mockFetch();

    await listOpsGraphs();
    await createOpsGraph({ name: "生产环境核心链路" });
    await getOpsGraph("graph.default");
    await createOpsGraphNode("graph.default", { id: "service.order-api", type: "service", name: "order-api" });
    await createOpsGraphRelationship("graph.default", { id: "e1", from: "service.order-api", type: "depends_on", to: "middleware.pg" });
    await saveOpsGraphLayout("graph.default", { nodes: [{ id: "service.order-api", position: { x: 1, y: 2 } }], viewport: { x: 0, y: 0, zoom: 1 } });
    await validateOpsGraph("graph.default");
    await lookupOpsGraph({ query: "order", types: ["service"] });
    await getOpsGraphNeighborhood("service.order-api", { depth: 2, graphId: "graph.default" });

    expect(calls.map((call) => [call.url, call.init.method])).toEqual([
      ["/api/v1/opsgraph/graphs", "GET"],
      ["/api/v1/opsgraph/graphs", "POST"],
      ["/api/v1/opsgraph/graphs/graph.default", "GET"],
      ["/api/v1/opsgraph/graphs/graph.default/entities", "POST"],
      ["/api/v1/opsgraph/graphs/graph.default/relationships", "POST"],
      ["/api/v1/opsgraph/graphs/graph.default/layout", "POST"],
      ["/api/v1/opsgraph/graphs/graph.default/validate", "GET"],
      ["/api/v1/opsgraph/lookup", "POST"],
      ["/api/v1/opsgraph/graphs/graph.default/entities/service.order-api/neighborhood?depth=2", "GET"],
    ]);
  });
});
