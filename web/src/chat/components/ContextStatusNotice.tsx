import { AlertTriangle, CheckCircle2, Info, ShieldAlert } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";

export type ContextStatusEvent = {
  kind: string;
  message?: string;
  retryAttempt?: number;
  retryMax?: number;
  summary?: string;
  compactSummary?: string;
  referenceIds?: string[];
  layer?: string;
};

type ContextStatusNoticeProps = {
  event?: ContextStatusEvent | null;
};

export const CONTEXT_STATUS_LABELS: Record<string, string> = {
  "context.compaction.started": "正在压缩上下文，当前任务会继续",
  "context.compaction.too_long": "上下文过长，已保留关键摘要并进入保守模式",
  "context.compaction.completed": "已整理早期上下文",
  "context.compaction.failed": "上下文过长或压缩失败，已进入保守模式",
  "context.small_context.enabled": "当前模型上下文较小，系统会优先保留当前任务和关键摘要",
};

export function ContextStatusNotice({ event }: ContextStatusNoticeProps) {
  if (!event) return null;

  const message = event.message || CONTEXT_STATUS_LABELS[event.kind] || "上下文状态已更新";
  const summary = event.compactSummary || event.summary || "";
  const tone = toneForKind(event.kind);
  const Icon = iconForKind(event.kind);

  return (
    <div
      className={cn(
        "flex min-w-0 items-start gap-2 rounded-md border px-3 py-2 text-sm",
        tone.className,
      )}
      role={event.kind === "context.compaction.failed" ? "alert" : "status"}
      data-testid="context-status-notice"
    >
      <Icon className="mt-0.5 h-4 w-4 shrink-0" />
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-2">
          {event.layer ? (
            <Badge variant="outline" className={tone.badgeClassName}>
              {event.layer}
            </Badge>
          ) : null}
          <span className="break-words font-medium">{message}</span>
        </div>
        {summary ? (
          <details className="mt-1 text-xs leading-5">
            <summary className="cursor-pointer font-medium">查看压缩摘要</summary>
            <div className="mt-1 whitespace-pre-wrap break-words">{summary}</div>
          </details>
        ) : null}
        {event.kind === "context.compaction.failed" ? (
          <div className="mt-1 text-xs leading-5 opacity-90">系统会优先保留当前任务和关键摘要，必要时可继续基于摘要排查。</div>
        ) : null}
        {event.referenceIds?.length ? (
          <div className="mt-1 flex flex-wrap gap-1 text-xs">
            <Badge variant="outline" className={tone.badgeClassName}>
              已保留 {event.referenceIds.length} 项上下文引用
            </Badge>
          </div>
        ) : null}
      </div>
    </div>
  );
}

function toneForKind(kind: string) {
  if (kind === "context.compaction.failed") {
    return {
      className: "border-red-200 bg-red-50 text-red-800",
      badgeClassName: "bg-red-50 text-red-700 border-red-200",
    };
  }
  if (kind === "context.compaction.started") {
    return {
      className: "border-amber-200 bg-amber-50 text-amber-900",
      badgeClassName: "bg-amber-50 text-amber-700 border-amber-200",
    };
  }
  if (kind === "context.compaction.completed") {
    return {
      className: "border-emerald-200 bg-emerald-50 text-emerald-800",
      badgeClassName: "bg-emerald-50 text-emerald-700 border-emerald-200",
    };
  }
  return {
    className: "border-blue-200 bg-blue-50 text-blue-900",
    badgeClassName: "bg-blue-50 text-blue-700 border-blue-200",
  };
}

function iconForKind(kind: string) {
  switch (kind) {
    case "context.compaction.failed":
    case "context.compaction.too_long":
      return ShieldAlert;
    case "context.compaction.completed":
      return CheckCircle2;
    case "context.compaction.started":
      return AlertTriangle;
    default:
      return Info;
  }
}
