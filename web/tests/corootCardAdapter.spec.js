import { describe, it, expect } from "vitest";
import fc from "fast-check";
import {
  adaptServiceOverview,
  adaptMetrics,
  adaptAlerts,
  adaptServiceStats,
  adaptHostOverview,
  adaptServiceDetail,
} from "../src/lib/corootCardAdapter.js";

describe("corootCardAdapter", () => {
  describe("adaptServiceOverview", () => {
    it("maps a full service result to McpSummaryCard", () => {
      const result = {
        id: "svc-1",
        name: "gateway",
        status: "ok",
        summary: { latency: "12ms", rps: "500" },
      };
      const card = adaptServiceOverview(result);
      expect(card.uiKind).toBe("readonly_summary");
      expect(card.title).toBe("gateway 服务概览");
      expect(card.status).toBe("ok");
      expect(card.rows).toEqual([
        { label: "服务 ID", value: "svc-1" },
        { label: "状态", value: "ok", highlight: true },
        { label: "latency", value: "12ms" },
        { label: "rps", value: "500" },
      ]);
    });

    it("uses N/A defaults for missing fields", () => {
      const card = adaptServiceOverview({});
      expect(card.title).toBe("N/A 服务概览");
      expect(card.status).toBe("N/A");
      expect(card.rows[0].value).toBe("N/A");
      expect(card.rows[1].value).toBe("N/A");
    });

    it("handles null/undefined input gracefully", () => {
      const card = adaptServiceOverview(null);
      expect(card.uiKind).toBe("readonly_summary");
      expect(card.title).toBe("N/A 服务概览");
      expect(card.rows.length).toBe(2);
    });

    it("handles summary with null values using N/A", () => {
      const card = adaptServiceOverview({ summary: { cpu: null, mem: undefined } });
      const summaryRows = card.rows.slice(2);
      expect(summaryRows[0]).toEqual({ label: "cpu", value: "N/A" });
      expect(summaryRows[1]).toEqual({ label: "mem", value: "N/A" });
    });
  });

  describe("adaptMetrics", () => {
    it("maps metrics to McpTimeseriesChartCard", () => {
      const result = {
        metrics: [
          { name: "cpu", values: [[1000, 0.5], [2000, 0.8]] },
        ],
      };
      const card = adaptMetrics(result);
      expect(card.uiKind).toBe("readonly_chart");
      expect(card.title).toBe("指标趋势");
      expect(card.visual.kind).toBe("timeseries");
      expect(card.visual.series).toHaveLength(1);
      expect(card.visual.series[0].name).toBe("cpu");
      expect(card.visual.series[0].data).toEqual([
        { timestamp: 1000, value: 0.5 },
        { timestamp: 2000, value: 0.8 },
      ]);
    });

    it("uses defaults for missing metric fields", () => {
      const card = adaptMetrics({ metrics: [{}] });
      expect(card.visual.series[0].name).toBe("N/A");
      expect(card.visual.series[0].data).toEqual([]);
    });

    it("returns empty series for null input", () => {
      const card = adaptMetrics(null);
      expect(card.visual.series).toEqual([]);
    });
  });

  describe("adaptAlerts", () => {
    it("maps alerts to McpStatusTableCard", () => {
      const alerts = [
        { id: "a1", name: "HighCPU", severity: "CRITICAL", status: "firing" },
      ];
      const card = adaptAlerts(alerts);
      expect(card.uiKind).toBe("readonly_chart");
      expect(card.title).toBe("告警列表");
      expect(card.visual.kind).toBe("status_table");
      expect(card.visual.columns).toEqual(["ID", "名称", "严重程度", "状态"]);
      expect(card.visual.rows[0].cells).toEqual(["a1", "HighCPU", "critical", "firing"]);
      expect(card.visual.rows[0].status).toBe("critical");
    });

    it("uses N/A for missing alert fields", () => {
      const card = adaptAlerts([{}]);
      expect(card.visual.rows[0].cells).toEqual(["N/A", "N/A", "n/a", "N/A"]);
    });

    it("returns empty rows for null input", () => {
      const card = adaptAlerts(null);
      expect(card.visual.rows).toEqual([]);
    });
  });

  describe("adaptServiceStats", () => {
    it("counts service statuses correctly", () => {
      const services = [
        { status: "ok" },
        { status: "healthy" },
        { status: "warning" },
        { status: "critical" },
        { status: "error" },
      ];
      const card = adaptServiceStats(services);
      expect(card.uiKind).toBe("readonly_summary");
      expect(card.title).toBe("服务健康概览");
      expect(card.visual.kind).toBe("kpi_strip");
      expect(card.kpis).toEqual([
        { label: "总服务数", value: 5 },
        { label: "健康", value: 2, color: "green" },
        { label: "告警", value: 1, color: "amber" },
        { label: "异常", value: 2, color: "red" },
      ]);
    });

    it("counts unknown statuses as critical", () => {
      const services = [{ status: "unknown" }, { status: "degraded" }];
      const card = adaptServiceStats(services);
      expect(card.kpis[3].value).toBe(2); // critical
    });

    it("returns all zeros for empty array", () => {
      const card = adaptServiceStats([]);
      expect(card.kpis[0].value).toBe(0);
    });

    it("returns all zeros for null input", () => {
      const card = adaptServiceStats(null);
      expect(card.kpis[0].value).toBe(0);
    });
  });
});

describe("adaptHostOverview", () => {
  it("maps a full host result to McpSummaryCard with KV rows", () => {
    const hostData = {
      name: "host-01",
      status: "online",
      os: "Ubuntu 22.04",
      cpu: "78%",
      memory: "85%",
      disk: "45%",
      network: "120 Mbps",
    };
    const card = adaptHostOverview(hostData);
    expect(card.uiKind).toBe("readonly_summary");
    expect(card.title).toBe("host-01 主机概览");
    expect(card.status).toBe("online");
    expect(card.rows).toEqual([
      { label: "主机名", value: "host-01" },
      { label: "状态", value: "online", highlight: true },
      { label: "操作系统", value: "Ubuntu 22.04" },
      { label: "CPU 使用率", value: "78%" },
      { label: "内存使用率", value: "85%" },
      { label: "磁盘使用率", value: "45%" },
      { label: "网络流量", value: "120 Mbps" },
    ]);
    expect(card.alertSummary).toBeUndefined();
  });

  it("uses N/A defaults for missing fields", () => {
    const card = adaptHostOverview({});
    expect(card.title).toBe("N/A 主机概览");
    expect(card.status).toBe("N/A");
    expect(card.rows[0].value).toBe("N/A");
    expect(card.rows[2].value).toBe("N/A"); // os
    expect(card.rows[3].value).toBe("N/A"); // cpu
  });

  it("handles null/undefined input gracefully", () => {
    const card = adaptHostOverview(null);
    expect(card.uiKind).toBe("readonly_summary");
    expect(card.title).toBe("N/A 主机概览");
    expect(card.rows).toHaveLength(7);
  });

  it("falls back to hostname field when name is missing", () => {
    const card = adaptHostOverview({ hostname: "srv-02" });
    expect(card.title).toBe("srv-02 主机概览");
    expect(card.rows[0].value).toBe("srv-02");
  });

  it("includes alertSummary when active alerts exist", () => {
    const card = adaptHostOverview({
      name: "host-01",
      alerts: [
        { id: "a1", status: "firing" },
        { id: "a2", status: "resolved" },
        { id: "a3", status: "active" },
      ],
    });
    expect(card.alertSummary).toBe("2 个活跃告警");
  });

  it("handles numeric 0 values correctly (not N/A)", () => {
    const card = adaptHostOverview({ cpu: 0, memory: 0, disk: 0, network: 0 });
    expect(card.rows[3].value).toBe("0"); // cpu
    expect(card.rows[4].value).toBe("0"); // memory
    expect(card.rows[5].value).toBe("0"); // disk
    expect(card.rows[6].value).toBe("0"); // network
  });
});

describe("adaptServiceDetail", () => {
  it("maps a full service detail to McpSummaryCard with enhanced rows", () => {
    const data = {
      id: "svc-1",
      name: "gateway",
      status: "ok",
      healthScore: 95,
      cpu: "45%",
      memory: "62%",
      latency: "12ms",
      errorRate: "0.1%",
    };
    const card = adaptServiceDetail(data);
    expect(card.uiKind).toBe("readonly_summary");
    expect(card.title).toBe("gateway 服务详情");
    expect(card.status).toBe("ok");
    expect(card.rows).toEqual([
      { label: "服务 ID", value: "svc-1" },
      { label: "状态", value: "ok", highlight: true },
      { label: "健康评分", value: "95" },
      { label: "CPU", value: "45%" },
      { label: "内存", value: "62%" },
      { label: "请求延迟", value: "12ms" },
      { label: "错误率", value: "0.1%" },
    ]);
    expect(card.alertSummary).toBeUndefined();
  });

  it("uses N/A defaults for missing fields", () => {
    const card = adaptServiceDetail({});
    expect(card.title).toBe("N/A 服务详情");
    expect(card.status).toBe("N/A");
    expect(card.rows[0].value).toBe("N/A"); // id
    expect(card.rows[2].value).toBe("N/A"); // healthScore
    expect(card.rows[5].value).toBe("N/A"); // latency
  });

  it("handles null/undefined input gracefully", () => {
    const card = adaptServiceDetail(null);
    expect(card.uiKind).toBe("readonly_summary");
    expect(card.title).toBe("N/A 服务详情");
    expect(card.rows).toHaveLength(7);
  });

  it("includes alertSummary when active alerts exist", () => {
    const card = adaptServiceDetail({
      name: "api-svc",
      alerts: [
        { id: "a1", status: "firing" },
        { id: "a2", status: "resolved" },
      ],
    });
    expect(card.alertSummary).toBe("1 个活跃告警");
  });

  it("handles numeric 0 values correctly (not N/A)", () => {
    const card = adaptServiceDetail({ healthScore: 0, cpu: 0, memory: 0, latency: 0, errorRate: 0 });
    expect(card.rows[2].value).toBe("0"); // healthScore
    expect(card.rows[3].value).toBe("0"); // cpu
    expect(card.rows[4].value).toBe("0"); // memory
    expect(card.rows[5].value).toBe("0"); // latency
    expect(card.rows[6].value).toBe("0"); // errorRate
  });
});

/**
 * Property-Based Tests (fast-check)
 * Feature: coroot-monitor-embed
 */
describe("corootCardAdapter — Property-Based Tests", () => {
  /**
   * Property 1: 服务概览数据映射完整性
   * Validates: Requirements 1, 8.1
   *
   * For any random ServiceOverviewResult with id, name, status, and summary fields:
   * - Output has uiKind === "readonly_summary"
   * - Output title contains the service name
   * - Output rows array contains at least 2 entries (id and status rows)
   * - If summary has N entries, rows has 2 + N entries
   * - Each row has label and value properties
   */
  describe("Property 1: 服务概览数据映射完整性", () => {
    const serviceOverviewArb = fc.record({
      id: fc.string({ minLength: 1, maxLength: 50 }),
      name: fc.string({ minLength: 1, maxLength: 50 }),
      status: fc.constantFrom("ok", "healthy", "warning", "critical", "error"),
      summary: fc.dictionary(
        fc.string({ minLength: 1, maxLength: 20 }),
        fc.string({ minLength: 1, maxLength: 30 })
      ),
    });

    it("output always has uiKind === 'readonly_summary'", () => {
      fc.assert(
        fc.property(serviceOverviewArb, (input) => {
          const card = adaptServiceOverview(input);
          expect(card.uiKind).toBe("readonly_summary");
        }),
        { numRuns: 100 }
      );
    });

    it("output title always contains the service name", () => {
      fc.assert(
        fc.property(serviceOverviewArb, (input) => {
          const card = adaptServiceOverview(input);
          expect(card.title).toContain(input.name);
        }),
        { numRuns: 100 }
      );
    });

    it("output rows always contains at least 2 entries (id and status)", () => {
      fc.assert(
        fc.property(serviceOverviewArb, (input) => {
          const card = adaptServiceOverview(input);
          expect(card.rows.length).toBeGreaterThanOrEqual(2);
        }),
        { numRuns: 100 }
      );
    });

    it("rows count equals 2 + number of summary entries", () => {
      fc.assert(
        fc.property(serviceOverviewArb, (input) => {
          const card = adaptServiceOverview(input);
          const summaryCount = Object.keys(input.summary).length;
          expect(card.rows.length).toBe(2 + summaryCount);
        }),
        { numRuns: 100 }
      );
    });

    it("every row has label and value properties", () => {
      fc.assert(
        fc.property(serviceOverviewArb, (input) => {
          const card = adaptServiceOverview(input);
          for (const row of card.rows) {
            expect(row).toHaveProperty("label");
            expect(row).toHaveProperty("value");
          }
        }),
        { numRuns: 100 }
      );
    });
  });

  /**
   * Property 2: 指标数据映射保真性
   * Validates: Requirements 3
   *
   * For any random MetricsResult with N metrics entries, each with M values:
   * - Output has uiKind === "readonly_chart"
   * - Output visual.kind === "timeseries"
   * - Output visual.series.length === input metrics.length
   * - For each series[i], data.length === input metrics[i].values.length
   */
  describe("Property 2: 指标数据映射保真性", () => {
    const metricValueArb = fc.tuple(
      fc.integer({ min: 0, max: 2000000000 }),
      fc.double({ min: -1e6, max: 1e6, noNaN: true })
    );

    const metricEntryArb = fc.record({
      name: fc.string({ minLength: 1, maxLength: 30 }),
      values: fc.array(metricValueArb, { minLength: 0, maxLength: 20 }),
    });

    const metricsResultArb = fc.record({
      metrics: fc.array(metricEntryArb, { minLength: 0, maxLength: 15 }),
    });

    it("output always has uiKind === 'readonly_chart'", () => {
      fc.assert(
        fc.property(metricsResultArb, (input) => {
          const card = adaptMetrics(input);
          expect(card.uiKind).toBe("readonly_chart");
        }),
        { numRuns: 100 }
      );
    });

    it("output visual.kind is always 'timeseries'", () => {
      fc.assert(
        fc.property(metricsResultArb, (input) => {
          const card = adaptMetrics(input);
          expect(card.visual.kind).toBe("timeseries");
        }),
        { numRuns: 100 }
      );
    });

    it("series count equals input metrics count", () => {
      fc.assert(
        fc.property(metricsResultArb, (input) => {
          const card = adaptMetrics(input);
          expect(card.visual.series.length).toBe(input.metrics.length);
        }),
        { numRuns: 100 }
      );
    });

    it("each series data length equals corresponding input values length", () => {
      fc.assert(
        fc.property(metricsResultArb, (input) => {
          const card = adaptMetrics(input);
          for (let i = 0; i < input.metrics.length; i++) {
            expect(card.visual.series[i].data.length).toBe(
              input.metrics[i].values.length
            );
          }
        }),
        { numRuns: 100 }
      );
    });
  });

  /**
   * Property 3: 告警数据映射行数一致性
   * Validates: Requirements 4
   *
   * For any random Alert array of length N:
   * - Output visual.rows.length === N
   * - Each row has a `status` field matching the lowercased severity
   * - Each row has a `cells` array of length 4
   */
  describe("Property 3: 告警数据映射行数一致性", () => {
    const alertArb = fc.record({
      id: fc.string({ minLength: 1, maxLength: 50 }),
      name: fc.string({ minLength: 1, maxLength: 50 }),
      severity: fc.constantFrom("CRITICAL", "WARNING", "INFO", "critical", "warning", "info", "Critical", "Warning"),
      status: fc.constantFrom("firing", "resolved", "pending", "acknowledged"),
    });

    const alertsArrayArb = fc.array(alertArb, { minLength: 0, maxLength: 30 });

    it("output rows length equals input alerts length", () => {
      fc.assert(
        fc.property(alertsArrayArb, (alerts) => {
          const card = adaptAlerts(alerts);
          expect(card.visual.rows.length).toBe(alerts.length);
        }),
        { numRuns: 100 }
      );
    });

    it("each row status matches lowercased severity of corresponding alert", () => {
      fc.assert(
        fc.property(alertsArrayArb, (alerts) => {
          const card = adaptAlerts(alerts);
          for (let i = 0; i < alerts.length; i++) {
            expect(card.visual.rows[i].status).toBe(
              alerts[i].severity.toLowerCase()
            );
          }
        }),
        { numRuns: 100 }
      );
    });

    it("each row has a cells array of length 4", () => {
      fc.assert(
        fc.property(alertsArrayArb, (alerts) => {
          const card = adaptAlerts(alerts);
          for (const row of card.visual.rows) {
            expect(row.cells).toHaveLength(4);
          }
        }),
        { numRuns: 100 }
      );
    });
  });

  /**
   * Property 4: 服务统计 KPI 计数一致性
   * Validates: Requirements 2
   *
   * For any random Service array of length N:
   * - kpis[0].value (总服务数) === N
   * - kpis[1].value (健康) + kpis[2].value (告警) + kpis[3].value (异常) === N
   * - kpis[1].color === "green", kpis[2].color === "amber", kpis[3].color === "red"
   */
  describe("Property 4: 服务统计 KPI 计数一致性", () => {
    const serviceArb = fc.record({
      id: fc.string({ minLength: 0, maxLength: 30 }),
      name: fc.string({ minLength: 0, maxLength: 30 }),
      status: fc.constantFrom("ok", "healthy", "warning", "critical", "error", "unknown", ""),
    });

    const servicesArrayArb = fc.array(serviceArb, { minLength: 0, maxLength: 50 });

    it("总服务数 KPI equals input array length", () => {
      fc.assert(
        fc.property(servicesArrayArb, (services) => {
          const card = adaptServiceStats(services);
          expect(card.kpis[0].value).toBe(services.length);
        }),
        { numRuns: 100 }
      );
    });

    it("健康 + 告警 + 异常 KPI sum equals input array length", () => {
      fc.assert(
        fc.property(servicesArrayArb, (services) => {
          const card = adaptServiceStats(services);
          const sum = card.kpis[1].value + card.kpis[2].value + card.kpis[3].value;
          expect(sum).toBe(services.length);
        }),
        { numRuns: 100 }
      );
    });

    it("KPI colors are green, amber, red respectively", () => {
      fc.assert(
        fc.property(servicesArrayArb, (services) => {
          const card = adaptServiceStats(services);
          expect(card.kpis[1].color).toBe("green");
          expect(card.kpis[2].color).toBe("amber");
          expect(card.kpis[3].color).toBe("red");
        }),
        { numRuns: 100 }
      );
    });
  });

  /**
   * Property 6: 适配器对部分数据的鲁棒性
   * Validates: Requirements 8.5
   *
   * For any random input (null, undefined, empty objects, objects with random/missing fields):
   * - adaptServiceOverview never throws, output has uiKind and title
   * - adaptMetrics never throws, output has uiKind and title
   * - adaptAlerts never throws, output has uiKind and title
   * - adaptServiceStats never throws, output has uiKind and title
   */
  describe("Property 6: 适配器对部分数据的鲁棒性", () => {
    const malformedInputArb = fc.oneof(
      fc.constant(null),
      fc.constant(undefined),
      fc.constant(0),
      fc.constant(""),
      fc.constant(false),
      fc.constant([]),
      fc.constant({}),
      fc.anything(),
      fc.object()
    );

    it("adaptServiceOverview never throws and output has uiKind and title", () => {
      fc.assert(
        fc.property(malformedInputArb, (input) => {
          const card = adaptServiceOverview(input);
          expect(card).toHaveProperty("uiKind");
          expect(card).toHaveProperty("title");
          expect(typeof card.uiKind).toBe("string");
          expect(typeof card.title).toBe("string");
        }),
        { numRuns: 200 }
      );
    });

    it("adaptMetrics never throws and output has uiKind and title", () => {
      fc.assert(
        fc.property(malformedInputArb, (input) => {
          const card = adaptMetrics(input);
          expect(card).toHaveProperty("uiKind");
          expect(card).toHaveProperty("title");
          expect(typeof card.uiKind).toBe("string");
          expect(typeof card.title).toBe("string");
        }),
        { numRuns: 200 }
      );
    });

    it("adaptAlerts never throws and output has uiKind and title", () => {
      fc.assert(
        fc.property(malformedInputArb, (input) => {
          const card = adaptAlerts(input);
          expect(card).toHaveProperty("uiKind");
          expect(card).toHaveProperty("title");
          expect(typeof card.uiKind).toBe("string");
          expect(typeof card.title).toBe("string");
        }),
        { numRuns: 200 }
      );
    });

    it("adaptServiceStats never throws and output has uiKind and title", () => {
      fc.assert(
        fc.property(malformedInputArb, (input) => {
          const card = adaptServiceStats(input);
          expect(card).toHaveProperty("uiKind");
          expect(card).toHaveProperty("title");
          expect(typeof card.uiKind).toBe("string");
          expect(typeof card.title).toBe("string");
        }),
        { numRuns: 200 }
      );
    });
  });
});
