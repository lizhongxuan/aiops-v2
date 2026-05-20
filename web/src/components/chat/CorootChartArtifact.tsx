import { useMemo, useState, type MouseEvent } from "react";

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
    reports?: unknown;
  };
};

type CorootChartReport = {
  name?: string;
  status?: string;
  widgets?: CorootWidget[];
};

type CorootWidget = {
  chart?: CorootChart;
  chart_group?: CorootChartGroup;
  chartGroup?: CorootChartGroup;
};

type CorootChartGroup = {
  title?: string;
  charts?: CorootChart[];
};

type CorootChart = {
  title?: string;
  ctx?: {
    from?: number | string;
    step?: number | string;
    to?: number | string;
  };
  series?: CorootSeries[];
  threshold?: CorootSeries;
  stacked?: boolean;
  sorted?: boolean;
  column?: boolean;
  yzoom?: boolean;
  hide_legend?: boolean;
};

type CorootSeries = {
  name?: string;
  color?: string;
  data?: unknown[];
  value?: string;
};

type ChartLine = {
  name: string;
  color: string;
  points: Array<{ x: number; y: number }>;
  latest: number | null;
  peak: number | null;
  threshold?: boolean;
};

type ChartHover = {
  x: number;
  y: number;
  dataX: number;
  entries: Array<{ name: string; color: string; value: number; y: number }>;
};

type CorootChartArtifactProps = {
  artifact: AiopsTransportAgentUiArtifact;
};

type VisibleCorootReport = CorootChartReport & {
  key: string;
  widgets: CorootWidget[];
};

export function CorootChartArtifact({ artifact }: CorootChartArtifactProps) {
  const card = (artifact.mcpCard as McpCard | undefined) || readMcpCard(artifact.inlineData) || readMcpCard(artifact.payload);
  const notices = noticesForCoroot(artifact, card);
  const reports = readCorootChartReports(artifact, card);
  const defaultReportName = readDefaultCorootReportName(artifact);

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
      {reports.length ? <CorootReportCharts reports={reports} defaultReportName={defaultReportName} /> : card ? <McpCardPreview card={card} defaultReportName={defaultReportName} /> : null}
    </>
  );
}

function McpCardPreview({ card, defaultReportName }: { card: McpCard; defaultReportName?: string }) {
  const visual = card.visual || {};
  if (visual.kind === "coroot_report_charts") {
    const reports = normalizeCorootReports(visual.reports);
    return reports.length ? <CorootReportCharts reports={reports} defaultReportName={defaultReportName} /> : null;
  }

  if (visual.kind === "timeseries" && Array.isArray(visual.series)) {
    const series = visual.series;
    const hasMetricValues = series.some((item) => (item.data || []).some((point) => Number.isFinite(Number(point.value))));
    return (
      <div className="mt-3 rounded-lg border border-slate-100 bg-slate-50 p-3">
        <div className="text-xs font-medium text-slate-500">{displayCorootChartTitle(card.title, "指标趋势")}</div>
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
        <div className="text-xs font-medium text-slate-500">{displayCorootChartTitle(card.title, "状态表")}</div>
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

function CorootReportCharts({ reports, defaultReportName }: { reports: CorootChartReport[]; defaultReportName?: string }) {
  const visibleReports = useMemo(() => reports
    .map((report, index) => ({
      ...report,
      key: reportKey(report, index),
      widgets: visibleWidgetsForReport(report).filter((widget) => widget.chart || widget.chart_group || widget.chartGroup),
    }))
    .filter((report) => (report.widgets || []).length > 0), [reports]);
  const defaultKey = preferredReportKey(visibleReports, defaultReportName);
  const [selectedKey, setSelectedKey] = useState(defaultKey);
  const selectedReport = visibleReports.find((report) => report.key === selectedKey) || visibleReports.find((report) => report.key === defaultKey) || visibleReports[0];

  if (!visibleReports.length) {
    return <div className="mt-3 rounded-lg border border-slate-100 bg-slate-50 p-3 text-sm text-slate-500">当前时间范围内暂无可用指标数据</div>;
  }

  return (
    <div className="mt-2 min-w-0 space-y-2" data-testid="coroot-native-charts">
      {visibleReports.length > 1 ? (
        <div className="flex gap-1 overflow-x-auto rounded-lg border border-slate-100 bg-slate-50 p-0.5" role="tablist" aria-label="Coroot 图表分组">
          {visibleReports.map((report) => {
            const selected = report.key === selectedReport.key;
            return (
              <button
                key={report.key}
                type="button"
                role="tab"
                aria-selected={selected}
                className={`whitespace-nowrap rounded-md px-2.5 py-1 text-xs font-medium transition ${selected ? "bg-white text-slate-950 shadow-sm" : "text-slate-500 hover:bg-white/70 hover:text-slate-800"}`}
                onClick={() => setSelectedKey(report.key)}
              >
                {text(report.name) || "Coroot report"}
              </button>
            );
          })}
        </div>
      ) : null}
      <CorootReportPanel key={selectedReport.key} report={selectedReport} />
    </div>
  );
}

function CorootReportPanel({ report }: { report: VisibleCorootReport }) {
  const widgetTabs = useMemo(() => report.widgets.map((widget, index) => ({
    key: widgetKey(widget, index),
    label: widgetLabel(widget, index),
    widget,
  })), [report.widgets]);
  const [selectedWidgetKey, setSelectedWidgetKey] = useState(widgetTabs[0]?.key || "");
  const selectedWidget = widgetTabs.find((widget) => widget.key === selectedWidgetKey) || widgetTabs[0];

  return (
    <section className="min-w-0 rounded-lg border border-slate-100 bg-slate-50 p-2">
      {widgetTabs.length > 1 ? (
        <div className="flex gap-1 overflow-x-auto rounded-lg border border-slate-100 bg-white p-0.5" role="tablist" aria-label="Coroot 图表" data-testid="coroot-widget-tabs">
          {widgetTabs.map((widget) => {
            const selected = widget.key === selectedWidget.key;
            return (
              <button
                key={widget.key}
                type="button"
                role="tab"
                aria-selected={selected}
                className={`max-w-56 shrink-0 truncate rounded-md px-2.5 py-1 text-xs font-medium transition ${selected ? "bg-slate-900 text-white shadow-sm" : "text-slate-500 hover:bg-slate-50 hover:text-slate-800"}`}
                onClick={() => setSelectedWidgetKey(widget.key)}
                title={widget.label}
                data-testid="coroot-widget-tab"
              >
                {widget.label}
              </button>
            );
          })}
        </div>
      ) : null}
      <div className="mt-2 min-w-0">
        {selectedWidget ? <CorootWidgetChart key={selectedWidget.key} widget={selectedWidget.widget} /> : null}
      </div>
    </section>
  );
}

function visibleWidgetsForReport(report: CorootChartReport) {
  const widgets = report.widgets || [];
  if (!isMemoryReport(report)) {
    return widgets;
  }
  return widgets
    .map((widget) => filterMemoryWidget(widget))
    .filter(Boolean) as CorootWidget[];
}

function isMemoryReport(report: CorootChartReport) {
  return text(report.name).toLowerCase() === "memory";
}

function filterMemoryWidget(widget: CorootWidget): CorootWidget | null {
  const group = widget.chart_group || widget.chartGroup;
  const label = `${text(widget.chart?.title)} ${text(group?.title)}`.toLowerCase();
  if (!label.includes("memory usage") || label.includes("node memory usage")) {
    return null;
  }
  if (!group) {
    return widget;
  }
  return {
    chart_group: {
      ...group,
      charts: preferredMemoryCharts(group.title, group.charts || []),
    },
  };
}

function preferredMemoryCharts(groupTitle: unknown, charts: CorootChart[]) {
  const title = text(groupTitle).toLowerCase();
  if (title.includes("memory usage")) {
    const preferred = charts.find((chart) => /rss\s*\+\s*page\s*cache/i.test(text(chart.title)));
    return preferred ? [preferred] : charts;
  }
  if (title.includes("memory stall time")) {
    const preferred = charts.find((chart) => text(chart.title).toLowerCase() === "full");
    return preferred ? [preferred] : charts;
  }
  return charts;
}

function CorootWidgetChart({ widget }: { widget: CorootWidget }) {
  if (widget.chart) {
    return <NativeChartPanel chart={widget.chart} title={text(widget.chart.title) || "Coroot chart"} />;
  }
  const group = widget.chart_group || widget.chartGroup;
  return group ? <CorootChartGroupPanel group={group} /> : null;
}

function CorootChartGroupPanel({ group }: { group: CorootChartGroup }) {
  const charts = (group.charts || []).filter(chartHasRenderableSeries);
  const initial = Math.max(0, charts.findIndex((chart) => Boolean((chart as Record<string, unknown>).featured)));
  const [selectedIndex, setSelectedIndex] = useState(initial);
  const selectedChart = charts[selectedIndex] || charts[initial] || charts[0];
  const title = chartGroupTitle(group.title, selectedChart?.title);

  if (!selectedChart) {
    return <div className="rounded-md bg-white px-3 py-2 text-sm text-slate-500">当前时间范围内暂无可用指标数据</div>;
  }

  return (
    <div className="min-w-0 overflow-hidden rounded-md bg-white p-2">
      {charts.length > 1 ? (
        <div className="mb-2 min-w-0 space-y-1.5">
          <span className="block min-w-0 break-words text-xs font-medium leading-5 text-slate-500">{title}</span>
          <div className="flex gap-1 overflow-x-auto rounded-md border border-slate-100 bg-slate-50 p-0.5" role="tablist" aria-label="Coroot 指标选择" data-testid="coroot-chart-tabs">
            {charts.map((chart, index) => {
              const selected = chart === selectedChart;
              const label = cleanCorootTabLabel(chart.title) || `序列 ${index + 1}`;
              return (
                <button
                  key={`${label}-${index}`}
                  type="button"
                  role="tab"
                  aria-selected={selected}
                  className={`max-w-52 shrink-0 truncate rounded px-2 py-0.5 text-xs transition ${selected ? "bg-white text-slate-900 shadow-sm" : "text-slate-500 hover:bg-white/70 hover:text-slate-800"}`}
                  onClick={() => setSelectedIndex(index)}
                  title={text(chart.title)}
                >
                  {label}
                </button>
              );
            })}
          </div>
        </div>
      ) : null}
      <NativeChartPanel chart={selectedChart} title={title} framed={false} />
    </div>
  );
}

function NativeChartPanel({ chart, title, framed = true }: { chart: CorootChart; title: string; framed?: boolean }) {
  const lines = useMemo(() => chartLines(chart), [chart]);
  const unit = unitFromChartTitle(title);
  const body = (
    <div className="space-y-2">
      <div className="break-words text-center text-xs font-medium leading-5 text-slate-600">{title}</div>
      {lines.length ? <CorootSvgChart chart={chart} lines={lines} unit={unit} /> : <div className="text-sm text-slate-500">当前时间范围内暂无可用指标数据</div>}
      {lines.length && !chart.hide_legend ? (
        <div className="flex max-h-16 min-w-0 flex-wrap gap-x-3 gap-y-1 overflow-auto text-[11px] text-slate-600">
          {lines.slice(0, 12).map((line) => (
            <span key={line.name} className="flex min-w-0 max-w-full items-center gap-1">
              <span className="h-3 w-1.5 shrink-0 rounded-sm" style={{ backgroundColor: line.color }} />
              <span className="truncate" title={line.name}>{line.name}</span>
            </span>
          ))}
        </div>
      ) : null}
    </div>
  );
  return framed ? <div className="min-w-0 overflow-hidden rounded-md bg-white p-3">{body}</div> : body;
}

function CorootSvgChart({ chart, lines, unit }: { chart: CorootChart; lines: ChartLine[]; unit: string }) {
  const width = 760;
  const height = 288;
  const pad = { left: 70, right: 22, top: 14, bottom: 44 };
  const [hover, setHover] = useState<ChartHover | null>(null);
  const allPoints = lines.flatMap((line) => line.points);
  const xs = allPoints.map((point) => point.x);
  const ys = allPoints.map((point) => point.y);
  const xMin = Math.min(...xs);
  const xMax = Math.max(...xs);
  const rawYMin = Math.min(...ys);
  const rawYMax = Math.max(...ys);
  const yDomain = chartYDomain(rawYMin, rawYMax, { yzoom: chart.yzoom, column: chart.column });
  const yMin = yDomain.min;
  const yMax = yDomain.max;
  const innerW = width - pad.left - pad.right;
  const innerH = height - pad.top - pad.bottom;
  const xScale = (value: number) => pad.left + ((value - xMin) / Math.max(1, xMax - xMin)) * innerW;
  const yRange = yMax - yMin;
  const yScale = (value: number) => pad.top + (1 - (value - yMin) / (yRange > 0 ? yRange : 1)) * innerH;
  const yTicks = chartYTicks(yMin, yMax, chart.yzoom);
  const xTicks = chartXTicks(xMin, xMax);
  const tooltipEntries = hover?.entries.slice(0, 3) || [];
  const tooltipWidth = hover
    ? clamp(Math.max(360, Math.max(...tooltipEntries.map((entry) => tooltipSeriesName(entry.name).length), 0) * 7 + 128), 360, width - pad.left - pad.right)
    : 360;
  const tooltipHeight = Math.min(120, 36 + tooltipEntries.length * 22);
  const tooltipX = hover
    ? clamp(hover.x + 12, pad.left, width - pad.right - tooltipWidth)
    : 0;
  const tooltipY = hover
    ? clamp(hover.y - tooltipHeight / 2, pad.top + 8, height - pad.bottom - tooltipHeight - 8)
    : 0;

  function updateHover(event: MouseEvent<SVGSVGElement>) {
    const rect = event.currentTarget.getBoundingClientRect();
    if (!rect.width || !rect.height) {
      return;
    }
    const svgX = ((event.clientX - rect.left) / rect.width) * width;
    const svgY = ((event.clientY - rect.top) / rect.height) * height;
    const boundedX = clamp(svgX, pad.left, width - pad.right);
    const ratio = (boundedX - pad.left) / Math.max(1, innerW);
    const targetX = xMin + ratio * (xMax - xMin);
    const nearestX = allPoints.reduce((nearest, point) => (
      Math.abs(point.x - targetX) < Math.abs(nearest - targetX) ? point.x : nearest
    ), allPoints[0]?.x ?? xMin);
    const entries = lines
      .map((line) => {
        const nearestPoint = line.points.reduce((nearest, point) => (
          Math.abs(point.x - nearestX) < Math.abs(nearest.x - nearestX) ? point : nearest
        ), line.points[0]);
        return nearestPoint ? {
          name: line.name,
          color: line.color,
          value: nearestPoint.y,
          y: yScale(nearestPoint.y),
        } : null;
      })
      .filter(Boolean) as ChartHover["entries"];
    setHover({
      x: xScale(nearestX),
      y: clamp(svgY, pad.top, height - pad.bottom),
      dataX: nearestX,
      entries,
    });
  }

  return (
    <svg
      className="h-72 min-h-[288px] w-full overflow-hidden"
      viewBox={`0 0 ${width} ${height}`}
      role="img"
      aria-label={text(chart.title) || "Coroot chart"}
      preserveAspectRatio="none"
      onMouseMove={updateHover}
      onMouseLeave={() => setHover(null)}
      onMouseOut={() => setHover(null)}
    >
      {xTicks.map((tick, index) => (
        <g key={`x-${tick}-${index}`}>
          <line x1={xScale(tick)} x2={xScale(tick)} y1={pad.top} y2={height - pad.bottom} stroke="#e5e7eb" strokeWidth="1" />
          <text x={xScale(tick)} y={height - 8} textAnchor={index === 0 ? "start" : index === xTicks.length - 1 ? "end" : "middle"} className="fill-slate-400 text-[10px]">
            {formatTimeLabel(tick)}
          </text>
        </g>
      ))}
      {yTicks.map((tick, index) => (
        <g key={`${tick}-${index}`}>
          <line x1={pad.left} x2={width - pad.right} y1={yScale(tick)} y2={yScale(tick)} stroke="#e2e8f0" strokeWidth="1" />
          <text x={pad.left - 8} y={yScale(tick) + 4} textAnchor="end" className="fill-slate-400 text-[10px]">
            {formatAxisMetric(tick, unit)}
          </text>
        </g>
      ))}
      <line x1={pad.left} x2={width - pad.right} y1={height - pad.bottom} y2={height - pad.bottom} stroke="#cbd5e1" strokeWidth="1" />
      {lines.map((line) => (
        <g key={line.name}>
          {chart.column ? (
            line.points.map((point, index) => {
              const barWidth = Math.max(2, innerW / Math.max(12, line.points.length * Math.max(1, lines.length)));
              const x = xScale(point.x) - barWidth / 2;
              const y = yScale(Math.max(0, point.y));
              const base = yScale(0);
              return <rect key={`${line.name}-${index}`} x={x} y={Math.min(y, base)} width={barWidth} height={Math.max(1, Math.abs(base - y))} fill={line.color} opacity="0.8" />;
            })
          ) : (
            <>
              <polyline
                fill="none"
                stroke={line.color}
                strokeDasharray={line.threshold ? "4 4" : undefined}
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={line.threshold ? 1.5 : 2}
                points={line.points.map((point) => `${xScale(point.x)},${yScale(point.y)}`).join(" ")}
              />
              {line.points.length === 1 ? <circle cx={xScale(line.points[0].x)} cy={yScale(line.points[0].y)} r="2.5" fill={line.color} /> : null}
            </>
          )}
        </g>
      ))}
      {hover ? (
        <g pointerEvents="none" data-testid="coroot-chart-tooltip">
          <line x1={hover.x} x2={hover.x} y1={pad.top} y2={height - pad.bottom} stroke="#64748b" strokeDasharray="4 4" strokeWidth="1.2" />
          {hover.entries.map((entry) => (
            <circle key={`${entry.name}-point`} cx={hover.x} cy={entry.y} r="3" fill={entry.color} stroke="#fff" strokeWidth="1.5" />
          ))}
          <rect data-testid="coroot-chart-tooltip-box" x={tooltipX} y={tooltipY} width={tooltipWidth} height={tooltipHeight} rx="6" fill="white" fillOpacity="0.96" stroke="#cbd5e1" />
          <text x={tooltipX + tooltipWidth / 2} y={tooltipY + 22} textAnchor="middle" className="fill-slate-800 text-[14px] font-medium">
            {formatTooltipTime(hover.dataX)}
          </text>
          {tooltipEntries.map((entry, index) => (
            <g key={`${entry.name}-tooltip`} transform={`translate(${tooltipX + 14}, ${tooltipY + 48 + index * 22})`}>
              <rect x="0" y="-10" width="7" height="14" rx="1" fill={entry.color} />
              <text data-testid="coroot-chart-tooltip-name" x="15" y="0" className="fill-slate-700 text-[13px]">
                {tooltipSeriesName(entry.name)}: 
              </text>
              <text data-testid="coroot-chart-tooltip-value" x={tooltipWidth - 18} y="0" textAnchor="end" className="fill-slate-900 text-[13px] font-semibold">
                {formatAxisMetric(entry.value, unit)}
              </text>
            </g>
          ))}
        </g>
      ) : null}
      <rect data-testid="coroot-chart-hit-area" x={pad.left} y={pad.top} width={innerW} height={innerH} fill="transparent" pointerEvents="all" />
    </svg>
  );
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

function readCorootChartReports(artifact: AiopsTransportAgentUiArtifact, card?: McpCard | null): CorootChartReport[] {
  for (const source of [artifact.inlineData, artifact.payload, card?.visual]) {
    const record = asRecord(source);
    const reports = normalizeCorootReports(record.chartReports || record.reports);
    if (reports.length) {
      return reports;
    }
  }
  return [];
}

function readDefaultCorootReportName(artifact: AiopsTransportAgentUiArtifact) {
  for (const source of [artifact.inlineData, artifact.payload, artifact.metadata]) {
    const record = asRecord(source);
    const value = text(record.defaultReportName || record.defaultReport || record.preferredReportName);
    if (value) {
      return value;
    }
  }
  return "";
}

function reportKey(report: CorootChartReport, index: number) {
  return `${text(report.name) || "report"}-${index}`;
}

function widgetKey(widget: CorootWidget, index: number) {
  return `${widgetLabel(widget, index)}-${index}`;
}

function widgetLabel(widget: CorootWidget, index: number) {
  const group = widget.chart_group || widget.chartGroup;
  return cleanCorootTabLabel(widget.chart?.title || group?.title || group?.charts?.[0]?.title) || `图表 ${index + 1}`;
}

function cleanCorootTabLabel(value?: unknown) {
  const raw = rawString(value) || text(value);
  const withoutSelector = raw.replace(/<selector>/gi, "").replace(/\s+,/g, ",").replace(/\s+/g, " ").trim();
  const commaIndex = withoutSelector.indexOf(",");
  const label = commaIndex > 0 ? withoutSelector.slice(0, commaIndex).trim() : withoutSelector;
  return text(label);
}

function displayCorootChartTitle(value: unknown, fallback: string) {
  const title = text(value) || fallback;
  return title
    .replace(/\s*Coroot\s*(?:charts|图表)\s*$/i, " 服务")
    .replace(/\s*图表\s*$/i, " 服务")
    .replace(/\s+/g, " ")
    .trim() || fallback;
}

function preferredReportKey(reports: Array<CorootChartReport & { key: string }>, preferredName?: string) {
  const preferred = text(preferredName).toLowerCase();
  if (preferred) {
    const matched = reports.find((report) => text(report.name).toLowerCase() === preferred || text(report.name).toLowerCase().includes(preferred));
    if (matched) {
      return matched.key;
    }
  }
  const cpu = reports.find((report) => text(report.name).toLowerCase().includes("cpu"));
  return (cpu || reports[0])?.key || "";
}

function normalizeCorootReports(value: unknown): CorootChartReport[] {
  return asArray(value)
    .map((item) => {
      const report = asRecord(item);
      const widgets = asArray(report.widgets)
        .map((widget) => normalizeCorootWidget(widget))
        .filter(Boolean) as CorootWidget[];
      return {
        name: text(report.name || report.title),
        status: text(report.status),
        widgets,
      };
    })
    .filter((report) => report.widgets.length > 0);
}

function normalizeCorootWidget(value: unknown): CorootWidget | null {
  const widget = asRecord(value);
  const chart = normalizeCorootChart(widget.chart);
  if (chart) {
    return { chart };
  }
  const rawGroup = asRecord(widget.chart_group || widget.chartGroup);
  const charts = asArray(rawGroup.charts)
    .map((item) => normalizeCorootChart(item))
    .filter(Boolean) as CorootChart[];
  if (charts.length) {
    return {
      chart_group: {
        title: rawString(rawGroup.title),
        charts,
      },
    };
  }
  return null;
}

function normalizeCorootChart(value: unknown): CorootChart | null {
  const chart = asRecord(value);
  if (!Object.keys(chart).length) {
    return null;
  }
  const normalized: CorootChart = {
    title: text(chart.title),
    ctx: asRecord(chart.ctx) as CorootChart["ctx"],
    series: asArray(chart.series).map((item) => asRecord(item) as CorootSeries),
    threshold: Object.keys(asRecord(chart.threshold)).length ? asRecord(chart.threshold) as CorootSeries : undefined,
    stacked: Boolean(chart.stacked),
    sorted: Boolean(chart.sorted),
    column: Boolean(chart.column),
    yzoom: Boolean(chart.yzoom),
    hide_legend: Boolean(chart.hide_legend),
  };
  return chartHasRenderableSeries(normalized) ? normalized : null;
}

function chartHasRenderableSeries(chart?: CorootChart | null) {
  return Boolean(chartLines(chart || {}).length);
}

function chartLines(chart: CorootChart): ChartLine[] {
  const palette = ["#0891b2", "#f97316", "#7c3aed", "#84cc16", "#64748b", "#ef4444", "#0f766e", "#eab308"];
  const rawSeries = [...(chart.series || [])];
  if (chart.threshold) {
    rawSeries.push({ ...chart.threshold, name: chart.threshold.name || "threshold" });
  }
  return rawSeries
    .map((series, index) => {
      const points = dataPoints(series.data, chart.ctx);
      if (!points.length) {
        return null;
      }
      const values = points.map((point) => point.y);
      return {
        name: text(series.name) || `series-${index + 1}`,
        color: colorForSeries(series.color, palette, index),
        points,
        latest: values[values.length - 1] ?? null,
        peak: Math.max(...values),
        threshold: Boolean(series === chart.threshold || series.name === chart.threshold?.name),
      };
    })
    .filter(Boolean) as ChartLine[];
}

function dataPoints(data: unknown, ctx?: CorootChart["ctx"]): Array<{ x: number; y: number }> {
  const from = numeric(ctx?.from);
  const step = numeric(ctx?.step) || 1;
  return asArray(data)
    .map((item, index) => {
      if (Array.isArray(item)) {
        const x = numeric(item[0]);
        const y = numeric(item[1]);
        return Number.isFinite(x) && Number.isFinite(y) ? { x, y } : null;
      }
      const record = asRecord(item);
      if (Object.keys(record).length) {
        const y = numeric(record.value);
        const x = numeric(record.timestamp || record.ts || record.time);
        return Number.isFinite(y) ? { x: Number.isFinite(x) ? x : from + index * step, y } : null;
      }
      const y = numeric(item);
      return Number.isFinite(y) ? { x: from + index * step, y } : null;
    })
    .filter(Boolean) as Array<{ x: number; y: number }>;
}

function chartGroupTitle(rawTitle?: unknown, selectedTitle?: unknown) {
  const selected = text(selectedTitle) || "selected";
  const raw = typeof rawTitle === "string" ? rawTitle : "";
  const replaced = raw.replace("<selector>", selected);
  return text(collapseRepeatedSelectorToken(replaced, selected)) || selected;
}

function collapseRepeatedSelectorToken(value: string, selected: string) {
  const firstToken = text(selected).split(/\s+/)[0];
  if (!firstToken) {
    return value;
  }
  return value.replace(new RegExp(`\\b${escapeRegExp(firstToken)}\\s+${escapeRegExp(firstToken)}(?=\\b|\\s*\\+)`, "i"), firstToken);
}

function escapeRegExp(value: string) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function unitFromChartTitle(title: string) {
  const clean = text(title);
  const idx = clean.lastIndexOf(",");
  return idx >= 0 ? clean.slice(idx + 1).trim() : "";
}

function colorForSeries(color: unknown, palette: string[], index: number) {
  const named = text(color).toLowerCase();
  if (named === "black") return "#0f172a";
  if (named === "red") return "#ef4444";
  if (named === "green") return "#22c55e";
  if (named === "orange") return "#f97316";
  if (named === "grey" || named === "gray" || named === "grey-lighten1") return "#94a3b8";
  if (/^#[0-9a-f]{3,8}$/i.test(named)) return named;
  return palette[index % palette.length];
}

function chartYTicks(yMin: number, yMax: number, yzoom?: boolean) {
  if (!Number.isFinite(yMin) || !Number.isFinite(yMax) || yMax <= yMin) {
    return [0, 0.5, 1];
  }
  if (yzoom || yMin !== 0) {
    return [yMin, yMin + (yMax - yMin) / 2, yMax];
  }
  const step = niceTickStep(yMax / 3);
  if (!Number.isFinite(step) || step <= 0) {
    return [0, yMax / 2, yMax];
  }
  const ticks: number[] = [];
  for (let value = 0; value <= yMax + step * 0.001 && ticks.length < 8; value += step) {
    ticks.push(roundTick(value));
  }
  if (!ticks.length) {
    ticks.push(0);
  }
  return ticks;
}

function chartYDomain(rawYMin: number, rawYMax: number, options?: { yzoom?: boolean; column?: boolean }) {
  if (!Number.isFinite(rawYMin) || !Number.isFinite(rawYMax)) {
    return { min: 0, max: 1 };
  }
  if (rawYMin === rawYMax) {
    if (rawYMin === 0) {
      return { min: 0, max: 1 };
    }
    const padding = Math.max(Math.abs(rawYMin) * 0.08, 1e-9);
    return { min: rawYMin - padding, max: rawYMax + padding };
  }
  if (!options?.column && (options?.yzoom || rawYMin > 0)) {
    const range = rawYMax - rawYMin;
    const padding = Math.max(range * 0.2, Math.abs(rawYMax) * 0.02, 1e-9);
    return { min: Math.max(0, rawYMin - padding), max: rawYMax + padding };
  }
  return { min: Math.min(0, rawYMin), max: rawYMax };
}

function tooltipSeriesName(name: string) {
  const clean = text(name) || name;
  if (clean.length <= 34) {
    return clean;
  }
  return `${clean.slice(0, 23)}...${clean.slice(-8)}`;
}

function niceTickStep(value: number) {
  if (!Number.isFinite(value) || value <= 0) {
    return 1;
  }
  const exponent = Math.floor(Math.log10(value));
  const base = 10 ** exponent;
  const fraction = value / base;
  const niceFraction = fraction <= 1.5 ? 1 : fraction <= 3 ? 2 : fraction <= 7 ? 5 : 10;
  return niceFraction * base;
}

function roundTick(value: number) {
  return Number(value.toPrecision(12));
}

function chartXTicks(xMin: number, xMax: number) {
  if (!Number.isFinite(xMin) || !Number.isFinite(xMax) || xMax <= xMin) {
    return [xMin].filter(Number.isFinite);
  }
  return [0, 1 / 3, 2 / 3, 1].map((ratio) => xMin + (xMax - xMin) * ratio);
}

function formatMetric(value: number | null, unit: string) {
  if (value === null || !Number.isFinite(value)) return "-";
  const abs = Math.abs(value);
  const displayUnit = compactMetricUnit(unit);
  if (displayUnit === "bytes") {
    const units = ["B", "KB", "MB", "GB", "TB"];
    let scaled = value;
    let idx = 0;
    while (Math.abs(scaled) >= 1024 && idx < units.length - 1) {
      scaled /= 1024;
      idx += 1;
    }
    return `${trimNumber(scaled)}${units[idx]}`;
  }
  if (abs >= 1000) return `${trimNumber(value / 1000)}K${displayUnit ? displayUnit : ""}`;
  if (abs > 0 && abs < 0.001) return `${trimNumber(value * 1000000)}µ${displayUnit ? displayUnit : ""}`;
  if (abs > 0 && abs < 1) return `${trimNumber(value * 1000)}m${displayUnit ? displayUnit : ""}`;
  return `${trimNumber(value)}${displayUnit ? displayUnit : ""}`;
}

function formatAxisMetric(value: number | null, unit: string) {
  const normalized = text(unit).toLowerCase();
  if (normalized === "bytes") {
    return formatMetric(value, unit);
  }
  return formatMetric(value, "");
}

function compactMetricUnit(unit: string) {
  const normalized = text(unit).toLowerCase();
  if (!normalized) return "";
  if (normalized === "bytes") return "bytes";
  if (normalized === "per second") return "/s";
  if (normalized === "seconds per second" || normalized === "second per second" || normalized === "seconds/second") {
    return "s/s";
  }
  if (normalized === "seconds") return "s";
  return text(unit);
}

function formatTimeLabel(value: number) {
  if (!Number.isFinite(value) || value <= 0) return "";
  const date = new Date(value > 10_000_000_000 ? value : value * 1000);
  if (Number.isNaN(date.getTime())) return "";
  return date.toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit", hour12: false });
}

function formatTooltipTime(value: number) {
  if (!Number.isFinite(value) || value <= 0) return "";
  const date = new Date(value > 10_000_000_000 ? value : value * 1000);
  if (Number.isNaN(date.getTime())) return "";
  return date.toLocaleString("en-US", {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  });
}

function clamp(value: number, min: number, max: number) {
  return Math.min(Math.max(value, min), max);
}

function trimNumber(value: number) {
  if (!Number.isFinite(value)) return "-";
  if (Math.abs(value) >= 100) return value.toFixed(0);
  if (Math.abs(value) >= 10) return value.toFixed(1).replace(/\.0$/, "");
  return value.toFixed(2).replace(/\.?0+$/, "");
}

function numeric(value: unknown) {
  const n = Number(value);
  return Number.isFinite(n) ? n : Number.NaN;
}

function asArray(value: unknown): unknown[] {
  return Array.isArray(value) ? value : [];
}

function asRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {};
}

function text(value?: unknown) {
  return typeof value === "string" ? value.replace(/<[^>]*>/g, "").trim().replace(/\s+/g, " ") : "";
}

function rawString(value?: unknown) {
  return typeof value === "string" ? value.trim() : "";
}
