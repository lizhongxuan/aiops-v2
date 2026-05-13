import type { AiopsTransportAgentUiArtifact } from "@/transport/aiopsTransportTypes";

type McpCard = {
  title?: string;
  summary?: string;
  error?: string;
  errors?: Array<{ message?: string; detail?: string }>;
  empty?: boolean;
  visual?: {
    kind?: string;
    series?: Array<{ name?: string; data?: Array<{ value?: number | string }> }>;
    rows?: Array<{ cells?: Array<string | number> }>;
  };
};

type CorootChartArtifactProps = {
  artifact: AiopsTransportAgentUiArtifact;
};

export function CorootChartArtifact({ artifact }: CorootChartArtifactProps) {
  const card = (artifact.mcpCard as McpCard | undefined) || readMcpCard(artifact.inlineData) || readMcpCard(artifact.payload);
  const notices = noticesForCoroot(artifact, card);

  return (
    <>
      {notices.length ? (
        <div className="mt-3 grid gap-2">
          {notices.map((notice) => (
            <div key={notice} className="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-900">
              {notice}
            </div>
          ))}
        </div>
      ) : null}
      {card ? <McpCardPreview card={card} /> : null}
    </>
  );
}

function McpCardPreview({ card }: { card: McpCard }) {
  const visual = card.visual || {};
  if (visual.kind === "timeseries" && Array.isArray(visual.series)) {
    const series = visual.series;
    const hasMetricValues = series.some((item) => (item.data || []).some((point) => Number.isFinite(Number(point.value))));
    return (
      <div className="mt-3 rounded-lg border border-slate-100 bg-slate-50 p-3">
        <div className="text-xs font-medium text-slate-500">{card.title || "指标趋势"}</div>
        <div className="mt-2 grid gap-2">
          {hasMetricValues && !card.empty ? (
            series.map((item, index) => <TimeseriesPreview key={`${item.name || "series"}-${index}`} series={item} />)
          ) : (
            <div className="text-sm text-slate-500">当前时间范围内暂无可用指标数据</div>
          )}
        </div>
      </div>
    );
  }

  if (visual.kind === "status_table" && Array.isArray(visual.rows)) {
    return (
      <div className="mt-3 overflow-x-auto rounded-lg border border-slate-100 bg-slate-50 p-3">
        <div className="text-xs font-medium text-slate-500">{card.title || "状态表"}</div>
        <table className="mt-2 w-full min-w-80 text-left text-xs">
          <tbody>
            {visual.rows.length ? (
              visual.rows.map((row, index) => (
                <tr key={index} className="border-t border-slate-200">
                  {(row.cells || []).map((cell, cellIndex) => (
                    <td key={cellIndex} className="py-1.5 pr-3">
                      {cell}
                    </td>
                  ))}
                </tr>
              ))
            ) : (
              <tr className="border-t border-slate-200">
                <td className="py-1.5 pr-3 text-slate-500">当前时间范围内暂无可用指标数据</td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    );
  }

  return text(card.summary) ? <p className="mt-3 rounded-lg bg-slate-50 p-3 text-sm text-slate-600">{text(card.summary)}</p> : null;
}

function TimeseriesPreview({ series }: { series: { name?: string; data?: Array<{ value?: number | string }> } }) {
  const values = (series.data || []).map((point) => Number(point.value)).filter(Number.isFinite);
  const latest = values.length ? values[values.length - 1] : null;
  const peak = values.length ? Math.max(...values) : null;
  return (
    <div className="flex flex-wrap items-center justify-between gap-2 rounded-md bg-white px-3 py-2">
      <span className="font-mono text-xs text-slate-700">{series.name || "metric"}</span>
      <span className="text-xs text-slate-500">最新：{latest ?? "-"} · 峰值：{peak ?? "-"}</span>
    </div>
  );
}

function noticesForCoroot(artifact: AiopsTransportAgentUiArtifact, card?: McpCard | null): string[] {
  const notices: string[] = [];
  const status = text(artifact.status).toLowerCase();
  const permissionScope = text(artifact.permissionScope).toLowerCase();
  const redactionStatus = text(artifact.redactionStatus).toLowerCase();
  const cardError = text(card?.error) || text(card?.errors?.[0]?.message) || text(card?.errors?.[0]?.detail);

  if (["blocked", "denied", "forbidden", "permission_denied"].includes(status) || ["restricted", "denied", "forbidden"].includes(permissionScope)) {
    notices.push("权限不足，无法查看完整 Coroot 指标。");
  }
  if (["redacted", "restricted"].includes(redactionStatus)) {
    notices.push("部分字段已脱敏，仅展示可见摘要。");
  }
  if (["error", "failed", "unavailable"].includes(status) || cardError) {
    notices.push(cardError ? `Coroot 暂不可用：${cardError}` : "Coroot 暂不可用。");
  }

  return notices;
}

function readMcpCard(value: unknown): McpCard | null {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return null;
  }
  const source = value as Record<string, unknown>;
  const card = source.mcpCard || source.mcpUiCard || source.card;
  return card && typeof card === "object" && !Array.isArray(card) ? (card as McpCard) : null;
}

function text(value?: unknown) {
  return typeof value === "string" ? value.replace(/<[^>]*>/g, "").trim().replace(/\s+/g, " ") : "";
}
