import { Settings2 } from "lucide-react";
import { useMemo } from "react";

import { WorkflowRunInspector, type WorkflowRunSessionRecord } from "./WorkflowRunInspector";
import { WorkflowStateBanner, type WorkflowStateBannerTone } from "./WorkflowStateBanner";
import { WorkflowTargetRow } from "./WorkflowTargetRow";
import {
  selectWorkflowPrimaryActionViewModel,
  selectWorkflowRunView,
  type WorkflowRunGateFacts,
  type WorkflowRunGateState,
} from "./workspace/workflowWorkspaceSelectors";
import type { WorkflowPrimaryActionTarget } from "./workspace/workflowInteractionContract";
import type { WorkflowWorkspaceState } from "./workspace/workflowWorkspaceTypes";

export type WorkflowRunPanelProps = {
  state: WorkflowWorkspaceState;
  facts?: WorkflowRunGateFacts;
  onRunPublished?: (
    target: Extract<WorkflowPrimaryActionTarget, { kind: "published_revision" }>,
  ) => void;
  sessionRuns?: WorkflowRunSessionRecord[];
  onRetrySessionRun?: (runId: string) => void;
};

export function WorkflowRunPanel({
  state,
  facts,
  onRunPublished,
  sessionRuns = [],
  onRetrySessionRun,
}: WorkflowRunPanelProps) {
  const view = useMemo(() => selectWorkflowRunView(state, facts), [facts, state]);
  const primaryAction = useMemo(
    () => selectWorkflowPrimaryActionViewModel(state, facts),
    [facts, state],
  );
  const status = RUN_GATE_COPY[view.gateState];
  const statusDescription = view.gateState === "validated" && !view.draftDryRun.enabled && !view.environment
    ? "静态检查已通过；当前没有可信运行环境，因此暂不能试运行。"
    : status.description;
  const lifecycleClosed = view.gateState === "configuration_required" || view.gateState === "saved_with_issues";
  const committedRevision = state.committed?.revision;

  return (
    <section
      aria-labelledby="workflow-run-panel-title"
      className="flex min-h-0 flex-col bg-white"
      data-testid="workflow-run-panel"
      data-gate-state={view.gateState}
      data-publish-allowed={String(view.canPublish)}
    >
      <header className="px-3 py-3">
        <h2 id="workflow-run-panel-title" className="text-sm font-semibold text-slate-950">运行</h2>
        <p className="mt-0.5 text-xs leading-5 text-slate-500">确认目标后，从顶部主操作执行下一步。</p>
      </header>

      <WorkflowStateBanner
        title={status.title}
        description={`${statusDescription}${lifecycleClosed ? " 发布与运行已关闭。" : ""}`}
        tone={status.tone}
      />

      {view.gateState === "configuration_required" ? (
        <div className="px-3 pt-3">
          <ConfigurationRequirements
            secretRefs={view.configuration.secretRefs}
            environments={view.configuration.environments}
          />
        </div>
      ) : null}

      <div className="mt-3 border-t border-slate-200" aria-label="运行目标">
        <WorkflowTargetRow
          kind="draft"
          title="当前草稿"
          badge="非生产"
          environment={view.environment || "环境未绑定"}
          availability={view.draftDryRun.enabled ? "可试运行" : view.draftDryRun.reason || "运行依据未就绪"}
          technicalFacts={committedRevision ? [
            { label: "resourceVersion", value: committedRevision.resourceVersion },
            { label: "semanticHash", value: committedRevision.semanticHash },
          ] : []}
        />
        {view.publishedRun.target ? (
          <WorkflowTargetRow
            kind="published"
            title="已发布版本"
            environment={view.environment}
            availability={view.publishedRun.enabled ? "可运行" : view.publishedRun.reason || "当前不可运行"}
            enabled={Boolean(
              view.publishedRun.enabled &&
              primaryAction?.id !== "run_published" &&
              onRunPublished,
            )}
            actionLabel="运行已发布版本"
            onAction={view.publishedRun.target && onRunPublished
              ? () => onRunPublished({
                  kind: "published_revision",
                  workflowKey: { ...view.publishedRun.target!.workflowKey },
                  resourceVersion: view.publishedRun.target!.resourceVersion,
                  semanticHash: view.publishedRun.target!.semanticHash,
                })
              : undefined}
            technicalFacts={[
              { label: "resourceVersion", value: view.publishedRun.target.resourceVersion },
              { label: "semanticHash", value: view.publishedRun.target.semanticHash },
            ]}
          />
        ) : (
          <div className="workflow-target-row px-3 py-3" data-testid="workflow-target-published-empty">
            <h3 className="text-sm font-semibold text-slate-950">已发布版本</h3>
            <p className="mt-1 text-xs text-slate-500">尚无已发布版本。发布后可从顶部主操作运行。</p>
          </div>
        )}
      </div>
      <WorkflowRunInspector records={sessionRuns} onRetry={onRetrySessionRun} />
    </section>
  );
}

function ConfigurationRequirements({
  secretRefs,
  environments,
}: {
  secretRefs: string[];
  environments: string[];
}) {
  return (
    <section aria-label="缺失配置" className="rounded-xl border border-amber-200 bg-amber-50 p-3 text-amber-950">
      <div className="flex items-center gap-2">
        <Settings2 className="h-4 w-4" aria-hidden="true" />
        <h3 className="text-sm font-semibold">缺失配置</h3>
      </div>
      {secretRefs.length ? <IdentifierList label="凭据引用" values={secretRefs} /> : null}
      {environments.length ? <IdentifierList label="环境" values={environments} /> : null}
      {!secretRefs.length && !environments.length ? <p className="mt-2 text-xs">配置详情尚未同步，请刷新后再试。</p> : null}
    </section>
  );
}

function IdentifierList({ label, values }: { label: string; values: string[] }) {
  return (
    <div className="mt-2 text-xs">
      <p className="font-medium">{label}</p>
      <ul className="mt-1 space-y-1">
        {values.map((value) => <li key={value} className="break-all font-mono">{value}</li>)}
      </ul>
    </div>
  );
}

const RUN_GATE_COPY: Record<WorkflowRunGateState, { title: string; description: string; tone: WorkflowStateBannerTone }> = {
  unavailable: { title: "运行依据尚未同步", description: "等待服务端同步当前版本、环境和检查依据。", tone: "neutral" },
  unchecked: { title: "待检查", description: "先检查当前草稿，再执行非生产试运行。", tone: "warning" },
  checking: { title: "正在检查", description: "检查完成前不能发布或运行。", tone: "info" },
  validated: { title: "静态检查已通过", description: "可以在绑定环境中试运行当前草稿。", tone: "info" },
  current: { title: "检查和试运行已通过", description: "结果与当前版本、Catalog、引用和环境绑定一致。", tone: "success" },
  stale: { title: "检查结果已过期", description: "版本或运行依据已变化，请重新检查和试运行。", tone: "warning" },
  configuration_required: { title: "还需配置", description: "工作流已保存；补齐配置后重新检查。", tone: "warning" },
  saved_with_issues: { title: "已保存但有问题", description: "工作流已保存，但环境检查或试运行存在阻断问题。", tone: "danger" },
};
