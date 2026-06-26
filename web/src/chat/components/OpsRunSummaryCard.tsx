import { useState } from "react";
import {
  Activity,
  Briefcase,
  Database,
  FileText,
  Lightbulb,
  Route,
} from "lucide-react";

import {
  archiveOpsRunCase,
  createOpsRunExperienceCandidates,
  createOpsRunRunRecord,
} from "@/api/chatOpsRuns";
import { Button } from "@/components/ui/button";
import type {
  AgentRunView,
  AgentStepView,
  AiopsTransportOpsRun,
  AiopsTransportState,
  PostRunSuggestion,
  PostRunSuggestionType,
} from "@/transport/aiopsTransportTypes";

type OpsRunSummaryCardProps = {
  state?: AiopsTransportState;
};

export function OpsRunSummaryCard({ state }: OpsRunSummaryCardProps) {
  const opsRun = state?.opsRun;
  const [pendingAction, setPendingAction] =
    useState<OpsRunArchiveAction | null>(null);
  const [archiveMessage, setArchiveMessage] = useState<string | null>(null);
  if (!opsRun?.id) {
    return null;
  }
  const agentRun = opsRun.agentRun;
  const latestStep = latestAgentStep(agentRun);
  const status = formatOpsRunStatus(agentRun?.status || opsRun.status);
  const title = agentRun?.userGoal || opsRun.title || "本次处理";
  const currentStep =
    agentRun?.currentStep || latestStep?.title || opsRun.currentStep;
  const targetSummary = agentRun?.targetSummary || opsRun.targetSummary;
  const routeMode = agentRun?.routeMode || opsRun.routeMode;
  const evidenceCount = agentRun?.evidenceCount ?? opsRun.evidenceCount;
  const postRunActions = supportedPostRunActions(opsRun.postRunSuggestions);
  const displayStatus = agentRun?.status || opsRun.status;
  const canArchive =
    isTerminalOpsRunStatus(opsRun.status) && postRunActions.length > 0;
  const hasUsefulTerminalContent =
    (evidenceCount || 0) > 0 || postRunActions.length > 0;

  if (isTerminalOpsRunStatus(displayStatus) && !hasUsefulTerminalContent) {
    return null;
  }

  return (
    <section
      className="mx-auto mb-2 w-[calc(100%-4rem)] max-w-[44.5rem] min-w-0 rounded-lg border border-slate-200 bg-white px-3 py-2 shadow-sm max-[520px]:w-[calc(100%-2.5rem)]"
      data-testid="ops-run-summary-card"
    >
      <div className="flex min-w-0 items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-2">
          <span className="flex size-7 shrink-0 items-center justify-center rounded-md bg-slate-100 text-slate-700">
            <Activity className="size-4" aria-hidden="true" />
          </span>
          <div className="min-w-0">
            <div className="flex min-w-0 items-center gap-2">
              <p className="truncate text-sm font-semibold text-slate-950">
                {title}
              </p>
              <span className={status.className}>{status.label}</span>
            </div>
            <div className="mt-0.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-slate-500">
              <span>{formatSource(opsRun.source)}</span>
              {currentStep ? (
                <span className="max-w-full truncate">
                  {currentStep}
                </span>
              ) : null}
              {latestStep?.title ? (
                <span className="max-w-full truncate">
                  最近：{latestStep.title} · {formatAgentStepStatus(latestStep.status)}
                </span>
              ) : null}
            </div>
          </div>
        </div>
        <OpsRunEvidencePill
          evidenceCount={evidenceCount}
          status={agentRun?.status || opsRun.status}
        />
      </div>
      {targetSummary ? (
        <div className="mt-2 flex min-w-0 flex-wrap items-center gap-1.5 rounded-md bg-slate-50 px-2 py-1 text-xs text-slate-600">
          <Route className="size-3.5 shrink-0" aria-hidden="true" />
          <span className="truncate">{targetSummary}</span>
          {routeMode ? (
            <span className="shrink-0 rounded border border-slate-200 bg-white px-1.5 py-0.5 text-[11px] text-slate-600">
              {formatRouteMode(routeMode)}
            </span>
          ) : null}
          {opsRun.toolSurfaceSummary ? (
            <span className="min-w-0 truncate rounded border border-slate-200 bg-white px-1.5 py-0.5 text-[11px] text-slate-600">
              {opsRun.toolSurfaceSummary}
            </span>
          ) : null}
        </div>
      ) : routeMode || opsRun.toolSurfaceSummary ? (
        <div className="mt-2 flex min-w-0 flex-wrap items-center gap-1.5 rounded-md bg-slate-50 px-2 py-1 text-xs text-slate-600">
          <Route className="size-3.5 shrink-0" aria-hidden="true" />
          {routeMode ? (
            <span className="shrink-0 rounded border border-slate-200 bg-white px-1.5 py-0.5 text-[11px] text-slate-600">
              {formatRouteMode(routeMode)}
            </span>
          ) : null}
          {opsRun.toolSurfaceSummary ? (
            <span className="min-w-0 truncate rounded border border-slate-200 bg-white px-1.5 py-0.5 text-[11px] text-slate-600">
              {opsRun.toolSurfaceSummary}
            </span>
          ) : null}
        </div>
      ) : null}
      {canArchive ? (
        <div className="mt-2 flex flex-wrap items-center gap-2 border-t border-slate-100 pt-2">
          {postRunActions.map((suggestion) => {
            const action = postRunArchiveActionFor(suggestion.type);
            if (!action) {
              return null;
            }
            const Icon = postRunActionIcon(action);
            return (
              <Button
                key={suggestion.type}
                type="button"
                variant="outline"
                size="xs"
                onClick={() =>
                  handleArchiveAction(
                    action,
                    opsRun,
                    setPendingAction,
                    setArchiveMessage,
                  )
                }
                disabled={pendingAction !== null}
                title={suggestion.reason || postRunActionDescription(action)}
                aria-label={postRunActionDescription(action)}
              >
                <Icon className="size-3.5" aria-hidden="true" />
                {suggestion.label || postRunActionLabel(action)}
              </Button>
            );
          })}
          {archiveMessage ? (
            <span
              className="min-w-0 text-xs text-slate-500"
              role="status"
              aria-live="polite"
            >
              {archiveMessage}
            </span>
          ) : null}
        </div>
      ) : null}
    </section>
  );
}

type OpsRunArchiveAction =
  | "case"
  | "run-record"
  | "processing-record"
  | "experience";

async function handleArchiveAction(
  action: OpsRunArchiveAction,
  opsRun: AiopsTransportOpsRun,
  setPendingAction: (action: OpsRunArchiveAction | null) => void,
  setArchiveMessage: (message: string | null) => void,
) {
  setPendingAction(action);
  setArchiveMessage(null);
  const payload = {
    sessionId: opsRun.sessionId,
    turnId: opsRun.turnId,
    title: opsRun.title,
    summary: opsRun.currentStep,
  };
  try {
    if (action === "case") {
      const result = await archiveOpsRunCase(opsRun.id, payload);
      const caseId = String(result?.case?.id || result?.case?.externalId || "");
      setArchiveMessage(caseId ? `已生成 Case：${caseId}` : "已生成 Case");
    } else if (action === "run-record" || action === "processing-record") {
      const result = await createOpsRunRunRecord(opsRun.id, payload);
      const recordId = String(result?.id || "");
      const messagePrefix =
        action === "processing-record" ? "已生成处理记录" : "已生成 Run Record";
      setArchiveMessage(recordId ? `${messagePrefix}：${recordId}` : messagePrefix);
    } else {
      const result = await createOpsRunExperienceCandidates(opsRun.id, payload);
      const count = Array.isArray(result?.items) ? result.items.length : 0;
      setArchiveMessage(
        count > 0 ? `已生成 ${count} 条经验候选` : "已生成经验候选",
      );
    }
  } catch (error) {
    const message = error instanceof Error ? error.message : "请求失败";
    setArchiveMessage(`生成失败：${message}`);
  } finally {
    setPendingAction(null);
  }
}

function supportedPostRunActions(suggestions?: PostRunSuggestion[]) {
  if (!Array.isArray(suggestions) || suggestions.length === 0) {
    return [];
  }
  const selected: PostRunSuggestion[] = [];
  const indexByAction = new Map<string, number>();
  for (const suggestion of suggestions) {
    if (!suggestion?.type) {
      continue;
    }
    const action = postRunArchiveActionFor(suggestion.type);
    if (!action) {
      continue;
    }
    const key = postRunActionDedupeKey(action);
    const existingIndex = indexByAction.get(key);
    if (existingIndex === undefined) {
      indexByAction.set(key, selected.length);
      selected.push(suggestion);
      continue;
    }
    const existingAction = postRunArchiveActionFor(selected[existingIndex]?.type);
    if (action === "processing-record" && existingAction === "run-record") {
      selected[existingIndex] = suggestion;
    }
  }
  return selected;
}

function postRunArchiveActionFor(
  type: PostRunSuggestionType,
): OpsRunArchiveAction | null {
  switch (type) {
    case "case":
      return "case";
    case "run_record":
      return "run-record";
    case "processing_record":
      return "processing-record";
    case "experience_candidate":
      return "experience";
    default:
      return null;
  }
}

function postRunActionDedupeKey(action: OpsRunArchiveAction) {
  if (action === "run-record" || action === "processing-record") {
    return "saved-record";
  }
  return action;
}

function postRunActionDescription(action: OpsRunArchiveAction) {
  switch (action) {
    case "case":
      return "把本轮对话和结果归档为一个 Case，方便后续跟踪。";
    case "experience":
      return "从本轮处理过程里提炼可复用的经验候选，后续可审核入库。";
    case "processing-record":
      return "保存本轮运维处理记录，包含目标、摘要和关联证据。";
    case "run-record":
    default:
      return "保存本轮 Agent Run 记录，包含目标、摘要和关联证据。";
  }
}

function postRunActionIcon(action: OpsRunArchiveAction) {
  switch (action) {
    case "case":
      return Briefcase;
    case "experience":
      return Lightbulb;
    case "run-record":
    case "processing-record":
    default:
      return FileText;
  }
}

function postRunActionLabel(action: OpsRunArchiveAction) {
  switch (action) {
    case "case":
      return "生成 Case";
    case "experience":
      return "生成经验候选";
    case "processing-record":
      return "生成处理记录";
    case "run-record":
    default:
      return "生成 Run Record";
  }
}

function OpsRunEvidencePill({
  evidenceCount,
  status,
}: {
  evidenceCount?: number;
  status?: string;
}) {
  const count = evidenceCount || 0;
  const label =
    count > 0
      ? `${count} 条证据`
      : isTerminalOpsRunStatus(status)
        ? "未采集证据"
        : "证据采集中";
  return (
    <div className="hidden shrink-0 items-center gap-1 rounded-md border border-slate-200 px-2 py-1 text-xs text-slate-600 sm:flex">
      <Database className="size-3.5" aria-hidden="true" />
      <span>{label}</span>
    </div>
  );
}

function latestAgentStep(agentRun?: AgentRunView): AgentStepView | undefined {
  const steps = agentRun?.steps || [];
  if (!steps.length) {
    return undefined;
  }
  const currentStepId = agentRun?.currentStepId;
  if (currentStepId) {
    const current = steps.find((step) => step.id === currentStepId);
    if (current) {
      return current;
    }
  }
  return steps[steps.length - 1];
}

function formatSource(source?: string) {
  if (source === "chat") {
    return "AI Chat";
  }
  return source || "AI Chat";
}

function formatOpsRunStatus(status?: string) {
  switch (status) {
    case "submitted":
    case "working":
    case "running":
      return {
        label: "处理中",
        className:
          "shrink-0 rounded-md bg-blue-50 px-1.5 py-0.5 text-[11px] font-medium text-blue-700",
      };
    case "blocked":
      return {
        label: "等待确认",
        className:
          "shrink-0 rounded-md bg-amber-50 px-1.5 py-0.5 text-[11px] font-medium text-amber-700",
      };
    case "completed":
      return {
        label: "已结束",
        className:
          "shrink-0 rounded-md bg-emerald-50 px-1.5 py-0.5 text-[11px] font-medium text-emerald-700",
      };
    case "failed":
      return {
        label: "失败",
        className:
          "shrink-0 rounded-md bg-red-50 px-1.5 py-0.5 text-[11px] font-medium text-red-700",
      };
    case "canceled":
    case "cancelled":
      return {
        label: "已停止",
        className:
          "shrink-0 rounded-md bg-slate-100 px-1.5 py-0.5 text-[11px] font-medium text-slate-600",
      };
    default:
      return {
        label: "处理中",
        className:
          "shrink-0 rounded-md bg-blue-50 px-1.5 py-0.5 text-[11px] font-medium text-blue-700",
      };
  }
}

function formatAgentStepStatus(status?: string) {
  switch (status) {
    case "waiting_approval":
      return "等待确认";
    case "running":
      return "执行中";
    case "pending":
      return "排队中";
    case "failed":
      return "失败";
    case "cancelled":
      return "已停止";
    case "skipped":
      return "已跳过";
    case "completed":
    default:
      return "已完成";
  }
}

function formatRouteMode(mode?: string) {
  switch (mode) {
    case "chat_advisory":
      return "咨询";
    case "evidence_rca":
      return "证据分析";
    case "host_bound_ops":
      return "单主机";
    case "multi_host_ops":
      return "多主机";
    default:
      return mode || "";
  }
}

function isTerminalOpsRunStatus(status?: string) {
  return (
    status === "completed" ||
    status === "failed" ||
    status === "canceled" ||
    status === "cancelled" ||
    status === "stopped"
  );
}
