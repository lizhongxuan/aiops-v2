export function createInitialRunState() {
  return {
    runId: "",
    status: "idle",
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

function pickRunId(event) {
  return pick(event, "run_id", "runId", "id") || "";
}

function pickNodeId(event) {
  return pick(event, "node_id", "nodeId", "step", "node", "id") || "";
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
    message: pick(event, "message", "data", "text", "summary") || "",
    ts: pick(event, "ts", "time", "timestamp") || "",
  });
}

function updateNode(state, event, status) {
  const nodeId = pickNodeId(event);
  if (!nodeId) return;
  const result = pick(event, "result", "output");
  state.nodes[nodeId] = {
    ...(state.nodes[nodeId] || {}),
    nodeId,
    status: pick(event, "status") || status,
    startedAt: pick(event, "started_at", "startedAt") || state.nodes[nodeId]?.startedAt || "",
    finishedAt: pick(event, "finished_at", "finishedAt") || state.nodes[nodeId]?.finishedAt || "",
    durationMs: pick(event, "duration_ms", "durationMs") || state.nodes[nodeId]?.durationMs || 0,
    result: result === undefined ? state.nodes[nodeId]?.result : result,
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
    next.finishedAt = pick(event, "ts", "finished_at", "finishedAt") || next.finishedAt;
    return next;
  }

  if (type === "node.started" || type === "node_started" || type === "step_start") updateNode(next, event, "running");
  if (type === "node.completed" || type === "node_finished" || type === "step_finish") {
    updateNode(next, event, pick(event, "status") || "success");
  }
  if (type === "node.failed") updateNode(next, event, "failed");
  if (type === "host_result") appendHostResult(next, event);

  if (type === "edge.traversed") {
    const edgeId = pick(event, "edge_id", "edgeId", "id") || `${pick(event, "source")}-${pick(event, "target")}`;
    next.edges[edgeId] = {
      edgeId,
      source: pick(event, "source") || "",
      target: pick(event, "target") || "",
      status: "traversed",
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

  if (type === "approval.requested" || type === "approval.completed") {
    const id = pick(event, "approval_id", "approvalId", "id");
    if (id) {
      next.approvals = upsertById(next.approvals, id, {
        id,
        nodeId: pick(event, "node_id", "nodeId") || "",
        summary: pick(event, "summary", "message") || "",
        status: type === "approval.completed" ? pick(event, "status", "decision") || "completed" : "pending",
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
