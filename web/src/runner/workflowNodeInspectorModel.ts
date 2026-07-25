import { variableToValueSource } from "@/components/runner/io/ioTypes";
import { getNodeCanvasMeta } from "@/components/runner/nodeTypeRegistry";
import { createInitialRunState } from "@/components/runner/runStateReducer";
import type { RunnerGraph, RunnerNode } from "@/components/runner/canvasGraphAdapter";

export type RunnerHostGroup = { label: string; hosts: string[] };
export type RunnerEndCallback = { event: string; url: string; payload: string };
export type RunnerVariableOption = {
  expression?: string;
  scope?: string;
  name?: string;
  type?: string;
  nodeId?: string;
  node_id?: string;
  sourceNodeId?: string;
  displayValue?: string;
};
export type RunnerRunState = ReturnType<typeof createInitialRunState>;

export const DEFAULT_HOST_GROUPS: RunnerHostGroup[] = [{ label: "local", hosts: ["local"] }];

export function objectValue(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {};
}

export function splitList(value: unknown) {
  if (Array.isArray(value)) return value.map((item) => String(item || "").trim()).filter(Boolean);
  return String(value || "")
    .split(/[\n,]+/)
    .map((item) => item.trim())
    .filter(Boolean);
}

export function cloneNode(node: RunnerNode): RunnerNode {
  return JSON.parse(JSON.stringify(node)) as RunnerNode;
}

export function isSystemRunnerNode(node: RunnerNode, meta?: { key?: string }) {
  const type = String(node.type || "").toLowerCase();
  return meta?.key === "start" || meta?.key === "end" || type === "start" || type === "end";
}

export function systemNodeLabel(node: RunnerNode, meta: { key?: string }) {
  if (meta.key === "end" || String(node.type || "").toLowerCase() === "end") return "结束节点";
  return "开始节点";
}

export function systemNodeHint(node: RunnerNode, meta: { key?: string }) {
  if (meta.key === "end" || String(node.type || "").toLowerCase() === "end") return "由工作流自动维护，负责结束流程并汇总最终状态。";
  return "由工作流自动维护，负责接收输入变量并初始化上下文。";
}

export function nodeDisplayName(node: RunnerNode, meta?: { key?: string }) {
  return String(node.label || node.step?.name || node.id);
}

export function actionEditorKind(action: string) {
  const value = String(action || "").trim();
  if (value === "shell.run" || value === "script.shell" || value === "script.python") return "script";
  if (value === "cmd.run") return "command";
  return "";
}

export function normalizeHostGroups(value: unknown): RunnerHostGroup[] {
  if (Array.isArray(value)) {
    return value
      .map((item, index) => {
        const group = objectValue(item);
        return {
          label: String(group.label || group.name || `group-${index + 1}`).trim(),
          hosts: splitList(group.hosts),
        };
      })
      .filter((group) => group.label && group.hosts.length);
  }
  const record = objectValue(value);
  return Object.entries(record)
    .map(([label, group]) => ({ label, hosts: splitList(objectValue(group).hosts || group) }))
    .filter((group) => group.label && group.hosts.length);
}

export function normalizeEditableHostGroups(value: unknown): RunnerHostGroup[] {
  if (Array.isArray(value)) {
    const groups = value.map((item, index) => {
      const group = objectValue(item);
      return {
        label: String(group.label || group.name || (index === 0 ? "local" : "")).trim(),
        hosts: splitList(group.hosts),
      };
    });
    return groups.length ? groups : DEFAULT_HOST_GROUPS;
  }
  const normalized = normalizeHostGroups(value);
  return normalized.length ? normalized : DEFAULT_HOST_GROUPS;
}

function hostGroupsFromInventory(graph: RunnerGraph): RunnerHostGroup[] {
  const inventory = objectValue(objectValue(graph.workflow).inventory);
  const groups = objectValue(inventory.groups);
  return Object.entries(groups)
    .map(([label, group]) => ({ label, hosts: splitList(objectValue(group).hosts || group) }))
    .filter((group) => group.label && group.hosts.length);
}

function readNodeHostGroups(node?: RunnerNode | null): RunnerHostGroup[] {
  if (!node) return [];
  return normalizeHostGroups(objectValue(node.ui).host_groups);
}

export function workflowHostGroups(graph: RunnerGraph, draft?: RunnerNode): RunnerHostGroup[] {
  const draftGroups = draft && (draft.type === "start" || getNodeCanvasMeta(draft).key === "start") ? readNodeHostGroups(draft) : [];
  if (draftGroups.length) return draftGroups;
  const startNode = (graph.nodes || []).find((item) => String(item.type || "").toLowerCase() === "start" || item.id === "start");
  const nodeGroups = readNodeHostGroups(startNode);
  if (nodeGroups.length) return nodeGroups;
  const inventoryGroups = hostGroupsFromInventory(graph);
  return inventoryGroups.length ? inventoryGroups : DEFAULT_HOST_GROUPS;
}

export function workflowDefaultTargetLabels(groups: RunnerHostGroup[]): string[] {
  const firstLabel = groups.map((group) => String(group.label || "").trim()).find(Boolean);
  return firstLabel ? [firstLabel] : ["local"];
}

export function shouldFillDefaultTargets(node: RunnerNode) {
  const action = String(node.step?.action || "").trim();
  if (!action) return false;
  if (normalizeTargetLabels(node.step?.targets || []).length) return false;
  const type = String(node.type || "").toLowerCase();
  if (["start", "end"].includes(type) || ["start", "end"].includes(node.id)) return false;
  const kind = actionEditorKind(action);
  return kind === "script" || kind === "command";
}

export function normalizeTargetLabels(value: unknown): string[] {
  return Array.from(new Set(splitList(value)));
}

export function normalizeEndCallbacks(value: unknown): RunnerEndCallback[] {
  const callbacks = Array.isArray(value)
    ? value.map((item) => {
      const callback = objectValue(item);
      return {
        event: String(callback.event || "success"),
        url: String(callback.url || ""),
        payload: String(callback.payload || ""),
      };
    })
    : [];
  return callbacks.length ? callbacks : [{ event: "success", url: "", payload: "" }];
}

export function readNodeCondition(node: RunnerNode): Record<string, unknown> {
  const data = objectValue(node.data);
  return { ...objectValue(data.condition), ...objectValue(node.condition) };
}

export function readNodeAggregator(node: RunnerNode): Record<string, unknown> {
  const data = objectValue(node.data);
  return {
    output_key: "aggregated_value",
    strategy: "first_non_empty",
    sources: [],
    ...objectValue(data.aggregator),
    ...objectValue(node.aggregator),
  };
}

export function normalizeAggregatorSources(sources: unknown): Record<string, unknown>[] {
  if (!Array.isArray(sources)) return [];
  return sources
    .map((source) => objectValue(source))
    .filter((source) => String(source.expression || objectValue(source.variable).name || "").trim());
}

export function variableToAggregatorSource(variable: RunnerVariableOption): Record<string, unknown> {
  const expression = String(variable.expression || "").trim();
  const valueSource = variableToValueSource(variable);
  return {
    expression,
    variable: objectValue(valueSource.variable),
  };
}

export function ensureAggregatorOutput(outputs: Record<string, unknown>[], outputKey: string) {
  if (!outputKey) return outputs;
  if (outputs.some((output) => String(output.key || "").trim() === outputKey)) return outputs;
  return [
    ...outputs,
    {
      key: outputKey,
      type: "any",
      description: "变量聚合输出",
    },
  ];
}

export function readableFailureMessage(value: unknown) {
  const message = String(value || "").trim();
  if (!message) return "";
  if (/failed to fetch/i.test(message)) {
    return `${message}（网络请求失败，请检查 ai-server 与 Runner 服务是否已启动，或 /api/runner-studio/runs 是否可访问。）`;
  }
  return message;
}

export function uniqueFailureLines(items: unknown[]) {
  const seen = new Set<string>();
  return items
    .map(readableFailureMessage)
    .flatMap((item) => item.split("\n").map((line) => line.trim()).filter(Boolean))
    .filter((item) => {
      if (seen.has(item)) return false;
      seen.add(item);
      return true;
    });
}

export function runnerNodeFailureMessage(
  node: { nodeId?: string; error?: unknown; message?: unknown; summary?: unknown; status?: unknown; result?: unknown },
  logs: { nodeId?: string; stream?: string; message?: string }[],
) {
  const failed = ["failed", "error"].includes(String(node.status || "").toLowerCase());
  if (!failed) return "";
  const nodeId = String(node.nodeId || "").trim();
  const result = objectValue(node.result);
  const nodeLogs = failed && nodeId
    ? logs
      .filter((log) => log.nodeId === nodeId && ["stderr", "stdout"].includes(String(log.stream || "")))
      .sort((a, b) => (a.stream === "stderr" ? -1 : 0) - (b.stream === "stderr" ? -1 : 0))
      .map((log) => `${log.stream || "log"}: ${String(log.message || "").trim()}`)
    : [];
  const details = uniqueFailureLines([
    node.error,
    node.message,
    node.summary,
    typeof node.result === "string" ? node.result : "",
    result.error,
    result.stderr,
    result.stdout,
    ...nodeLogs,
  ]);
  return details.join("\n") || (failed ? "未返回具体错误" : "");
}

export function collectNodeRunFailureDetails(state: RunnerRunState, nodeId: string) {
  const node = nodeId ? state.nodes?.[nodeId] : null;
  const nodeFailed = ["failed", "error"].includes(String(node?.status || "").toLowerCase());
  const runFailed = ["failed", "error"].includes(String(state.status || "").toLowerCase());
  if (node && !nodeFailed) return [];
  if (!node && !runFailed) return [];
  const logs = (state.logs || []).filter((log: { nodeId?: string }) => log.nodeId === nodeId);
  return uniqueFailureLines([
    runFailed ? state.message : "",
    runFailed ? state.error : "",
    node?.error,
    node?.message,
    runnerNodeFailureMessage(node || {}, logs),
  ]);
}

export function formatRuntimeValue(value: unknown) {
  if (value === undefined) return "";
  if (value === null) return "null";
  if (typeof value === "object") return JSON.stringify(value, null, 2);
  return String(value);
}
