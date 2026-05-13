const FAILED_STATUSES = new Set(["failed", "error"]);
const RUNNING_STATUSES = new Set(["running", "queued", "waiting", "pending"]);
const SUCCESS_STATUSES = new Set(["success", "completed", "succeeded"]);

function normalizeStatus(status) {
  return String(status || "").trim().toLowerCase();
}

function pickMessage(nodeRun) {
  const direct = nodeRun?.message || nodeRun?.error || nodeRun?.summary;
  if (typeof direct === "string") return direct;
  const result = nodeRun?.result;
  if (typeof result === "string") return result;
  if (result && typeof result === "object") return result.stderr || result.stdout || "";
  return "";
}

function graphNodeIds(graph = {}) {
  return (graph.nodes || []).map((node) => String(node?.id || "")).filter(Boolean);
}

function isSystemNode(node) {
  const type = normalizeStatus(node?.type);
  return type === "start" || type === "end";
}

export function firstRunnableNodeId(graph = {}) {
  const runnable = (graph.nodes || []).find((node) => node?.id && !isSystemNode(node));
  return String(runnable?.id || "");
}

export function getRunnerNodeRunState(runState = {}, nodeId = "") {
  const nodeRun = runState?.nodes?.[nodeId];
  const rawStatus = normalizeStatus(nodeRun?.status);
  const message = pickMessage(nodeRun);

  if (!rawStatus) return { status: "", label: "", message: "" };
  if (FAILED_STATUSES.has(rawStatus)) return { status: "failed", label: "失败", message };
  if (RUNNING_STATUSES.has(rawStatus)) return { status: "running", label: "运行中", message };
  if (SUCCESS_STATUSES.has(rawStatus)) return { status: "success", label: "成功", message };
  if (rawStatus === "skipped") return { status: "skipped", label: "跳过", message };
  return { status: rawStatus, label: rawStatus, message };
}

export function getRunnerFocusNodeId({ graph = {}, runState = {}, explicitNodeId = "" } = {}) {
  const nodeIds = graphNodeIds(graph);
  const nodes = runState?.nodes || {};
  const failed = nodeIds.find((nodeId) => FAILED_STATUSES.has(normalizeStatus(nodes[nodeId]?.status)));
  if (failed) return failed;

  const running = nodeIds.find((nodeId) => RUNNING_STATUSES.has(normalizeStatus(nodes[nodeId]?.status)));
  if (running) return running;

  if (explicitNodeId && nodeIds.includes(explicitNodeId)) return explicitNodeId;
  return "";
}
