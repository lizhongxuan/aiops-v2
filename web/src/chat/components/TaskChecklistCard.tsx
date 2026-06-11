import {
  BanIcon,
  CheckCircle2Icon,
  ChevronDownIcon,
  CircleIcon,
  LoaderCircleIcon,
  OctagonAlertIcon,
  XCircleIcon,
  type LucideIcon,
} from "lucide-react";
import { useMemo, useState } from "react";

import { cn } from "@/lib/utils";

import {
  buildTaskChecklistViewModel,
  type TaskChecklistItemInput,
  type TaskChecklistItemViewModel,
  type TaskChecklistStatus,
} from "./taskChecklistViewModel";

type TaskChecklistCardProps = {
  title: string;
  items: TaskChecklistItemInput[];
  defaultCollapsed?: boolean;
  summaryText?: (totalCount: number, completedCount: number) => string;
  className?: string;
  onItemClick?: (item: TaskChecklistItemViewModel) => void;
};

export function TaskChecklistCard({
  title,
  items,
  defaultCollapsed = false,
  summaryText = defaultSummaryText,
  className,
  onItemClick,
}: TaskChecklistCardProps) {
  const [collapsed, setCollapsed] = useState(defaultCollapsed);
  const viewModel = useMemo(() => buildTaskChecklistViewModel(items), [items]);
  const summary = summaryText(viewModel.totalCount, viewModel.completedCount);

  return (
    <section className={cn("min-w-0 px-3 py-2 text-xs", className)} data-testid="task-checklist-card">
      <button
        type="button"
        className="flex w-full min-w-0 items-center gap-2 text-left"
        aria-expanded={!collapsed}
        data-testid="task-checklist-toggle"
        onClick={() => setCollapsed((value) => !value)}
      >
        <ChevronDownIcon
          className={cn("size-3.5 shrink-0 text-zinc-500 transition-transform", collapsed ? "-rotate-90" : "rotate-0")}
          aria-hidden="true"
        />
        <span className="shrink-0 font-medium text-zinc-700">{title}</span>
        <span className="min-w-0 truncate text-zinc-500">{summary}</span>
      </button>

      {!collapsed && viewModel.items.length > 0 ? (
        <ol className="mt-2 grid min-w-0 gap-1">
          {viewModel.items.map((item) => (
            <li key={item.id} className="min-w-0">
              <TaskChecklistItem item={item} onClick={onItemClick} />
            </li>
          ))}
        </ol>
      ) : null}
    </section>
  );
}

function TaskChecklistItem({
  item,
  onClick,
}: {
  item: TaskChecklistItemViewModel;
  onClick?: (item: TaskChecklistItemViewModel) => void;
}) {
  const Icon = statusIcon(item.normalizedStatus);
  const clickable = Boolean(onClick);
  const content = (
    <>
      <Icon
        className={cn("size-3.5 shrink-0", statusColor(item.normalizedStatus), item.normalizedStatus === "running" && "animate-spin")}
        data-status={item.normalizedStatus}
        aria-label={statusLabel(item.normalizedStatus)}
      />
      <span className="min-w-0 flex-1 truncate text-zinc-700">
        {item.index}. {item.title}
      </span>
      {item.description ? <span className="min-w-0 flex-1 truncate text-zinc-500">{item.description}</span> : null}
      {item.meta ? <span className="shrink-0 text-zinc-500">{item.meta}</span> : null}
      {item.actionLabel ? (
        <span className="shrink-0 rounded-md border border-zinc-200 px-2 py-0.5 text-xs font-medium text-zinc-700">
          {item.actionLabel}
        </span>
      ) : null}
    </>
  );

  if (clickable) {
    return (
      <button
        type="button"
        className="flex min-h-7 w-full min-w-0 items-center gap-2 rounded-md px-1.5 py-1 text-left hover:bg-zinc-50"
        data-testid={item.testId || `task-checklist-item-${item.id}`}
        onClick={() => onClick?.(item)}
      >
        {content}
      </button>
    );
  }

  return (
    <div
      className="flex min-h-7 min-w-0 items-center gap-2 rounded-md px-1.5 py-1"
      data-testid={item.testId || `task-checklist-item-${item.id}`}
    >
      {content}
    </div>
  );
}

function defaultSummaryText(totalCount: number, completedCount: number) {
  return `共 ${totalCount} 个步骤，已经完成 ${completedCount} 个`;
}

function statusIcon(status: TaskChecklistStatus): LucideIcon {
  switch (status) {
    case "running":
      return LoaderCircleIcon;
    case "completed":
      return CheckCircle2Icon;
    case "blocked":
      return OctagonAlertIcon;
    case "failed":
      return XCircleIcon;
    case "cancelled":
      return BanIcon;
    case "pending":
    default:
      return CircleIcon;
  }
}

function statusColor(status: TaskChecklistStatus) {
  switch (status) {
    case "running":
      return "text-sky-600";
    case "completed":
      return "text-emerald-600";
    case "blocked":
      return "text-amber-600";
    case "failed":
      return "text-red-600";
    case "cancelled":
      return "text-zinc-400";
    case "pending":
    default:
      return "text-zinc-400";
  }
}

function statusLabel(status: TaskChecklistStatus) {
  switch (status) {
    case "running":
      return "运行中";
    case "completed":
      return "已完成";
    case "blocked":
      return "阻塞";
    case "failed":
      return "失败";
    case "cancelled":
      return "已取消";
    case "pending":
    default:
      return "待处理";
  }
}
