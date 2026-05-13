function objectValue(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

function eventType(event = {}) {
  return event.type || event.event || event.kind || "";
}

function eventNodeId(event = {}) {
  const output = objectValue(event.output);
  return String(event.node_id || event.nodeId || output.node_id || output.nodeId || "").trim();
}

function graphSystemNodeId(graph = {}, type = "") {
  const wantedType = String(type || "").toLowerCase();
  const node = (graph.nodes || []).find((item) => String(item?.type || "").toLowerCase() === wantedType || String(item?.id || "").toLowerCase() === wantedType);
  return String(node?.id || "").trim();
}

function terminalNodeStatus(event = {}) {
  const type = eventType(event);
  const status = String(event.status || "").trim().toLowerCase();
  if (["failed", "error"].includes(status) || type === "run.failed") return "failed";
  if (["cancelled", "canceled"].includes(status) || type === "run.cancelled" || type === "run.canceled") return "skipped";
  return "success";
}

function systemNodeEvent(base = {}, type, nodeId, status, message = "") {
  return {
    type,
    run_id: base.run_id || base.runId || "",
    workflow: base.workflow || base.workflow_name || base.workflowName || "",
    node_id: nodeId,
    status,
    message,
    timestamp: base.timestamp || base.ts || base.time || "",
  };
}

function isTerminalRunEvent(event = {}) {
  const type = eventType(event);
  return ["run_finish", "run.completed", "run.failed", "run.cancelled", "run.canceled"].includes(type);
}

function buildNodeLookup(graph = {}) {
  const lookup = new Map();
  for (const node of graph.nodes || []) {
    if (!node?.id) continue;
    const candidates = [
      node.id,
      node.step?.id,
      node.step?.name,
      node.step_name,
      node.stepName,
      objectValue(node.data).step_name,
      objectValue(node.data).stepName,
    ];
    for (const candidate of candidates) {
      const key = String(candidate || "").trim();
      if (key && !lookup.has(key)) lookup.set(key, node.id);
    }
  }
  return lookup;
}

export function unwrapRunnerPayload(payload) {
  const value = objectValue(payload).data;
  return value && (Array.isArray(value) || typeof value === "object") ? value : payload;
}

export function extractRunnerRunEvents(payload) {
  const value = unwrapRunnerPayload(payload);
  if (Array.isArray(value)) return value.filter((event) => event && typeof event === "object");
  const boxed = objectValue(value);
  const events = Array.isArray(boxed.items) ? boxed.items : Array.isArray(boxed.events) ? boxed.events : [];
  return events.filter((event) => event && typeof event === "object");
}

export function mapRunnerRunEventsToGraph(events = [], graph = {}) {
  const lookup = buildNodeLookup(graph);
  const mapped = events.map((event) => {
    const nodeId = eventNodeId(event);
    if (nodeId) return event.node_id ? event : { ...event, node_id: nodeId };
    const stepName = String(event.step || event.step_name || event.stepName || event.node || "").trim();
    const mapped = stepName ? lookup.get(stepName) : "";
    if (!mapped) return event;
    return { ...event, node_id: mapped, step_name: stepName };
  });
  const startID = graphSystemNodeId(graph, "start");
  const endID = graphSystemNodeId(graph, "end");
  const hasNodeEvent = (nodeId) => mapped.some((event) => eventNodeId(event) === nodeId);
  const enriched = [...mapped];

  if (startID && !hasNodeEvent(startID)) {
    const startIndex = enriched.findIndex((event) => ["run_start", "run.started"].includes(eventType(event)));
    if (startIndex >= 0) {
      const base = enriched[startIndex];
      enriched.splice(
        startIndex + 1,
        0,
        systemNodeEvent(base, "node_started", startID, "running", "开始检查主机和运行配置"),
        systemNodeEvent(base, "node_finished", startID, "success", "主机和运行配置检查通过"),
      );
    }
  }

  if (endID && !hasNodeEvent(endID)) {
    const terminalIndex = enriched.findIndex(isTerminalRunEvent);
    if (terminalIndex >= 0) {
      const base = enriched[terminalIndex];
      const status = terminalNodeStatus(base);
      const message = String(base.message || base.error || (status === "success" ? "工作流运行成功" : "工作流运行失败"));
      enriched.splice(
        terminalIndex,
        0,
        systemNodeEvent(base, "node_started", endID, "running", "汇总运行结果"),
        systemNodeEvent(base, "node_finished", endID, status, message),
      );
    }
  }

  return enriched;
}

export function isRunnerRunHistoryTerminal(events = []) {
  return events.some((event) => {
    const type = eventType(event);
    const status = String(event.status || "").toLowerCase();
    return ["run_finish", "run.completed", "run.failed", "run.cancelled", "run.canceled"].includes(type)
      || ["success", "completed", "failed", "cancelled", "canceled"].includes(status);
  });
}

export function finalRunnerRunStatus(events = []) {
  let sawFailure = false;
  for (let index = events.length - 1; index >= 0; index -= 1) {
    const event = events[index] || {};
    const type = eventType(event);
    const status = String(event.status || "").toLowerCase();
    if (["failed", "error"].includes(status) || type === "node.failed" || type === "run.failed") sawFailure = true;
    if (type === "run_finish") {
      const finalStatus = String(event.status || "completed");
      if (["running", "queued", "pending"].includes(finalStatus.toLowerCase()) && sawFailure) return "failed";
      return finalStatus;
    }
    if (type === "run.completed") return "completed";
    if (type === "run.failed") return "failed";
    if (type === "run.cancelled" || type === "run.canceled") return "cancelled";
  }
  return sawFailure ? "failed" : "";
}
