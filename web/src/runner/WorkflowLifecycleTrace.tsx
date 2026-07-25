import {
  AlertTriangle,
  CheckCircle2,
  Circle,
  LoaderCircle,
} from "lucide-react";

import type {
  WorkflowLifecycleStageState,
  WorkflowLifecycleTraceViewModel,
} from "./workspace/workflowInteractionContract";

export type WorkflowLifecycleTraceProps = {
  view: WorkflowLifecycleTraceViewModel;
};

export function WorkflowLifecycleTrace({ view }: WorkflowLifecycleTraceProps) {
  return (
    <div
      className="workflow-lifecycle-trace min-w-0"
      data-testid="workflow-lifecycle-trace"
      aria-label={view.ariaLabel}
    >
      <ol
        className="workflow-lifecycle-trace__full hidden min-[1180px]:flex min-w-0 items-center gap-2"
        aria-hidden="true"
      >
        {view.stages.map((stage, index) => (
          <li key={stage.id} className="contents">
            {index > 0 ? <span className="h-px min-w-3 flex-1 bg-slate-200" /> : null}
            <span
              className={`workflow-lifecycle-trace__stage inline-flex min-w-0 items-center gap-1.5 text-xs ${stageTone(stage.state)}`}
              data-lifecycle-stage={stage.id}
              data-testid={stage.id === "save"
                ? "workflow-save-status"
                : stage.id === "check"
                  ? "workflow-validation-status"
                  : undefined}
              data-state={stage.state}
              title={`${stage.label}：${stage.description}`}
            >
              {stageIcon(stage.state)}
              <span className="truncate">{stage.description}</span>
            </span>
          </li>
        ))}
      </ol>
      <span
        className="workflow-lifecycle-trace__compact inline-flex min-h-8 max-w-full items-center gap-1.5 rounded-full bg-slate-100 px-2.5 text-xs font-medium text-slate-700 min-[1180px]:hidden"
        aria-hidden="true"
      >
        {stageIcon(currentSummaryState(view))}
        <span className="truncate">{view.compactLabel}</span>
      </span>
    </div>
  );
}

function currentSummaryState(view: WorkflowLifecycleTraceViewModel): WorkflowLifecycleStageState {
  return view.stages.find((stage) => stage.state === "blocked")?.state ??
    view.stages.find((stage) => stage.state === "in_progress")?.state ??
    view.stages.find((stage) => stage.state === "current")?.state ??
    "complete";
}

function stageIcon(state: WorkflowLifecycleStageState) {
  if (state === "complete") return <CheckCircle2 className="h-3.5 w-3.5 shrink-0" />;
  if (state === "in_progress") return <LoaderCircle className="h-3.5 w-3.5 shrink-0" />;
  if (state === "blocked") return <AlertTriangle className="h-3.5 w-3.5 shrink-0" />;
  return <Circle className="h-3.5 w-3.5 shrink-0" />;
}

function stageTone(state: WorkflowLifecycleStageState): string {
  if (state === "complete") return "text-emerald-700";
  if (state === "current") return "font-medium text-blue-700";
  if (state === "blocked") return "font-medium text-red-700";
  if (state === "in_progress") return "font-medium text-blue-700";
  return "text-slate-500";
}
