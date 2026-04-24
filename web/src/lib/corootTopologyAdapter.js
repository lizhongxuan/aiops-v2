/**
 * Coroot Topology Adapter
 *
 * Adapts raw Coroot Topology and ServiceDependencies API responses into
 * frontend-friendly graph data structures (nodes + edges).
 * Each function handles missing/null fields with safe defaults.
 */

/**
 * adaptTopology(topologyResult) → { nodes, edges }
 *
 * Converts a Coroot Topology API response into a frontend topology graph
 * data structure with normalized nodes and edges.
 *
 * @param {object|null|undefined} topologyResult - Raw TopologyResult from Coroot API
 * @returns {{ nodes: Array<{id: string, name: string, status: string, kind: string}>, edges: Array<{source: string, target: string, label: string}> }}
 */
export function adaptTopology(topologyResult) {
  try {
    const data = topologyResult && typeof topologyResult === "object" ? topologyResult : {};
    const rawNodes = Array.isArray(data.nodes) ? data.nodes : [];
    const rawEdges = Array.isArray(data.edges) ? data.edges : [];

    const nodes = rawNodes.map((n) => {
      const node = n && typeof n === "object" ? n : {};
      return {
        id: String(node.id || ""),
        name: String(node.name || node.id || "N/A"),
        status: String(node.status || "N/A"),
        kind: String(node.kind || node.type || "service"),
      };
    });

    const edges = rawEdges.map((e) => {
      const edge = e && typeof e === "object" ? e : {};
      return {
        source: String(edge.source || edge.from || ""),
        target: String(edge.target || edge.to || ""),
        label: String(edge.label || edge.protocol || ""),
      };
    });

    return { nodes, edges };
  } catch {
    return { nodes: [], edges: [] };
  }
}

/**
 * adaptServiceDependencies(depResult, serviceID) → { upstream, downstream }
 *
 * Converts a Coroot DependencyResult into upstream/downstream node lists
 * for a given service. Each node includes id, name, status, and direction.
 *
 * @param {object|null|undefined} depResult - Raw DependencyResult from Coroot API
 * @param {string} serviceID - The target service ID to contextualize dependencies
 * @returns {{ serviceID: string, upstream: Array<{id: string, name: string, status: string, kind: string}>, downstream: Array<{id: string, name: string, status: string, kind: string}> }}
 */
export function adaptServiceDependencies(depResult, serviceID) {
  try {
    const sid = String(serviceID || "");
    const data = depResult && typeof depResult === "object" ? depResult : {};

    const rawUpstream = Array.isArray(data.upstream) ? data.upstream : [];
    const rawDownstream = Array.isArray(data.downstream) ? data.downstream : [];

    const mapNode = (n) => {
      const node = n && typeof n === "object" ? n : {};
      return {
        id: String(node.id || ""),
        name: String(node.name || node.id || "N/A"),
        status: String(node.status || "N/A"),
        kind: String(node.kind || node.type || "service"),
      };
    };

    return {
      serviceID: sid,
      upstream: rawUpstream.map(mapNode),
      downstream: rawDownstream.map(mapNode),
    };
  } catch {
    return {
      serviceID: String(serviceID || ""),
      upstream: [],
      downstream: [],
    };
  }
}
