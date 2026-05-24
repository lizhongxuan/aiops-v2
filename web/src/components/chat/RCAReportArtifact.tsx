import type { AiopsTransportAgentUiArtifact } from "@/transport/aiopsTransportTypes";
import { RCAEvidenceList } from "./rca/RCAEvidenceList";
import { RCAHypothesisRank } from "./rca/RCAHypothesisRank";
import { RCAPropagationMap } from "./rca/RCAPropagationMap";
import { RCATimeline } from "./rca/RCATimeline";
import { RCATimeseriesGrid } from "./rca/RCATimeseriesGrid";
import { normalizeRCAReport } from "./rca/rcaReportModel";

const UNSAFE_PROTOCOL_GLOBAL_PATTERN = new RegExp("\\bjava" + "scr" + "ipt:", "gi");

const STATUS_LABELS = {
  ok: "已分析",
  partial: "部分证据",
  inconclusive: "证据不足",
  error: "分析失败",
};

export function RCAReportArtifact({ artifact }: { artifact: AiopsTransportAgentUiArtifact }) {
  const report = normalizeRCAReport(artifact.inlineData);
  const restricted = ["restricted", "denied", "forbidden"].includes(text(artifact.permissionScope).toLowerCase());
  const title = text(artifact.titleZh) || text(artifact.title) || "根因分析";
  const showEvidenceSummary = restricted || report.evidenceRefs.length > 0 || report.rawRefs.length > 0;

  return (
    <div className="mt-3 space-y-4 border-t border-slate-100 pt-3" data-testid="rca-report-artifact">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h4 className="text-sm font-semibold text-slate-950">{title}</h4>
          <p className="mt-1 text-sm leading-6 text-slate-700">{report.conclusion.summaryZh}</p>
        </div>
        <div className="rounded border border-slate-200 bg-slate-50 px-2 py-1 text-xs text-slate-600">
          {STATUS_LABELS[report.status]} · {Math.round(report.conclusion.confidence * 100)}%
        </div>
      </div>

      {restricted ? (
        <div className="rounded border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-900">
          权限受限，仅展示可见摘要。
        </div>
      ) : null}

      {report.hypotheses.length && !restricted ? (
        <section className="space-y-2">
          <h5 className="text-xs font-semibold text-slate-600">假设排序</h5>
          <RCAHypothesisRank hypotheses={report.hypotheses} />
        </section>
      ) : null}

      {!restricted && report.sections.length ? (
        <div className="space-y-4">
          {report.sections.map((section) => (
            <section key={section.id} className="border-t border-slate-100 pt-3">
              <h5 className="text-xs font-semibold text-slate-700">{section.titleZh}</h5>
              {section.summaryZh ? <p className="mt-1 text-xs leading-5 text-slate-500">{section.summaryZh}</p> : null}
              <div className="mt-3">{renderSection(section.kind, section.payload)}</div>
            </section>
          ))}
        </div>
      ) : null}

      {showEvidenceSummary ? (
        <div className="border-t border-slate-100 pt-3">
          <RCAEvidenceList evidenceRefs={report.evidenceRefs} rawRefs={report.rawRefs} restricted={restricted} />
        </div>
      ) : null}
    </div>
  );
}

function renderSection(kind: string, payload: Record<string, unknown>) {
  if (kind === "propagation_map") {
    return <RCAPropagationMap payload={payload} />;
  }
  if (kind === "timeseries_grid") {
    return <RCATimeseriesGrid payload={payload} />;
  }
  if (kind === "event_timeline") {
    return <RCATimeline payload={payload} />;
  }
  return <p className="text-xs text-slate-500">该 RCA 分析段暂不支持专用展示。</p>;
}

function text(value?: unknown) {
  return typeof value === "string"
    ? value
      .replace(/<script\b[^>]*>[\s\S]*?<\/script>/gi, "")
      .replace(/<[^>]*>/g, "")
      .replace(/\bon\w+\s*=\s*(?:"[^"]*"|'[^']*'|[^\s>]+)/gi, "")
      .replace(UNSAFE_PROTOCOL_GLOBAL_PATTERN, "")
      .trim()
      .replace(/\s+/g, " ")
    : "";
}
