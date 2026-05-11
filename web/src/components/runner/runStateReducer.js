export function createInitialRunState() {
  return {
    runId: "",
    status: "idle",
    message: "",
    error: "",
    startedAt: "",
    finishedAt: "",
    nodes: {},
    edges: {},
    hosts: {},
    logs: [],
    approvals: [],
    retries: [],
    variables: {
      inputs: [],
      outputs: [],
      exports: [],
      nodeResults: [],
    },
  };
}

function eventType(event) {
  return event?.type || event?.event || event?.kind || "";
}

function pick(event, ...keys) {
  for (const key of keys) {
    if (event?.[key] !== undefined && event?.[key] !== null) return event[key];
  }
  return undefined;
}

function objectValue(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

function pickRunId(event) {
  return pick(event, "run_id", "runId", "id") || "";
}

function pickNodeId(event) {
  const output = objectValue(event?.output);
  return pick(event, "node_id", "nodeId", "step", "node", "id") || output.node_id || output.nodeId || "";
}

function cloneState(state) {
  return {
    ...createInitialRunState(),
    ...(state || {}),
    nodes: { ...(state?.nodes || {}) },
    edges: { ...(state?.edges || {}) },
    hosts: { ...(state?.hosts || {}) },
    logs: [...(state?.logs || [])],
    approvals: [...(state?.approvals || [])],
    retries: [...(state?.retries || [])],
    variables: {
      inputs: [...(state?.variables?.inputs || [])],
      outputs: [...(state?.variables?.outputs || [])],
      exports: [...(state?.variables?.exports || [])],
      nodeResults: [...(state?.variables?.nodeResults || [])],
    },
  };
}

function upsertById(items, id, nextItem) {
  const index = items.findIndex((item) => item.id === id);
  if (index < 0) return [...items, nextItem];
  return items.map((item, itemIndex) => (itemIndex === index ? { ...item, ...nextItem } : item));
}

function appendLog(state, event, stream) {
  state.logs.push({
    stream,
    event: pick(event, "event", "name") || eventType(event),
    nodeId: pickNodeId(event),
    hostId: pick(event, "host_id", "hostId", "host") || "",
    message: pick(event, "message", "data", "text", "summary", "chunk") || "",
    ts: pick(event, "ts", "time", "timestamp") || "",
  });
}

function mergeNodeResult(previous, next) {
  if (
    previous &&
    next &&
    typeof previous === "object" &&
    typeof next === "object" &&
    !Array.isArray(previous) &&
    !Array.isArray(next)
  ) {
    return { ...previous, ...next };
  }
  return next;
}

function updateNode(state, event, status) {
  const nodeId = pickNodeId(event);
  if (!nodeId) return;
  const current = state.nodes[nodeId] || {};
  const result = pick(event, "result", "output");
  const message = pick(event, "message", "summary");
  const error = pick(event, "error");
  state.nodes[nodeId] = {
    ...current,
    nodeId,
    stepName: pick(event, "step_name", "stepName", "step") || current.stepName || "",
    status: pick(event, "status") || status,
    startedAt: pick(event, "started_at", "startedAt") || current.startedAt || "",
    finishedAt: pick(event, "finished_at", "finishedAt") || current.finishedAt || "",
    durationMs: pick(event, "duration_ms", "durationMs") || current.durationMs || 0,
    message: message === undefined ? current.message || "" : message,
    error: error === undefined ? current.error || "" : error,
    result: result === undefined ? current.result : mergeNodeResult(current.result, result),
  };
  if (result !== undefined) {
    state.variables.nodeResults.push({ nodeId, result });
  }
}

function updateHost(state, event, stream) {
  const hostId = pick(event, "host_id", "hostId", "host");
  if (!hostId) return;
  const nodeId = pickNodeId(event);
  const message = pick(event, "message", "data", "text") || "";
  state.hosts[hostId] = {
    ...(state.hosts[hostId] || {}),
    hostId,
    nodeId,
    lastStream: stream,
    lastMessage: message,
  };
}

function appendVariable(state, event, bucket) {
  state.variables[bucket].push({
    nodeId: pickNodeId(event),
    key: pick(event, "key", "name") || "",
    value: pick(event, "value"),
  });
}

function appendHostResult(state, event) {
  const nodeId = pickNodeId(event);
  if (!nodeId) return;
  const output = pick(event, "output", "result") || {};
  updateNode(state, { ...event, node_id: nodeId, result: output }, pick(event, "status") || "success");

  const stdout = typeof output.stdout === "string" ? output.stdout : "";
  const stderr = typeof output.stderr === "string" ? output.stderr : "";
  if (stdout) {
    appendLog(state, { ...event, node_id: nodeId, message: stdout }, "stdout");
    updateHost(state, { ...event, node_id: nodeId, message: stdout }, "stdout");
  }
  if (stderr) {
    appendLog(state, { ...event, node_id: nodeId, message: stderr }, "stderr");
    updateHost(state, { ...event, node_id: nodeId, message: stderr }, "stderr");
  }
}

export function reduceRunEvent(state = createInitialRunState(), event = {}) {
  const next = cloneState(state);
  const type = eventType(event);

  if (type === "run_queued") {
    next.runId = pickRunId(event) || next.runId;
    next.status = pick(event, "status") || "queued";
    next.startedAt = pick(event, "ts", "time", "timestamp") || next.startedAt;
    return next;
  }

  if (type === "run.started" || type === "run_start") {
    next.runId = pickRunId(event) || next.runId;
    next.status = "running";
    next.startedAt = pick(event, "ts", "started_at", "startedAt") || next.startedAt;
    return next;
  }

  if (type === "run.completed" || type === "run.failed" || type === "run.cancelled" || type === "run_finish") {
    next.runId = pickRunId(event) || next.runId;
    next.status = type === "run_finish" ? pick(event, "status") || "completed" : type.replace("run.", "");
    next.message = pick(event, "message", "summary") || next.message || "";
    next.error = pick(event, "error") || next.error || "";
    next.finishedAt = pick(event, "ts", "finished_at", "finishedAt") || next.finishedAt;
    return next;
  }

  if (type === "node.started" || type === "node_started" || type === "step_start") updateNode(next, event, "running");
  if (type === "node.completed" || type === "node_finished" || type === "step_finish") {
    updateNode(next, event, pick(event, "status") || "success");
  }
  if (type === "node.failed") updateNode(next, event, "failed");
  if (type === "host_result") appendHostResult(next, event);

  if (type === "edge.traversed" || type === "edge_selected") {
    const output = objectValue(event.output);
    const source = pick(event, "source") || output.source || "";
    const target = pick(event, "target") || output.target || "";
    const edgeId = pick(event, "edge_id", "edgeId", "id") || output.edge_id || output.edgeId || `${source}-${target}`;
    next.edges[edgeId] = {
      edgeId,
      source,
      target,
      kind: pick(event, "kind") || output.kind || "",
      status: type === "edge_selected" ? "selected" : "traversed",
      ts: pick(event, "ts", "time", "timestamp") || "",
    };
  }

  if (type === "host.stdout" || type === "stdout") {
    appendLog(next, event, "stdout");
    updateHost(next, event, "stdout");
  }

  if (type === "host.stderr" || type === "stderr") {
    appendLog(next, event, "stderr");
    updateHost(next, event, "stderr");
  }

  if (type === "sse.event") {
    appendLog(next, event, "sse");
  }

  if (type === "output_delta") {
    const stream = pick(event, "stream") || "stdout";
    appendLog(next, event, stream);
    updateHost(next, event, stream);
  }

  if (type === "approval.requested" || type === "approval.completed" || type === "approval_waiting" || type === "approval_resolved") {
    const id = pick(event, "approval_id", "approvalId", "id");
    if (id) {
      const existing = next.approvals.find((approval) => approval.id === id);
      next.approvals = upsertById(next.approvals, id, {
        id,
        nodeId: pick(event, "node_id", "nodeId") || existing?.nodeId || "",
        summary: pick(event, "summary", "message") || existing?.summary || "",
        status: type === "approval.completed" || type === "approval_resolved" ? pick(event, "status", "decision") || "completed" : "pending",
      });
    }
  }

  if (type === "retry.scheduled" || type === "retry") {
    next.retries.push({
      nodeId: pick(event, "node_id", "nodeId") || "",
      attempt: Number(pick(event, "attempt") || 0),
      maxAttempts: Number(pick(event, "max_attempts", "maxAttempts") || 0),
      reason: pick(event, "reason", "message") || "",
    });
  }

  if (type === "vars.input") appendVariable(next, event, "inputs");
  if (type === "vars.output") appendVariable(next, event, "outputs");
  if (type === "vars.exported") appendVariable(next, event, "exports");

  return next;
}

export function reduceRunEvents(events = [], initialState = createInitialRunState()) {
  return events.reduce((state, event) => reduceRunEvent(state, event), initialState);
}
