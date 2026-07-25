import { AlertTriangle, CheckCircle2 } from "lucide-react";

export type WorkflowStateBannerTone = "neutral" | "info" | "success" | "warning" | "danger";

export type WorkflowStateBannerProps = {
  title: string;
  description: string;
  tone?: WorkflowStateBannerTone;
  meta?: string;
};

export function WorkflowStateBanner({
  title,
  description,
  tone = "neutral",
  meta,
}: WorkflowStateBannerProps) {
  const positive = tone === "success" || tone === "info";
  return (
    <div
      role="status"
      className={`workflow-state-banner flex items-start gap-2 border-y px-3 py-2 ${TONE_CLASS[tone]}`}
      data-testid="workflow-run-gate-status"
      data-tone={tone}
    >
      {positive
        ? <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
        : <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />}
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-baseline justify-between gap-x-3">
          <p className="text-xs font-semibold">{title}</p>
          {meta ? <span className="text-[11px] opacity-75">{meta}</span> : null}
        </div>
        <p className="mt-0.5 text-xs leading-5 opacity-80">{description}</p>
      </div>
    </div>
  );
}

const TONE_CLASS: Record<WorkflowStateBannerTone, string> = {
  neutral: "border-slate-200 bg-slate-50 text-slate-700",
  info: "border-sky-200 bg-sky-50 text-sky-900",
  success: "border-emerald-200 bg-emerald-50 text-emerald-900",
  warning: "border-amber-200 bg-amber-50 text-amber-950",
  danger: "border-rose-200 bg-rose-50 text-rose-950",
};
