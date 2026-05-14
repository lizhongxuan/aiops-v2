import { Activity, AlertTriangle, CheckCircle2, GitBranch, LineChart, ListChecks, ShieldCheck } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import type { AiopsTransportAgentUiArtifact } from "@/transport/aiopsTransportTypes";
import { CorootChartArtifact } from "./CorootChartArtifact";
import { ExperienceMatchArtifact } from "./ExperienceMatchArtifact";
import { RunnerWorkflowCandidateArtifact } from "./RunnerWorkflowCandidateArtifact";
import { TopologySliceArtifact } from "./TopologySliceArtifact";
import { TraceSummaryArtifact } from "./TraceSummaryArtifact";
import { VerificationResultArtifact } from "./VerificationResultArtifact";
import { WorkflowResultArtifact } from "./WorkflowResultArtifact";

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
  coroot_chart: "Coroot 图表",
  trace_summary: "Trace 摘要",
  topology_slice: "拓扑片段",
  workflow_result: "Workflow 结果",
  verification_result: "验证结果",
  experience_match: "经验命中",
  experience_pack_candidate: "经验候选",
  runner_workflow_candidate: "Runner 草稿",
};

const REDACTION_LABELS: Record<string, string> = {
  redacted: "已脱敏",
  none: "未脱敏",
  restricted: "权限受限",
};

export function AgentUiArtifactPart({ artifact }: AgentUiArtifactPartProps) {
  const typeLabel = ARTIFACT_LABELS[artifact.type] || "暂不支持的卡片类型";
  const title = text(artifact.titleZh) || text(artifact.title) || typeLabel;
  const summary = text(artifact.summaryZh) || text(artifact.summary) || (ARTIFACT_LABELS[artifact.type] ? "暂无摘要" : "该卡片类型未注册，已按安全模式展示。");
  const Icon = iconForArtifact(artifact.type);
  const actions = unifiedActionsForArtifact(artifact);
  const content = artifactContent(artifact);

  return (
    <section className="rounded-lg border border-slate-200 bg-white p-3 text-sm shadow-sm" data-testid="agent-ui-artifact">
      <div className="flex items-start gap-2">
        <span className="mt-0.5 rounded-md bg-slate-100 p-1.5 text-slate-600">
          <Icon className="h-4 w-4" />
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <h3 className="min-w-0 font-medium text-slate-950">{title}</h3>
            <Badge variant="outline">{typeLabel}</Badge>
            {artifact.redactionStatus ? <Badge variant="secondary">{REDACTION_LABELS[artifact.redactionStatus] || artifact.redactionStatus}</Badge> : null}
          </div>
          <p className="mt-1 leading-6 text-slate-600">{summary}</p>
        </div>
      </div>

      {content || <InlineDataPreview artifact={artifact} />}

      <div className="mt-3 flex flex-wrap items-center gap-2 border-t border-slate-100 pt-3 text-xs text-slate-500">
        {artifact.source ? <span>来源：{artifact.source}</span> : null}
        {artifact.createdAt ? <span>生成时间：{formatDate(artifact.createdAt)}</span> : null}
        {actions.map((action) => (
          <a key={action.id} className="font-medium text-slate-900 underline-offset-4 hover:underline" href={action.href} aria-disabled={action.disabled || undefined}>
            {action.label}
          </a>
        ))}
      </div>
    </section>
  );
}

function artifactContent(artifact: AiopsTransportAgentUiArtifact) {
  switch (artifact.type) {
    case "coroot_chart":
      return <CorootChartArtifact artifact={artifact} />;
    case "trace_summary":
      return <TraceSummaryArtifact artifact={artifact} />;
    case "topology_slice":
      return <TopologySliceArtifact artifact={artifact} />;
    case "workflow_result":
      return <WorkflowResultArtifact artifact={artifact} />;
    case "verification_result":
      return <VerificationResultArtifact artifact={artifact} />;
    case "experience_match":
      return <ExperienceMatchArtifact artifact={artifact} />;
    case "experience_pack_candidate":
      return (
        <div data-testid="experience-pack-candidate-artifact">
          <InlineDataPreview artifact={artifact} />
        </div>
      );
    case "runner_workflow_candidate":
      return <RunnerWorkflowCandidateArtifact artifact={artifact} />;
    default:
      return null;
  }
}

function InlineDataPreview({ artifact }: { artifact: AiopsTransportAgentUiArtifact }) {
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

  if (artifact.type === "runner_workflow_candidate") {
    return actions;
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
      return GitBranch;
    case "workflow_result":
      return ListChecks;
    case "verification_result":
      return CheckCircle2;
    case "experience_match":
      return ShieldCheck;
    case "experience_pack_candidate":
      return ListChecks;
    case "runner_workflow_candidate":
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
