import { Component, type ErrorInfo, type ReactNode } from "react";
import { Activity, AlertTriangle, CheckCircle2, GitBranch, LineChart, ListChecks, ShieldCheck } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { defaultAgentUiCardRegistry, lookupAgentUiCardRenderer } from "@/lib/agentUiCardRegistry";
import type { AgentUiCardDisplay } from "@/lib/agentUiCardDefinitions";
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

const REDACTION_LABELS: Record<string, string> = {
  redacted: "已脱敏",
  none: "未脱敏",
  restricted: "权限受限",
};

export function AgentUiArtifactPart({ artifact }: AgentUiArtifactPartProps) {
  const rendererLookup = lookupAgentUiCardRenderer(defaultAgentUiCardRegistry, artifact);
  if (rendererLookup.state === "invalid_payload") {
    return <InvalidArtifactCard artifact={artifact} reason={rendererLookup.reason} />;
  }
  if (!("Renderer" in rendererLookup)) {
    return <UnsupportedArtifactCard artifact={artifact} reason={rendererLookup.reason} />;
  }

  const display = rendererLookup.definition?.display || {};
  const typeLabel = display.label || rendererLookup.definition?.label || "暂不支持的卡片类型";
  const title = artifactTitle(artifact, typeLabel, display);
  const summary = text(artifact.summaryZh) || text(artifact.summary) || (rendererLookup.state === "fallback_renderer" ? rendererLookup.reason : "") || (rendererLookup.definition ? "暂无摘要" : "该卡片类型未注册，已按安全模式展示。");
  const Icon = iconForName(display.icon);
  const actions = unifiedActionsForArtifact(artifact);
  const Renderer = rendererLookup.Renderer;
  const compact = display.density === "compact";
  const showSummary = !display.hideSummary && Boolean(summary);
  const showFooter = !display.hideFooter && Boolean(artifact.source || artifact.createdAt || actions.length);
  const showTypeBadge = !display.hideTypeBadge;
  const redactionLabel = redactionStatusLabel(artifact.redactionStatus);

  if (display.selfRendered) {
    return (
      <AgentUiRendererBoundary artifact={artifact}>
        <Renderer artifact={artifact} />
      </AgentUiRendererBoundary>
    );
  }

  return (
    <section className={`min-w-0 overflow-hidden rounded-lg border border-slate-200 bg-white shadow-sm ${compact ? "p-2 text-xs" : "p-3 text-sm"}`} data-testid="agent-ui-artifact">
      <div className={`flex items-start ${compact ? "gap-1.5" : "gap-2"}`}>
        <span className={`mt-0.5 rounded-md bg-slate-100 text-slate-600 ${compact ? "p-1" : "p-1.5"}`}>
          <Icon className={compact ? "h-3.5 w-3.5" : "h-4 w-4"} />
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

      <AgentUiRendererBoundary artifact={artifact}>
        <Renderer artifact={artifact} />
      </AgentUiRendererBoundary>
      <InlineDataPreview artifact={artifact} suppress={Boolean(display.suppressInlineData)} />

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

type AgentUiRendererBoundaryProps = {
  artifact: AiopsTransportAgentUiArtifact;
  children: ReactNode;
};

type AgentUiRendererBoundaryState = {
  error?: Error;
};

export class AgentUiRendererBoundary extends Component<AgentUiRendererBoundaryProps, AgentUiRendererBoundaryState> {
  state: AgentUiRendererBoundaryState = {};

  static getDerivedStateFromError(error: Error): AgentUiRendererBoundaryState {
    return { error };
  }

  componentDidCatch(_error: Error, _info: ErrorInfo) {
    // React still records the component stack in development; the UI remains local to this card.
  }

  render() {
    if (this.state.error) {
      return (
        <div className="mt-3 rounded-md border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-800" data-testid="agent-ui-renderer-error">
          Renderer 渲染失败，已保留卡片外壳。
        </div>
      );
    }
    return this.props.children;
  }
}

function artifactTitle(artifact: AiopsTransportAgentUiArtifact, fallback: string, display: AgentUiCardDisplay = {}) {
  const value = text(artifact.titleZh) || text(artifact.title) || fallback;
  if (display.titleTransform !== "strip_chart_suffix_to_subject") {
    return value;
  }
  const subjectLabel = text(display.subjectLabel) || fallback;
  const providerLabel = text(display.providerLabel);
  const providerSuffix = providerLabel ? new RegExp(`\\s*${escapeRegExp(providerLabel)}\\s*(?:charts|图表)\\s*$`, "i") : null;
  const withoutProviderSuffix = providerSuffix ? value.replace(providerSuffix, ` ${subjectLabel}`) : value;
  return withoutProviderSuffix
    .replace(/\s*(?:charts|图表)\s*$/i, ` ${subjectLabel}`)
    .replace(new RegExp(`(${escapeRegExp(subjectLabel)})\\s+\\1$`), subjectLabel)
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

function InlineDataPreview({ artifact, suppress }: { artifact: AiopsTransportAgentUiArtifact; suppress?: boolean }) {
  if (suppress) {
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

function iconForName(name?: string) {
  const icons = {
    activity: Activity,
    alert: AlertTriangle,
    "check-circle": CheckCircle2,
    "git-branch": GitBranch,
    "line-chart": LineChart,
    "list-checks": ListChecks,
    "shield-check": ShieldCheck,
  } as const;
  return icons[text(name) as keyof typeof icons] || AlertTriangle;
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

function escapeRegExp(value: string) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function formatDate(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}
