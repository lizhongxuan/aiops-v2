import { AlertTriangle, CheckCircle2, CircleDot, LocateFixed } from "lucide-react";
import { useMemo } from "react";

import { selectCommittedValidation } from "./workspace/workflowWorkspaceSelectors";
import type { WorkflowDiagnostic } from "./workspace/workflowMcpContracts";
import type { WorkflowWorkspaceState } from "./workspace/workflowWorkspaceTypes";

export type WorkflowValidationLocation = {
  nodeId?: string;
  edgeId?: string;
  field?: string;
};

export const WORKFLOW_VALIDATION_FOCUS_EVENT = "aiops:workflow-validation:focus";

export function dispatchWorkflowValidationFocus(location: WorkflowValidationLocation) {
  if (typeof window === "undefined") return;
  const detail = canonicalValidationLocation(location);
  if (!detail) return;
  window.dispatchEvent(new CustomEvent<WorkflowValidationLocation>(WORKFLOW_VALIDATION_FOCUS_EVENT, { detail }));
}

export function readWorkflowValidationFocusEvent(event: Event): WorkflowValidationLocation | undefined {
  if (event.type !== WORKFLOW_VALIDATION_FOCUS_EVENT || !(event instanceof CustomEvent)) return undefined;
  return canonicalValidationLocation(event.detail);
}

export type WorkflowValidationPanelProps = {
  state: WorkflowWorkspaceState;
  onFocusIssue?: (location: WorkflowValidationLocation) => void;
};

export function WorkflowValidationPanel({ state, onFocusIssue }: WorkflowValidationPanelProps) {
  const committed = useMemo(() => selectCommittedValidation(state), [state]);
  const candidateValidation = state.candidate?.changeSet.validation;

  return (
    <section className="flex min-h-0 flex-col gap-4 bg-white p-4" aria-labelledby="workflow-validation-title" data-testid="workflow-validation-panel">
      <header className="space-y-1">
        <h2 id="workflow-validation-title" className="text-sm font-semibold text-slate-950">检查结果</h2>
        <p className="text-xs leading-5 text-slate-500">发布门禁只读取当前已保存版本的检查结果。</p>
      </header>

      <ValidationGroup
        title="已保存版本"
        authority="committed"
        status={committed?.status || "unchecked"}
        diagnostics={committed?.diagnostics || []}
        onFocusIssue={onFocusIssue}
      />

      {candidateValidation ? (
        <ValidationGroup
          title="待应用修改"
          authority="candidate"
          status={candidateValidation.valid ? "valid" : "blocking"}
          diagnostics={candidateValidation.diagnostics}
          onFocusIssue={onFocusIssue}
          note="仅用于审阅候选修改；不能解锁已保存版本的发布或运行。"
        />
      ) : null}
    </section>
  );
}

function ValidationGroup({
  title,
  authority,
  status,
  diagnostics,
  onFocusIssue,
  note,
}: {
  title: string;
  authority: "committed" | "candidate";
  status: string;
  diagnostics: WorkflowDiagnostic[];
  onFocusIssue?: (location: WorkflowValidationLocation) => void;
  note?: string;
}) {
  const blocking = diagnostics.filter((item) => ["error", "fatal", "blocking"].includes(item.severity.toLowerCase())).length;
  const summary = validationSummary(status, blocking);
  return (
    <article className="rounded-xl border border-slate-200 p-3" data-validation-authority={authority} data-validation-status={status}>
      <div className="flex items-start justify-between gap-3">
        <div>
          <h3 className="text-sm font-semibold text-slate-950">{title}</h3>
          <p className="mt-1 text-xs text-slate-500">{summary.description}</p>
        </div>
        <span className={`inline-flex items-center gap-1 rounded-full px-2 py-1 text-xs font-medium ${summary.tone}`}>
          {summary.icon}{summary.label}
        </span>
      </div>
      {note ? <p className="mt-2 rounded-lg bg-amber-50 px-2 py-1.5 text-xs leading-5 text-amber-900">{note}</p> : null}
      {diagnostics.length ? (
        <ol className="mt-3 space-y-2" aria-label={`${title}问题`}>
          {diagnostics.map((diagnostic, index) => {
            const location = diagnosticLocation(diagnostic);
            const label = safeDiagnosticLabel(diagnostic, index);
            return (
              <li key={`${diagnostic.code}-${diagnostic.nodeId || diagnostic.edgeId || "workflow"}-${diagnostic.field || ""}-${index}`}>
                <button
                  type="button"
                  data-workflow-validation-issue
                  data-testid={`workflow-validation-issue-${authority}-${index}`}
                  data-workflow-node-id={location?.nodeId}
                  data-workflow-edge-id={location?.edgeId}
                  data-workflow-field={location?.field}
                  disabled={!onFocusIssue || !location}
                  className="flex min-h-11 w-full items-center justify-between gap-3 rounded-lg border border-slate-200 px-3 py-2 text-left hover:bg-slate-50 disabled:cursor-default disabled:bg-slate-50"
                  onClick={() => {
                    if (!location || !onFocusIssue) return;
                    onFocusIssue(location);
                    dispatchWorkflowValidationFocus(location);
                  }}
                >
                  <span className="min-w-0">
                    <span className="block truncate text-xs font-semibold text-slate-800">{label.title}</span>
                    <span className="mt-0.5 block truncate text-[11px] text-slate-500">{label.location}</span>
                  </span>
                  {location ? <LocateFixed className="h-4 w-4 shrink-0 text-blue-600" aria-label="定位问题" /> : null}
                </button>
              </li>
            );
          })}
        </ol>
      ) : <p className="mt-3 text-xs text-slate-500">暂无问题明细。</p>}
    </article>
  );
}

function canonicalValidationLocation(value: unknown): WorkflowValidationLocation | undefined {
  if (!value || typeof value !== "object" || Array.isArray(value)) return undefined;
  const source = value as Record<string, unknown>;
  if (Object.keys(source).some((key) => !["nodeId", "edgeId", "field"].includes(key))) return undefined;
  const canonical = (item: unknown) => typeof item === "string" && item.length > 0 && item === item.trim() && !/[\0\r\n]/.test(item)
    ? item
    : undefined;
  const nodeId = canonical(source.nodeId);
  const edgeId = canonical(source.edgeId);
  const field = canonical(source.field);
  if (nodeId && edgeId) return undefined;
  if (!nodeId && !edgeId && !field) return undefined;
  return {
    ...(nodeId ? { nodeId } : {}),
    ...(edgeId ? { edgeId } : {}),
    ...(field ? { field } : {}),
  };
}

function diagnosticLocation(diagnostic: WorkflowDiagnostic): WorkflowValidationLocation | undefined {
  const location: WorkflowValidationLocation = {
    ...(diagnostic.nodeId ? { nodeId: diagnostic.nodeId } : {}),
    ...(diagnostic.edgeId ? { edgeId: diagnostic.edgeId } : {}),
    ...(diagnostic.field ? { field: diagnostic.field } : {}),
  };
  return Object.keys(location).length ? location : undefined;
}

function safeDiagnosticLabel(diagnostic: WorkflowDiagnostic, index: number) {
  const target = diagnostic.nodeId ? `节点 ${diagnostic.nodeId}` : diagnostic.edgeId ? `连线 ${diagnostic.edgeId}` : "工作流";
  const location = diagnostic.field ? `${target} · 字段 ${diagnostic.field}` : target;
  return { title: `${severityLabel(diagnostic.severity)} ${index + 1} · ${diagnostic.code}`, location };
}

function severityLabel(severity: string) {
  if (["error", "fatal", "blocking"].includes(severity.toLowerCase())) return "阻断问题";
  if (severity.toLowerCase() === "warning") return "提醒";
  return "检查项";
}

function validationSummary(status: string, blocking: number) {
  if (status === "valid" || status === "valid_with_warnings") return {
    label: status === "valid_with_warnings" ? "通过，有提醒" : "已通过",
    description: "结果与这个版本的完整检查依据绑定。",
    tone: "bg-emerald-50 text-emerald-800",
    icon: <CheckCircle2 className="h-3.5 w-3.5" aria-hidden="true" />,
  };
  if (status === "checking") return {
    label: "检查中",
    description: "完成前不会解锁发布或运行。",
    tone: "bg-sky-50 text-sky-800",
    icon: <CircleDot className="h-3.5 w-3.5" aria-hidden="true" />,
  };
  if (status === "configuration_required") return {
    label: "还需配置",
    description: "结构已保存；完成配置后需要重新检查。",
    tone: "bg-amber-50 text-amber-900",
    icon: <AlertTriangle className="h-3.5 w-3.5" aria-hidden="true" />,
  };
  if (status === "saved_with_issues" || status === "blocking") return {
    label: blocking > 0 ? `${blocking} 个阻断问题` : "存在阻断问题",
    description: "工作流仍已保存，但不能发布或运行。",
    tone: "bg-rose-50 text-rose-800",
    icon: <AlertTriangle className="h-3.5 w-3.5" aria-hidden="true" />,
  };
  return {
    label: "待检查",
    description: "尚无与当前已保存版本匹配的检查结果。",
    tone: "bg-slate-100 text-slate-700",
    icon: <CircleDot className="h-3.5 w-3.5" aria-hidden="true" />,
  };
}
