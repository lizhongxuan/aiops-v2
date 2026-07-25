import { useMemo, useState } from "react";
import { Bot, X } from "lucide-react";
import type { WorkflowAiEvent } from "./workflowAiTypes";

type WorkflowEventFilter = "all" | "ai" | "manual" | "run" | "error";

const WORKFLOW_EVENT_FILTERS: Array<{ key: WorkflowEventFilter; label: string }> = [
  { key: "all", label: "全部" },
  { key: "ai", label: "AI" },
  { key: "manual", label: "人工" },
  { key: "run", label: "运行" },
  { key: "error", label: "错误" },
];

function workflowEventMatchesFilter(event: WorkflowAiEvent, filter: WorkflowEventFilter) {
  if (filter === "all") return true;
  const type = String(event.type || "").toLowerCase();
  if (filter === "ai") {
    return (
      event.actor === "assistant" ||
      type.startsWith("workflow.ai") ||
      type.startsWith("workflow.graph.") ||
      type.startsWith("workflow.node.") ||
      type.includes(".script.")
    );
  }
  if (filter === "manual") return event.actor === "user";
  if (filter === "run") return type.includes("run") || type.includes("node.started") || type.includes("node.completed");
  return type.includes("failed") || type.includes("error") || type.includes("conflict");
}

function workflowEventTimeLabel(createdAt?: string) {
  if (!createdAt) return "-";
  const date = new Date(createdAt);
  if (Number.isNaN(date.getTime())) return createdAt;
  return date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });
}

export function WorkflowEventDrawer({
  open,
  events,
  onClose,
  onBackToAi,
  onSelectNodeIds,
}: {
  open: boolean;
  events: WorkflowAiEvent[];
  onClose?: () => void;
  onBackToAi?: () => void;
  onSelectNodeIds?: (nodeIds: string[]) => void;
}) {
  const [filter, setFilter] = useState<WorkflowEventFilter>("all");
  const visibleEvents = useMemo(
    () => events
      .filter((event) => workflowEventMatchesFilter(event, filter))
      .slice()
      .sort((left, right) => new Date(right.createdAt || 0).getTime() - new Date(left.createdAt || 0).getTime()),
    [events, filter],
  );
  if (!open) return null;
  return (
    <aside className="workflow-event-drawer" data-testid="workflow-event-drawer">
      <header>
        <div>
          <h2>事件</h2>
          <span>{events.length} 条工作流 AI 事件</span>
        </div>
        <div className="workflow-event-header-actions">
          <button type="button" className="workflow-event-back-ai" onClick={onBackToAi}>
            <Bot size={14} />
            返回 AI
          </button>
          <button type="button" aria-label="关闭事件" onClick={onClose}>
            <X size={16} />
          </button>
        </div>
      </header>
      <nav className="workflow-event-filters" aria-label="事件筛选">
        {WORKFLOW_EVENT_FILTERS.map((item) => (
          <button
            key={item.key}
            type="button"
            className={filter === item.key ? "active" : ""}
            data-testid={`workflow-event-filter-${item.key}`}
            onClick={() => setFilter(item.key)}
          >
            {item.label}
          </button>
        ))}
      </nav>
      <div className="workflow-event-list">
        {visibleEvents.length ? visibleEvents.map((event) => (
          <button
            key={event.id}
            type="button"
            className="workflow-event-row"
            data-testid="workflow-event-row"
            onClick={() => onSelectNodeIds?.(event.visibleNodeIds || [])}
          >
            <span>{workflowEventTimeLabel(event.createdAt)} · {workflowEventActorLabel(event.actor)} · {workflowEventTypeLabel(event.type)}</span>
            <strong>{event.summary}</strong>
            {event.planItemId ? <small>计划步骤：{event.planItemId}</small> : null}
            {event.visibleNodeIds?.length ? <small>{event.visibleNodeIds.join(", ")}</small> : null}
          </button>
        )) : (
          <p className="workflow-event-empty">还没有工作流 AI 修改事件。</p>
        )}
      </div>
    </aside>
  );
}

function workflowEventActorLabel(actor?: string) {
  return ({
    assistant: "AI 助手",
    agent: "AI 助手",
    tool: "工具执行",
    user: "用户",
    system: "系统",
  } as Record<string, string>)[String(actor || "system").toLowerCase()] || "系统";
}

function workflowEventTypeLabel(type?: string) {
  const normalized = String(type || "").toLowerCase();
  const exact = ({
    "workflow.ai.chat": "AI 对话",
    "workflow.graph.node.added": "新增工作流步骤",
    "workflow.graph.node.updated": "更新工作流步骤",
    "workflow.graph.node.deleted": "删除工作流步骤",
    "workflow.graph.edge.added": "新增工作流连线",
    "workflow.graph.edge.updated": "更新工作流连线",
    "workflow.graph.edge.deleted": "删除工作流连线",
    "workflow.conflict": "修改发生冲突",
  } as Record<string, string>)[normalized];
  if (exact) return exact;
  if (normalized.includes("conflict")) return "修改发生冲突";
  if (normalized.includes("failed") || normalized.includes("error")) return "执行失败";
  if (normalized.includes("run") || normalized.includes("started") || normalized.includes("completed")) return "运行状态变化";
  if (normalized.includes("candidate") || normalized.includes("preview")) return "待确认修改已更新";
  return "工作流事件";
}
