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
  AiopsTransportOpsRun,
  AiopsTransportState,
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
  const status = formatOpsRunStatus(opsRun.status);
  const title = opsRun.title || "本次处理";
  const canArchive = isTerminalOpsRunStatus(opsRun.status);

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
              {opsRun.currentStep ? (
                <span className="max-w-full truncate">
                  {opsRun.currentStep}
                </span>
              ) : null}
            </div>
          </div>
        </div>
        <OpsRunEvidencePill opsRun={opsRun} />
      </div>
      {opsRun.targetSummary ? (
        <div className="mt-2 flex min-w-0 items-center gap-1.5 rounded-md bg-slate-50 px-2 py-1 text-xs text-slate-600">
          <Route className="size-3.5 shrink-0" aria-hidden="true" />
          <span className="truncate">{opsRun.targetSummary}</span>
        </div>
      ) : null}
      {canArchive ? (
        <div className="mt-2 flex flex-wrap items-center gap-2 border-t border-slate-100 pt-2">
          <Button
            type="button"
            variant="outline"
            size="xs"
            onClick={() =>
              handleArchiveAction(
                "case",
                opsRun,
                setPendingAction,
                setArchiveMessage,
              )
            }
            disabled={pendingAction !== null}
          >
            <Briefcase className="size-3.5" aria-hidden="true" />
            生成 Case
          </Button>
          <Button
            type="button"
            variant="outline"
            size="xs"
            onClick={() =>
              handleArchiveAction(
                "run-record",
                opsRun,
                setPendingAction,
                setArchiveMessage,
              )
            }
            disabled={pendingAction !== null}
          >
            <FileText className="size-3.5" aria-hidden="true" />
            生成 Run Record
          </Button>
          <Button
            type="button"
            variant="outline"
            size="xs"
            onClick={() =>
              handleArchiveAction(
                "experience",
                opsRun,
                setPendingAction,
                setArchiveMessage,
              )
            }
            disabled={pendingAction !== null}
          >
            <Lightbulb className="size-3.5" aria-hidden="true" />
            生成经验候选
          </Button>
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

type OpsRunArchiveAction = "case" | "run-record" | "experience";

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
    } else if (action === "run-record") {
      const result = await createOpsRunRunRecord(opsRun.id, payload);
      const recordId = String(result?.id || "");
      setArchiveMessage(
        recordId ? `已生成 Run Record：${recordId}` : "已生成 Run Record",
      );
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

function OpsRunEvidencePill({ opsRun }: { opsRun: AiopsTransportOpsRun }) {
  const count = opsRun.evidenceCount || 0;
  return (
    <div className="hidden shrink-0 items-center gap-1 rounded-md border border-slate-200 px-2 py-1 text-xs text-slate-600 sm:flex">
      <Database className="size-3.5" aria-hidden="true" />
      <span>{count > 0 ? `${count} 条证据` : "证据采集中"}</span>
    </div>
  );
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

function isTerminalOpsRunStatus(status?: string) {
  return status === "completed" || status === "failed" || status === "canceled";
}
