import { describe, expect, it } from "vitest";
import { normalizeMcpBundle, normalizeMcpUiCard } from "../src/lib/mcpUiCardModel";

describe("mcpUiCardModel", () => {
  it("normalizes a minimal readonly_chart card with safe defaults", () => {
    const normalized = normalizeMcpUiCard({
      uiKind: "readonly_chart",
      placement: "side-panel",
      title: "CPU 使用率",
      source: "remote-mcp",
      mcpServer: "metrics-prod",
      visual: {
        kind: "timeseries",
      },
    });

    expect(normalized).toMatchObject({
      id: "mcp-ui-card",
      uiKind: "readonly_chart",
      placement: "inline_final",
      title: "CPU 使用率",
      source: "remote-mcp",
      mcpServer: "metrics-prod",
      summary: "",
      freshness: {
        capturedAt: "",
        ttlSec: 0,
        staleAt: "",
      },
      scope: {
        hostId: "",
        service: "",
        cluster: "",
        env: "",
        timeRange: "",
        resourceType: "",
        resourceId: "",
      },
      actions: [],
    });
    expect(normalized.visual).toMatchObject({
      kind: "timeseries",
    });
  });

  it("normalizes a monitor_bundle while preserving section kinds and cards", () => {
    const normalized = normalizeMcpBundle({
      bundleKind: "monitor_bundle",
      bundleId: "bundle-nginx-1",
      subject: "nginx",
      summary: "nginx 中间件监控概览",
      freshness: "2m ago",
      sections: [
        {
          kind: "overview",
          title: "概览",
          cards: [
            {
              uiKind: "readonly_summary",
              title: "当前状态",
              summary: "总体健康",
            },
          ],
        },
        {
          kind: "alerts",
          title: "告警",
          cards: [
            {
              uiKind: "readonly_chart",
              title: "P95 延迟",
              visual: { kind: "timeseries" },
            },
          ],
        },
      ],
    });

    expect(normalized).toMatchObject({
      bundleKind: "monitor_bundle",
      bundleId: "bundle-nginx-1",
      subject: {
        type: "service",
        name: "nginx",
      },
      summary: "nginx 中间件监控概览",
      freshness: {
        label: "2m ago",
      },
    });
    expect(normalized.sections).toHaveLength(5);
    expect(normalized.sections[0]).toMatchObject({
      kind: "overview",
      title: "概览",
    });
    expect(normalized.sections[0].cards).toHaveLength(1);
    expect(normalized.sections[0].cards[0]).toMatchObject({
      uiKind: "readonly_summary",
      title: "当前状态",
      summary: "总体健康",
    });
    const alertsSection = normalized.sections.find((section) => section.kind === "alerts");
    expect(alertsSection).toMatchObject({
      kind: "alerts",
      title: "告警",
    });
    expect(alertsSection.cards).toHaveLength(1);
    expect(alertsSection.cards[0]).toMatchObject({
      uiKind: "readonly_chart",
      title: "P95 延迟",
    });
  });

  it("falls back to safe defaults for invalid or missing fields", () => {
    expect(() => normalizeMcpUiCard(null)).not.toThrow();
    expect(() => normalizeMcpBundle(undefined)).not.toThrow();

    const card = normalizeMcpUiCard({
      uiKind: "unknown_kind",
      placement: "somewhere",
      actions: "not-an-array",
      visual: null,
    });
    const bundle = normalizeMcpBundle({
      bundleKind: "unknown_bundle",
      sections: null,
    });

    expect(card).toMatchObject({
      uiKind: "readonly_summary",
      placement: "inline_final",
      title: "MCP 卡片",
      summary: "",
      freshness: {
        capturedAt: "",
        ttlSec: 0,
        staleAt: "",
      },
      scope: {
        hostId: "",
        service: "",
        cluster: "",
        env: "",
        timeRange: "",
        resourceType: "",
        resourceId: "",
      },
      actions: [],
    });
    expect(card.id).toBeTruthy();
    expect(card.visual).toEqual({});

    expect(bundle).toMatchObject({
      bundleKind: "monitor_bundle",
      bundleId: expect.any(String),
      subject: {
        type: "service",
        name: "",
      },
      summary: "",
      freshness: {
        capturedAt: "",
        ttlSec: 0,
        staleAt: "",
      },
    });
    expect(bundle.sections).toHaveLength(5);
  });
});
