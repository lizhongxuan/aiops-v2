import {
  ArrowLeft,
  Bot,
  FlaskConical,
  Play,
  Redo2,
  Rocket,
  SearchCheck,
  Undo2,
  Wrench,
} from "lucide-react";

import {
  WorkflowActionMenu,
  type WorkflowActionMenuItem,
} from "./WorkflowActionMenu";
import { WorkflowLifecycleTrace } from "./WorkflowLifecycleTrace";
import type {
  WorkflowLifecycleTraceViewModel,
  WorkflowPrimaryActionId,
  WorkflowPrimaryActionViewModel,
} from "./workspace/workflowInteractionContract";

const EMPTY_MENU_ITEMS: readonly WorkflowActionMenuItem[] = [];

export type WorkflowCommandBarProps = {
  title: string;
  lifecycleLabel: string;
  trace: WorkflowLifecycleTraceViewModel;
  primaryAction?: WorkflowPrimaryActionViewModel | null;
  blockedReason?: string;
  onPrimaryAction?: (action: WorkflowPrimaryActionViewModel) => void;
  onBack?: () => void;
  onUndo?: () => void;
  onRedo?: () => void;
  onOpenWorkbench?: () => void;
  menuItems?: readonly WorkflowActionMenuItem[];
  showIdentity?: boolean;
};

export function WorkflowCommandBar({
  title,
  lifecycleLabel,
  trace,
  primaryAction,
  blockedReason,
  onPrimaryAction,
  onBack,
  onUndo,
  onRedo,
  onOpenWorkbench,
  menuItems = EMPTY_MENU_ITEMS,
  showIdentity = true,
}: WorkflowCommandBarProps) {
  const commandBarAction = primaryAction?.owner === "command_bar" &&
    primaryAction.enabled &&
    onPrimaryAction
    ? primaryAction
    : undefined;

  return (
    <div
      className="workflow-command-bar flex min-h-16 min-w-0 items-center gap-4 border-b border-slate-200 bg-white px-4"
      data-testid="workflow-command-bar"
    >
      {showIdentity ? <div className="workflow-command-bar__identity flex min-w-0 shrink items-center gap-2">
        {onBack ? (
          <button
            type="button"
            className="inline-flex min-h-9 shrink-0 items-center gap-1 rounded-lg border border-slate-300 bg-white px-2.5 text-sm font-semibold text-slate-900 hover:bg-slate-50"
            onClick={onBack}
            aria-label="返回工作流列表"
            data-testid="runner-back-to-library"
          >
            <ArrowLeft className="h-4 w-4" aria-hidden="true" />
            <span>工作流</span>
          </button>
        ) : null}
        <strong className="truncate text-base font-semibold text-slate-950" title={title}>{title}</strong>
        <span className="shrink-0 rounded-full border border-slate-200 bg-slate-50 px-2 py-1 text-xs font-medium text-slate-600">
          {lifecycleLabel}
        </span>
      </div> : null}

      <div className="workflow-command-bar__status min-w-0 flex-1">
        <WorkflowLifecycleTrace view={trace} />
      </div>

      <div className="workflow-command-bar__actions flex shrink-0 items-center justify-end gap-2">
        {onUndo ? (
          <IconButton label="撤销上一次修改" onClick={onUndo}>
            <Undo2 className="h-4 w-4" aria-hidden="true" />
          </IconButton>
        ) : null}
        {onRedo ? (
          <IconButton label="重做上一次修改" onClick={onRedo}>
            <Redo2 className="h-4 w-4" aria-hidden="true" />
          </IconButton>
        ) : null}
        {onOpenWorkbench ? (
          <button
            type="button"
            className="workflow-workbench-trigger inline-flex min-h-9 items-center gap-1.5 rounded-lg border border-slate-300 bg-white px-3 text-sm font-medium text-slate-800 hover:bg-slate-50 max-[859px]:w-9 max-[859px]:justify-center max-[859px]:px-0"
            data-testid="workflow-workbench-trigger"
            onClick={onOpenWorkbench}
            aria-label="打开工作台"
          >
            <Bot className="h-4 w-4" aria-hidden="true" />
            <span className="max-[859px]:sr-only">工作台</span>
          </button>
        ) : null}
        {commandBarAction ? (
          <button
            type="button"
            className="workflow-primary-action inline-flex min-h-9 items-center justify-center gap-2 rounded-lg bg-blue-600 px-3 text-sm font-semibold text-white shadow-sm hover:bg-blue-700"
            data-testid="workflow-primary-action"
            data-action={commandBarAction.id}
            onClick={() => onPrimaryAction(commandBarAction)}
          >
            {primaryActionIcon(commandBarAction.id)}
            <span>{commandBarAction.label}</span>
          </button>
        ) : blockedReason ? (
          <span className="max-w-40 text-right text-xs font-medium text-slate-500" data-testid="workflow-primary-action-blocked">
            {blockedReason}
          </span>
        ) : null}
        <WorkflowActionMenu items={menuItems} />
      </div>
    </div>
  );
}

function IconButton({
  label,
  onClick,
  children,
}: {
  label: string;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      className="inline-flex h-9 w-9 items-center justify-center rounded-lg border border-slate-300 bg-white text-slate-700 hover:bg-slate-50"
      onClick={onClick}
      aria-label={label}
      title={label}
    >
      {children}
    </button>
  );
}

function primaryActionIcon(id: WorkflowPrimaryActionId) {
  if (id === "check") return <SearchCheck className="h-4 w-4" aria-hidden="true" />;
  if (id === "dry_run") return <FlaskConical className="h-4 w-4" aria-hidden="true" />;
  if (id === "run_published") return <Play className="h-4 w-4" aria-hidden="true" />;
  if (id === "fix_issues" || id === "configure" || id === "resolve_conflict") {
    return <Wrench className="h-4 w-4" aria-hidden="true" />;
  }
  return <Rocket className="h-4 w-4" aria-hidden="true" />;
}
