import { ChevronDown, ChevronUp, Copy } from "lucide-react";
import { useState } from "react";

export type WorkflowTargetTechnicalFact = {
  label: string;
  value: string;
};

export type WorkflowTargetRowProps = {
  kind: "draft" | "published";
  title: string;
  badge?: string;
  environment?: string;
  availability: string;
  enabled?: boolean;
  actionLabel?: string;
  onAction?: () => void;
  technicalFacts?: WorkflowTargetTechnicalFact[];
};

export function WorkflowTargetRow({
  kind,
  title,
  badge,
  environment,
  availability,
  enabled = false,
  actionLabel,
  onAction,
  technicalFacts = [],
}: WorkflowTargetRowProps) {
  const [detailsOpen, setDetailsOpen] = useState(false);
  const hasAction = enabled && actionLabel && onAction;
  return (
    <article className="workflow-target-row border-b border-slate-200 px-3 py-3 last:border-b-0" data-testid={`workflow-target-${kind}`}>
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h3 className="text-sm font-semibold text-slate-950">{title}</h3>
            {badge ? <span className="rounded-full bg-amber-100 px-2 py-0.5 text-[11px] font-semibold text-amber-800">{badge}</span> : null}
          </div>
          <p className="mt-1 text-xs text-slate-500">
            {environment ? `${environment} · ` : ""}
            {availability}
          </p>
        </div>
        {hasAction ? (
          <button
            type="button"
            data-testid={kind === "draft" ? "workflow-draft-dry-run" : "workflow-run-published"}
            className="inline-flex min-h-9 shrink-0 items-center rounded-lg border border-slate-300 px-3 text-xs font-medium text-slate-800 hover:bg-slate-50"
            onClick={onAction}
          >
            {actionLabel}
          </button>
        ) : null}
      </div>
      {technicalFacts.length ? (
        <div className="mt-2">
          <button
            type="button"
            data-testid="workflow-run-technical-details-toggle"
            aria-expanded={detailsOpen}
            className="inline-flex min-h-8 items-center gap-1 text-xs font-medium text-slate-600"
            onClick={() => setDetailsOpen((value) => !value)}
          >
            技术详情
            {detailsOpen ? <ChevronUp className="h-3.5 w-3.5" aria-hidden="true" /> : <ChevronDown className="h-3.5 w-3.5" aria-hidden="true" />}
          </button>
          {detailsOpen ? (
            <dl className="mt-2 space-y-2 rounded-lg bg-slate-50 p-2" data-testid="workflow-run-technical-details">
              {technicalFacts.map((fact) => (
                <div key={`${fact.label}:${fact.value}`} className="grid min-w-0 grid-cols-[auto_1fr_auto] items-start gap-2 text-xs">
                  <dt className="text-slate-500">{fact.label}</dt>
                  <dd className="break-all font-mono text-slate-800">{fact.value}</dd>
                  <button type="button" className="inline-flex min-h-8 min-w-8 items-center justify-center" aria-label={`复制${fact.label}`} onClick={() => void copyText(fact.value)}>
                    <Copy className="h-3.5 w-3.5" aria-hidden="true" />
                  </button>
                </div>
              ))}
            </dl>
          ) : null}
        </div>
      ) : null}
    </article>
  );
}

function copyText(value: string) {
  if (!navigator.clipboard?.writeText) return Promise.resolve();
  return navigator.clipboard.writeText(value).catch(() => undefined);
}
