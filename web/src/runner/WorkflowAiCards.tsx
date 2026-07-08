import { CheckCircle, Clock, FileText, GitBranch, RotateCcw, ShieldCheck, TriangleAlert, Wrench } from "lucide-react";
import type {
  WorkflowAiContext,
  WorkflowAiEffectStatus,
  WorkflowAiActiveStep,
  WorkflowAiToolLogEntry,
  WorkflowEditPlan,
  WorkflowManualCandidateSummary,
  WorkflowPatch,
  WorkflowPatchResult,
  WorkflowPatchValidation,
} from "./workflowAiTypes";

export function WorkflowContextCard({ context }: { context: WorkflowAiContext }) {
  return (
    <section className="workflow-ai-card" data-testid="workflow-ai-context-card">
      <header>
        <GitBranch size={16} />
        <h3>{context.workflowName || context.workflowId || "Workflow"}</h3>
      </header>
      <dl className="workflow-ai-facts">
        <div><dt>Revision</dt><dd>{context.revision || "-"}</dd></div>
        <div><dt>Save</dt><dd>{context.saveState || "-"}</dd></div>
        <div><dt>Selected</dt><dd>{context.selectedNodeId || "-"}</dd></div>
        <div><dt>Validation</dt><dd>{context.validation?.valid === false ? "needs review" : "ready"}</dd></div>
        <div><dt>Manual</dt><dd>{context.manualBinding?.title || context.manualBinding?.manualId || "-"}</dd></div>
      </dl>
    </section>
  );
}

export function WorkflowEditPlanCard({ plan }: { plan: WorkflowEditPlan }) {
  return (
    <section className="workflow-ai-card" data-testid="workflow-ai-plan-card">
      <header>
        <FileText size={16} />
        <h3>修改计划</h3>
      </header>
      <div className="workflow-ai-plan-list">
        {plan.items.map((item, index) => (
          <div className="workflow-ai-plan-row" key={item.id} style={{ overflowWrap: "anywhere", whiteSpace: "normal" }}>
            <span className="workflow-ai-plan-index">{index + 1}</span>
            <div>
              <strong>{item.title}</strong>
              {item.description ? <p>{item.description}</p> : null}
            </div>
          </div>
        ))}
      </div>
      <div className="workflow-ai-plan-reply-hint" data-testid="workflow-ai-plan-reply-hint">
        <strong>这个计划可以吗？</strong>
        <p>回复「确认」开始，或直接说明要调整的步骤。</p>
      </div>
    </section>
  );
}

export function WorkflowPatchPreviewCard({
  patch,
  validation,
  effectStatus,
  onApply,
}: {
  patch: WorkflowPatch;
  validation?: WorkflowPatchValidation;
  effectStatus?: WorkflowAiEffectStatus;
  onApply?: () => void;
}) {
  const nonEffect = effectStatus && effectStatus !== "changed";
  return (
    <section className={`workflow-ai-card ${nonEffect ? "workflow-ai-card-muted" : ""}`} data-testid="workflow-ai-patch-card">
      <header>
        <Wrench size={16} />
        <h3>{patch.summary || patch.id}</h3>
      </header>
      <p>{patch.operations.length} 个图层操作</p>
      {validation ? <WorkflowPatchValidationCard validation={validation} /> : null}
      {effectStatus ? <p data-testid="workflow-ai-effect-status">变更效果：{effectStatus}</p> : null}
      {onApply ? <button type="button" onClick={onApply} disabled={Boolean(nonEffect)}>应用</button> : null}
    </section>
  );
}

export function WorkflowPatchApplyCard({ patch }: { patch: WorkflowPatch }) {
  return (
    <section className="workflow-ai-card" data-testid="workflow-ai-apply-card">
      <header>
        <ShieldCheck size={16} />
        <h3>Ready to apply</h3>
      </header>
      <p>{patch.summary || patch.id}</p>
    </section>
  );
}

export function WorkflowPatchResultCard({ result, onUndo }: { result: WorkflowPatchResult; onUndo?: () => void }) {
  return (
    <section className="workflow-ai-card" data-testid="workflow-ai-result-card">
      <header>
        <CheckCircle size={16} />
        <h3>已完成修改</h3>
      </header>
      <p>{result.describe?.summary || result.patchId}</p>
      <p>变更节点：{(result.effect?.affectedNodes || []).join(", ") || "-"}</p>
      <button type="button" onClick={onUndo}>
        <RotateCcw size={14} />
        撤销
      </button>
    </section>
  );
}

export function WorkflowAiStepGenerationCard({ step, status = "running", history = false }: { step: WorkflowAiActiveStep; status?: "running" | "completed"; history?: boolean }) {
  const completed = status === "completed";
  const environment = Array.isArray(step.environment) ? step.environment : [step.environment || "-"];
  return (
    <section className="workflow-ai-card workflow-ai-progress-card" data-testid={history ? "workflow-ai-step-history-card" : "workflow-ai-step-progress-card"}>
      <header>
        <span className={`workflow-ai-spinner-dot ${completed ? "completed" : ""}`} aria-hidden="true" />
        <h3>{completed ? "完成步骤" : "正在生成步骤"} {step.index}/{step.total}</h3>
      </header>
      <strong>{step.title}</strong>
      <dl className="workflow-ai-step-facts">
        <div><dt>目标</dt><dd>{step.goal || "-"}</dd></div>
        <div><dt>环境</dt><dd>{environment.filter(Boolean).join("；") || "-"}</dd></div>
        <div><dt>输入</dt><dd>{formatWorkflowAiVariables(step.inputVariables)}</dd></div>
        <div><dt>输出</dt><dd>{formatWorkflowAiVariables(step.outputVariables)}</dd></div>
        <div><dt>脚本</dt><dd>{step.scriptSummary || "-"}</dd></div>
        <div><dt>校验</dt><dd>{step.validationSummary || "-"}</dd></div>
      </dl>
    </section>
  );
}

function formatWorkflowAiVariables(variables: WorkflowAiActiveStep["inputVariables"]) {
  if (!variables?.length) return "-";
  return variables
    .map((variable) => `${variable.name}${variable.type ? `:${variable.type}` : ""}${variable.required ? " 必填" : ""}`)
    .join("，");
}

export function WorkflowPatchValidationCard({ validation }: { validation: WorkflowPatchValidation }) {
  return (
    <div className="workflow-ai-validation-card" data-testid="workflow-ai-validation-card">
      <span>{validation.valid ? "Validation passed" : "Validation needed"}</span>
      {(validation.errors || []).map((error) => <p key={error}>{error}</p>)}
    </div>
  );
}

export function WorkflowAiConflictCard({ reason }: { reason: string }) {
  return (
    <section className="workflow-ai-card workflow-ai-conflict-card" data-testid="workflow-ai-conflict-card">
      <header>
        <TriangleAlert size={16} />
        <h3>Conflict</h3>
      </header>
      <p>{reason}</p>
    </section>
  );
}

export function WorkflowManualCandidateCard({ candidate }: { candidate: WorkflowManualCandidateSummary }) {
  return (
    <section className="workflow-ai-card" data-testid="workflow-ai-manual-card">
      <header>
        <FileText size={16} />
        <h3>{candidate.title || candidate.manualId || "Ops Manual candidate"}</h3>
      </header>
      <dl className="workflow-ai-facts">
        <div><dt>Status</dt><dd>{candidate.reviewStatus || "pending"}</dd></div>
        <div><dt>Operation</dt><dd>{candidate.operationType || "-"}</dd></div>
        <div><dt>Risk</dt><dd>{candidate.riskLevel || "-"}</dd></div>
        <div><dt>Workflow</dt><dd>{candidate.workflowId || "-"}</dd></div>
        <div><dt>Digest</dt><dd>{candidate.workflowDigest || "-"}</dd></div>
        <div><dt>Binding</dt><dd>{candidate.staleBinding ? "stale" : "current"}</dd></div>
      </dl>
      {candidate.requiredEvidence?.length ? <p>Evidence: {candidate.requiredEvidence.join(", ")}</p> : null}
      {candidate.cannotConditions?.length ? <p>Cannot: {candidate.cannotConditions.join(", ")}</p> : null}
      {candidate.preflightSummary ? <p>Preflight: {candidate.preflightSummary}</p> : null}
      {candidate.verifySummary ? <p>Verify: {candidate.verifySummary}</p> : null}
      {candidate.rollbackSummary ? <p>Rollback: {candidate.rollbackSummary}</p> : null}
      <div className="workflow-ai-plan-row">
        <button type="button">保存为候选</button>
        <button type="button">去审核页</button>
      </div>
    </section>
  );
}

export function WorkflowAiToolTimeline({ entries }: { entries: WorkflowAiToolLogEntry[] }) {
  return (
    <section className="workflow-ai-card" data-testid="workflow-ai-tool-timeline">
      <header>
        <Clock size={16} />
        <h3>执行过程</h3>
      </header>
      {entries.map((entry) => (
        <details key={entry.id} className="workflow-ai-tool-row">
          <summary>
            <span className="workflow-ai-tool-main">{entry.outputSummary || entry.inputSummary || entry.toolName}</span>
            <span className={`workflow-ai-tool-status status-${entry.status}`}>{workflowAiToolStatusLabel(entry.status)}</span>
          </summary>
          <dl className="workflow-ai-tool-detail-grid">
            <div><dt>工具</dt><dd>{entry.toolName}</dd></div>
            {entry.inputSummary ? <div><dt>输入</dt><dd>{entry.inputSummary}</dd></div> : null}
            {entry.outputSummary ? <div><dt>结果</dt><dd>{entry.outputSummary}</dd></div> : null}
            {entry.durationMs !== undefined ? <div><dt>耗时</dt><dd>{entry.durationMs}ms</dd></div> : null}
            {entry.traceId ? <div><dt>Trace</dt><dd>{entry.traceId}</dd></div> : null}
            {entry.error ? <div><dt>错误</dt><dd>{entry.error}</dd></div> : null}
          </dl>
        </details>
      ))}
    </section>
  );
}

function workflowAiToolStatusLabel(status: WorkflowAiToolLogEntry["status"]) {
  if (status === "running") return "执行中";
  if (status === "completed") return "已完成";
  if (status === "failed") return "失败";
  return String(status || "-");
}

export function WorkflowAiHealthCard({ stage }: { stage: string }) {
  return (
    <section className="workflow-ai-card" data-testid="workflow-ai-health-card">
      <header>
        <CheckCircle size={16} />
        <h3>Status</h3>
      </header>
      <p>{stage}</p>
    </section>
  );
}
