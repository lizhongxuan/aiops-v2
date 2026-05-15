import { AlertTriangle, CheckCircle2, Clock3, Eye, FileText, GitBranch, LoaderCircle, Search, ShieldCheck, Wrench } from "lucide-react";
import { useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import type { AiopsTransportAgentUiArtifact } from "@/transport/aiopsTransportTypes";

type LooseRecord = Record<string, unknown>;
type SearchResultAction = {
  id: string;
  label: string;
  confirmationAction?: string;
  primary?: boolean;
};

const STATE_LABELS: Record<string, string> = {
  direct_execute: "可直接执行",
  need_info: "需补充信息",
  adapt: "需适配",
  direct: "可直接执行",
  need_more_info: "需补充信息",
  adapt_required: "需适配",
  reference_only: "仅参考",
  no_match: "无可用手册",
};

const STATE_TONE: Record<string, string> = {
  direct_execute: "border-emerald-200 bg-emerald-50 text-emerald-700",
  need_info: "border-amber-200 bg-amber-50 text-amber-700",
  adapt: "border-sky-200 bg-sky-50 text-sky-700",
  direct: "border-emerald-200 bg-emerald-50 text-emerald-700",
  need_more_info: "border-amber-200 bg-amber-50 text-amber-700",
  adapt_required: "border-sky-200 bg-sky-50 text-sky-700",
  reference_only: "border-slate-200 bg-slate-50 text-slate-700",
  no_match: "border-slate-200 bg-slate-50 text-slate-500",
};

const ACTIONS_BY_STATE: Record<string, Array<{ id: string; label: string; action?: string }>> = {
  direct_execute: [
    { id: "view-parameters", label: "查看参数" },
    { id: "start-precheck", label: "开始前置检查" },
  ],
  need_info: [
    { id: "fill-context", label: "补充信息" },
    { id: "authorize-coroot", label: "授权读取 Coroot" },
  ],
  adapt: [{ id: "generate-variant", label: "生成适配工作流", action: "generate_runner_workflow_candidate" }],
  direct: [
    { id: "view-parameters", label: "查看参数" },
    { id: "start-precheck", label: "开始前置检查" },
  ],
  need_more_info: [
    { id: "fill-context", label: "补充信息" },
    { id: "authorize-coroot", label: "授权读取 Coroot" },
  ],
  adapt_required: [{ id: "generate-variant", label: "生成适配工作流", action: "generate_runner_workflow_candidate" }],
  reference_only: [{ id: "follow-steps", label: "按步骤参考" }],
};

const STEP_LABELS: Record<string, string> = {
  waiting: "等待中",
  running: "执行中",
  passed: "已通过",
  failed: "失败",
  skipped: "已跳过",
};

const STEP_TONE: Record<string, string> = {
  waiting: "bg-slate-100 text-slate-600",
  running: "bg-sky-50 text-sky-700",
  passed: "bg-emerald-50 text-emerald-700",
  failed: "bg-red-50 text-red-700",
  skipped: "bg-slate-50 text-slate-500",
};

export function OpsManualMatchArtifact({ artifact }: { artifact: AiopsTransportAgentUiArtifact }) {
  const data = artifactData(artifact);
  const state = text(pick(data, "state", "decisionState", "decision_state"), "no_match");
  const manualTitle = text(pick(data, "manualTitle", "manual_title", "title"), text(pick(asRecord(pick(data, "manual")), "title"), "运维手册"));
  const manualId = text(pick(data, "manualId", "manual_id"), text(pick(asRecord(pick(data, "manual")), "id")));
  const workflowRef = asRecord(pick(data, "workflowRef", "workflow_ref")) || {};
  const workflowId = text(pick(workflowRef, "workflowId", "workflow_id"), text(pick(data, "workflowId", "workflow_id")));
  const reasons = stringArray(pick(data, "reasons", "reason"));
  const missingContext = stringArray(pick(data, "missingContext", "missing_context"));
  const compatibilityGaps = stringArray(pick(data, "compatibilityGaps", "compatibility_gaps"));
  const recommendedNextActions = stringArray(pick(data, "recommendedNextActions", "recommended_next_actions"));
  const summary = asRecord(pick(data, "runRecordSummary", "run_record_summary"));
  const actions: Array<{ id: string; label: string; action?: string }> = [
    ...(ACTIONS_BY_STATE[state] || []),
    ...stringArray(pick(data, "suggestedActions", "suggested_actions")).map((label) => ({ id: label, label })),
  ];

  return (
    <div className="mt-3 grid gap-3 rounded-lg border border-slate-100 bg-slate-50 p-3 text-xs" data-testid="ops-manual-match-card">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant="outline" className={STATE_TONE[state] || STATE_TONE.no_match}>
              {STATE_LABELS[state] || state}
            </Badge>
            {manualId ? <span className="font-mono text-slate-500">{manualId}</span> : null}
          </div>
          <div className="mt-1 text-sm font-medium text-slate-950">{manualTitle}</div>
        </div>
        <ShieldCheck className="h-4 w-4 text-slate-500" />
      </div>

      <dl className="grid gap-2 sm:grid-cols-[7rem_1fr]">
        {workflowId ? (
          <>
            <dt className="font-medium text-slate-500">绑定 Workflow</dt>
            <dd className="font-mono text-slate-700">Workflow {workflowId}</dd>
          </>
        ) : null}
        {reasons.length ? (
          <>
            <dt className="font-medium text-slate-500">判定原因</dt>
            <dd>{reasons.join("；")}</dd>
          </>
        ) : null}
        {missingContext.length ? (
          <>
            <dt className="font-medium text-slate-500">缺失条件</dt>
            <dd>{missingContext.join("；")}</dd>
          </>
        ) : null}
        {compatibilityGaps.length ? (
          <>
            <dt className="font-medium text-slate-500">适配差异</dt>
            <dd>{compatibilityGaps.join("；")}</dd>
          </>
        ) : null}
        {recommendedNextActions.length ? (
          <>
            <dt className="font-medium text-slate-500">下一步</dt>
            <dd>{recommendedNextActions.join("；")}</dd>
          </>
        ) : null}
        {Object.keys(summary).length ? (
          <>
            <dt className="font-medium text-slate-500">执行记录</dt>
            <dd>
              成功 {text(pick(summary, "successCount", "success_count"), "0")}，失败 {text(pick(summary, "failureCount", "failure_count"), "0")}
              {text(pick(summary, "recentResult", "recent_result")) ? `，最近 ${text(pick(summary, "recentResult", "recent_result"))}` : ""}
            </dd>
          </>
        ) : null}
      </dl>

      {actions.length ? (
        <div className="flex flex-wrap gap-2 border-t border-slate-200 pt-3">
          {actions.map((action) => (
            <Button
              key={action.id}
              type="button"
              size="sm"
              variant={action.id.includes("precheck") ? "default" : "outline"}
              className="h-8 rounded-md"
              onClick={() => {
                if (action.action) {
                  dispatchComposerConfirmation(action.action, action.label, manualTitle, artifact.id);
                }
              }}
            >
              {action.action ? <Wrench className="h-3.5 w-3.5" /> : <FileText className="h-3.5 w-3.5" />}
              {action.label}
            </Button>
          ))}
          <Button
            type="button"
            size="sm"
            variant="outline"
            className="h-8 rounded-md"
            onClick={() => dispatchComposerConfirmation("generate_ops_manual_candidate", "生成运维手册候选", manualTitle, artifact.id)}
          >
            <FileText className="h-3.5 w-3.5" />
            生成运维手册
          </Button>
        </div>
      ) : null}
    </div>
  );
}

export function OpsManualSearchResultArtifact({ artifact }: { artifact: AiopsTransportAgentUiArtifact }) {
  const data = artifactData(artifact);
  const decision = normalizeDecision(text(pick(data, "decision", "state"), "no_match"));
  const summary = text(pick(data, "summary", "message"), defaultSearchSummary(decision));
  const operationFrame = asRecord(pick(data, "operationFrame", "operation_frame"));
  const manuals = arrayRecords(pick(data, "manuals", "hits", "matches", "items"));
  const nextQuestions = stringArray(pick(data, "nextQuestions", "next_questions"));
  const searchedFields = stringArray(pick(data, "searchedFields", "searched_fields"));
  const recommendedNextAction = text(pick(data, "recommendedNextAction", "recommended_next_action"));
  const primaryTitle = manualTitleFromHit(manuals[0]) || searchResultTitle(decision);
  const actions = searchActionsForDecision(decision, manuals);

  return (
    <div className="mt-3 grid gap-3 rounded-lg border border-slate-100 bg-slate-50 p-3 text-xs" data-testid="ops-manual-search-result-card">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant="outline" className={STATE_TONE[decision] || STATE_TONE.no_match}>
              {STATE_LABELS[decision] || decision}
            </Badge>
            {operationFrameLabel(operationFrame) ? <span className="font-mono text-slate-500">{operationFrameLabel(operationFrame)}</span> : null}
          </div>
          <div className="mt-1 text-sm font-medium text-slate-950">{primaryTitle}</div>
          <p className="mt-1 leading-5 text-slate-600">{summary}</p>
        </div>
        <Search className="h-4 w-4 text-slate-500" />
      </div>

      {nextQuestions.length ? (
        <section className="rounded-md border border-amber-100 bg-white p-2">
          <div className="font-medium text-amber-800">需要补充</div>
          <ul className="mt-1 grid gap-1 text-slate-700">
            {nextQuestions.map((question) => (
              <li key={question}>{question}</li>
            ))}
          </ul>
        </section>
      ) : null}

      {manuals.length ? (
        <div className="grid gap-2">
          {manuals.map((hit, index) => (
            <SearchManualHit key={manualIdFromHit(hit) || String(index)} hit={hit} index={index} />
          ))}
        </div>
      ) : null}

      {searchedFields.length || recommendedNextAction ? (
        <dl className="grid gap-2 sm:grid-cols-[7rem_1fr]">
          {searchedFields.length ? (
            <>
              <dt className="font-medium text-slate-500">已检索字段</dt>
              <dd>{searchedFields.join("；")}</dd>
            </>
          ) : null}
          {recommendedNextAction ? (
            <>
              <dt className="font-medium text-slate-500">下一步</dt>
              <dd>{recommendedNextAction}</dd>
            </>
          ) : null}
        </dl>
      ) : null}

      {actions.length ? (
        <div className="flex flex-wrap gap-2 border-t border-slate-200 pt-3">
          {actions.map((action) => (
            <Button
              key={action.id}
              type="button"
              size="sm"
              variant={action.primary ? "default" : "outline"}
              className="h-8 rounded-md"
              onClick={() => {
                if (action.confirmationAction) {
                  dispatchComposerConfirmation(action.confirmationAction, action.label, primaryTitle, artifact.id);
                }
              }}
            >
              {action.confirmationAction ? <Wrench className="h-3.5 w-3.5" /> : <FileText className="h-3.5 w-3.5" />}
              {action.label}
            </Button>
          ))}
        </div>
      ) : null}
    </div>
  );
}

export function RunnerWorkflowGenerationArtifact({ artifact }: { artifact: AiopsTransportAgentUiArtifact }) {
  const [previewOpen, setPreviewOpen] = useState(false);
  const data = artifactData(artifact);
  const title = text(pick(data, "workflowTitle", "workflow_title", "title"), "Runner Workflow 生成进度");
  const steps = arrayRecords(pick(data, "steps", "timeline", "nodes")).filter((step) => !isManualApprovalStep(step));
  const workflowId = text(pick(data, "workflowId", "workflow_id"));

  return (
    <div className="mt-3 rounded-lg border border-slate-100 bg-slate-50 p-3 text-xs" data-testid="runner-workflow-generation-card">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-sm font-medium text-slate-950">
            <GitBranch className="h-4 w-4 text-slate-500" />
            {title}
          </div>
          <p className="mt-1 leading-5 text-slate-500">节点会在对话中逐步生成；预览只读，不会跳转 Runner Studio 或修改工作流。</p>
        </div>
        <Button type="button" size="sm" variant="outline" className="h-8 rounded-md" onClick={() => setPreviewOpen(true)}>
          <Eye className="h-3.5 w-3.5" />
          预览 Runner 草稿
        </Button>
      </div>
      <ol className="mt-3 grid gap-2">
        {steps.map((step, index) => {
          const status = text(pick(step, "status", "state"), "waiting");
          const Icon = iconForStep(status);
          return (
            <li key={text(pick(step, "id"), String(index))} className="flex items-start gap-2 rounded-md border border-slate-200 bg-white p-2">
              <span className={`mt-0.5 rounded-full p-1 ${STEP_TONE[status] || STEP_TONE.waiting}`}>
                <Icon className="h-3.5 w-3.5" />
              </span>
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="font-medium text-slate-900">{text(pick(step, "title", "name"), `节点 ${index + 1}`)}</span>
                  <Badge variant="outline" className={STEP_TONE[status] || STEP_TONE.waiting}>
                    {STEP_LABELS[status] || status}
                  </Badge>
                </div>
                {text(pick(step, "summary", "description")) ? <p className="mt-1 leading-5 text-slate-600">{text(pick(step, "summary", "description"))}</p> : null}
              </div>
            </li>
          );
        })}
      </ol>
      <Dialog open={previewOpen} onOpenChange={setPreviewOpen}>
        <DialogContent className="max-h-[86vh] overflow-y-auto sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>Runner Workflow 只读预览</DialogTitle>
            <DialogDescription>这是 AI 在对话中生成的 Runner 草稿预览，只读展示节点、状态和说明，不支持在弹窗内编辑或执行。</DialogDescription>
          </DialogHeader>
          <div className="grid gap-3 text-sm">
            <section className="rounded-lg border border-slate-200 bg-slate-50 p-3">
              <div className="text-xs font-medium text-slate-500">工作流</div>
              <div className="mt-1 font-medium text-slate-950">{title}</div>
              {workflowId ? <div className="mt-1 font-mono text-xs text-slate-500">{workflowId}</div> : null}
            </section>
            <section className="rounded-lg border border-slate-200 bg-white p-3">
              <div className="text-xs font-medium text-slate-500">只读节点</div>
              <ol className="mt-3 grid gap-2">
                {steps.map((step, index) => {
                  const status = text(pick(step, "status", "state"), "waiting");
                  return (
                    <li key={`preview-${text(pick(step, "id"), String(index))}`} className="rounded-md border border-slate-200 bg-slate-50 p-2">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="font-medium text-slate-950">{index + 1}. {text(pick(step, "title", "name"), `节点 ${index + 1}`)}</span>
                        <Badge variant="outline" className={STEP_TONE[status] || STEP_TONE.waiting}>
                          {STEP_LABELS[status] || status}
                        </Badge>
                      </div>
                      {text(pick(step, "summary", "description")) ? <p className="mt-1 leading-5 text-slate-600">{text(pick(step, "summary", "description"))}</p> : null}
                    </li>
                  );
                })}
              </ol>
            </section>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function dispatchComposerConfirmation(action: string, title: string, sourceTitle: string, artifactId: string) {
  window.dispatchEvent(
    new CustomEvent("aiops:composer-confirmation", {
      detail: { action, title, sourceTitle, artifactId },
    }),
  );
}

function SearchManualHit({ hit, index }: { hit: LooseRecord; index: number }) {
  const manualTitle = manualTitleFromHit(hit) || `相关手册 ${index + 1}`;
  const manualId = manualIdFromHit(hit);
  const boundWorkflowId = text(pick(hit, "boundWorkflowId", "bound_workflow_id", "workflowId", "workflow_id"));
  const usableMode = normalizeDecision(text(pick(hit, "usableMode", "usable_mode", "decision", "state")));
  const matchedFields = stringArray(pick(hit, "matchedFields", "matched_fields"));
  const missingFields = stringArray(pick(hit, "missingFields", "missing_fields", "missingContext", "missing_context"));
  const environmentDiffs = stringArray(pick(hit, "environmentDiffs", "environment_diffs", "compatibilityGaps", "compatibility_gaps"));
  const blockedReasons = stringArray(pick(hit, "blockedReasons", "blocked_reasons"));
  const recommendedAction = text(pick(hit, "recommendedAction", "recommended_action"));
  const runRecordSummary = asRecord(pick(hit, "runRecordSummary", "run_record_summary"));

  return (
    <section className="rounded-md border border-slate-200 bg-white p-2">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-medium text-slate-950">{manualTitle}</span>
            <Badge variant="outline" className={STATE_TONE[usableMode] || STATE_TONE.no_match}>
              {STATE_LABELS[usableMode] || usableMode}
            </Badge>
          </div>
          {manualId ? <div className="mt-1 font-mono text-slate-500">{manualId}</div> : null}
        </div>
      </div>
      <dl className="mt-2 grid gap-2 sm:grid-cols-[7rem_1fr]">
        {boundWorkflowId ? (
          <>
            <dt className="font-medium text-slate-500">绑定 Workflow</dt>
            <dd className="font-mono text-slate-700">{boundWorkflowId}</dd>
          </>
        ) : null}
        {matchedFields.length ? (
          <>
            <dt className="font-medium text-slate-500">匹配字段</dt>
            <dd>{matchedFields.join("；")}</dd>
          </>
        ) : null}
        {missingFields.length ? (
          <>
            <dt className="font-medium text-slate-500">缺失条件</dt>
            <dd>{missingFields.join("；")}</dd>
          </>
        ) : null}
        {environmentDiffs.length ? (
          <>
            <dt className="font-medium text-slate-500">环境差异</dt>
            <dd>{environmentDiffs.join("；")}</dd>
          </>
        ) : null}
        {blockedReasons.length ? (
          <>
            <dt className="font-medium text-slate-500">阻止原因</dt>
            <dd>{blockedReasons.join("；")}</dd>
          </>
        ) : null}
        {recommendedAction ? (
          <>
            <dt className="font-medium text-slate-500">建议动作</dt>
            <dd>{recommendedAction}</dd>
          </>
        ) : null}
        {Object.keys(runRecordSummary).length ? (
          <>
            <dt className="font-medium text-slate-500">执行记录</dt>
            <dd>
              成功 {text(pick(runRecordSummary, "successCount", "success_count"), "0")}，失败 {text(pick(runRecordSummary, "failureCount", "failure_count"), "0")}
              {text(pick(runRecordSummary, "recentResult", "recent_result", "latestStatus", "latest_status")) ? `，最近 ${text(pick(runRecordSummary, "recentResult", "recent_result", "latestStatus", "latest_status"))}` : ""}
            </dd>
          </>
        ) : null}
      </dl>
    </section>
  );
}

function normalizeDecision(value: string) {
  const normalized = value.toLowerCase();
  if (normalized === "direct" || normalized === "direct_execute" || normalized === "executable") return "direct_execute";
  if (normalized === "need_info" || normalized === "need_more_info" || normalized === "missing_info") return "need_info";
  if (normalized === "adapt" || normalized === "adapt_required" || normalized === "generate_variant") return "adapt";
  if (normalized === "reference" || normalized === "reference_only") return "reference_only";
  return "no_match";
}

function defaultSearchSummary(decision: string) {
  if (decision === "direct_execute") return "找到可直接使用的运维手册。";
  if (decision === "need_info") return "信息不足，不能直接使用工作流。";
  if (decision === "adapt") return "找到相关运维手册，但当前环境需要适配。";
  if (decision === "reference_only") return "找到可参考手册，但不能直接执行绑定工作流。";
  return "没有找到合适的运维手册。";
}

function searchResultTitle(decision: string) {
  if (decision === "direct_execute") return "找到可直接使用的运维手册";
  if (decision === "need_info") return "需要补充信息后再判断";
  if (decision === "adapt") return "找到需要适配的运维手册";
  if (decision === "reference_only") return "找到可参考的运维手册";
  return "未找到合适的运维手册";
}

function searchActionsForDecision(decision: string, manuals: LooseRecord[]): SearchResultAction[] {
  const hasManual = manuals.length > 0;
  if (decision === "direct_execute") {
    return [
      ...(hasManual ? [{ id: "view-manual", label: "查看手册" }] : []),
      { id: "fill-parameters", label: "填写参数" },
      { id: "dry-run", label: "Dry Run", primary: true },
    ];
  }
  if (decision === "need_info") {
    return [
      { id: "authorize-coroot", label: "授权读取 Coroot" },
      { id: "select-target", label: "选择目标实例" },
      { id: "select-topology", label: "选择部署形态" },
    ];
  }
  if (decision === "adapt") {
    return [
      ...(hasManual ? [{ id: "view-manual", label: "查看手册" }] : []),
      { id: "generate-variant", label: "生成适配工作流", confirmationAction: "generate_runner_workflow_candidate", primary: true },
      { id: "follow-steps", label: "按手册逐步执行" },
      { id: "continue-without-manual", label: "不用手册继续" },
    ];
  }
  if (decision === "reference_only") {
    return [
      { id: "follow-steps", label: "按步骤执行", primary: true },
      ...(hasManual ? [{ id: "view-manual", label: "查看手册" }] : []),
      { id: "continue-debug", label: "继续普通排查" },
    ];
  }
  return [
    { id: "continue-debug", label: "继续普通排查", primary: true },
    { id: "generate-manual", label: "成功后生成运维手册", confirmationAction: "generate_ops_manual_candidate" },
  ];
}

function manualTitleFromHit(hit?: LooseRecord) {
  if (!hit) return "";
  const manual = asRecord(pick(hit, "manual", "opsManual", "ops_manual"));
  return text(pick(hit, "title", "manualTitle", "manual_title"), text(pick(manual, "title", "name")));
}

function manualIdFromHit(hit?: LooseRecord) {
  if (!hit) return "";
  const manual = asRecord(pick(hit, "manual", "opsManual", "ops_manual"));
  return text(pick(hit, "manualId", "manual_id", "id"), text(pick(manual, "id", "manualId", "manual_id")));
}

function operationFrameLabel(frame: LooseRecord) {
  const target = asRecord(pick(frame, "target"));
  const operation = asRecord(pick(frame, "operation"));
  const objectType = text(pick(frame, "objectType", "object_type"), text(pick(target, "type")));
  const operationType = text(pick(frame, "operationType", "operation_type"), text(pick(operation, "action", "type")));
  return [objectType, operationType].filter(Boolean).join(" / ");
}

function iconForStep(status: string) {
  if (status === "passed") return CheckCircle2;
  if (status === "failed") return AlertTriangle;
  if (status === "running") return LoaderCircle;
  return Clock3;
}

function artifactData(artifact: AiopsTransportAgentUiArtifact): LooseRecord {
  const inline = asRecord(artifact.inlineData);
  return Object.keys(inline).length ? inline : { ...asRecord(artifact.payload), ...asRecord(artifact.metadata) };
}

function asRecord(value: unknown): LooseRecord {
  return value && typeof value === "object" && !Array.isArray(value) ? (value as LooseRecord) : {};
}

function arrayRecords(value: unknown): LooseRecord[] {
  return Array.isArray(value) ? value.map(asRecord) : [];
}

function isManualApprovalStep(step: LooseRecord): boolean {
  const id = text(pick(step, "id", "key", "type")).toLowerCase();
  const title = text(pick(step, "title", "name", "label")).toLowerCase();
  const action = text(pick(step, "action", "actionType", "action_type")).toLowerCase();
  return (
    id.includes("approval") ||
    action.includes("approval") ||
    title.includes("approval") ||
    title.includes("人工审批") ||
    title.includes("人工审核")
  );
}

function stringArray(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  return value.map((item) => text(item)).filter(Boolean);
}

function pick(source: LooseRecord, ...keys: string[]) {
  for (const key of keys) {
    const value = source[key];
    if (value !== undefined && value !== null && value !== "") return value;
  }
  return "";
}

function text(value: unknown, fallback = "") {
  if (value === undefined || value === null) return fallback;
  const normalized = String(value).replace(/<[^>]*>/g, "").trim().replace(/\s+/g, " ");
  return normalized || fallback;
}
