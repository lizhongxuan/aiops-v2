import {
  AlertTriangle,
  CheckCircle2,
  Clock3,
  Eye,
  FileText,
  GitBranch,
  LoaderCircle,
  Search,
  ShieldCheck,
  Wrench,
} from "lucide-react";
import { useEffect, useState } from "react";

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
type PreflightAction = {
  id: string;
  label: string;
  confirmationAction?: string;
  icon?: string;
  primary?: boolean;
};
type ContextFormField = {
  id: string;
  label: string;
  type?: string;
  required?: boolean;
  sensitive?: boolean;
  uiControl?: string;
  placeholder?: string;
  default?: unknown;
  candidates?: ParamCandidate[];
};

type ParamCandidate = {
  value: unknown;
  label?: string;
  hint?: string;
  source?: string;
  confidence?: number;
  evidence?: string;
};

const OPS_MANUAL_SKIP_ACTION = "skip_ops_manual";

const STATE_LABELS: Record<string, string> = {
  direct_execute: "可进入预检",
  need_info: "手册缺上下文",
  adapt: "需适配",
  direct: "可进入预检",
  need_more_info: "手册缺上下文",
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

const ACTIONS_BY_STATE: Record<
  string,
  Array<{ id: string; label: string; action?: string }>
> = {
  direct_execute: [{ id: "run-preflight", label: "运行预检" }],
  need_info: [{ id: "fill-context", label: "补充信息" }],
  adapt: [
    {
      id: "generate-variant",
      label: "生成适配工作流",
      action: "generate_runner_workflow_candidate",
    },
  ],
  direct: [{ id: "run-preflight", label: "运行预检" }],
  need_more_info: [{ id: "fill-context", label: "补充信息" }],
  adapt_required: [
    {
      id: "generate-variant",
      label: "生成适配工作流",
      action: "generate_runner_workflow_candidate",
    },
  ],
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

const PREFLIGHT_LABELS: Record<string, string> = {
  passed: "预检通过",
  blocked: "Workflow 预检阻断",
  failed: "预检失败",
  not_applicable: "无需预检",
  unknown: "预检未知",
};

const PREFLIGHT_TONE: Record<string, string> = {
  passed: "border-emerald-200 bg-emerald-50 text-emerald-700",
  blocked: "border-amber-200 bg-amber-50 text-amber-700",
  failed: "border-red-200 bg-red-50 text-red-700",
  not_applicable: "border-slate-200 bg-slate-50 text-slate-700",
  unknown: "border-slate-200 bg-slate-50 text-slate-500",
};

export function OpsManualMatchArtifact({
  artifact,
}: {
  artifact: AiopsTransportAgentUiArtifact;
}) {
  const data = artifactData(artifact);
  const state = text(
    pick(data, "state", "decisionState", "decision_state"),
    "no_match",
  );
  const manualTitle = text(
    pick(data, "manualTitle", "manual_title", "title"),
    text(pick(asRecord(pick(data, "manual")), "title"), "运维手册"),
  );
  const manualId = text(
    pick(data, "manualId", "manual_id"),
    text(pick(asRecord(pick(data, "manual")), "id")),
  );
  const workflowRef = asRecord(pick(data, "workflowRef", "workflow_ref")) || {};
  const workflowId = text(
    pick(workflowRef, "workflowId", "workflow_id"),
    text(pick(data, "workflowId", "workflow_id")),
  );
  const reasons = stringArray(pick(data, "reasons", "reason"));
  const missingContext = stringArray(
    pick(data, "missingContext", "missing_context"),
  );
  const compatibilityGaps = stringArray(
    pick(data, "compatibilityGaps", "compatibility_gaps"),
  );
  const recommendedNextActions = stringArray(
    pick(data, "recommendedNextActions", "recommended_next_actions"),
  );
  const summary = asRecord(
    pick(data, "runRecordSummary", "run_record_summary"),
  );
  const actions: Array<{ id: string; label: string; action?: string }> = [
    ...(ACTIONS_BY_STATE[state] || []),
    ...stringArray(pick(data, "suggestedActions", "suggested_actions")).map(
      (label) => ({ id: label, label }),
    ),
  ].filter((action) => Boolean(action.action));

  return (
    <div
      className="mt-3 grid gap-3 rounded-lg border border-slate-100 bg-slate-50 p-3 text-xs"
      data-testid="ops-manual-match-card"
    >
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <Badge
              variant="outline"
              className={STATE_TONE[state] || STATE_TONE.no_match}
            >
              {STATE_LABELS[state] || state}
            </Badge>
            {manualId ? (
              <span className="font-mono text-slate-500">{manualId}</span>
            ) : null}
          </div>
          <div className="mt-1 text-sm font-medium text-slate-950">
            {manualTitle}
          </div>
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
              成功 {text(pick(summary, "successCount", "success_count"), "0")}
              ，失败 {text(pick(summary, "failureCount", "failure_count"), "0")}
              {text(pick(summary, "recentResult", "recent_result"))
                ? `，最近 ${text(pick(summary, "recentResult", "recent_result"))}`
                : ""}
            </dd>
          </>
        ) : null}
      </dl>

      {state === "direct" || state === "direct_execute" ? (
        <div className="rounded-md border border-slate-200 bg-white px-2 py-1.5 text-slate-600">
          下一步：先运行预检，通过并确认后再进入 Dry Run。
        </div>
      ) : null}

      {actions.length ? (
        <div className="flex flex-wrap gap-2 border-t border-slate-200 pt-3">
          {actions.map((action) => (
            <Button
              key={action.id}
              type="button"
              size="sm"
              variant={action.id.includes("preflight") ? "default" : "outline"}
              className="h-8 rounded-md"
              onClick={() => {
                if (action.action) {
                  dispatchComposerConfirmation(
                    action.action,
                    action.label,
                    manualTitle,
                    artifact.id,
                  );
                }
              }}
            >
              {action.action ? (
                <Wrench className="h-3.5 w-3.5" />
              ) : (
                <FileText className="h-3.5 w-3.5" />
              )}
              {action.label}
            </Button>
          ))}
        </div>
      ) : null}
    </div>
  );
}

export function OpsManualSearchResultArtifact({
  artifact,
}: {
  artifact: AiopsTransportAgentUiArtifact;
}) {
  const data = artifactData(artifact);
  const rawDecision = normalizeDecision(
    text(pick(data, "decision", "state"), "no_match"),
  );
  const operationFrame = asRecord(
    pick(data, "operationFrame", "operation_frame"),
  );
  const rawManuals = arrayRecords(
    pick(data, "manuals", "hits", "matches", "items"),
  );
  const manuals = rawManuals.filter(
    (hit) => !isCrossObjectHit(hit, operationFrame),
  );
  const crossObjectOnly =
    rawDecision === "reference_only" &&
    rawManuals.length > 0 &&
    manuals.length === 0;
  const decision = crossObjectOnly ? "no_match" : rawDecision;
  const summary = crossObjectOnly
    ? crossObjectNoMatchSummary(operationFrame)
    : text(pick(data, "summary", "message"), defaultSearchSummary(decision));
  const recommendedNextAction = text(
    pick(data, "recommendedNextAction", "recommended_next_action"),
  );
  const mergedParamResolution = asRecord(
    pick(data, "mergedParamResolution", "merged_param_resolution"),
  );
  const hasMergedParamResolution =
    Object.keys(mergedParamResolution).length > 0;
  const mergedPreflightResult = asRecord(
    pick(data, "mergedPreflightResult", "merged_preflight_result"),
  );
  const hasMergedPreflightResult =
    Object.keys(mergedPreflightResult).length > 0;
  const primaryTitle =
    manualTitleFromHit(manuals[0]) || searchResultTitle(decision);
  const bodyText = decision === "no_match" ? summary : primaryTitle;
  const actions = searchActionsForDecision(decision, manuals);
  const compact = decision === "need_info" || decision === "no_match";
  const visibleManuals = compact ? [] : limitItems(manuals, 1);
  const stage = searchStage(decision);
  const nextStep = searchNextStep(decision, recommendedNextAction);

  if (hasMergedParamResolution) {
    return (
      <OpsManualProgressCard
        artifact={artifact}
        data={data}
        decision={decision}
        operationFrame={operationFrame}
        manualHit={manuals[0]}
        paramResolution={mergedParamResolution}
        preflightResult={
          hasMergedPreflightResult ? mergedPreflightResult : undefined
        }
      />
    );
  }

  if (decision === "need_info") {
    const mergedPreflightStatus = hasMergedPreflightResult
      ? normalizePreflightStatus(
          text(
            pick(mergedPreflightResult, "status", "preflight_status"),
            "unknown",
          ),
        )
      : "";
    return (
      <div
        className="mt-3 rounded-lg border border-slate-100 bg-slate-50 p-3 text-xs"
        data-testid="ops-manual-search-result-card"
      >
        <div className="flex flex-wrap items-start justify-between gap-2">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <Badge
                variant="outline"
                className="border-slate-200 bg-white text-slate-600"
              >
                运维手册检索
              </Badge>
              {operationFrameLabel(operationFrame) ? (
                <span className="font-mono text-slate-500">
                  {operationFrameLabel(operationFrame)}
                </span>
              ) : null}
            </div>
            <div className="mt-1 text-sm font-medium text-slate-950">
              {hasMergedPreflightResult
                ? preflightTitle(mergedPreflightStatus)
                : "运维手册暂未进入 Workflow 预检"}
            </div>
          </div>
          <Search className="h-4 w-4 text-slate-500" />
        </div>
        {manuals.length ? (
          <CompactManualCandidate
            hit={manuals[0]}
            operationFrame={operationFrame}
            artifactId={artifact.id}
            autoContinueContext={false}
          />
        ) : null}
        {hasMergedPreflightResult ? (
          <div className="mt-3">
            <MergedPreflightSummary
              data={mergedPreflightResult}
              artifactId={artifact.id}
              sourceTitle={primaryTitle}
            />
          </div>
        ) : null}
        {actions.length ? (
          <div className="mt-3 flex flex-wrap gap-2 border-t border-slate-200 pt-3">
            {actions.map((action) => (
              <Button
                key={action.id}
                type="button"
                size="sm"
                variant={action.primary ? "default" : "outline"}
                className="h-8 rounded-md"
              >
                <FileText className="h-3.5 w-3.5" />
                {action.label}
              </Button>
            ))}
          </div>
        ) : null}
      </div>
    );
  }

  return (
    <div
      className="mt-3 grid gap-3 rounded-lg border border-slate-100 bg-slate-50 p-3 text-xs"
      data-testid="ops-manual-search-result-card"
    >
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <Badge
              variant="outline"
              className={STATE_TONE[decision] || STATE_TONE.no_match}
            >
              {STATE_LABELS[decision] || decision}
            </Badge>
            {operationFrameLabel(operationFrame) ? (
              <span className="font-mono text-slate-500">
                {operationFrameLabel(operationFrame)}
              </span>
            ) : null}
          </div>
          <div className="mt-1 text-sm font-medium text-slate-950">{stage}</div>
          <p className="mt-1 leading-5 text-slate-600">{bodyText}</p>
        </div>
        <Search className="h-4 w-4 text-slate-500" />
      </div>

      {nextStep && !hasMergedPreflightResult ? (
        <div className="rounded-md border border-slate-200 bg-white px-2 py-1.5 text-slate-600">
          {nextStep}
        </div>
      ) : null}

      {compact && manuals.length ? (
        <CompactManualCandidate
          hit={manuals[0]}
          operationFrame={operationFrame}
          artifactId={artifact.id}
          autoContinueContext={false}
        />
      ) : null}

      {visibleManuals.length ? (
        <div className="grid gap-2">
          {visibleManuals.map((hit, index) => (
            <SearchManualHit
              key={manualIdFromHit(hit) || String(index)}
              hit={hit}
              index={index}
              operationFrame={operationFrame}
            />
          ))}
        </div>
      ) : null}

      {hasMergedPreflightResult ? (
        <MergedPreflightSummary
          data={mergedPreflightResult}
          artifactId={artifact.id}
          sourceTitle={primaryTitle}
        />
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
                  dispatchComposerConfirmation(
                    action.confirmationAction,
                    action.label,
                    primaryTitle,
                    artifact.id,
                  );
                }
              }}
            >
              {action.confirmationAction ? (
                <Wrench className="h-3.5 w-3.5" />
              ) : (
                <FileText className="h-3.5 w-3.5" />
              )}
              {action.label}
            </Button>
          ))}
        </div>
      ) : null}
    </div>
  );
}

function OpsManualProgressCard({
  artifact,
  data,
  decision,
  operationFrame,
  manualHit,
  paramResolution,
  preflightResult,
}: {
  artifact: AiopsTransportAgentUiArtifact;
  data: LooseRecord;
  decision: string;
  operationFrame?: LooseRecord;
  manualHit?: LooseRecord;
  paramResolution: LooseRecord;
  preflightResult?: LooseRecord;
}) {
  const [skipSubmitted, setSkipSubmitted] = useState(false);
  const [preflightRunning, setPreflightRunning] = useState(false);
  const manualTitle = manualTitleFromHit(manualHit) || "运维手册";
  const manualId = text(
    pick(paramResolution, "manualId", "manual_id"),
    manualIdFromHit(manualHit),
  );
  const workflowId = text(
    pick(paramResolution, "workflowId", "workflow_id"),
    text(
      pick(
        manualHit || {},
        "boundWorkflowId",
        "bound_workflow_id",
        "workflowId",
        "workflow_id",
      ),
    ),
  );
  const status = normalizeParamResolutionStatus(
    text(pick(paramResolution, "status"), "unresolved"),
  );
  const resolvedParams = arrayRecords(
    pick(paramResolution, "resolvedParams", "resolved_params"),
  );
  const fields = paramResolutionFormFields(paramResolution);
  const needsInput =
    (status === "ambiguous" || status === "need_user_input") &&
    fields.length > 0;
  const preflightStatus = preflightResult
    ? normalizePreflightStatus(
        text(pick(preflightResult, "status", "preflight_status"), "unknown"),
      )
    : "";
  const title = preflightResult
    ? preflightTitle(preflightStatus)
    : needsInput
      ? ""
      : status === "resolved"
        ? "参数已补齐，下一步运行预检"
        : searchStage(decision);
  const badgeLabel = preflightResult
    ? PREFLIGHT_LABELS[preflightStatus] || "预检已完成"
    : needsInput
      ? "等待补充"
      : status === "resolved"
        ? "可进入预检"
        : STATE_LABELS[decision] || "运维手册";
  const badgeClass = preflightResult
    ? PREFLIGHT_TONE[preflightStatus] || PREFLIGHT_TONE.unknown
    : needsInput
      ? "border-amber-200 bg-amber-50 text-amber-700"
      : status === "resolved"
        ? "border-emerald-200 bg-emerald-50 text-emerald-700"
        : STATE_TONE[decision] || STATE_TONE.no_match;
  const workflowPreview = workflowPreviewFromHit(manualHit || {}, manualTitle);
  const manualPreview = manualPreviewFromHit(manualHit || {});
  const [matchExpanded, setMatchExpanded] = useState(false);
  const [paramsExpanded, setParamsExpanded] = useState(false);
  const [workflowOpen, setWorkflowOpen] = useState(false);
  const [manualOpen, setManualOpen] = useState(false);

  useEffect(() => {
    if (!needsInput) return;
    const key = `${artifact.id}:${fields.map((field) => field.id).join("|")}`;
    if (dispatchedParamResolutionForms.has(key)) return;
    const timer = window.setTimeout(() => {
      if (dispatchedParamResolutionForms.has(key)) return;
      dispatchedParamResolutionForms.add(key);
      dispatchContextRequest(
        artifact.id,
        "补充运维手册参数",
        fields,
        true,
        "",
        {
          manualId,
          workflowId,
          submitAction: "submit_ops_manual_param_form",
        },
      );
    }, 0);
    return () => window.clearTimeout(timer);
  }, [
    artifact.id,
    manualId,
    workflowId,
    needsInput,
    fields
      .map((field) => `${field.id}:${field.candidates?.length || 0}`)
      .join("|"),
  ]);

  return (
    <div
      className="mt-3 rounded-lg border border-slate-100 bg-slate-50 p-3 text-xs"
      data-testid="ops-manual-progress-card"
    >
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <Badge
              variant="outline"
              className="border-slate-200 bg-white text-slate-600"
            >
              运维手册
            </Badge>
            <Badge variant="outline" className={badgeClass}>
              {badgeLabel}
            </Badge>
            {operationFrameLabel(operationFrame || {}) ? (
              <span className="font-mono text-slate-500">
                {operationFrameLabel(operationFrame || {})}
              </span>
            ) : null}
          </div>
          {title ? (
            <div className="mt-1 text-sm font-medium text-slate-950">
              {title}
            </div>
          ) : null}
          <p className="mt-1 leading-5 text-slate-600">
            {manualTitle}
            {manualId ? (
              <span className="ml-2 font-mono text-slate-500">{manualId}</span>
            ) : null}
          </p>
        </div>
        <ShieldCheck className="h-4 w-4 text-slate-500" />
      </div>

      {resolvedParams.length ? (
        <section className="mt-3 rounded-md border border-slate-200 bg-white p-2">
          <button
            type="button"
            className="flex w-full items-center justify-between gap-2 text-left"
            aria-expanded={paramsExpanded}
            onClick={() => setParamsExpanded((current) => !current)}
          >
            <span className="font-medium text-slate-700">已解析参数</span>
            <span className="text-[11px] text-slate-400">
              {paramsExpanded ? "收起" : "查看证据"}
            </span>
          </button>
          <dl
            className={[
              "mt-2 grid gap-2",
              paramsExpanded
                ? "sm:grid-cols-[8rem_1fr]"
                : "sm:grid-cols-[6rem_1fr]",
            ].join(" ")}
          >
            {(paramsExpanded ? resolvedParams : resolvedParams.slice(0, 2)).map(
              (param, index) => (
                <CompactParamValueRow
                  key={`${text(pick(param, "id"), String(index))}-${index}`}
                  param={param}
                  showEvidence={paramsExpanded}
                />
              ),
            )}
          </dl>
        </section>
      ) : null}

      {preflightResult ? (
        <div className="mt-3">
          <MergedPreflightSummary
            data={preflightResult}
            artifactId={artifact.id}
            sourceTitle={manualTitle}
          />
        </div>
      ) : null}

      {manualHit ? (
        <div className="mt-3 rounded-md border border-slate-200 bg-white text-slate-600">
          <button
            type="button"
            className="flex w-full items-center justify-between gap-2 px-2 py-1.5 text-left hover:bg-slate-50 focus:outline-none focus:ring-2 focus:ring-slate-300"
            aria-expanded={matchExpanded}
            onClick={() => setMatchExpanded((current) => !current)}
            data-testid="ops-manual-candidate-toggle"
          >
            <span className="font-medium text-slate-700">命中依据</span>
            <span className="text-[11px] text-slate-400">
              {matchExpanded ? "收起" : "查看"}
            </span>
          </button>
          {matchExpanded ? (
            <dl
              className="grid gap-2 border-t border-slate-100 px-2 py-2 sm:grid-cols-[5rem_1fr]"
              data-testid="ops-manual-candidate-match-detail"
            >
              <dt className="font-medium text-slate-500">依据</dt>
              <dd>{progressMatchText(manualHit, operationFrame)}</dd>
              {workflowId ? (
                <>
                  <dt className="font-medium text-slate-500">Workflow</dt>
                  <dd className="font-mono text-slate-700">{workflowId}</dd>
                </>
              ) : null}
            </dl>
          ) : null}
        </div>
      ) : null}

      <div className="mt-3 flex flex-wrap gap-2 border-t border-slate-200 pt-3">
        {status === "resolved" && !preflightResult ? (
          <Button
            type="button"
            size="sm"
            className="h-8 rounded-md"
            disabled={preflightRunning}
            onClick={() => {
              setPreflightRunning(true);
              dispatchComposerContextSubmit(
                artifact.id,
                `运行运维手册预检：${manualId || workflowId || manualTitle}`,
                {
                  opsManualAction: "run_ops_manual_preflight",
                  manualId,
                  workflowId,
                  resolvedParamsJson: JSON.stringify(
                    resolvedParamsToPayload(resolvedParams),
                  ),
                },
              );
            }}
          >
            {preflightRunning ? (
              <LoaderCircle className="h-3.5 w-3.5 animate-spin" />
            ) : (
              <ShieldCheck className="h-3.5 w-3.5" />
            )}
            {preflightRunning ? "预检中" : "运行预检"}
          </Button>
        ) : null}
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="h-8 rounded-md"
          disabled={skipSubmitted}
          onClick={() => {
            setSkipSubmitted(true);
            dispatchComposerContextSubmit(
              artifact.id,
              skipManualText(manualTitle, operationFrame),
              skipManualMetadata(manualId, workflowId, manualTitle),
            );
          }}
        >
          {skipSubmitted ? "已切换" : "不使用"}
        </Button>
        {manualHit ? (
          <>
            <Button
              type="button"
              size="sm"
              variant="outline"
              className="h-8 rounded-md"
              onClick={() => setWorkflowOpen(true)}
            >
              <GitBranch className="h-3.5 w-3.5" />
              查看工作流
            </Button>
            <Button
              type="button"
              size="sm"
              variant="outline"
              className="h-8 rounded-md"
              onClick={() => setManualOpen(true)}
            >
              <FileText className="h-3.5 w-3.5" />
              查看手册
            </Button>
          </>
        ) : null}
      </div>

      {preflightRunning ? (
        <div
          className="mt-3 rounded-md border border-sky-200 bg-sky-50 px-2 py-1.5 text-sky-700"
          data-testid="ops-manual-preflight-running"
        >
          预检请求已提交，正在等待只读探针结果。
        </div>
      ) : null}

      <WorkflowReadOnlyDialog
        open={workflowOpen}
        onOpenChange={setWorkflowOpen}
        preview={workflowPreview}
        fallbackWorkflowId={workflowId}
      />
      <ManualReadOnlyDialog
        open={manualOpen}
        onOpenChange={setManualOpen}
        preview={manualPreview}
      />
      {skipSubmitted ? (
        <div
          className="mt-2 rounded-md border border-slate-200 bg-white px-2 py-1.5 text-slate-600"
          data-testid="ops-manual-skip-submitted"
        >
          已切换为普通只读排查，等待 Agent 继续处理。
        </div>
      ) : null}
    </div>
  );
}

function CompactParamValueRow({
  param,
  showEvidence,
}: {
  param: LooseRecord;
  showEvidence: boolean;
}) {
  const id = text(pick(param, "id"));
  const value = text(pick(param, "value"), "已解析");
  const source = text(pick(param, "source"));
  const evidence = text(pick(param, "evidence"));
  return (
    <>
      <dt className="font-medium text-slate-500">{paramDisplayLabel(id)}</dt>
      <dd className="min-w-0">
        <span className="font-mono text-slate-800">{value}</span>
        {showEvidence && (source || evidence) ? (
          <span className="block break-words text-slate-500 sm:mt-0.5">
            {[paramSourceLabel(source), evidence].filter(Boolean).join("；")}
          </span>
        ) : null}
      </dd>
    </>
  );
}

function MergedPreflightSummary({
  data,
  artifactId,
  sourceTitle,
}: {
  data: LooseRecord;
  artifactId: string;
  sourceTitle: string;
}) {
  const status = normalizePreflightStatus(
    text(pick(data, "status", "preflight_status"), "unknown"),
  );
  const ready = Boolean(pick(data, "ready"));
  const reason = text(pick(data, "reason"));
  const probeId = text(pick(data, "probeId", "probe_id"));
  const nextAction = text(pick(data, "nextAction", "next_action"));
  const evidence = arrayRecords(pick(data, "evidence"));
  const actions = preflightActions(status, nextAction);

  return (
    <section
      className="rounded-md border border-slate-200 bg-white p-2"
      data-testid="ops-manual-merged-preflight"
    >
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <Badge
              variant="outline"
              className="border-slate-200 bg-white text-slate-600"
            >
              Workflow 预检
            </Badge>
            <Badge
              variant="outline"
              className={PREFLIGHT_TONE[status] || PREFLIGHT_TONE.unknown}
            >
              {PREFLIGHT_LABELS[status] || status}
            </Badge>
            {ready ? (
              <Badge
                variant="outline"
                className="border-emerald-200 bg-emerald-50 text-emerald-700"
              >
                可进入下一步
              </Badge>
            ) : null}
          </div>
          <div className="mt-1 text-sm font-medium text-slate-950">
            {preflightTitle(status)}
          </div>
          {reason ? (
            <p className="mt-1 leading-5 text-slate-600">{reason}</p>
          ) : null}
        </div>
        <ShieldCheck className="h-4 w-4 text-slate-500" />
      </div>

      {probeId || evidence.length ? (
        <div className="mt-2 flex flex-wrap items-center gap-2 text-slate-600">
          {probeId ? (
            <span className="font-mono text-slate-500">{probeId}</span>
          ) : null}
          {evidence.slice(0, 5).map((item, index) => (
            <span
              key={`${text(pick(item, "name"), String(index))}-${index}`}
              className="inline-flex items-center gap-1 rounded-md border border-slate-200 bg-slate-50 px-2 py-1"
            >
              <span
                className={
                  text(pick(item, "status")) === "passed"
                    ? "text-emerald-700"
                    : "text-slate-500"
                }
              >
                {text(pick(item, "status"), "unknown")}
              </span>
              <span className="font-mono text-slate-700">
                {text(pick(item, "name"), `evidence_${index + 1}`)}
              </span>
            </span>
          ))}
        </div>
      ) : null}

      {actions.length ? (
        <div className="mt-3 flex flex-wrap gap-2 border-t border-slate-200 pt-3">
          {actions.map((action) => (
            <Button
              key={action.id}
              type="button"
              size="sm"
              variant={action.primary ? "default" : "outline"}
              className="h-8 rounded-md"
              onClick={() => {
                if (action.confirmationAction) {
                  dispatchComposerConfirmation(
                    action.confirmationAction,
                    action.label,
                    sourceTitle,
                    artifactId,
                  );
                }
              }}
            >
              {action.icon === "warning" ? (
                <AlertTriangle className="h-3.5 w-3.5" />
              ) : (
                <ShieldCheck className="h-3.5 w-3.5" />
              )}
              {action.label}
            </Button>
          ))}
        </div>
      ) : null}
    </section>
  );
}

export function OpsManualPreflightResultArtifact({
  artifact,
}: {
  artifact: AiopsTransportAgentUiArtifact;
}) {
  const data = artifactData(artifact);
  const status = normalizePreflightStatus(
    text(pick(data, "status", "preflight_status"), "unknown"),
  );
  const ready = Boolean(pick(data, "ready"));
  const reason = text(pick(data, "reason"));
  const manualId = text(pick(data, "manualId", "manual_id"));
  const workflowId = text(pick(data, "workflowId", "workflow_id"));
  const probeId = text(pick(data, "probeId", "probe_id"));
  const evidence = arrayRecords(pick(data, "evidence"));
  const missingPermissions = stringArray(
    pick(data, "missingPermissions", "missing_permissions"),
  );
  const environmentDiffs = stringArray(
    pick(data, "environmentDiffs", "environment_diffs"),
  );
  const nextAction = text(pick(data, "nextAction", "next_action"));
  const title = preflightTitle(status);

  return (
    <div
      className="mt-3 grid gap-3 rounded-lg border border-slate-100 bg-slate-50 p-3 text-xs"
      data-testid="ops-manual-preflight-result-card"
    >
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <Badge
              variant="outline"
              className="border-slate-200 bg-white text-slate-600"
            >
              Workflow 预检
            </Badge>
            <Badge
              variant="outline"
              className={PREFLIGHT_TONE[status] || PREFLIGHT_TONE.unknown}
            >
              {PREFLIGHT_LABELS[status] || status}
            </Badge>
            {ready ? (
              <Badge
                variant="outline"
                className="border-emerald-200 bg-emerald-50 text-emerald-700"
              >
                可进入下一步
              </Badge>
            ) : null}
          </div>
          <div className="mt-1 text-sm font-medium text-slate-950">{title}</div>
          {reason ? (
            <p className="mt-1 leading-5 text-slate-600">{reason}</p>
          ) : null}
        </div>
        <ShieldCheck className="h-4 w-4 text-slate-500" />
      </div>

      <dl className="grid gap-2 sm:grid-cols-[7rem_1fr]">
        {manualId ? (
          <>
            <dt className="font-medium text-slate-500">运维手册</dt>
            <dd className="font-mono text-slate-700">{manualId}</dd>
          </>
        ) : null}
        {workflowId ? (
          <>
            <dt className="font-medium text-slate-500">Workflow</dt>
            <dd className="font-mono text-slate-700">{workflowId}</dd>
          </>
        ) : null}
        {probeId ? (
          <>
            <dt className="font-medium text-slate-500">预检探针</dt>
            <dd className="font-mono text-slate-700">{probeId}</dd>
          </>
        ) : null}
        {missingPermissions.length ? (
          <>
            <dt className="font-medium text-slate-500">缺少权限</dt>
            <dd>{missingPermissions.join("；")}</dd>
          </>
        ) : null}
        {environmentDiffs.length ? (
          <>
            <dt className="font-medium text-slate-500">环境差异</dt>
            <dd>{environmentDiffs.join("；")}</dd>
          </>
        ) : null}
        {nextAction ? (
          <>
            <dt className="font-medium text-slate-500">下一步</dt>
            <dd>{preflightNextActionLabel(nextAction)}</dd>
          </>
        ) : null}
      </dl>

      {evidence.length ? (
        <section className="rounded-md border border-slate-200 bg-white p-2">
          <div className="font-medium text-slate-700">只读证据</div>
          <ul className="mt-2 grid gap-1">
            {evidence.map((item, index) => (
              <li
                key={`${text(pick(item, "name"), String(index))}-${index}`}
                className="flex flex-wrap items-center gap-2"
              >
                <Badge
                  variant="outline"
                  className={
                    text(pick(item, "status")) === "passed"
                      ? PREFLIGHT_TONE.passed
                      : PREFLIGHT_TONE.unknown
                  }
                >
                  {text(pick(item, "status"), "unknown")}
                </Badge>
                <span className="font-mono text-slate-700">
                  {text(pick(item, "name"), `evidence_${index + 1}`)}
                </span>
                {text(pick(item, "note")) ? (
                  <span className="text-slate-500">
                    {text(pick(item, "note"))}
                  </span>
                ) : null}
              </li>
            ))}
          </ul>
        </section>
      ) : null}

      <div className="flex flex-wrap gap-2 border-t border-slate-200 pt-3">
        {preflightActions(status, nextAction).map((action) => (
          <Button
            key={action.id}
            type="button"
            size="sm"
            variant={action.primary ? "default" : "outline"}
            className="h-8 rounded-md"
            onClick={() => {
              if (action.confirmationAction) {
                dispatchComposerConfirmation(
                  action.confirmationAction,
                  action.label,
                  manualId || workflowId || title,
                  artifact.id,
                );
              }
            }}
          >
            {action.icon === "warning" ? (
              <AlertTriangle className="h-3.5 w-3.5" />
            ) : (
              <ShieldCheck className="h-3.5 w-3.5" />
            )}
            {action.label}
          </Button>
        ))}
      </div>
    </div>
  );
}

const dispatchedParamResolutionForms = new Set<string>();

export function OpsManualParamResolutionArtifact({
  artifact,
}: {
  artifact: AiopsTransportAgentUiArtifact;
}) {
  const data = artifactData(artifact);
  const [preflightRunning, setPreflightRunning] = useState(false);
  const [skipSubmitted, setSkipSubmitted] = useState(false);
  const status = normalizeParamResolutionStatus(
    text(pick(data, "status"), "unresolved"),
  );
  const manualId = text(pick(data, "manualId", "manual_id"));
  const workflowId = text(pick(data, "workflowId", "workflow_id"));
  const resolvedParams = arrayRecords(
    pick(data, "resolvedParams", "resolved_params"),
  );
  const fields = paramResolutionFormFields(data);
  const mergedPreflightResult = asRecord(
    pick(data, "mergedPreflightResult", "merged_preflight_result"),
  );
  const hasMergedPreflightResult =
    Object.keys(mergedPreflightResult).length > 0;
  const mergedPreflightStatus = hasMergedPreflightResult
    ? normalizePreflightStatus(
        text(
          pick(mergedPreflightResult, "status", "preflight_status"),
          "unknown",
        ),
      )
    : "";
  const needsForm =
    (status === "ambiguous" || status === "need_user_input") &&
    fields.length > 0;
  const title = hasMergedPreflightResult
    ? "参数已补齐，预检已完成"
    : paramResolutionTitle(status);

  useEffect(() => {
    if (!needsForm) return;
    const key = `${artifact.id}:${fields.map((field) => field.id).join("|")}`;
    if (dispatchedParamResolutionForms.has(key)) return;
    const timer = window.setTimeout(() => {
      if (dispatchedParamResolutionForms.has(key)) return;
      dispatchedParamResolutionForms.add(key);
      dispatchContextRequest(
        artifact.id,
        "补充运维手册参数",
        fields,
        true,
        "",
        {
          manualId,
          workflowId,
          submitAction: "submit_ops_manual_param_form",
        },
      );
    }, 0);
    return () => window.clearTimeout(timer);
  }, [
    artifact.id,
    manualId,
    workflowId,
    needsForm,
    fields
      .map((field) => `${field.id}:${field.candidates?.length || 0}`)
      .join("|"),
  ]);

  return (
    <div
      className="mt-3 grid gap-3 rounded-lg border border-slate-100 bg-slate-50 p-3 text-xs"
      data-testid="ops-manual-param-resolution-card"
    >
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant="outline" className={paramResolutionTone(status)}>
              {status === "resolved" && hasMergedPreflightResult
                ? "预检已完成"
                : status === "resolved"
                  ? "可进入预检"
                  : "参数确认"}
            </Badge>
            {manualId ? (
              <span className="font-mono text-slate-500">{manualId}</span>
            ) : null}
          </div>
          <div className="mt-1 text-sm font-medium text-slate-950">{title}</div>
        </div>
        <ShieldCheck className="h-4 w-4 text-slate-500" />
      </div>

      {resolvedParams.length ? (
        <section className="rounded-md border border-slate-200 bg-white p-2">
          <div className="font-medium text-slate-700">已解析参数</div>
          <dl className="mt-2 grid gap-2 sm:grid-cols-[8rem_1fr]">
            {resolvedParams.slice(0, 6).map((param, index) => (
              <ParamValueRow
                key={`${text(pick(param, "id"), String(index))}-${index}`}
                param={param}
              />
            ))}
          </dl>
        </section>
      ) : null}

      {fields.length ? (
        <section className="rounded-md border border-slate-200 bg-white p-2">
          <div className="font-medium text-slate-700">
            {status === "ambiguous" ? "需要选择" : "需要补充"}
          </div>
          <div className="mt-2 grid gap-2">
            {fields.map((field) => (
              <div
                key={field.id}
                className="rounded-md border border-slate-100 bg-slate-50 px-2 py-1.5"
              >
                <div className="flex flex-wrap items-center gap-2">
                  <span className="font-medium text-slate-900">
                    {field.label || paramDisplayLabel(field.id)}
                  </span>
                  {field.required ? (
                    <Badge
                      variant="outline"
                      className="border-slate-200 bg-white text-slate-600"
                    >
                      必填
                    </Badge>
                  ) : null}
                </div>
                {field.candidates?.length ? (
                  <div className="mt-1 flex flex-wrap gap-1">
                    {field.candidates.slice(0, 4).map((candidate, index) => (
                      <span
                        key={`${candidateLabel(candidate)}-${index}`}
                        className="rounded-md border border-slate-200 bg-white px-2 py-0.5 text-slate-600"
                      >
                        {candidateLabel(candidate)}
                      </span>
                    ))}
                  </div>
                ) : field.placeholder ? (
                  <p className="mt-1 text-slate-500">{field.placeholder}</p>
                ) : null}
              </div>
            ))}
          </div>
        </section>
      ) : null}

      <div className="flex flex-wrap gap-2 border-t border-slate-200 pt-3">
        {status === "resolved" && !hasMergedPreflightResult ? (
          <Button
            type="button"
            size="sm"
            className="h-8 rounded-md"
            disabled={preflightRunning}
            onClick={() => {
              setPreflightRunning(true);
              dispatchComposerContextSubmit(
                artifact.id,
                `运行运维手册预检：${manualId || workflowId || "当前手册"}`,
                {
                  opsManualAction: "run_ops_manual_preflight",
                  manualId,
                  workflowId,
                  resolvedParamsJson: JSON.stringify(
                    resolvedParamsToPayload(resolvedParams),
                  ),
                },
              );
            }}
          >
            {preflightRunning ? (
              <LoaderCircle className="h-3.5 w-3.5 animate-spin" />
            ) : (
              <ShieldCheck className="h-3.5 w-3.5" />
            )}
            {preflightRunning ? "预检中" : "运行预检"}
          </Button>
        ) : null}
        {status === "resolved" && hasMergedPreflightResult ? (
          <span
            className="inline-flex h-8 items-center gap-1 rounded-md border border-emerald-200 bg-emerald-50 px-2 text-emerald-700"
            data-testid="ops-manual-param-preflight-completed"
          >
            <ShieldCheck className="h-3.5 w-3.5" />
            {PREFLIGHT_LABELS[mergedPreflightStatus] || "预检已完成"}
          </span>
        ) : null}
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="h-8 rounded-md"
          disabled={skipSubmitted}
          onClick={() => {
            setSkipSubmitted(true);
            dispatchComposerContextSubmit(
              artifact.id,
              skipManualText(manualId || "当前运维手册"),
              skipManualMetadata(
                manualId,
                workflowId,
                manualId || "当前运维手册",
              ),
            );
          }}
        >
          {skipSubmitted ? "已切换" : "不使用"}
        </Button>
      </div>
      {preflightRunning ? (
        <div
          className="rounded-md border border-sky-200 bg-sky-50 px-2 py-1.5 text-sky-700"
          data-testid="ops-manual-preflight-running"
        >
          预检请求已提交，正在等待只读探针结果。
        </div>
      ) : null}
      {skipSubmitted ? (
        <div
          className="rounded-md border border-slate-200 bg-white px-2 py-1.5 text-slate-600"
          data-testid="ops-manual-skip-submitted"
        >
          已切换为普通只读排查，等待 Agent 继续处理。
        </div>
      ) : null}
    </div>
  );
}

function ParamValueRow({ param }: { param: LooseRecord }) {
  const id = text(pick(param, "id"));
  const value = text(pick(param, "value"), "已解析");
  const source = text(pick(param, "source"));
  const evidence = text(pick(param, "evidence"));
  return (
    <>
      <dt className="font-medium text-slate-500">{paramDisplayLabel(id)}</dt>
      <dd className="min-w-0">
        <span className="font-mono text-slate-800">{value}</span>
        {source || evidence ? (
          <span className="block break-words text-slate-500 sm:mt-0.5">
            {[paramSourceLabel(source), evidence].filter(Boolean).join("；")}
          </span>
        ) : null}
      </dd>
    </>
  );
}

export function OpsManualFallbackGuideArtifact({
  artifact,
}: {
  artifact: AiopsTransportAgentUiArtifact;
}) {
  const data = artifactData(artifact);
  const title = text(
    pick(data, "title", "manualTitle", "manual_title"),
    "降级运维步骤",
  );
  const reason = text(pick(data, "reason", "summary"));
  const steps = stringArray(pick(data, "steps", "fallback_steps"));

  return (
    <div
      className="mt-3 grid gap-3 rounded-lg border border-slate-100 bg-slate-50 p-3 text-xs"
      data-testid="ops-manual-fallback-guide-card"
    >
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="min-w-0">
          <Badge variant="outline" className={STATE_TONE.reference_only}>
            仅参考
          </Badge>
          <div className="mt-1 text-sm font-medium text-slate-950">{title}</div>
          {reason ? (
            <p className="mt-1 leading-5 text-slate-600">{reason}</p>
          ) : null}
        </div>
        <FileText className="h-4 w-4 text-slate-500" />
      </div>
      {steps.length ? (
        <ol className="grid gap-2">
          {steps.map((step, index) => (
            <li
              key={`${index}-${step}`}
              className="rounded-md border border-slate-200 bg-white p-2 leading-5 text-slate-700"
            >
              {index + 1}. {step}
            </li>
          ))}
        </ol>
      ) : (
        <div className="rounded-md border border-slate-200 bg-white p-2 text-slate-600">
          当前没有可直接运行的工作流，按手册内容逐步确认后再执行。
        </div>
      )}
    </div>
  );
}

export function RunnerWorkflowGenerationArtifact({
  artifact,
}: {
  artifact: AiopsTransportAgentUiArtifact;
}) {
  const [previewOpen, setPreviewOpen] = useState(false);
  const data = artifactData(artifact);
  const title = text(
    pick(data, "workflowTitle", "workflow_title", "title"),
    "Runner Workflow 生成进度",
  );
  const steps = arrayRecords(pick(data, "steps", "timeline", "nodes")).filter(
    (step) => !isManualApprovalStep(step),
  );
  const workflowId = text(pick(data, "workflowId", "workflow_id"));

  return (
    <div
      className="mt-3 rounded-lg border border-slate-100 bg-slate-50 p-3 text-xs"
      data-testid="runner-workflow-generation-card"
    >
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-sm font-medium text-slate-950">
            <GitBranch className="h-4 w-4 text-slate-500" />
            {title}
          </div>
          <p className="mt-1 leading-5 text-slate-500">
            节点会在对话中逐步生成；预览只读，不会跳转 Runner Studio
            或修改工作流。
          </p>
        </div>
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="h-8 rounded-md"
          onClick={() => setPreviewOpen(true)}
        >
          <Eye className="h-3.5 w-3.5" />
          预览 Runner 草稿
        </Button>
      </div>
      <ol className="mt-3 grid gap-2">
        {steps.map((step, index) => {
          const status = text(pick(step, "status", "state"), "waiting");
          const Icon = iconForStep(status);
          return (
            <li
              key={text(pick(step, "id"), String(index))}
              className="flex items-start gap-2 rounded-md border border-slate-200 bg-white p-2"
            >
              <span
                className={`mt-0.5 rounded-full p-1 ${STEP_TONE[status] || STEP_TONE.waiting}`}
              >
                <Icon className="h-3.5 w-3.5" />
              </span>
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="font-medium text-slate-900">
                    {text(pick(step, "title", "name"), `节点 ${index + 1}`)}
                  </span>
                  <Badge
                    variant="outline"
                    className={STEP_TONE[status] || STEP_TONE.waiting}
                  >
                    {STEP_LABELS[status] || status}
                  </Badge>
                </div>
                {text(pick(step, "summary", "description")) ? (
                  <p className="mt-1 leading-5 text-slate-600">
                    {text(pick(step, "summary", "description"))}
                  </p>
                ) : null}
              </div>
            </li>
          );
        })}
      </ol>
      <Dialog open={previewOpen} onOpenChange={setPreviewOpen}>
        <DialogContent className="max-h-[86vh] overflow-y-auto sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>Runner Workflow 只读预览</DialogTitle>
            <DialogDescription>
              这是 AI 在对话中生成的 Runner
              草稿预览，只读展示节点、状态和说明，不支持在弹窗内编辑或执行。
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-3 text-sm">
            <section className="rounded-lg border border-slate-200 bg-slate-50 p-3">
              <div className="text-xs font-medium text-slate-500">工作流</div>
              <div className="mt-1 font-medium text-slate-950">{title}</div>
              {workflowId ? (
                <div className="mt-1 font-mono text-xs text-slate-500">
                  {workflowId}
                </div>
              ) : null}
            </section>
            <section className="rounded-lg border border-slate-200 bg-white p-3">
              <div className="text-xs font-medium text-slate-500">只读节点</div>
              <ol className="mt-3 grid gap-2">
                {steps.map((step, index) => {
                  const status = text(pick(step, "status", "state"), "waiting");
                  return (
                    <li
                      key={`preview-${text(pick(step, "id"), String(index))}`}
                      className="rounded-md border border-slate-200 bg-slate-50 p-2"
                    >
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="font-medium text-slate-950">
                          {index + 1}.{" "}
                          {text(
                            pick(step, "title", "name"),
                            `节点 ${index + 1}`,
                          )}
                        </span>
                        <Badge
                          variant="outline"
                          className={STEP_TONE[status] || STEP_TONE.waiting}
                        >
                          {STEP_LABELS[status] || status}
                        </Badge>
                      </div>
                      {text(pick(step, "summary", "description")) ? (
                        <p className="mt-1 leading-5 text-slate-600">
                          {text(pick(step, "summary", "description"))}
                        </p>
                      ) : null}
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

function dispatchComposerConfirmation(
  action: string,
  title: string,
  sourceTitle: string,
  artifactId: string,
) {
  window.dispatchEvent(
    new CustomEvent("aiops:composer-confirmation", {
      detail: { action, title, sourceTitle, artifactId },
    }),
  );
}

function dispatchContextRequest(
  artifactId: string,
  title: string,
  fields: ContextFormField[],
  force = false,
  contextText = "",
  extra: Record<string, unknown> = {},
) {
  if (!fields.length) return;
  window.dispatchEvent(
    new CustomEvent("aiops:composer-context-request", {
      detail: {
        artifactId,
        force,
        title,
        contextText,
        fields,
        ...extra,
      },
    }),
  );
}

function dispatchComposerContextSubmit(
  artifactId: string,
  text: string,
  metadata: Record<string, string> = {},
) {
  window.dispatchEvent(
    new CustomEvent("aiops:composer-context-submit", {
      detail: {
        artifactId,
        text,
        metadata,
      },
    }),
  );
}

function CompactManualCandidate({
  hit,
  operationFrame,
  artifactId,
  autoContinueContext = false,
}: {
  hit: LooseRecord;
  operationFrame?: LooseRecord;
  artifactId?: string;
  autoContinueContext?: boolean;
}) {
  const [skipSubmitted, setSkipSubmitted] = useState(false);
  const manualTitle = manualTitleFromHit(hit) || "候选运维手册";
  const manualId = manualIdFromHit(hit);
  const boundWorkflowId = text(
    pick(
      hit,
      "boundWorkflowId",
      "bound_workflow_id",
      "workflowId",
      "workflow_id",
    ),
  );
  const manualPreview = manualPreviewFromHit(hit);
  const workflowPreview = workflowPreviewFromHit(hit, manualTitle);
  const explicitMatchedFields = stringArray(
    pick(hit, "matchedFields", "matched_fields"),
  )
    .map((item) => matchedFieldLabel(item))
    .filter(Boolean);
  const matchedFields = explicitMatchedFields.length
    ? explicitMatchedFields
    : inferredMatchedFieldLabels(operationFrame);
  const blockedReasons = stringArray(
    pick(hit, "blockedReasons", "blocked_reasons"),
  ).map((item) => blockedReasonLabel(item, hit, operationFrame));
  const reasonText = autoContinueContext
    ? "目标位置默认当前主机，实例/服务和访问入口自动探测；发现多个候选时再让你选择。"
    : blockedReasons.length
      ? blockedReasons.join("；")
      : "缺少目标位置、实例对象或访问入口，暂不能进入 Workflow 预检。";
  const [expanded, setExpanded] = useState(false);
  const [workflowOpen, setWorkflowOpen] = useState(false);
  const [manualOpen, setManualOpen] = useState(false);
  return (
    <div
      className="mt-3 rounded-md border border-slate-200 bg-white text-slate-600"
      data-testid="ops-manual-candidate-manual"
    >
      <button
        type="button"
        className="flex w-full items-center justify-between gap-2 px-2 py-1.5 text-left hover:bg-slate-50 focus:outline-none focus:ring-2 focus:ring-slate-300"
        aria-expanded={expanded}
        onClick={() => setExpanded((current) => !current)}
        data-testid="ops-manual-candidate-toggle"
      >
        <span>
          候选手册：
          <span className="font-medium text-slate-800">{manualTitle}</span>
        </span>
        <span className="text-[11px] text-slate-400">查看命中依据</span>
      </button>
      {expanded ? (
        <dl
          className="grid gap-2 border-t border-slate-100 px-2 py-2 sm:grid-cols-[5rem_1fr]"
          data-testid="ops-manual-candidate-match-detail"
        >
          {matchedFields.length ? (
            <>
              <dt className="font-medium text-slate-500">命中依据</dt>
              <dd>{matchedFields.join("；")}</dd>
            </>
          ) : null}
          {boundWorkflowId ? (
            <>
              <dt className="font-medium text-slate-500">绑定 Workflow</dt>
              <dd className="font-mono text-slate-700">{boundWorkflowId}</dd>
            </>
          ) : null}
          <dt className="font-medium text-slate-500">
            {autoContinueContext ? "使用方式" : "不能直用"}
          </dt>
          <dd>{reasonText}</dd>
        </dl>
      ) : null}
      <div className="flex flex-wrap gap-2 border-t border-slate-100 px-2 py-2">
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="h-8 rounded-md"
          disabled={skipSubmitted}
          onClick={() => {
            setSkipSubmitted(true);
            dispatchComposerContextSubmit(
              artifactId || "",
              skipManualText(manualTitle, operationFrame),
              skipManualMetadata(manualId, boundWorkflowId, manualTitle),
            );
          }}
        >
          {skipSubmitted ? "已切换" : "不使用"}
        </Button>
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="h-8 rounded-md"
          onClick={() => setWorkflowOpen(true)}
        >
          <GitBranch className="h-3.5 w-3.5" />
          查看工作流
        </Button>
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="h-8 rounded-md"
          onClick={() => setManualOpen(true)}
        >
          <FileText className="h-3.5 w-3.5" />
          查看手册
        </Button>
      </div>
      <WorkflowReadOnlyDialog
        open={workflowOpen}
        onOpenChange={setWorkflowOpen}
        preview={workflowPreview}
        fallbackWorkflowId={boundWorkflowId}
      />
      <ManualReadOnlyDialog
        open={manualOpen}
        onOpenChange={setManualOpen}
        preview={manualPreview}
      />
      {skipSubmitted ? (
        <div
          className="border-t border-slate-100 px-2 py-2 text-slate-600"
          data-testid="ops-manual-skip-submitted"
        >
          已切换为普通只读排查，等待 Agent 继续处理。
        </div>
      ) : null}
    </div>
  );
}

function WorkflowReadOnlyDialog({
  open,
  onOpenChange,
  preview,
  fallbackWorkflowId,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  preview: { id: string; title: string; nodes: LooseRecord[] };
  fallbackWorkflowId: string;
}) {
  const nodes = preview.nodes;
  const [selectedNodeId, setSelectedNodeId] = useState("");
  const activeNode =
    nodes.find((node) => text(pick(node, "id", "key")) === selectedNodeId) ||
    nodes[0] ||
    {};
  useEffect(() => {
    if (!open) return;
    const firstNodeId = text(pick(nodes[0] || {}, "id", "key"));
    setSelectedNodeId((current) => current || firstNodeId);
  }, [open, nodes.map((node) => text(pick(node, "id", "key"))).join("|")]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[86vh] overflow-y-auto sm:max-w-4xl">
        <DialogHeader>
          <DialogTitle>工作流只读预览</DialogTitle>
          <DialogDescription>
            只能查看工作流节点和脚本内容，不能在这里修改、执行或发布。
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-3 text-sm">
          <section className="rounded-lg border border-slate-200 bg-slate-50 p-3">
            <div className="text-xs font-medium text-slate-500">Workflow</div>
            <div className="mt-1 font-medium text-slate-950">
              {preview.title}
            </div>
            {preview.id || fallbackWorkflowId ? (
              <div className="mt-1 font-mono text-xs text-slate-500">
                {preview.id || fallbackWorkflowId}
              </div>
            ) : null}
          </section>
          {nodes.length ? (
            <div className="grid gap-3 md:grid-cols-[16rem_1fr]">
              <section className="rounded-lg border border-slate-200 bg-white p-2">
                <div className="px-1 text-xs font-medium text-slate-500">
                  节点
                </div>
                <div className="mt-2 grid gap-1">
                  {nodes.map((node, index) => {
                    const nodeId = text(pick(node, "id", "key"), String(index));
                    const active =
                      nodeId === text(pick(activeNode, "id", "key"), "0");
                    return (
                      <button
                        key={`${nodeId}-${index}`}
                        type="button"
                        className={[
                          "rounded-md px-2 py-2 text-left text-sm",
                          active
                            ? "bg-slate-950 text-white"
                            : "bg-slate-50 text-slate-700 hover:bg-slate-100",
                        ].join(" ")}
                        onClick={() => setSelectedNodeId(nodeId)}
                      >
                        <span className="block font-medium">
                          {index + 1}.{" "}
                          {text(
                            pick(node, "title", "name", "label"),
                            `节点 ${index + 1}`,
                          )}
                        </span>
                        {text(pick(node, "type", "kind")) ? (
                          <span className="mt-1 block text-xs opacity-75">
                            {text(pick(node, "type", "kind"))}
                          </span>
                        ) : null}
                      </button>
                    );
                  })}
                </div>
              </section>
              <section className="rounded-lg border border-slate-200 bg-white p-3">
                <div className="text-xs font-medium text-slate-500">
                  节点内容
                </div>
                <div className="mt-1 text-base font-semibold text-slate-950">
                  {text(pick(activeNode, "title", "name", "label"), "节点详情")}
                </div>
                {text(pick(activeNode, "summary", "description")) ? (
                  <p className="mt-2 leading-6 text-slate-600">
                    {text(pick(activeNode, "summary", "description"))}
                  </p>
                ) : null}
                {text(
                  pick(activeNode, "command", "script", "code", "shell"),
                ) ? (
                  <pre className="mt-3 overflow-x-auto rounded-lg border border-slate-200 bg-slate-950 p-3 text-xs leading-5 text-slate-100">
                    {text(
                      pick(activeNode, "command", "script", "code", "shell"),
                    )}
                  </pre>
                ) : (
                  <div className="mt-3 rounded-lg border border-slate-200 bg-slate-50 p-3 text-slate-500">
                    该节点没有返回脚本内容，只能查看节点名称和说明。
                  </div>
                )}
              </section>
            </div>
          ) : (
            <div className="rounded-lg border border-slate-200 bg-white p-3 text-slate-600">
              当前检索结果没有返回工作流节点明细，只能确认绑定关系，不能在这里修改或执行。
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}

function ManualReadOnlyDialog({
  open,
  onOpenChange,
  preview,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  preview: {
    id: string;
    title: string;
    description: string;
    content: string;
    sections: LooseRecord[];
  };
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[86vh] overflow-y-auto sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>运维手册只读预览</DialogTitle>
          <DialogDescription>
            这里只展示手册文档、适用范围和关键步骤，不能在弹窗内编辑。
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-3 text-sm">
          <section className="rounded-lg border border-slate-200 bg-slate-50 p-3">
            <div className="text-xs font-medium text-slate-500">运维手册</div>
            <div className="mt-1 font-medium text-slate-950">
              {preview.title}
            </div>
            {preview.id ? (
              <div className="mt-1 font-mono text-xs text-slate-500">
                {preview.id}
              </div>
            ) : null}
            {preview.description ? (
              <p className="mt-2 leading-6 text-slate-600">
                {preview.description}
              </p>
            ) : null}
          </section>
          {preview.content ? (
            <section className="rounded-lg border border-slate-200 bg-white p-3">
              <div className="text-xs font-medium text-slate-500">文档</div>
              <p className="mt-2 whitespace-pre-wrap leading-6 text-slate-700">
                {preview.content}
              </p>
            </section>
          ) : null}
          {preview.sections.length ? (
            <section className="rounded-lg border border-slate-200 bg-white p-3">
              <div className="text-xs font-medium text-slate-500">
                结构化内容
              </div>
              <div className="mt-3 grid gap-2">
                {preview.sections.map((section, index) => (
                  <div
                    key={`${text(pick(section, "title", "name"), String(index))}-${index}`}
                    className="rounded-md border border-slate-200 bg-slate-50 p-2"
                  >
                    <div className="font-medium text-slate-950">
                      {text(
                        pick(section, "title", "name"),
                        `章节 ${index + 1}`,
                      )}
                    </div>
                    {text(
                      pick(section, "content", "summary", "description"),
                    ) ? (
                      <p className="mt-1 leading-5 text-slate-600">
                        {text(
                          pick(section, "content", "summary", "description"),
                        )}
                      </p>
                    ) : null}
                  </div>
                ))}
              </div>
            </section>
          ) : null}
        </div>
      </DialogContent>
    </Dialog>
  );
}

function SearchManualHit({
  hit,
  index,
  operationFrame,
}: {
  hit: LooseRecord;
  index: number;
  operationFrame?: LooseRecord;
}) {
  const manualTitle = manualTitleFromHit(hit) || `相关手册 ${index + 1}`;
  const boundWorkflowId = text(
    pick(
      hit,
      "boundWorkflowId",
      "bound_workflow_id",
      "workflowId",
      "workflow_id",
    ),
  );
  const usableMode = normalizeDecision(
    text(pick(hit, "usableMode", "usable_mode", "decision", "state")),
  );
  const environmentDiffs = stringArray(
    pick(
      hit,
      "environmentDiffs",
      "environment_diffs",
      "compatibilityGaps",
      "compatibility_gaps",
    ),
  ).map((item) => taxonomyLabel(item));
  const blockedReasons = stringArray(
    pick(hit, "blockedReasons", "blocked_reasons"),
  ).map((item) => blockedReasonLabel(item, hit, operationFrame));
  const referenceRelation = referenceRelationLabel(
    hit,
    operationFrame,
    usableMode,
  );

  return (
    <section className="rounded-md border border-slate-200 bg-white p-2">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-medium text-slate-950">{manualTitle}</span>
            <Badge
              variant="outline"
              className={STATE_TONE[usableMode] || STATE_TONE.no_match}
            >
              {STATE_LABELS[usableMode] || usableMode}
            </Badge>
          </div>
        </div>
      </div>
      {referenceRelation ? (
        <div className="mt-2 rounded-md border border-slate-200 bg-slate-50 px-2 py-1.5 text-slate-600">
          {referenceRelation}
        </div>
      ) : null}
      <dl className="mt-2 grid gap-2 sm:grid-cols-[7rem_1fr]">
        {boundWorkflowId ? (
          <>
            <dt className="font-medium text-slate-500">绑定 Workflow</dt>
            <dd className="font-mono text-slate-700">{boundWorkflowId}</dd>
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
      </dl>
    </section>
  );
}

function normalizeParamResolutionStatus(value: string) {
  const normalized = value.trim().toLowerCase();
  if (normalized === "resolved") return "resolved";
  if (normalized === "ambiguous") return "ambiguous";
  if (
    normalized === "need_user_input" ||
    normalized === "need_info" ||
    normalized === "missing"
  )
    return "need_user_input";
  return "unresolved";
}

function paramResolutionTitle(status: string) {
  if (status === "resolved") return "参数已补齐，可进入预检";
  if (status === "ambiguous") return "需要确认参数";
  if (status === "need_user_input") return "需要补充参数";
  return "参数暂未补齐";
}

function paramResolutionTone(status: string) {
  if (status === "resolved")
    return "border-emerald-200 bg-emerald-50 text-emerald-700";
  if (status === "ambiguous" || status === "need_user_input")
    return "border-amber-200 bg-amber-50 text-amber-700";
  return "border-slate-200 bg-white text-slate-600";
}

function paramResolutionFormFields(data: LooseRecord): ContextFormField[] {
  const raw = pick(data, "fields", "formFields", "form_fields");
  if (!Array.isArray(raw)) return [];
  const seen = new Set<string>();
  return raw
    .map((field) => normalizeParamResolutionFormField(field))
    .filter((field): field is ContextFormField => Boolean(field))
    .filter((field) => {
      if (seen.has(field.id)) return false;
      seen.add(field.id);
      return true;
    });
}

function normalizeParamResolutionFormField(
  field: unknown,
): ContextFormField | null {
  const record = asRecord(field);
  if (!Object.keys(record).length) return null;
  const id = text(pick(record, "id", "key", "name")).trim();
  if (!id) return null;
  const typeValue = text(pick(record, "type"));
  return {
    id,
    label: text(pick(record, "label", "title"), paramDisplayLabel(id)),
    type: typeValue,
    required: booleanValue(pick(record, "required")),
    sensitive: booleanValue(pick(record, "sensitive")),
    uiControl: text(pick(record, "uiControl", "ui_control")),
    placeholder: text(pick(record, "placeholder", "hint")),
    default: pick(record, "default", "defaultValue", "default_value"),
    candidates: paramCandidates(pick(record, "candidates", "options")),
  };
}

function paramCandidates(value: unknown): ParamCandidate[] {
  if (!Array.isArray(value)) return [];
  return value
    .map((item) => {
      const record = asRecord(item);
      if (!Object.keys(record).length) return null;
      return {
        value: pick(record, "value", "id"),
        label: text(pick(record, "label", "name")),
        hint: text(pick(record, "hint", "description")),
        source: text(pick(record, "source")),
        confidence: numberValue(pick(record, "confidence")),
        evidence: text(pick(record, "evidence")),
      };
    })
    .filter((candidate): candidate is ParamCandidate => Boolean(candidate));
}

function candidateLabel(candidate: ParamCandidate) {
  return candidate.label || text(candidate.value) || "候选项";
}

function paramDisplayLabel(id: string) {
  const normalized = id.trim().toLowerCase();
  const labels: Record<string, string> = {
    target_host: "目标主机",
    target_location: "目标位置",
    target_instance: "目标实例",
    redis_instance: "Redis 实例",
    mysql_instance: "MySQL 实例",
    pg_instance: "PostgreSQL 实例",
    execution_surface: "访问/执行入口",
    backup_path: "备份路径",
    symptom: "现象/证据",
  };
  return labels[normalized] || taxonomyLabel(id) || id;
}

function paramSourceLabel(source: string) {
  const normalized = source.trim().toLowerCase();
  const labels: Record<string, string> = {
    selected_host: "当前选择主机",
    conversation: "对话上下文",
    manual_default: "手册默认值",
    run_record: "历史执行记录",
    coroot: "Coroot 只读探测",
    host_readonly: "主机只读探测",
    docker: "Docker 只读探测",
    k8s: "Kubernetes 只读探测",
    user_confirmed: "用户已确认",
  };
  return labels[normalized] || source;
}

function progressMatchText(hit: LooseRecord, operationFrame?: LooseRecord) {
  const explicitMatchedFields = stringArray(
    pick(hit, "matchedFields", "matched_fields"),
  )
    .map((item) => matchedFieldLabel(item))
    .filter(Boolean);
  const matchedFields = explicitMatchedFields.length
    ? explicitMatchedFields
    : inferredMatchedFieldLabels(operationFrame);
  return matchedFields.length
    ? matchedFields.join("；")
    : "结构化检索命中当前请求。";
}

function resolvedParamsToPayload(params: LooseRecord[]) {
  return Object.fromEntries(
    params
      .map((param) => [text(pick(param, "id")), pick(param, "value")])
      .filter(([key]) => Boolean(key)),
  );
}

function normalizeDecision(value: string) {
  const normalized = value.toLowerCase();
  if (
    normalized === "direct" ||
    normalized === "direct_execute" ||
    normalized === "executable"
  )
    return "direct_execute";
  if (
    normalized === "need_info" ||
    normalized === "need_more_info" ||
    normalized === "missing_info"
  )
    return "need_info";
  if (
    normalized === "adapt" ||
    normalized === "adapt_required" ||
    normalized === "generate_variant"
  )
    return "adapt";
  if (normalized === "reference" || normalized === "reference_only")
    return "reference_only";
  return "no_match";
}

function isCrossObjectHit(hit: LooseRecord, operationFrame?: LooseRecord) {
  const currentObject = operationFrameObjectLabel(operationFrame);
  if (!currentObject) return false;
  const manual = asRecord(pick(hit, "manual", "opsManual", "ops_manual"));
  const manualOperation = asRecord(pick(manual, "operation"));
  const manualObject = taxonomyLabel(
    text(
      pick(manualOperation, "target_type", "targetType"),
      text(pick(asRecord(pick(manual, "applicability")), "middleware")),
    ),
  );
  return Boolean(manualObject && currentObject !== manualObject);
}

function crossObjectNoMatchSummary(operationFrame?: LooseRecord) {
  const objectType = operationFrameObjectLabel(operationFrame);
  return objectType
    ? `没有找到适用于 ${objectType} 的可用运维手册。`
    : "没有找到适用于当前对象的可用运维手册。";
}

function defaultSearchSummary(decision: string) {
  if (decision === "direct_execute") return "找到可进入预检的运维手册。";
  if (decision === "need_info")
    return "缺少目标位置、实例对象或访问入口，先补齐必要上下文。";
  if (decision === "adapt") return "找到相关运维手册，但当前环境需要适配。";
  if (decision === "reference_only")
    return "没有可直接运行的 Workflow，可继续只读自动化排查。";
  return "没有找到合适的运维手册。";
}

function searchResultTitle(decision: string) {
  if (decision === "direct_execute") return "找到可进入预检的运维手册";
  if (decision === "need_info") return "运维手册待补目标信息";
  if (decision === "adapt") return "找到需要适配的运维手册";
  if (decision === "reference_only") return "找到可参考的运维手册";
  return "未找到合适的运维手册";
}

function searchActionsForDecision(
  decision: string,
  manuals: LooseRecord[],
): SearchResultAction[] {
  if (decision === "direct_execute") {
    return [];
  }
  if (decision === "need_info") {
    return [];
  }
  if (decision === "adapt") {
    return [
      {
        id: "generate-variant",
        label: "生成适配工作流",
        confirmationAction: "generate_runner_workflow_candidate",
        primary: true,
      },
    ];
  }
  if (decision === "reference_only") {
    return [];
  }
  return [];
}

function searchStage(decision: string) {
  if (decision === "direct_execute") return "运维手册已匹配，下一步是运行预检";
  if (decision === "adapt") return "手册可参考，但 Workflow 需要适配";
  if (decision === "reference_only") return "没有可直接运行的 Workflow";
  if (decision === "no_match") return "未找到适用手册，AI 将继续只读排查";
  return searchResultTitle(decision);
}

function searchNextStep(decision: string, recommendedNextAction: string) {
  if (decision === "direct_execute")
    return "下一步：AI 会先运行只读预检；通过并确认后再 Dry Run。";
  if (decision === "adapt")
    return "下一步：AI 会生成适配 Workflow 草稿，并先做只读预检。";
  if (decision === "reference_only")
    return normalizeSearchNextStep(
      recommendedNextAction,
      "下一步：AI 会继续自动只读排查；如果缺少 Kafka 集群、时间范围、权限或观测数据，会先让你补齐必要信息。",
    );
  if (decision === "no_match")
    return normalizeSearchNextStep(
      recommendedNextAction,
      "下一步：AI 不使用不匹配的手册，会继续自动只读收集证据；如果缺少目标、时间范围、权限或观测数据，会先让你补齐必要信息。",
    );
  return "";
}

function normalizeSearchNextStep(value: string, fallback: string) {
  const normalized = value.trim();
  if (!normalized) return fallback;
  if (
    normalized.includes("继续普通 Agent 运维流程") ||
    normalized.includes("继续普通排查") ||
    normalized.includes("按手册步骤参考执行")
  ) {
    return fallback;
  }
  if (!normalized.startsWith("下一步")) {
    return `下一步：${normalized}`;
  }
  return normalized;
}

function referenceRelationLabel(
  hit: LooseRecord,
  operationFrame?: LooseRecord,
  usableMode = "",
) {
  if (usableMode !== "reference_only") return "";
  const manual = asRecord(pick(hit, "manual", "opsManual", "ops_manual"));
  const manualOperation = asRecord(pick(manual, "operation"));
  const currentObject = operationFrameObjectLabel(operationFrame);
  const manualObject = taxonomyLabel(
    text(
      pick(manualOperation, "target_type", "targetType"),
      text(pick(asRecord(pick(manual, "applicability")), "middleware")),
    ),
  );
  if (currentObject && manualObject && currentObject !== manualObject) {
    return "";
  }
  const action = taxonomyLabel(text(pick(manualOperation, "action")));
  return `参考关系：同属 ${currentObject || manualObject || "当前对象"} 的${action ? `「${action}」` : "排查"}经验，可参考排查顺序和验证点；不能直接套用 Workflow。`;
}

function matchedFieldLabel(field: string) {
  const normalized = field.trim().toLowerCase();
  const labels: Record<string, string> = {
    object_type: "对象类型",
    target_type: "对象类型",
    operation_type: "操作类型",
    action: "操作类型",
    middleware: "中间件类型",
    execution_surface: "访问/执行入口",
    environment: "运行环境",
    required_context: "必要上下文",
    signal: "现象/信号",
  };
  return labels[normalized] || taxonomyLabel(field);
}

function inferredMatchedFieldLabels(operationFrame?: LooseRecord) {
  const labels: string[] = [];
  const objectLabel = operationFrameObjectLabel(operationFrame);
  const operationLabel = operationFrameOperationLabel(operationFrame);
  if (objectLabel) labels.push(`对象类型 ${objectLabel}`);
  if (operationLabel) labels.push(`操作类型 ${operationLabel}`);
  return labels;
}

function skipManualText(manualTitle: string, operationFrame?: LooseRecord) {
  const operation = operationFrameLabel(operationFrame);
  const operationText = operation ? `当前请求：${operation}；` : "";
  return `已选择跳过运维手册「${manualTitle}」。本轮后续不要再调用 search_ops_manuals、resolve_ops_manual_params 或 run_ops_manual_preflight；请按普通只读排查继续。${operationText}默认使用当前选择主机；先做只读检查。`;
}

function skipManualMetadata(
  manualId: string,
  workflowId: string,
  manualTitle: string,
): Record<string, string> {
  return {
    opsManualAction: OPS_MANUAL_SKIP_ACTION,
    opsManualSkipped: "true",
    ...(manualId ? { opsManualManualId: manualId, manualId } : {}),
    ...(workflowId ? { opsManualWorkflowId: workflowId, workflowId } : {}),
    ...(manualTitle ? { opsManualManualTitle: manualTitle } : {}),
  };
}

function manualPreviewFromHit(hit: LooseRecord) {
  const manual = asRecord(pick(hit, "manual", "opsManual", "ops_manual"));
  return {
    id: manualIdFromHit(hit),
    title: manualTitleFromHit(hit) || "候选运维手册",
    description: text(
      pick(manual, "description", "summary", "abstract"),
      text(pick(hit, "summary", "description")),
    ),
    content: text(
      pick(
        manual,
        "content",
        "document",
        "markdown",
        "skill_md",
        "skillMd",
        "body",
      ),
    ),
    sections: arrayRecords(pick(manual, "sections", "chapters", "steps")),
  };
}

function workflowPreviewFromHit(hit: LooseRecord, manualTitle: string) {
  const workflow = asRecord(
    pick(
      hit,
      "workflowPreview",
      "workflow_preview",
      "workflow",
      "runnerWorkflow",
      "runner_workflow",
    ),
  );
  const fallbackWorkflowId = text(
    pick(
      hit,
      "boundWorkflowId",
      "bound_workflow_id",
      "workflowId",
      "workflow_id",
    ),
  );
  const nodes = arrayRecords(pick(workflow, "nodes", "steps", "timeline"));
  return {
    id: text(
      pick(workflow, "id", "workflowId", "workflow_id"),
      fallbackWorkflowId,
    ),
    title: text(
      pick(workflow, "title", "name", "workflowTitle", "workflow_title"),
      `${manualTitle} Workflow`,
    ),
    nodes,
  };
}

function blockedReasonLabel(
  reason: string,
  hit: LooseRecord,
  operationFrame?: LooseRecord,
) {
  const normalized = reason.trim().toLowerCase().replace(/[_-]+/g, " ");
  const manual = asRecord(pick(hit, "manual", "opsManual", "ops_manual"));
  const manualOperation = asRecord(pick(manual, "operation"));
  const currentObject = operationFrameObjectLabel(operationFrame);
  const manualObject = taxonomyLabel(
    text(
      pick(manualOperation, "target_type", "targetType"),
      text(pick(asRecord(pick(manual, "applicability")), "middleware")),
    ),
  );
  if (normalized === "object type differs" || normalized === "object differs") {
    if (currentObject && manualObject) {
      return `对象类型不匹配：当前请求是 ${currentObject}，候选手册适用于 ${manualObject}。`;
    }
    if (manualObject) {
      return `对象类型不匹配：候选手册适用于 ${manualObject}，不能直接用于当前对象。`;
    }
    return "对象类型不匹配，不能直接使用该 Workflow。";
  }
  if (
    normalized === "operation type differs" ||
    normalized === "operation differs"
  ) {
    return "操作类型不匹配，不能把该手册升级为可执行 Workflow。";
  }
  if (normalized === "workflow unavailable")
    return "该手册没有可安全运行的 Workflow。";
  if (
    normalized === "required context missing" ||
    normalized === "missing context" ||
    normalized === "context missing"
  ) {
    return "缺少目标位置、实例对象或访问入口，暂不能进入 Workflow 预检。";
  }
  if (
    normalized === "risk exceeds policy" ||
    normalized === "risk out of scope"
  )
    return "风险级别超出该手册允许范围。";
  if (normalized === "recent validation failed")
    return "该 Workflow 最近验证失败，只能作为参考。";
  return reason;
}

function operationFrameObjectLabel(frame?: LooseRecord) {
  const current = asRecord(frame);
  const target = asRecord(pick(current, "target"));
  const operation = asRecord(pick(current, "operation"));
  const raw = text(
    pick(current, "objectType", "object_type"),
    text(
      pick(target, "type"),
      text(pick(operation, "target_type", "targetType")),
    ),
  );
  return taxonomyLabel(raw);
}

function operationFrameOperationLabel(frame?: LooseRecord) {
  const current = asRecord(frame);
  const operation = asRecord(pick(current, "operation"));
  const raw = text(
    pick(current, "operationType", "operation_type"),
    text(pick(operation, "action", "type")),
  );
  return taxonomyLabel(raw) || raw;
}

function taxonomyLabel(value: string) {
  const normalized = value.trim().toLowerCase();
  if (!normalized) return "";
  const labels: Record<string, string> = {
    redis: "Redis",
    mysql: "MySQL",
    postgresql: "PostgreSQL",
    postgres: "PostgreSQL",
    pg: "PostgreSQL",
    kafka: "Kafka",
    kubernetes: "Kubernetes",
    kubernetes_pod: "Kubernetes Pod",
    k8s_pod: "Kubernetes Pod",
    kubernetes_workload: "Kubernetes 工作负载",
    host: "主机",
    network: "网络",
    tool: "工具",
    backup: "备份",
    restore: "恢复",
    restart: "重启",
    rca_or_repair: "排障/修复",
    status_check: "状态检查",
    execution_surface: "执行方式",
    package_manager: "包管理器",
    os: "操作系统",
    platform: "运行平台",
  };
  return labels[normalized] || value;
}

function normalizePreflightStatus(value: string) {
  const normalized = value.toLowerCase();
  if (
    ["passed", "blocked", "failed", "not_applicable", "unknown"].includes(
      normalized,
    )
  )
    return normalized;
  return "unknown";
}

function preflightNextActionLabel(action: string) {
  if (action === "run_preflight_probe") return "运行预检";
  if (action === "start_dry_run") return "进入 Dry Run";
  if (action === "collect_required_context") return "补充 Workflow 参数";
  if (action === "request_permission") return "申请权限";
  if (action === "generate_workflow_variant") return "生成适配工作流";
  if (action === "fallback_guide") return "查看降级步骤";
  return action;
}

function preflightActions(
  status: string,
  nextAction: string,
): PreflightAction[] {
  if (status === "passed" || status === "not_applicable") {
    return [
      {
        id: "start-dry-run",
        label: "进入 Dry Run",
        confirmationAction: "start_runner_workflow_dry_run",
        primary: true,
      },
    ];
  }
  if (status === "failed") {
    return [
      {
        id: "fallback-guide",
        label: "查看降级步骤",
        icon: "warning",
        primary: true,
      },
    ];
  }
  if (nextAction === "request_permission") {
    return [{ id: "request-permission", label: "申请权限", primary: true }];
  }
  if (nextAction === "generate_workflow_variant") {
    return [
      {
        id: "generate-variant",
        label: "生成适配工作流",
        confirmationAction: "generate_runner_workflow_candidate",
        primary: true,
      },
    ];
  }
  return [
    { id: "collect-context", label: "补充 Workflow 参数", primary: true },
  ];
}

function preflightTitle(status: string) {
  if (status === "passed") return "Workflow 预检通过";
  if (status === "blocked") return "Workflow 预检缺参数、权限或环境条件";
  if (status === "failed") return "Workflow 预检失败";
  if (status === "not_applicable") return "该手册无需 Workflow 预检";
  return "Workflow 预检结果";
}

function limitItems<T>(items: T[], limit: number): T[] {
  return items.length > limit ? items.slice(0, limit) : items;
}

function manualTitleFromHit(hit?: LooseRecord) {
  if (!hit) return "";
  const manual = asRecord(pick(hit, "manual", "opsManual", "ops_manual"));
  return text(
    pick(hit, "title", "manualTitle", "manual_title"),
    text(pick(manual, "title", "name")),
  );
}

function manualIdFromHit(hit?: LooseRecord) {
  if (!hit) return "";
  const manual = asRecord(pick(hit, "manual", "opsManual", "ops_manual"));
  return text(
    pick(hit, "manualId", "manual_id", "id"),
    text(pick(manual, "id", "manualId", "manual_id")),
  );
}

function operationFrameLabel(frame: LooseRecord) {
  const target = asRecord(pick(frame, "target"));
  const operation = asRecord(pick(frame, "operation"));
  const objectType = text(
    pick(frame, "objectType", "object_type"),
    text(pick(target, "type")),
  );
  const operationType = text(
    pick(frame, "operationType", "operation_type"),
    text(pick(operation, "action", "type")),
  );
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
  return Object.keys(inline).length
    ? inline
    : { ...asRecord(artifact.payload), ...asRecord(artifact.metadata) };
}

function asRecord(value: unknown): LooseRecord {
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as LooseRecord)
    : {};
}

function arrayRecords(value: unknown): LooseRecord[] {
  return Array.isArray(value) ? value.map(asRecord) : [];
}

function isManualApprovalStep(step: LooseRecord): boolean {
  const id = text(pick(step, "id", "key", "type")).toLowerCase();
  const title = text(pick(step, "title", "name", "label")).toLowerCase();
  const action = text(
    pick(step, "action", "actionType", "action_type"),
  ).toLowerCase();
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
  const normalized = String(value)
    .replace(/<[^>]*>/g, "")
    .trim()
    .replace(/\s+/g, " ");
  return normalized || fallback;
}

function booleanValue(value: unknown) {
  if (typeof value === "boolean") return value;
  if (typeof value === "string")
    return ["true", "1", "yes"].includes(value.trim().toLowerCase());
  return Boolean(value);
}

function numberValue(value: unknown) {
  if (typeof value === "number" && Number.isFinite(value)) return value;
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : undefined;
}
