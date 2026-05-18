import { AlertTriangle } from "lucide-react";

import type { AiopsTransportAgentUiArtifact } from "@/transport/aiopsTransportTypes";

type UnsupportedArtifactCardProps = {
  artifact: AiopsTransportAgentUiArtifact;
  reason?: string;
};

export function UnsupportedArtifactCard({ artifact, reason = "未注册的卡片类型。" }: UnsupportedArtifactCardProps) {
  return (
    <section className="rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm text-amber-950" data-testid="unsupported-agent-ui-artifact">
      <div className="flex items-start gap-2">
        <span className="mt-0.5 rounded-md bg-amber-100 p-1.5 text-amber-700">
          <AlertTriangle className="h-4 w-4" />
        </span>
        <div className="min-w-0 flex-1">
          <h3 className="font-medium">无法渲染 Agent UI 卡片</h3>
          <p className="mt-1 text-xs leading-5 text-amber-800">暂不支持的卡片类型。{reason}</p>
        </div>
      </div>
      <TerminalArtifactMeta artifact={artifact} />
    </section>
  );
}

export function TerminalArtifactMeta({ artifact }: { artifact: AiopsTransportAgentUiArtifact }) {
  const actions = traceLinksForArtifact(artifact);
  return (
    <div className="mt-3 flex flex-wrap items-center gap-2 border-t border-amber-200 pt-3 text-xs text-amber-900">
      <span>类型：{artifact.type || "unknown"}</span>
      {artifact.source ? <span>来源：{artifact.source}</span> : null}
      {actions.map((action) => (
        <a key={action.href} className="font-medium underline-offset-4 hover:underline" href={action.href}>
          {action.label}
        </a>
      ))}
    </div>
  );
}

function traceLinksForArtifact(artifact: AiopsTransportAgentUiArtifact): Array<{ label: string; href: string }> {
  const caseId = text(pickArtifactValue(artifact, ["caseId", "case_id"]));
  const evidenceRef = text(pickArtifactValue(artifact, ["evidenceRef", "evidence_ref", "evidenceId", "evidence_id", "dataRef", "data_ref"]));
  const promptTraceId = text(pickArtifactValue(artifact, ["promptTraceId", "prompt_trace_id", "promptTraceRef", "prompt_trace_ref"]));
  const links: Array<{ label: string; href: string }> = [];

  if (caseId) {
    links.push({ label: "查看 Case", href: `/incidents/${encodeURIComponent(caseId)}` });
  }
  if (evidenceRef) {
    links.push({
      label: "查看证据",
      href: caseId ? `/incidents/${encodeURIComponent(caseId)}?evidence=${encodeURIComponent(evidenceRef)}` : `#evidence-${encodeURIComponent(evidenceRef)}`,
    });
  }
  if (promptTraceId) {
    links.push({ label: "查看 Prompt Trace", href: `/debug/prompts?trace_id=${encodeURIComponent(promptTraceId)}` });
  }

  return links;
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

function asRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {};
}

function text(value?: unknown) {
  return typeof value === "string" ? value.trim().replace(/\s+/g, " ") : "";
}
