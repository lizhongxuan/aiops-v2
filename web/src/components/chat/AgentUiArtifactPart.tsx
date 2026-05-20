import { Activity, AlertTriangle, CheckCircle2, GitBranch, LineChart, ListChecks, ShieldCheck } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { defaultAgentUiCardRegistry, lookupAgentUiCardRenderer } from "@/lib/agentUiCardRegistry";
import type { AiopsTransportAgentUiArtifact } from "@/transport/aiopsTransportTypes";
import { InvalidArtifactCard } from "./InvalidArtifactCard";
import { UnsupportedArtifactCard } from "./UnsupportedArtifactCard";

type AgentUiArtifactPartProps = {
  artifact: AiopsTransportAgentUiArtifact;
};

type ArtifactAction = {
  id: string;
  label: string;
  href: string;
  disabled?: boolean;
};

const ARTIFACT_LABELS: Record<string, string> = {
  coroot_chart: "服务",
  trace_summary: "Trace 摘要",
  topology_slice: "拓扑片段",
  rca_report: "根因分析",
  workflow_result: "Workflow 结果",
  verification_result: "验证结果",
  experience_match: "经验命中",
  ops_manual_match: "运维手册判定",
  ops_manual_search_result: "运维手册检索",
  ops_manual_param_resolution: "运维手册参数解析",
  ops_manual_param_form: "运维手册参数表单",
  ops_manual_preflight_result: "运维手册预检",
  ops_manual_fallback_guide: "运维手册降级步骤",
  runner_workflow_generation: "Workflow 生成进度",
};

const REDACTION_LABELS: Record<string, string> = {
  redacted: "已脱敏",
  none: "未脱敏",
  restricted: "权限受限",
};

const SELF_RENDERED_TYPES = new Set([
  "ops_manual_match",
  "ops_manual_search_result",
  "ops_manual_param_resolution",
  "ops_manual_param_form",
  "ops_manual_preflight_result",
]);

export function AgentUiArtifactPart({ artifact }: AgentUiArtifactPartProps) {
  const rendererLookup = lookupAgentUiCardRenderer(defaultAgentUiCardRegistry, artifact);
  if (rendererLookup.state === "invalid_payload") {
    return <InvalidArtifactCard artifact={artifact} reason={rendererLookup.reason} />;
  }
  if (!("Renderer" in rendererLookup)) {
    return <UnsupportedArtifactCard artifact={artifact} reason={rendererLookup.reason} />;
  }

  const typeLabel = ARTIFACT_LABELS[artifact.type] || "暂不支持的卡片类型";
  const title = artifactTitle(artifact, typeLabel);
  const summary = text(artifact.summaryZh) || text(artifact.summary) || (ARTIFACT_LABELS[artifact.type] ? "暂无摘要" : "该卡片类型未注册，已按安全模式展示。");
  const Icon = iconForArtifact(artifact.type);
  const actions = unifiedActionsForArtifact(artifact);
  const Renderer = rendererLookup.Renderer;
  const isCorootChart = artifact.type === "coroot_chart";
  const showSummary = !isCorootChart && Boolean(summary);
  const showFooter = !isCorootChart && Boolean(artifact.source || artifact.createdAt || actions.length);
  const showTypeBadge = !isCorootChart;
  const redactionLabel = redactionStatusLabel(artifact.redactionStatus);

  if (SELF_RENDERED_TYPES.has(artifact.type)) {
    return <Renderer artifact={artifact} />;
  }

  return (
    <section className={`min-w-0 overflow-hidden rounded-lg border border-slate-200 bg-white shadow-sm ${isCorootChart ? "p-2 text-xs" : "p-3 text-sm"}`} data-testid="agent-ui-artifact">
      <div className={`flex items-start ${isCorootChart ? "gap-1.5" : "gap-2"}`}>
        <span className={`mt-0.5 rounded-md bg-slate-100 text-slate-600 ${isCorootChart ? "p-1" : "p-1.5"}`}>
          <Icon className={isCorootChart ? "h-3.5 w-3.5" : "h-4 w-4"} />
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <h3 className="min-w-0 font-medium text-slate-950">{title}</h3>
            {showTypeBadge ? <Badge variant="outline">{typeLabel}</Badge> : null}
            {redactionLabel ? <Badge variant="secondary">{redactionLabel}</Badge> : null}
          </div>
          {showSummary ? <p className="mt-1 leading-6 text-slate-600">{summary}</p> : null}
        </div>
      </div>

      <Renderer artifact={artifact} />
      <InlineDataPreview artifact={artifact} />

      {showFooter ? (
        <div className="mt-3 flex flex-wrap items-center gap-2 border-t border-slate-100 pt-3 text-xs text-slate-500">
          {artifact.source ? <span>来源：{artifact.source}</span> : null}
          {artifact.createdAt ? <span>生成时间：{formatDate(artifact.createdAt)}</span> : null}
          {actions.map((action) => (
            <a key={action.id} className="font-medium text-slate-900 underline-offset-4 hover:underline" href={action.href} aria-disabled={action.disabled || undefined}>
              {action.label}
            </a>
          ))}
        </div>
      ) : null}
    </section>
  );
}

function artifactTitle(artifact: AiopsTransportAgentUiArtifact, fallback: string) {
  const value = text(artifact.titleZh) || text(artifact.title) || fallback;
  if (artifact.type !== "coroot_chart") {
    return value;
  }
  return value
    .replace(/\s*Coroot\s*(?:charts|图表)\s*$/i, " 服务")
    .replace(/\s*图表\s*$/i, " 服务")
    .replace(/\s+/g, " ")
    .trim() || fallback;
}

function redactionStatusLabel(value: unknown) {
  const status = text(value).toLowerCase();
  if (!status || status === "none") {
    return "";
  }
  return REDACTION_LABELS[status] || text(value);
}

function InlineDataPreview({ artifact }: { artifact: AiopsTransportAgentUiArtifact }) {
  if (artifact.type === "rca_report" || artifact.type === "coroot_chart") {
    return null;
  }
  const data = artifact.inlineData;
  if (!data || typeof data !== "object") {
    return null;
  }
  if (["restricted", "denied", "forbidden"].includes(text(artifact.permissionScope).toLowerCase())) {
    return (
      <div className="mt-3 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-900">
        权限受限，仅展示可见摘要。
      </div>
    );
  }
  const entries = Object.entries(data as Record<string, unknown>).filter(([key]) => !["html", "script", "dangerouslySetInnerHTML", "innerHTML"].includes(key));
  if (!entries.length) {
    return null;
  }
  return (
    <dl className="mt-3 grid gap-2 rounded-lg border border-slate-100 bg-slate-50 p-3 text-xs">
      {entries.slice(0, 6).map(([key, value]) => (
        <div key={key} className="flex flex-wrap justify-between gap-2">
          <dt className="font-medium text-slate-500">{key}</dt>
          <dd className="font-mono text-slate-700">{formatDisplayValue(value)}</dd>
        </div>
      ))}
    </dl>
  );
}

function unifiedActionsForArtifact(artifact: AiopsTransportAgentUiArtifact): ArtifactAction[] {
  const caseId = text(pickArtifactValue(artifact, ["caseId", "case_id"]));
  const evidenceRef = text(pickArtifactValue(artifact, ["evidenceRef", "evidence_ref", "evidenceId", "evidence_id", "dataRef", "data_ref"]));
  const promptTraceId = text(pickArtifactValue(artifact, ["promptTraceId", "prompt_trace_id", "promptTraceRef", "prompt_trace_ref"]));
  const actions: ArtifactAction[] = [];

  if (caseId) {
    actions.push({
      id: "view-case",
      label: "查看 Case",
      href: `/incidents/${encodeURIComponent(caseId)}`,
    });
  }
  if (evidenceRef) {
    actions.push({
      id: "view-evidence",
      label: "查看证据",
      href: caseId ? `/incidents/${encodeURIComponent(caseId)}?evidence=${encodeURIComponent(evidenceRef)}` : `#evidence-${encodeURIComponent(evidenceRef)}`,
    });
  }
  if (promptTraceId) {
    actions.push({
      id: "view-prompt-trace",
      label: "查看 Prompt Trace",
      href: `/debug/prompts?trace_id=${encodeURIComponent(promptTraceId)}`,
    });
  }

  for (const action of artifact.actions || []) {
    const record = asRecord(action);
    const label = text(record.label) || text(record.title);
    const href = text(record.href) || hrefFromAction(record);
    const id = text(record.id) || `${label}-${href}`;
    if (!label || !href || actions.some((item) => item.id === id || item.href === href || item.label === label)) {
      continue;
    }
    actions.push({ id, label, href, disabled: Boolean(record.disabled) });
  }

  return actions;
}

function hrefFromAction(action: Record<string, unknown>): string {
  const target = asRecord(action.target);
  const kind = text(target.kind);
  const id = text(target.id);
  if (kind === "case" && id) return `/incidents/${encodeURIComponent(id)}`;
  if (kind === "evidence" && id) {
    const caseId = text(target.caseId);
    return caseId ? `/incidents/${encodeURIComponent(caseId)}?evidence=${encodeURIComponent(id)}` : `#evidence-${encodeURIComponent(id)}`;
  }
  if (kind === "prompt_trace" && id) return `/debug/prompts?trace_id=${encodeURIComponent(id)}`;
  return "";
}

function pickArtifactValue(artifact: AiopsTransportAgentUiArtifact, keys: string[]): unknown {
  const sources = [
    artifact as unknown as Record<string, unknown>,
    asRecord(artifact.payload),
    asRecord(artifact.inlineData),
    asRecord(artifact.metadata),
  ];
  for (const source of sources) {
    for (const key of keys) {
      if (source[key] !== undefined && source[key] !== null && source[key] !== "") {
        return source[key];
      }
    }
  }
  return undefined;
}

function formatDisplayValue(value: unknown): string {
  if (value === undefined || value === null || value === "") {
    return "";
  }
  if (typeof value === "string") {
    return text(value);
  }
  if (typeof value === "number" || typeof value === "boolean") {
    return String(value);
  }
  if (Array.isArray(value)) {
    return value.map(formatDisplayValue).filter(Boolean).join("；");
  }
  if (typeof value === "object") {
    const entries = Object.entries(value as Record<string, unknown>)
      .map(([key, entry]) => {
        const formatted = formatDisplayValue(entry);
        return formatted ? `${key}：${formatted}` : "";
      })
      .filter(Boolean);
    return entries.join("；");
  }
  return String(value);
}

function asRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {};
}

function iconForArtifact(type: string) {
  switch (type) {
    case "coroot_chart":
      return LineChart;
    case "trace_summary":
      return Activity;
    case "topology_slice":
    case "rca_report":
      return GitBranch;
    case "workflow_result":
      return ListChecks;
    case "verification_result":
      return CheckCircle2;
    case "experience_match":
    case "ops_manual_match":
    case "ops_manual_search_result":
    case "ops_manual_preflight_result":
    case "ops_manual_fallback_guide":
      return ShieldCheck;
    case "runner_workflow_generation":
      return GitBranch;
    default:
      return AlertTriangle;
  }
}

function text(value?: unknown) {
  if (typeof value !== "string") {
    return "";
  }
  return value
    .replace(/<[^>]*>/g, "")
    .replace(/\bon\w+\s*=\s*(?:"[^"]*"|'[^']*'|[^\s>]+)/gi, "")
    .replace(/\bjavascript:/gi, "")
    .trim()
    .replace(/\s+/g, " ");
}

function formatDate(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}
