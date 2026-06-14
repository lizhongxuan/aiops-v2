export type TaskChecklistStatus = "pending" | "running" | "completed" | "blocked" | "failed" | "cancelled";

export type TaskChecklistItemInput = {
  id?: string;
  index?: number;
  title: string;
  description?: string;
  status?: string;
  meta?: string;
  actionLabel?: string;
  testId?: string;
};

export type TaskChecklistItemViewModel = TaskChecklistItemInput & {
  id: string;
  index: number;
  normalizedStatus: TaskChecklistStatus;
};

export type TaskChecklistViewModel = {
  items: TaskChecklistItemViewModel[];
  totalCount: number;
  completedCount: number;
};

const completedStatuses = new Set(["completed", "done", "success"]);

export function buildTaskChecklistViewModel(items: TaskChecklistItemInput[]): TaskChecklistViewModel {
  const normalizedItems = items.map((item, index) => ({
    ...item,
    id: item.id || `task-${index + 1}`,
    index: item.index ?? index + 1,
    normalizedStatus: normalizeTaskChecklistStatus(item.status),
  }));

  return {
    items: normalizedItems,
    totalCount: normalizedItems.length,
    completedCount: normalizedItems.filter((item) => completedStatuses.has(String(item.status || "").toLowerCase()))
      .length,
  };
}

export function normalizeTaskChecklistStatus(status?: string): TaskChecklistStatus {
  switch (String(status || "").toLowerCase()) {
    case "running":
    case "spawning":
    case "in_progress":
      return "running";
    case "completed":
    case "done":
    case "success":
      return "completed";
    case "approval_required":
    case "blocked":
      return "blocked";
    case "failed":
    case "error":
      return "failed";
    case "cancelled":
    case "canceled":
      return "cancelled";
    case "pending":
    case "planned":
    case "queued":
    case "waiting":
    default:
      return "pending";
  }
}
