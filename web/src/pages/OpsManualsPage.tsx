import { AlertTriangle, CheckCircle2, Eye, FileText, History, Search, ShieldCheck, Trash2, Undo2, Workflow } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useLocation } from "react-router-dom";

import { opsManualsApi, type OpsManualCandidateView, type OpsManualView, type RunRecordView } from "@/api/opsManuals";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { EmptyState, SettingsPageFrame, ToneBadge } from "@/pages/settingsComponents";
import { compactText } from "@/pages/settingsCatalogViewModels";

type TabKey = "verified" | "review" | "records";

const TAB_LABELS: Array<[TabKey, string]> = [
  ["verified", "已验证手册"],
  ["review", "待审核手册"],
  ["records", "执行记录"],
];

export function OpsManualsPage() {
  const location = useLocation();
  const [activeTab, setActiveTab] = useState<TabKey>("verified");
  const [query, setQuery] = useState("");
  const [manuals, setManuals] = useState<OpsManualView[]>([]);
  const [candidates, setCandidates] = useState<OpsManualCandidateView[]>([]);
  const [records, setRecords] = useState<RunRecordView[]>([]);
  const [error, setError] = useState("");
  const [selectedManual, setSelectedManual] = useState<OpsManualView | null>(null);
  const [previewWorkflow, setPreviewWorkflow] = useState<WorkflowPreview | null>(null);
  const migratedFromExperiencePacks = location.pathname.includes("experience-packs");

  useEffect(() => {
    async function load() {
      setError("");
      try {
        const [manualList, candidateList, recordList] = await Promise.all([
          opsManualsApi.list({ status: "verified", limit: 100 }),
          opsManualsApi.listCandidates({ review_status: "pending", limit: 100 }),
          opsManualsApi.listAllRunRecords({ limit: 100 }),
        ]);
        setManuals(manualList.items);
        setCandidates(candidateList.items);
        setRecords(recordList.items);
      } catch (cause) {
        setError((cause as Error).message || "运维手册加载失败。");
      }
    }
    void load();
  }, []);

  const filteredManuals = useMemo(() => {
    const keyword = compactText(query).toLowerCase();
    if (!keyword) return manuals;
    return manuals.filter((manual) =>
      [
        manual.title,
        manual.operation.targetType,
        manual.operation.action,
        manual.workflowRef.workflowId,
        manual.applicability.middleware,
        ...manual.applicability.os,
        ...manual.applicability.platform,
        ...manual.applicability.executionSurface,
      ]
        .filter(Boolean)
        .join(" ")
        .toLowerCase()
        .includes(keyword),
    );
  }, [manuals, query]);

  return (
    <SettingsPageFrame
      title="运维手册"
      description="运维手册用于说明工作流解决什么问题、适用于什么环境、参数如何填写、如何验证以及什么时候不能使用。"
    >
      {migratedFromExperiencePacks ? (
        <div className="rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm text-amber-900">
          旧入口已迁移到运维手册。新入口为 /settings/ops-manuals，历史语义只保留兼容跳转。
        </div>
      ) : null}
      {error ? <div className="rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm text-amber-900">{error}</div> : null}

      <Card className="rounded-lg bg-white">
        <CardContent className="flex flex-col gap-3 pt-0 lg:flex-row lg:items-center">
          <label className="relative min-w-0 flex-1">
            <Search className="pointer-events-none absolute left-2.5 top-2 h-4 w-4 text-slate-400" />
            <Input className="pl-8" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索手册、目标对象、操作、Workflow、环境标签" />
          </label>
          <div className="flex flex-wrap gap-2">
            <Badge variant="secondary">Workflow Library</Badge>
            <Badge variant="outline">人工审核</Badge>
            <Badge variant="outline">Dry Run</Badge>
            <Badge variant="outline">Run Record</Badge>
          </div>
        </CardContent>
      </Card>

      <Card className="rounded-lg bg-white" data-testid="ops-manual-workbench">
        <CardHeader>
          <CardTitle>运维手册库</CardTitle>
          <CardDescription>卡片仅展示执行决策需要的字段，详情通过弹窗查看。</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="grid gap-4">
            <div className="flex flex-wrap gap-2" role="tablist" aria-label="运维手册视图">
              {TAB_LABELS.map(([key, label]) => (
                <button
                  key={key}
                  type="button"
                  role="tab"
                  aria-selected={activeTab === key}
                  className={`rounded-lg border px-3 py-2 text-sm ${activeTab === key ? "bg-slate-900 text-white" : "bg-white text-slate-700"}`}
                  onClick={() => setActiveTab(key)}
                >
                  {label}
                </button>
              ))}
            </div>

            {activeTab === "verified" ? (
              <section className="grid gap-2">
                {filteredManuals.length ? (
                  filteredManuals.map((manual) => <OpsManualCard key={manual.id} manual={manual} onOpen={() => setSelectedManual(manual)} />)
                ) : (
                  <EmptyState title="暂无已验证手册" description="通过审核并绑定 Runner Workflow 后会出现在这里。" />
                )}
              </section>
            ) : null}

            {activeTab === "review" ? <CandidateReviewList candidates={candidates} onPreview={setPreviewWorkflow} /> : null}

            {activeTab === "records" ? <RunRecordList records={records} manuals={manuals} /> : null}
          </div>
        </CardContent>
      </Card>

      <ManualDetailDialog manual={selectedManual} records={records.filter((record) => record.manualId === selectedManual?.id)} onOpenChange={(open) => !open && setSelectedManual(null)} />
      <WorkflowPreviewDialog preview={previewWorkflow} onOpenChange={(open) => !open && setPreviewWorkflow(null)} />
    </SettingsPageFrame>
  );
}

type WorkflowPreview = {
  workflowId: string;
  title: string;
  manualTitle: string;
  validationReport: string[];
};

function OpsManualCard({ manual, onOpen }: { manual: OpsManualView; onOpen: () => void }) {
  const environment = [
    manual.applicability.middleware,
    ...manual.applicability.os,
    ...manual.applicability.platform,
    ...manual.applicability.executionSurface,
  ].filter(Boolean);
  const recent = manual.runRecordSummary.recentResult || "暂无执行";

  return (
    <button
      type="button"
      data-testid={`ops-manual-card-${manual.id}`}
      className="rounded-lg border border-slate-200 bg-white p-0 text-left transition hover:border-slate-300 hover:shadow-sm"
      onClick={onOpen}
    >
      <div className="grid gap-3 p-3">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0">
            <div className="font-medium text-slate-950">{manual.title}</div>
            <div className="mt-1 text-xs text-slate-500">
              {manual.operation.targetType || "-"} / {manual.operation.action || "-"}
            </div>
          </div>
          <ToneBadge tone={manual.status === "verified" ? "success" : "warning"}>{manual.status === "verified" ? "已验证" : manual.status}</ToneBadge>
        </div>
        <div className="flex flex-wrap gap-2 text-xs text-slate-600">
          <ToneBadge>Workflow {manual.workflowRef.workflowId || "未绑定"}</ToneBadge>
          {environment.slice(0, 5).map((item) => (
            <ToneBadge key={item}>{item}</ToneBadge>
          ))}
          <ToneBadge>最近执行 {statusLabel(recent)}</ToneBadge>
        </div>
      </div>
    </button>
  );
}

function CandidateReviewList({ candidates, onPreview }: { candidates: OpsManualCandidateView[]; onPreview: (preview: WorkflowPreview) => void }) {
  if (!candidates.length) {
    return <EmptyState title="暂无待审核手册" description="AI 生成的手册候选会先进入这里，审核和验证通过后才能发布。" />;
  }
  return (
    <section className="grid gap-2">
      {candidates.map((candidate) => {
        const manual = candidate.proposedManual;
        return (
          <Card key={candidate.id} size="sm" className="rounded-lg bg-white">
            <CardContent className="grid gap-3 pt-0">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div>
                  <div className="font-medium text-slate-950">{manual.title}</div>
                  <div className="mt-1 text-xs text-slate-500">
                    {manual.operation.targetType || "-"} / {manual.operation.action || "-"} · Workflow {manual.workflowRef.workflowId || "未绑定"}
                  </div>
                </div>
                <ToneBadge tone="warning">{candidate.reviewStatus || "pending"}</ToneBadge>
              </div>
              {candidate.validationReport.length ? <p className="text-xs leading-5 text-slate-500">{candidate.validationReport.join("；")}</p> : null}
              <div className="flex flex-wrap gap-2">
                <Button type="button" size="sm" variant="outline" className="h-8 rounded-md">
                  <CheckCircle2 className="h-3.5 w-3.5" />
                  通过
                </Button>
                <Button type="button" size="sm" variant="outline" className="h-8 rounded-md">
                  <Undo2 className="h-3.5 w-3.5" />
                  退回修改
                </Button>
                <Button type="button" size="sm" variant="outline" className="h-8 rounded-md">
                  <Trash2 className="h-3.5 w-3.5" />
                  删除候选
                </Button>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  className="h-8 rounded-md"
                  onClick={() =>
                    onPreview({
                      workflowId: manual.workflowRef.workflowId,
                      title: manual.workflowRef.workflowId ? `${manual.workflowRef.workflowId} 只读草稿` : "未绑定 Workflow",
                      manualTitle: manual.title,
                      validationReport: candidate.validationReport,
                    })
                  }
                >
                  <Eye className="h-3.5 w-3.5" />
                  只读预览
                </Button>
              </div>
            </CardContent>
          </Card>
        );
      })}
    </section>
  );
}

function WorkflowPreviewDialog({ preview, onOpenChange }: { preview: WorkflowPreview | null; onOpenChange: (open: boolean) => void }) {
  return (
    <Dialog open={Boolean(preview)} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[86vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>绑定 Workflow 只读预览</DialogTitle>
          <DialogDescription>候选手册引用的 Runner Workflow 只读预览；审核通过前不能在这里编辑、发布或执行。</DialogDescription>
        </DialogHeader>
        {preview ? (
          <div className="grid gap-3 text-sm">
            <section className="rounded-lg border border-slate-200 bg-slate-50 p-3">
              <div className="text-xs font-medium text-slate-500">候选手册</div>
              <div className="mt-1 font-medium text-slate-950">{preview.manualTitle}</div>
            </section>
            <section className="rounded-lg border border-slate-200 bg-white p-3">
              <div className="text-xs font-medium text-slate-500">绑定 Workflow</div>
              <div className="mt-1 font-mono text-slate-900">{preview.workflowId || "未绑定"}</div>
              <p className="mt-2 text-xs leading-5 text-slate-500">只读模式用于确认候选手册引用的工作流身份，避免审核时误改脚本或绕过 Dry Run。</p>
            </section>
            <section className="rounded-lg border border-slate-200 bg-slate-50 p-3">
              <div className="text-xs font-medium text-slate-500">审核提示</div>
              {preview.validationReport.length ? (
                <ul className="mt-2 list-disc space-y-1 pl-5 leading-6 text-slate-700">
                  {preview.validationReport.map((item) => (
                    <li key={item}>{item}</li>
                  ))}
                </ul>
              ) : (
                <p className="mt-2 leading-6 text-slate-700">暂无额外提示。</p>
              )}
            </section>
          </div>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}

function RunRecordList({ records, manuals }: { records: RunRecordView[]; manuals: OpsManualView[] }) {
  if (!records.length) {
    return <EmptyState title="暂无执行记录" description="Runner Workflow 通过运维手册触发后，会记录 Dry Run、执行、验证和失败原因。" />;
  }
  const manualTitleById = new Map(manuals.map((manual) => [manual.id, manual.title]));
  const summary = records.reduce(
    (acc, record) => {
      if (["success", "succeeded", "passed"].includes(record.executionStatus)) acc.success += 1;
      if (["failed", "error"].includes(record.executionStatus) || ["failed", "error"].includes(record.validationStatus)) acc.failure += 1;
      return acc;
    },
    { success: 0, failure: 0 },
  );

  return (
    <section className="grid gap-3">
      <div className="flex flex-wrap gap-2 text-sm">
        <ToneBadge tone="success">成功 {summary.success}</ToneBadge>
        <ToneBadge tone={summary.failure ? "danger" : "default"}>失败 {summary.failure}</ToneBadge>
      </div>
      <div className="grid gap-2">
        {records.map((record) => (
          <Card key={record.id} size="sm" className="rounded-lg bg-white">
            <CardContent className="grid gap-2 pt-0 text-sm">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div className="font-medium text-slate-950">{record.id}</div>
                <ToneBadge tone={record.executionStatus === "success" ? "success" : record.executionStatus === "failed" ? "danger" : "default"}>
                  {statusLabel(record.executionStatus || record.validationStatus)}
                </ToneBadge>
              </div>
              <div className="grid gap-1 text-xs text-slate-600 sm:grid-cols-2">
                <span>手册：{manualTitleById.get(record.manualId) || record.manualId || "-"}</span>
                <span>Workflow：{record.workflowId || "-"}</span>
                <span>Dry Run：{statusLabel(record.dryRunStatus) || "-"}</span>
                <span>验证：{statusLabel(record.validationStatus) || "-"}</span>
                <span>操作人：{record.operator || "-"}</span>
                <span>完成：{record.completedAt || "-"}</span>
              </div>
              {record.failureReason ? <p className="text-xs text-red-700">失败原因：{record.failureReason}</p> : null}
            </CardContent>
          </Card>
        ))}
      </div>
    </section>
  );
}

function ManualDetailDialog({ manual, records, onOpenChange }: { manual: OpsManualView | null; records: RunRecordView[]; onOpenChange: (open: boolean) => void }) {
  return (
    <Dialog open={Boolean(manual)} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[86vh] overflow-y-auto sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>{manual?.title || "运维手册详情"}</DialogTitle>
          <DialogDescription>详情以人工审核和执行准入为中心，不展示内部检索评分。</DialogDescription>
        </DialogHeader>
        {manual ? (
          <div className="grid gap-4 text-sm">
            <DetailSection icon={FileText} title="使用说明" content={manual.documentMarkdown || manual.title} />
            <DetailSection
              icon={ShieldCheck}
              title="适用环境"
              content={[
                manual.applicability.middleware,
                ...manual.applicability.middlewareVersions,
                ...manual.applicability.os,
                ...manual.applicability.platform,
                ...manual.applicability.executionSurface,
                ...manual.applicability.topology,
              ]
                .filter(Boolean)
                .join("；") || "-"}
            />
            <DetailSection icon={Workflow} title="参数说明" content={[...manual.requiredContext.requiredInputs, ...Object.keys(manual.parameterRules)].filter(Boolean).join("；") || "-"} />
            <DetailList icon={CheckCircle2} title="前置检查" items={manual.preconditions} />
            <DetailList icon={CheckCircle2} title="验证方式" items={manual.validation} />
            <DetailList icon={AlertTriangle} title="不能使用条件" items={manual.cannotUseWhen} />
            <DetailSection icon={Workflow} title="绑定 Workflow" content={`${manual.workflowRef.workflowId || "-"} ${manual.workflowRef.workflowVersion ? `(${manual.workflowRef.workflowVersion})` : ""}`} />
            <DetailSection icon={History} title="执行记录" content={records.length ? records.map((record) => `${record.id}: ${statusLabel(record.executionStatus || record.validationStatus)}`).join("；") : "暂无执行记录"} />
          </div>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}

function DetailSection({ icon: Icon, title, content }: { icon: typeof FileText; title: string; content: string }) {
  return (
    <section className="rounded-lg border border-slate-200 bg-slate-50 p-3">
      <div className="flex items-center gap-2 text-xs font-medium text-slate-500">
        <Icon className="h-4 w-4" />
        {title}
      </div>
      <p className="mt-2 leading-6 text-slate-700">{content}</p>
    </section>
  );
}

function DetailList({ icon: Icon, title, items }: { icon: typeof FileText; title: string; items: string[] }) {
  return (
    <section className="rounded-lg border border-slate-200 bg-slate-50 p-3">
      <div className="flex items-center gap-2 text-xs font-medium text-slate-500">
        <Icon className="h-4 w-4" />
        {title}
      </div>
      {items.length ? (
        <ul className="mt-2 list-disc space-y-1 pl-5 leading-6 text-slate-700">
          {items.map((item) => (
            <li key={item}>{item}</li>
          ))}
        </ul>
      ) : (
        <p className="mt-2 leading-6 text-slate-700">-</p>
      )}
    </section>
  );
}

function statusLabel(value = "") {
  const labels: Record<string, string> = {
    success: "成功",
    succeeded: "成功",
    passed: "通过",
    failed: "失败",
    error: "异常",
    pending: "待处理",
    running: "执行中",
    skipped: "跳过",
  };
  return labels[value] || value;
}
