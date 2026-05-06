import type { RunEvent, RunStatus, WorkflowGraph } from "../types/workflow";

export interface RunTimelineItem {
  id: string;
  type: string;
  nodeId?: string;
  edgeId?: string;
  status?: string;
  message?: string;
  timestamp?: string;
}

export interface RunHostResult {
  id: string;
  step?: string;
  host?: string;
  status?: string;
  message?: string;
  exitCode?: string | number;
  stdout?: string;
  stderr?: string;
  output?: Record<string, unknown>;
  timestamp?: string;
}

export interface RunLogLine {
  id: string;
  stream: "stdout" | "stderr";
  step?: string;
  host?: string;
  content: string;
  timestamp?: string;
}

export interface RunState {
  runId?: string;
  status: RunStatus;
  activeNodeIds: string[];
  timeline: RunTimelineItem[];
  nodeStatus: Record<string, RunStatus | string>;
  edgeStatus: Record<string, RunStatus | string>;
  hostResults: RunHostResult[];
  stdout: RunLogLine[];
  stderr: RunLogLine[];
  exportedVars: Record<string, unknown>;
  runnerDebug: Record<string, unknown>;
}

export const initialRunState: RunState = createInitialRunState();

export function createInitialRunState(): RunState {
  return {
    status: "idle",
    activeNodeIds: [],
    timeline: [],
    nodeStatus: {},
    edgeStatus: {},
    hostResults: [],
    stdout: [],
    stderr: [],
    exportedVars: {},
    runnerDebug: {},
  };
}

export function reduceRunEvent(state: RunState, event: RunEvent): RunState {
  const eventData = event.output || event.payload || {};
  const status = normalizeStatus(event.status || eventData.status);
  const nodeId = event.node_id || stringValue(eventData.node_id);
  const edgeId = event.edge_id || stringValue(eventData.edge_id);
  const timelineItem: RunTimelineItem = {
    id: event.id || `${event.type}-${state.timeline.length + 1}`,
    type: event.type,
    nodeId,
    edgeId,
    status,
    message: event.message || stringValue(eventData.message),
    timestamp: event.ts || event.timestamp,
  };

  const nextNodeStatus = { ...state.nodeStatus };
  if (nodeId && status) {
    nextNodeStatus[nodeId] = status;
  }
  const nextEdgeStatus = { ...state.edgeStatus };
  if (edgeId && status) {
    nextEdgeStatus[edgeId] = status;
  }

  const activeNodeIds = Object.entries(nextNodeStatus)
    .filter(([, value]) => value === "running" || value === "waiting")
    .map(([id]) => id);

  let nextHostResults = state.hostResults;
  let nextStdout = state.stdout;
  let nextStderr = state.stderr;
  let nextExportedVars = state.exportedVars;
  let nextRunnerDebug = state.runnerDebug;

  if (event.type === "host_result") {
    const output = recordValue(event.output) || recordValue(event.payload) || {};
    const result = buildHostResult(event, output, status);
    nextHostResults = upsertHostResult(nextHostResults, result);
    nextStdout = appendFinalStreamIfNeeded(nextStdout, result, "stdout");
    nextStderr = appendFinalStreamIfNeeded(nextStderr, result, "stderr");
    nextExportedVars = mergeRecords(nextExportedVars, exportedVarsFromOutput(output));
    nextRunnerDebug = mergeRecords(nextRunnerDebug, scopedRunnerDebugFromOutput(output, result.step, result.host));
  }

  if (event.type === "output_delta" || event.type === "output_chunk") {
    const stream = normalizeStream(eventData.stream);
    const content = stringValue(eventData.chunk) || stringValue(eventData.content) || stringValue(eventData.text);
    if (stream && content) {
      const line = buildLogLine(event, stream, content, state[stream].length + 1);
      if (stream === "stdout") {
        nextStdout = appendLogLine(nextStdout, line);
      } else {
        nextStderr = appendLogLine(nextStderr, line);
      }
    }
  }

  return {
    runId: event.run_id || state.runId,
    status: resolveRunStatus(state.status, event.type, status),
    activeNodeIds,
    timeline: [timelineItem, ...state.timeline].slice(0, 200),
    nodeStatus: nextNodeStatus,
    edgeStatus: nextEdgeStatus,
    hostResults: nextHostResults,
    stdout: nextStdout,
    stderr: nextStderr,
    exportedVars: nextExportedVars,
    runnerDebug: nextRunnerDebug,
  };
}

export function applyRunStateToGraph(graph: WorkflowGraph, state: RunState): WorkflowGraph {
  return {
    ...graph,
    nodes: graph.nodes.map((node) => ({
      ...node,
      state: state.nodeStatus[node.id]
        ? {
            ...node.state,
            run_id: state.runId,
            status: state.nodeStatus[node.id],
          }
        : node.state,
    })),
    edges: graph.edges.map((edge) => ({
      ...edge,
      state: state.edgeStatus[edge.id]
        ? {
            ...edge.state,
            status: state.edgeStatus[edge.id],
          }
        : edge.state,
    })),
  };
}

function resolveRunStatus(current: RunStatus, eventType: string, eventStatus?: string): RunStatus {
  const type = eventType.toLowerCase();
  if (type.includes("cancel")) return "canceled";
  if (type.includes("fail") || eventStatus === "failed") return "failed";
  if (type === "run_finish" || type === "run_complete" || type === "run_completed") {
    return eventStatus === "failed" ? "failed" : eventStatus === "canceled" || eventStatus === "cancelled" ? "canceled" : "success";
  }
  if (type === "run_start" || type === "run_started" || type === "run_running") return "running";
  if (type === "run_queued" || eventStatus === "queued") return "queued";
  if (type === "approval_waiting") return "waiting";
  if (type === "approval_resolved") {
    if (eventStatus === "failed") return "failed";
    return current === "waiting" ? "running" : current;
  }
  if (type === "node_started" || type === "step_start") return current === "idle" || current === "queued" ? "running" : current;
  if (type.startsWith("node_") || type.startsWith("step_") || type === "host_result") return current;
  if (eventStatus === "waiting") return "waiting";
  if (eventStatus === "selected") return current;
  return current;
}

function normalizeStatus(value: unknown): RunStatus | string | undefined {
  return typeof value === "string" ? value : undefined;
}

function stringValue(value: unknown): string | undefined {
  return typeof value === "string" ? value : undefined;
}

function numberValue(value: unknown): number | undefined {
  return typeof value === "number" ? value : undefined;
}

function recordValue(value: unknown): Record<string, unknown> | undefined {
  if (!value || typeof value !== "object" || Array.isArray(value)) return undefined;
  return value as Record<string, unknown>;
}

function buildHostResult(event: RunEvent, output: Record<string, unknown>, status?: string): RunHostResult {
  const step = event.step || stringValue(output.step);
  const host = event.host || stringValue(output.host);
  return {
    id: event.id || `host-result-${step || "step"}-${host || "host"}`,
    step,
    host,
    status,
    message: event.message || stringValue(output.message),
    exitCode: numberValue(output.exit_code) ?? numberValue(output.exitCode) ?? stringValue(output.exit_code) ?? stringValue(output.exitCode),
    stdout: stringValue(output.stdout),
    stderr: stringValue(output.stderr),
    output,
    timestamp: event.ts || event.timestamp,
  };
}

function upsertHostResult(items: RunHostResult[], result: RunHostResult): RunHostResult[] {
  const key = hostResultKey(result);
  const next = items.filter((item) => hostResultKey(item) !== key);
  return [result, ...next].slice(0, 200);
}

function hostResultKey(result: RunHostResult): string {
  return `${result.step || ""}\u0000${result.host || ""}`;
}

function appendFinalStreamIfNeeded(lines: RunLogLine[], result: RunHostResult, stream: "stdout" | "stderr"): RunLogLine[] {
  const content = stream === "stdout" ? result.stdout : result.stderr;
  if (!content) return lines;
  const hasDelta = lines.some((line) => line.step === result.step && line.host === result.host);
  if (hasDelta) return lines;
  return appendLogLine(lines, {
    id: `${result.id}-${stream}`,
    stream,
    step: result.step,
    host: result.host,
    content,
    timestamp: result.timestamp,
  });
}

function buildLogLine(event: RunEvent, stream: "stdout" | "stderr", content: string, index: number): RunLogLine {
  const data = event.output || event.payload || {};
  return {
    id: event.id ? `${event.id}-${stream}` : `${event.type}-${stream}-${index}`,
    stream,
    step: event.step || stringValue(data.step),
    host: event.host || stringValue(data.host),
    content,
    timestamp: event.ts || event.timestamp,
  };
}

function appendLogLine(lines: RunLogLine[], line: RunLogLine): RunLogLine[] {
  return [line, ...lines].slice(0, 500);
}

function exportedVarsFromOutput(output: Record<string, unknown>): Record<string, unknown> {
  return recordValue(output.vars) || recordValue(output.exported_vars) || recordValue(output.exportedVars) || {};
}

function scopedRunnerDebugFromOutput(output: Record<string, unknown>, step?: string, host?: string): Record<string, unknown> {
  const debug = recordValue(output.runner_debug) || recordValue(output.runnerDebug);
  if (!debug) return {};
  const key = [step, host].filter(Boolean).join("/") || "run";
  return { [key]: debug };
}

function mergeRecords(left: Record<string, unknown>, right: Record<string, unknown>): Record<string, unknown> {
  if (Object.keys(right).length === 0) return left;
  return { ...left, ...right };
}

function normalizeStream(value: unknown): "stdout" | "stderr" | undefined {
  return value === "stdout" || value === "stderr" ? value : undefined;
}
