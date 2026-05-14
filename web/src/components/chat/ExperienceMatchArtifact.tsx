import type { AiopsTransportAgentUiArtifact } from "@/transport/aiopsTransportTypes";

export function ExperienceMatchArtifact({ artifact }: { artifact: AiopsTransportAgentUiArtifact }) {
  return <ArtifactDetails rows={compactRows([
    { label: "Skill", value: formatDisplayValue(pickArtifactValue(artifact, ["skillName", "skill_name", "skill"])) },
    { label: "兼容状态", value: formatCompatibilityStatus(pickArtifactValue(artifact, ["compatibilityStatus", "compatibility_status"])) },
    { label: "适配差异", value: formatDisplayValue(pickArtifactValue(artifact, ["compatibilityGaps", "compatibility_gaps"])) },
    { label: "命中原因", value: formatDisplayValue(pickArtifactValue(artifact, ["matchReasons", "match_reasons", "reasons", "reason", "matchedSignals", "matched_signals", "signals"])) },
    { label: "命中信号", value: formatDisplayValue(pickArtifactValue(artifact, ["matchedSignals", "matched_signals", "signals"])) },
    { label: "缺失前置条件", value: formatDisplayValue(pickArtifactValue(artifact, ["preconditionGaps", "precondition_gaps"])) },
    { label: "风险", value: formatDisplayValue(pickArtifactValue(artifact, ["riskWarnings", "risk_warnings", "risks", "risk"])) },
    { label: "OS 变体", value: formatDisplayValue(pickArtifactValue(artifact, ["osVariant", "os_variant", "os"])) },
    { label: "Runner Binding", value: formatDisplayValue(pickArtifactValue(artifact, ["runnerBinding", "runner_binding"])) },
    { label: "历史成功/失败", value: formatHistory(pickArtifactValue(artifact, ["history", "historicalEffect", "historical_effect"])) },
    { label: "验证项", value: formatDisplayValue(pickArtifactValue(artifact, ["validationItems", "validation_items", "verificationItems", "verification_items", "checks"])) },
  ])} />;
}

function ArtifactDetails({ rows }: { rows: Array<{ label: string; value: string }> }) {
  if (!rows.length) return null;
  return <dl className="mt-3 grid gap-2 rounded-lg border border-slate-100 bg-slate-50 p-3 text-xs">{rows.map((row) => <div key={row.label} className="grid gap-1 sm:grid-cols-[8rem_1fr] sm:items-start"><dt className="font-medium text-slate-500">{row.label}</dt><dd className="break-words font-mono text-slate-700">{row.value}</dd></div>)}</dl>;
}

function compactRows(rows: Array<{ label: string; value: string }>) { return rows.filter((row) => row.value); }
function pickArtifactValue(artifact: AiopsTransportAgentUiArtifact, keys: string[]): unknown {
  const sources = [artifact as unknown as Record<string, unknown>, asRecord(artifact.payload), asRecord(artifact.inlineData), asRecord(artifact.metadata)];
  for (const source of sources) for (const key of keys) if (source[key] !== undefined && source[key] !== null && source[key] !== "") return source[key];
  return undefined;
}
function formatDisplayValue(value: unknown): string {
  if (value === undefined || value === null || value === "") return "";
  if (typeof value === "string") return text(value);
  if (typeof value === "number" || typeof value === "boolean") return String(value);
  if (Array.isArray(value)) return value.map(formatDisplayValue).filter(Boolean).join("；");
  if (typeof value === "object") {
    const record = value as Record<string, unknown>;
    if (record.name || record.title || record.summary) {
      return [record.name, record.title, record.summary].map(formatDisplayValue).filter(Boolean).join("；");
    }
    return Object.entries(record)
      .filter(([key]) => key !== "actions" && key !== "gene" && key !== "capsules")
      .map(([key, entry]) => `${key}：${formatDisplayValue(entry)}`)
      .filter(Boolean)
      .join("；");
  }
  return String(value);
}
function formatHistory(value: unknown): string {
  const record = asRecord(value);
  if (!Object.keys(record).length) return formatDisplayValue(value);
  const success = record.successCount ?? record.success_count ?? record.successes ?? 0;
  const failure = record.failureCount ?? record.failure_count ?? record.failures ?? 0;
  const recent = record.recentResult ?? record.recent_result ?? record.lastResult ?? record.last_result;
  return `成功 ${formatDisplayValue(success)}；失败 ${formatDisplayValue(failure)}${recent ? `；最近 ${formatDisplayValue(recent)}` : ""}`;
}
function formatCompatibilityStatus(value: unknown): string {
  const status = text(value);
  if (status === "direct") return "可直接推荐，仍需用户确认";
  if (status === "adapt_required") return "需适配后使用";
  if (status === "reference_only") return "仅作参考，不使用原 Runner";
  return status;
}
function asRecord(value: unknown): Record<string, unknown> { return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {}; }
function text(value?: unknown) { return typeof value === "string" ? value.replace(/<[^>]*>/g, "").trim().replace(/\s+/g, " ") : ""; }
