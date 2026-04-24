/**
 * Coroot Card Adapter
 *
 * Adapts raw Coroot API responses into MCP UI card payloads.
 * Each function handles missing fields with safe defaults ("N/A" for strings, 0 for numbers).
 */

// Re-export topology adapters so consumers can import from a single module
export { adaptTopology, adaptServiceDependencies } from "./corootTopologyAdapter.js";

/**
 * adaptServiceOverview(result) → McpSummaryCard
 * Converts a Coroot service overview result into a readonly_summary card.
 */
export function adaptServiceOverview(result) {
  try {
    const data = result && typeof result === "object" ? result : {};
    const summaryEntries = data.summary && typeof data.summary === "object" && !Array.isArray(data.summary)
      ? Object.entries(data.summary)
      : [];

    return {
      uiKind: "readonly_summary",
      title: `${data.name || "N/A"} 服务概览`,
      status: data.status || "N/A",
      rows: [
        { label: "服务 ID", value: data.id || "N/A" },
        { label: "状态", value: data.status || "N/A", highlight: true },
        ...summaryEntries.map(([k, v]) => ({ label: k, value: v ?? "N/A" })),
      ],
    };
  } catch {
    return {
      uiKind: "readonly_summary",
      title: "N/A 服务概览",
      status: "N/A",
      rows: [],
    };
  }
}

/**
 * adaptMetrics(result) → McpTimeseriesChartCard
 * Converts Coroot metrics data into a readonly_chart card with timeseries visual.
 */
export function adaptMetrics(result) {
  try {
    const data = result && typeof result === "object" ? result : {};
    const metrics = Array.isArray(data.metrics) ? data.metrics : [];

    return {
      uiKind: "readonly_chart",
      title: "指标趋势",
      visual: {
        kind: "timeseries",
        series: metrics.map((m) => {
          const metric = m && typeof m === "object" ? m : {};
          const values = Array.isArray(metric.values) ? metric.values : [];
          return {
            name: metric.name || "N/A",
            data: values.map((pair) => {
              const arr = Array.isArray(pair) ? pair : [];
              return { timestamp: arr[0] || 0, value: arr[1] || 0 };
            }),
          };
        }),
      },
    };
  } catch {
    return {
      uiKind: "readonly_chart",
      title: "指标趋势",
      visual: { kind: "timeseries", series: [] },
    };
  }
}

/**
 * adaptAlerts(alerts) → McpStatusTableCard
 * Converts Coroot alerts array into a readonly_chart card with status_table visual.
 */
export function adaptAlerts(alerts) {
  try {
    const list = Array.isArray(alerts) ? alerts : [];

    return {
      uiKind: "readonly_chart",
      title: "告警列表",
      visual: {
        kind: "status_table",
        columns: ["ID", "名称", "严重程度", "状态"],
        rows: list.map((a) => {
          const alert = a && typeof a === "object" ? a : {};
          return {
            cells: [
              alert.id || "N/A",
              alert.name || "N/A",
              (alert.severity || "N/A").toLowerCase(),
              alert.status || "N/A",
            ],
            status: (alert.severity || "N/A").toLowerCase(),
          };
        }),
      },
    };
  } catch {
    return {
      uiKind: "readonly_chart",
      title: "告警列表",
      visual: { kind: "status_table", columns: ["ID", "名称", "严重程度", "状态"], rows: [] },
    };
  }
}

/**
 * adaptHostOverview(hostData) → McpSummaryCard
 * Converts a Coroot host overview result into a readonly_summary card
 * with KV rows for CPU, memory, disk, network, and other host info.
 */
export function adaptHostOverview(hostData) {
  try {
    const data = hostData && typeof hostData === "object" ? hostData : {};
    const alerts = Array.isArray(data.alerts) ? data.alerts : [];
    const activeAlerts = alerts.filter(
      (a) => a && typeof a === "object" && (a.status === "firing" || a.status === "active")
    );

    const rows = [
      { label: "主机名", value: data.name || data.hostname || "N/A" },
      { label: "状态", value: data.status || "N/A", highlight: true },
      { label: "操作系统", value: data.os || "N/A" },
      { label: "CPU 使用率", value: data.cpu != null ? String(data.cpu) : "N/A" },
      { label: "内存使用率", value: data.memory != null ? String(data.memory) : "N/A" },
      { label: "磁盘使用率", value: data.disk != null ? String(data.disk) : "N/A" },
      { label: "网络流量", value: data.network != null ? String(data.network) : "N/A" },
    ];

    const card = {
      uiKind: "readonly_summary",
      title: `${data.name || data.hostname || "N/A"} 主机概览`,
      status: data.status || "N/A",
      rows,
    };

    if (activeAlerts.length > 0) {
      card.alertSummary = `${activeAlerts.length} 个活跃告警`;
    }

    return card;
  } catch {
    return {
      uiKind: "readonly_summary",
      title: "N/A 主机概览",
      status: "N/A",
      rows: [],
    };
  }
}

/**
 * adaptServiceDetail(serviceOverview) → McpSummaryCard
 * Enhanced version of adaptServiceOverview that includes health score,
 * key metrics (CPU, memory, latency, error rate), and alert summary.
 */
export function adaptServiceDetail(serviceOverview) {
  try {
    const data = serviceOverview && typeof serviceOverview === "object" ? serviceOverview : {};
    const alerts = Array.isArray(data.alerts) ? data.alerts : [];
    const activeAlerts = alerts.filter(
      (a) => a && typeof a === "object" && (a.status === "firing" || a.status === "active")
    );

    const rows = [
      { label: "服务 ID", value: data.id || "N/A" },
      { label: "状态", value: data.status || "N/A", highlight: true },
      { label: "健康评分", value: data.healthScore != null ? String(data.healthScore) : "N/A" },
      { label: "CPU", value: data.cpu != null ? String(data.cpu) : "N/A" },
      { label: "内存", value: data.memory != null ? String(data.memory) : "N/A" },
      { label: "请求延迟", value: data.latency != null ? String(data.latency) : "N/A" },
      { label: "错误率", value: data.errorRate != null ? String(data.errorRate) : "N/A" },
    ];

    const card = {
      uiKind: "readonly_summary",
      title: `${data.name || "N/A"} 服务详情`,
      status: data.status || "N/A",
      rows,
    };

    if (activeAlerts.length > 0) {
      card.alertSummary = `${activeAlerts.length} 个活跃告警`;
    }

    return card;
  } catch {
    return {
      uiKind: "readonly_summary",
      title: "N/A 服务详情",
      status: "N/A",
      rows: [],
    };
  }
}

/**
 * adaptServiceStats(services) → McpKpiStripCard
 * Aggregates service health statuses into a readonly_summary card with KPI strip visual.
 */
export function adaptServiceStats(services) {
  try {
    const list = Array.isArray(services) ? services : [];
    const total = list.length;
    let healthy = 0;
    let warning = 0;
    let critical = 0;

    for (const svc of list) {
      const status = (svc && typeof svc === "object" && typeof svc.status === "string"
        ? svc.status
        : ""
      ).toLowerCase();

      if (status === "ok" || status === "healthy") {
        healthy++;
      } else if (status === "warning") {
        warning++;
      } else {
        // "critical", "error", or any unknown status counts toward critical
        critical++;
      }
    }

    return {
      uiKind: "readonly_summary",
      title: "服务健康概览",
      kpis: [
        { label: "总服务数", value: total },
        { label: "健康", value: healthy, color: "green" },
        { label: "告警", value: warning, color: "amber" },
        { label: "异常", value: critical, color: "red" },
      ],
      visual: { kind: "kpi_strip" },
    };
  } catch {
    return {
      uiKind: "readonly_summary",
      title: "服务健康概览",
      kpis: [
        { label: "总服务数", value: 0 },
        { label: "健康", value: 0, color: "green" },
        { label: "告警", value: 0, color: "amber" },
        { label: "异常", value: 0, color: "red" },
      ],
      visual: { kind: "kpi_strip" },
    };
  }
}
