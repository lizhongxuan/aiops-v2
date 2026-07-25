import { AlertTriangle, CheckCircle2, ChevronDown, ChevronUp, RotateCcw } from "lucide-react";
import { useMemo, useState } from "react";

export type WorkflowRunSessionRecord = {
  id: string;
  status: "running" | "completed" | "failed" | "canceled";
  targetLabel: string;
  startedAt: string;
  durationMs?: number;
  variables?: Array<{ name: string; value: string }>;
  path?: string[];
  logs?: string[];
  failure?: { action: string; savedState: string; message: string };
};

export type WorkflowRunInspectorProps = {
  records: WorkflowRunSessionRecord[];
  onRetry?: (runId: string) => void;
};

export function WorkflowRunInspector({ records, onRetry }: WorkflowRunInspectorProps) {
  const validRecords = useMemo(() => records.filter(validRecord), [records]);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const selected = selectedId === null
    ? undefined
    : validRecords.find((record) => record.id === selectedId) || validRecords[0];

  return (
    <section className="workflow-run-inspector min-h-0" aria-labelledby="workflow-run-inspector-title" data-testid="workflow-run-inspector">
      <header className="flex items-baseline justify-between gap-3 px-3 pb-2 pt-4">
        <h3 id="workflow-run-inspector-title" className="text-sm font-semibold text-slate-950">最近运行</h3>
        <span className="text-[11px] text-slate-500">当前会话</span>
      </header>
      {!validRecords.length ? (
        <p className="px-3 pb-4 text-xs leading-5 text-slate-500">还没有运行记录。试运行后会显示结果和耗时。</p>
      ) : (
        <ol className="divide-y divide-slate-100 border-y border-slate-200" aria-label="当前会话运行记录">
          {validRecords.map((record) => {
            const expanded = selected?.id === record.id;
            return (
              <li key={record.id}>
                <button
                  type="button"
                  className="flex min-h-11 w-full items-center gap-2 px-3 text-left"
                  aria-expanded={expanded}
                  onClick={() => setSelectedId(expanded ? null : record.id)}
                >
                  <RunStatusIcon status={record.status} />
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-xs font-medium text-slate-900">{record.targetLabel}</span>
                    <span className="block text-[11px] text-slate-500">{formatTime(record.startedAt)}{record.durationMs !== undefined ? ` · ${formatDuration(record.durationMs)}` : ""}</span>
                  </span>
                  {expanded ? <ChevronUp className="h-4 w-4 text-slate-500" aria-hidden="true" /> : <ChevronDown className="h-4 w-4 text-slate-500" aria-hidden="true" />}
                </button>
                {expanded ? <RunDetails record={record} onRetry={onRetry} /> : null}
              </li>
            );
          })}
        </ol>
      )}
    </section>
  );
}

function RunDetails({ record, onRetry }: { record: WorkflowRunSessionRecord; onRetry?: (runId: string) => void }) {
  return (
    <div className="space-y-3 bg-slate-50 px-3 py-3 text-xs" data-testid="workflow-run-details">
      {record.failure ? (
        <div className="rounded-lg border border-rose-200 bg-rose-50 p-2 text-rose-950" role="alert">
          <p className="font-semibold">{record.failure.action}失败</p>
          <p className="mt-1">{record.failure.savedState}。{record.failure.message}</p>
          {onRetry ? (
            <button type="button" data-testid="workflow-run-retry" className="mt-2 inline-flex min-h-9 items-center gap-1 rounded-lg border border-rose-300 px-2 font-medium" onClick={() => onRetry(record.id)}>
              <RotateCcw className="h-3.5 w-3.5" aria-hidden="true" />重试
            </button>
          ) : null}
        </div>
      ) : null}
      {record.variables?.length ? <DetailList title="变量" items={record.variables.map((item) => `${item.name} = ${item.value}`)} /> : null}
      {record.path?.length ? <DetailList title="路径" items={[record.path.join(" → ")]} /> : null}
      {record.logs?.length ? <DetailList title="日志" items={record.logs} mono /> : null}
    </div>
  );
}

function DetailList({ title, items, mono = false }: { title: string; items: string[]; mono?: boolean }) {
  return (
    <section>
      <h4 className="font-semibold text-slate-800">{title}</h4>
      <ul className={`mt-1 space-y-1 text-slate-600 ${mono ? "font-mono" : ""}`}>
        {items.map((item, index) => <li key={`${item}:${index}`} className="break-all">{item}</li>)}
      </ul>
    </section>
  );
}

function RunStatusIcon({ status }: { status: WorkflowRunSessionRecord["status"] }) {
  if (status === "failed") return <AlertTriangle className="h-4 w-4 shrink-0 text-rose-700" aria-label="失败" />;
  return <CheckCircle2 className={`h-4 w-4 shrink-0 ${status === "completed" ? "text-emerald-700" : "text-slate-500"}`} aria-label={status === "completed" ? "完成" : status === "running" ? "运行中" : "已取消"} />;
}

function validRecord(record: WorkflowRunSessionRecord) {
  return Boolean(record.id.trim() && record.targetLabel.trim() && !Number.isNaN(Date.parse(record.startedAt)));
}

function formatTime(value: string) {
  return new Intl.DateTimeFormat("zh-CN", { hour: "2-digit", minute: "2-digit", hour12: false }).format(new Date(value));
}

function formatDuration(value: number) {
  if (value < 1000) return `${value}ms`;
  return `${(value / 1000).toFixed(value < 10000 ? 1 : 0)}s`;
}
