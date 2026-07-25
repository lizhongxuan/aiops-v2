import { useState } from "react";
import { AlertTriangle, CheckCircle2, ChevronRight, GitBranch, ListChecks, ShieldCheck } from "lucide-react";

import type {
  AiopsTransportAgentUiArtifact,
  AiopsTransportArtifactAction,
  WorkflowChangeArtifactType,
} from "@/transport/aiopsTransportTypes";

export const WORKFLOW_CHANGE_ACTION_EVENT = "aiops:workflow-change-action";

export type WorkflowChangeActionEventDetail = {
  artifactId: string;
  action: Required<Pick<AiopsTransportArtifactAction, "id" | "intent" | "label">> & AiopsTransportArtifactAction;
};

type ChangeKind = "plan" | "preview" | "receipt" | "validation" | "conflict";
type ChangeRow = { id: string; objectLabel: string; fieldLabel: string; before: string; after: string };

const PRESENTATION: Record<WorkflowChangeArtifactType, { kind: ChangeKind; title: string; eyebrow: string }> = {
  workflow_edit_plan: { kind: "plan", title: "修改计划", eyebrow: "准备修改" },
  workflow_patch_preview: { kind: "preview", title: "待确认修改", eyebrow: "尚未保存" },
  workflow_patch_result: { kind: "receipt", title: "修改已保存", eyebrow: "保存结果" },
  workflow_patch_validation: { kind: "validation", title: "检查结果", eyebrow: "工作流检查" },
  workflow_conflict: { kind: "conflict", title: "画布已发生变化", eyebrow: "需要处理" },
};

const ACTION_INTENTS = new Set([
  "workflow.focus_object",
  "workflow.review_change",
  "workflow.apply_candidate",
  "workflow.discard_candidate",
  "workflow.retry_from_current",
  "workflow.undo_transaction",
  "workflow.open_configuration",
]);

export function WorkflowChangeArtifact({ artifact }: { artifact: AiopsTransportAgentUiArtifact }) {
  const presentation = PRESENTATION[artifact.type as WorkflowChangeArtifactType];
  if (!presentation) return null;
  const envelope = asRecord(artifact.payload || artifact.inlineData);
  const structuredContent = asRecord(envelope.structuredContent);
  const payload = Object.keys(structuredContent).length ? structuredContent : envelope;
  const object = asRecord(payload.object);
  const previewDetail = asRecord(payload.diff);
  const risk = Object.keys(asRecord(payload.risk)).length ? asRecord(payload.risk) : asRecord(previewDetail.risk);
  const validation = asRecord(payload.validation);
  const changes = changeRows(payload);
  const actions = typedActions(artifact.actions);
  const conflict = presentation.kind === "conflict" ? conflictPresentation(payload) : undefined;
  const [showChanges, setShowChanges] = useState(false);
  const [showTechnical, setShowTechnical] = useState(false);
  const workflowKey = asRecord(payload.workflowKey || asRecord(payload.changeSet).workflowKey);
  const objectLabel = cleanText(object.label || object.name || payload.workflowName || payload.objectLabel || workflowKey.workflowId) || "当前工作流";
  const changeSummary = cleanText(payload.changeSummary || payload.summary || artifact.summaryZh || artifact.summary) || summarizePayloadChanges(payload, previewDetail, changes);
  const riskSummary = cleanText(risk.summary || payload.riskSummary) || riskLabel(risk.level || payload.riskLevel);
  const validationSummary = validationLabel(validation, payload);
  const nextStep = cleanText(payload.nextStep || payload.retryHint) || defaultNextStep(presentation.kind);
  const Icon = iconForKind(presentation.kind);

  return (
    <section className="min-w-0 overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm" data-testid="workflow-change-artifact" data-change-kind={presentation.kind}>
      <header className="flex items-start gap-3 border-b border-slate-100 p-4">
        <span className="mt-0.5 rounded-lg bg-slate-100 p-2 text-slate-700"><Icon className="h-4 w-4" aria-hidden="true" /></span>
        <div className="min-w-0 flex-1">
          <p className="text-xs font-medium text-slate-500">{presentation.eyebrow}</p>
          <h3 className="mt-0.5 text-sm font-semibold text-slate-950">{presentation.title}</h3>
          <p className="mt-1 text-sm leading-6 text-slate-600">{objectLabel}</p>
        </div>
      </header>

      <dl className="grid gap-3 p-4 text-sm sm:grid-cols-2">
        <SummaryRow label="修改" value={changeSummary} />
        <SummaryRow label="风险" value={riskSummary} />
        <SummaryRow label="检查" value={validationSummary} />
        <SummaryRow label="下一步" value={nextStep} />
      </dl>

      {conflict ? <ConflictSummary conflict={conflict} /> : null}

      {changes.length ? (
        <details
          className="border-t border-slate-100 px-4 py-3"
          data-testid="workflow-change-details"
          open={showChanges}
          onToggle={(event) => setShowChanges(event.currentTarget.open)}
        >
          <summary className="flex min-h-11 cursor-pointer select-none items-center text-sm font-medium text-slate-800">查看字段差异（{changes.length}）</summary>
          {showChanges ? <ChangeList rows={changes} /> : null}
        </details>
      ) : null}

      <details
        className="border-t border-slate-100 px-4 py-3"
        data-testid="workflow-change-technical"
        open={showTechnical}
        onToggle={(event) => setShowTechnical(event.currentTarget.open)}
      >
        <summary className="flex min-h-11 cursor-pointer select-none items-center text-xs font-medium text-slate-500">技术详情</summary>
        {showTechnical ? <TechnicalDetails artifact={artifact} payload={payload} /> : null}
      </details>

      {actions.length ? (
        <div className="flex flex-wrap gap-2 border-t border-slate-100 p-4">
          {actions.map((action) => (
            <button
              key={action.id}
              type="button"
              disabled={Boolean(action.disabled)}
              className="inline-flex min-h-11 items-center gap-1.5 rounded-lg border border-slate-300 bg-white px-3 text-sm font-medium text-slate-800 hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-50"
              onClick={() => emitAction(artifact.id, action)}
            >
              {action.label}<ChevronRight className="h-3.5 w-3.5" aria-hidden="true" />
            </button>
          ))}
        </div>
      ) : null}
    </section>
  );
}

type ConflictPresentation = {
  baseLabel: string;
  currentLabel: string;
  paths: string[];
  safePathCount: number;
};

function ConflictSummary({ conflict }: { conflict: ConflictPresentation }) {
  return (
    <section className="border-t border-amber-100 bg-amber-50/70 px-4 py-3" data-testid="workflow-conflict-three-way" aria-label="三方冲突摘要">
      <p className="text-sm font-semibold text-amber-950">我的候选修改尚未保存，也没有覆盖当前画布。</p>
      <dl className="mt-2 grid gap-2 text-xs text-amber-950 sm:grid-cols-2">
        <div><dt className="text-amber-800">候选基于</dt><dd className="mt-0.5 font-medium">{conflict.baseLabel}</dd></div>
        <div><dt className="text-amber-800">当前已保存</dt><dd className="mt-0.5 font-medium">{conflict.currentLabel}</dd></div>
      </dl>
      <div className="mt-3">
        <p className="text-xs font-medium text-amber-900">需要重新确认的字段</p>
        <ul className="mt-1 space-y-1 text-xs text-amber-950">
          {conflict.paths.map((path) => <li key={path}>· {humanizeConflictPath(path)}</li>)}
        </ul>
      </div>
      <p className="mt-2 text-xs leading-5 text-amber-900">
        {conflict.safePathCount > 0
          ? `另有 ${conflict.safePathCount} 项不冲突修改可在重新计算时保留。`
          : "保留我的输入后，请基于当前版本重新计算并再次审阅。"}
      </p>
    </section>
  );
}

function SummaryRow({ label, value }: { label: string; value: string }) {
  return <div className="min-w-0"><dt className="text-xs font-medium text-slate-500">{label}</dt><dd className="mt-1 break-words leading-6 text-slate-800">{value}</dd></div>;
}

function ChangeList({ rows }: { rows: ChangeRow[] }) {
  return (
    <ul className="mt-3 space-y-2">
      {rows.map((row) => (
        <li key={row.id} className="rounded-lg bg-slate-50 p-3 text-xs">
          <p className="font-medium text-slate-800">{row.objectLabel} · {row.fieldLabel}</p>
          <p className="mt-1 break-words text-slate-600"><span className="line-through">{row.before || "未设置"}</span><span className="px-2">→</span><span className="font-medium text-slate-900">{row.after || "已移除"}</span></p>
        </li>
      ))}
    </ul>
  );
}

function TechnicalDetails({ artifact, payload }: { artifact: AiopsTransportAgentUiArtifact; payload: Record<string, unknown> }) {
  const changeSet = asRecord(payload.changeSet);
  const candidate = asRecord(payload.candidate);
  const identifiers = [
    ["Artifact ID", artifact.id],
    ["变更 ID", changeSet.id || payload.changeSetId],
    ["候选 ID", candidate.id || payload.candidateId],
    ["事务 ID", payload.transactionId],
    ["Trace ID", artifact.promptTraceId || payload.traceId],
  ].filter((entry): entry is [string, string] => Boolean(cleanText(entry[1])));
  return (
    <dl className="mt-3 grid gap-2 rounded-lg bg-slate-50 p-3 text-xs">
      <TechnicalRow label="Schema" value={cleanText(artifact.schemaVersion)} />
      <TechnicalRow label="Renderer" value={cleanText(artifact.renderer)} />
      {identifiers.map(([label, value]) => <TechnicalRow key={label} label={label} value={redactIdentifier(value)} />)}
    </dl>
  );
}

function TechnicalRow({ label, value }: { label: string; value: string }) {
  if (!value) return null;
  return <div className="grid gap-1 sm:grid-cols-[100px_minmax(0,1fr)]"><dt className="text-slate-500">{label}</dt><dd className="break-all font-mono text-slate-700">{value}</dd></div>;
}

function changeRows(payload: Record<string, unknown>): ChangeRow[] {
  const values = Array.isArray(payload.changes) ? payload.changes : [];
  const explicit = values.flatMap((value, index) => {
    const row = asRecord(value);
    const object = asRecord(row.object);
    const objectLabel = cleanText(object.label || row.objectLabel || row.label);
    const fieldLabel = cleanText(row.fieldLabel || row.field || row.path);
    if (!objectLabel && !fieldLabel) return [];
    return [{
      id: cleanText(row.id) || `change-${index + 1}`,
      objectLabel: objectLabel || "工作流对象",
      fieldLabel: fieldLabel || "配置",
      before: displayScalar(row.before),
      after: displayScalar(row.after),
    }];
  });
  if (explicit.length) return explicit;
  const changeSet = asRecord(payload.changeSet);
  const operations = Array.isArray(payload.operations) ? payload.operations : Array.isArray(changeSet.operations) ? changeSet.operations : [];
  return operations.flatMap(operationRows);
}

function operationRows(value: unknown, index: number): ChangeRow[] {
  const operation = asRecord(value);
  const op = cleanText(operation.op);
  const node = asRecord(operation.node);
  const edge = asRecord(operation.edge);
  const target = cleanText(operation.nodeId || operation.edgeId || node.id || edge.id || operation.clientRef) || "工作流";
  const labels: Record<string, [string, string, string]> = {
    add_node: ["节点", "未设置", "新增"],
    update_node: ["节点配置", "当前值", "更新"],
    delete_node: ["节点", "已存在", "删除"],
    add_edge: ["连线", "未设置", "新增"],
    update_edge: ["连线配置", "当前值", "更新"],
    delete_edge: ["连线", "已存在", "删除"],
    update_workflow: ["工作流配置", "当前值", "更新"],
    update_layout: ["画布布局", "当前位置", "新位置"],
    attach_manual_binding: ["运维手册绑定", "未绑定", "新增绑定"],
  };
  const label = labels[op];
  if (!label) return [];
  return [{ id: cleanText(operation.id) || `${op}-${index + 1}`, objectLabel: target, fieldLabel: label[0], before: label[1], after: label[2] }];
}

function typedActions(actions: AiopsTransportAgentUiArtifact["actions"]): WorkflowChangeActionEventDetail["action"][] {
  return (actions || []).flatMap((action) => {
    const id = cleanText(action.id);
    const intent = cleanText(action.intent);
    const label = cleanText(action.label);
    if (!id || !intent || !label || !ACTION_INTENTS.has(intent)) return [];
    return [{ ...action, id, intent, label, target: asRecord(action.target) }];
  });
}

function emitAction(artifactId: string, action: WorkflowChangeActionEventDetail["action"]) {
  window.dispatchEvent(new CustomEvent<WorkflowChangeActionEventDetail>(WORKFLOW_CHANGE_ACTION_EVENT, { detail: { artifactId, action } }));
}

function validationLabel(validation: Record<string, unknown>, payload: Record<string, unknown>): string {
  const diagnostics = Array.isArray(validation.diagnostics) ? validation.diagnostics : Array.isArray(payload.diagnostics) ? payload.diagnostics : [];
  if (validation.valid === true || payload.valid === true) return diagnostics.length ? `检查通过，有 ${diagnostics.length} 条提醒` : "检查通过";
  if (validation.valid === false || payload.valid === false) return `发现 ${diagnostics.length || 1} 个待处理问题`;
  return "尚未检查";
}

function defaultNextStep(kind: ChangeKind): string {
  return { plan: "确认计划后生成预览", preview: "审阅后可应用修改", receipt: "查看检查结果", validation: "按问题定位并修复", conflict: "保留当前修改并让 AI 重新计算" }[kind];
}

function conflictPresentation(payload: Record<string, unknown>): ConflictPresentation | undefined {
  const details = asRecord(payload.details);
  const base = asRecord(details.base);
  const current = asRecord(details.current);
  const paths = canonicalConflictPaths(
    Array.isArray(details.conflictingPaths) ? details.conflictingPaths : payload.conflictingPaths,
  );
  const safePaths = canonicalConflictPaths(details.safeRebasePaths);
  const baseVersion = cleanText(base.resourceVersion || payload.baseRevision);
  const currentVersion = cleanText(current.resourceVersion || payload.currentRevision);
  if (!baseVersion || !currentVersion || baseVersion === currentVersion || paths.length === 0) return undefined;
  return {
    baseLabel: shortRevisionLabel(baseVersion),
    currentLabel: shortRevisionLabel(currentVersion),
    paths,
    safePathCount: safePaths.length,
  };
}

function canonicalConflictPaths(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  const seen = new Set<string>();
  return value.flatMap((item) => {
    const path = cleanText(item);
    if (!path.startsWith("/") || path.includes("..") || path.length > 240 || seen.has(path)) return [];
    seen.add(path);
    return [path];
  }).slice(0, 20);
}

function shortRevisionLabel(value: string): string {
  const normalized = cleanText(value);
  return normalized.length <= 8 ? "已识别版本" : `版本 …${normalized.slice(-6)}`;
}

function humanizeConflictPath(value: string): string {
  const segments = value.split("/").filter(Boolean).map(decodePointerSegment);
  if (segments[0] === "nodes" && segments[1]) {
    const field = segments.slice(2).map(humanizeConflictField).filter(Boolean).join(" · ");
    return `节点 ${segments[1]}${field ? ` · ${field}` : ""}`;
  }
  if (segments[0] === "edges" && segments[1]) {
    const field = segments.slice(2).map(humanizeConflictField).filter(Boolean).join(" · ");
    return `连线 ${segments[1]}${field ? ` · ${field}` : ""}`;
  }
  if (segments[0] === "workflow") {
    const field = segments.slice(1).map(humanizeConflictField).filter(Boolean).join(" · ");
    return field ? `工作流 · ${field}` : "工作流配置";
  }
  return "工作流字段";
}

function decodePointerSegment(value: string): string {
  return value.replaceAll("~1", "/").replaceAll("~0", "~").slice(0, 80);
}

function humanizeConflictField(value: string): string {
  const labels: Record<string, string> = {
    label: "名称",
    position: "位置",
    step: "执行配置",
    args: "参数",
    source: "起点",
    target: "终点",
    condition: "条件",
    workflow: "工作流配置",
  };
  return labels[value] || value.replaceAll("_", " ");
}

function summarizeChanges(changes: ChangeRow[]): string {
  return changes.length ? `${changes.length} 项字段修改` : "暂无可展示的修改摘要";
}

function summarizePayloadChanges(payload: Record<string, unknown>, previewDetail: Record<string, unknown>, changes: ChangeRow[]): string {
  if (changes.length) return summarizeChanges(changes);
  const delta = firstDelta(payload, previewDetail);
  const counts = [
    ["新增节点", listLength(delta.addedNodeIds)],
    ["更新节点", listLength(delta.updatedNodeIds)],
    ["删除节点", listLength(delta.deletedNodeIds)],
    ["新增连线", listLength(delta.addedEdgeIds)],
    ["更新连线", listLength(delta.updatedEdgeIds)],
    ["删除连线", listLength(delta.deletedEdgeIds)],
  ].filter((entry) => entry[1] > 0);
  return counts.length ? counts.map(([label, count]) => `${label} ${count}`).join("，") : "暂无可展示的修改摘要";
}

function firstDelta(payload: Record<string, unknown>, previewDetail: Record<string, unknown>): Record<string, unknown> {
  const direct = asRecord(payload.delta);
  if (Object.keys(direct).length) return direct;
  return asRecord(asRecord(asRecord(previewDetail.diff).graph).delta);
}

function listLength(value: unknown): number {
  return Array.isArray(value) ? value.length : 0;
}

function riskLabel(value: unknown): string {
  return ({ low: "低风险", medium: "中风险", high: "高风险" } as Record<string, string>)[cleanText(value).toLowerCase()] || "风险待评估";
}

function redactIdentifier(value: string): string {
  const normalized = cleanText(value);
  return normalized.length <= 8 ? "••••" : `${normalized.slice(0, 4)}…${normalized.slice(-4)}`;
}

function iconForKind(kind: ChangeKind) {
  return { plan: ListChecks, preview: GitBranch, receipt: CheckCircle2, validation: ShieldCheck, conflict: AlertTriangle }[kind];
}

function displayScalar(value: unknown): string {
  if (typeof value === "string" || typeof value === "number" || typeof value === "boolean") return cleanText(String(value));
  if (Array.isArray(value) && value.every((item) => ["string", "number", "boolean"].includes(typeof item))) return value.map(String).join("、");
  return "";
}

function asRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {};
}

function cleanText(value: unknown): string {
  return typeof value === "string" ? value.replace(/<[^>]*>/g, "").replace(/\s+/g, " ").trim() : "";
}
