import { describe, expect, it } from "vitest";
import { adaptMcpUiPayload, adaptMcpUiPayloadFromCard } from "../src/lib/mcpUiPayloadAdapter";

describe("mcpUiPayloadAdapter", () => {
  it("normalizes wrapped ui card payloads from remote MCP envelopes", () => {
    const adapted = adaptMcpUiPayload({
      source: "remote-mcp",
      mcp_server: "metrics-prod",
      scope: {
        host_id: "web-01",
        environment: "prod",
      },
      freshness: {
        updated_at: "2026-04-03T10:24:00Z",
        ttl_sec: 120,
      },
      availableActions: [
        {
          id: "pin-dashboard",
          label: "固定到右侧",
          intent: "pin",
        },
      ],
      errors: [
        {
          code: "STALE_SNAPSHOT",
          message: "当前数据快照已过期",
        },
      ],
      payload: {
        mcpUi: {
          id: "cpu-card",
          uiKind: "readonly_chart",
          title: "CPU 使用率（5m）",
          summary: "web-01 在 10:24 出现峰值 92%",
        },
      },
    });

    expect(adapted.items).toHaveLength(1);
    expect(adapted.items[0]).toMatchObject({
      id: "cpu-card",
      kind: "mcp_ui_card",
      source: "remote",
      mcpServer: "metrics-prod",
      model: {
        id: "cpu-card",
        source: "remote",
        mcpServer: "metrics-prod",
        uiKind: "readonly_chart",
        title: "CPU 使用率（5m）",
        scope: {
          hostId: "web-01",
          env: "prod",
        },
        freshness: {
          capturedAt: "2026-04-03T10:24:00Z",
          ttlSec: 120,
        },
      },
    });
    expect(adapted.items[0].model.actions).toHaveLength(1);
    expect(adapted.items[0].model.actions[0]).toMatchObject({
      id: "pin-dashboard",
      intent: "pin",
      mutation: false,
    });
    expect(adapted.items[0].errors[0]).toMatchObject({
      code: "STALE_SNAPSHOT",
      message: "当前数据快照已过期",
      source: "remote",
    });
  });

  it("normalizes nested workspace bundle payloads through payload/result/data/detail wrappers", () => {
    const adapted = adaptMcpUiPayload({
      origin: "workspace-agent",
      mcpServer: "ops-console",
      detail: {
        freshness: "2m ago",
        result: {
          data: {
            scope: {
              service: "redis",
              cluster: "prod-cn",
            },
            mcpBundle: {
              bundleKind: "remediation_bundle",
              bundleId: "redis-rca-1",
              summary: "redis-prod 存在连接池抖动，需要验证降载策略",
              subject: {
                type: "middleware",
                name: "redis",
                env: "prod",
              },
              sections: [
                {
                  kind: "root_cause",
                  cards: [
                    {
                      id: "root-cause-card",
                      uiKind: "readonly_summary",
                      title: "根因",
                      summary: "慢查询放大了连接重试",
                    },
                  ],
                },
              ],
            },
          },
        },
      },
    });

    expect(adapted.items).toHaveLength(1);
    expect(adapted.items[0]).toMatchObject({
      id: "redis-rca-1",
      kind: "mcp_bundle",
      source: "workspace",
      mcpServer: "ops-console",
      model: {
        bundleId: "redis-rca-1",
        bundleKind: "remediation_bundle",
        source: "workspace",
        mcpServer: "ops-console",
        freshness: {
          label: "2m ago",
        },
        scope: {
          service: "redis",
          cluster: "prod-cn",
        },
      },
    });
    expect(adapted.items[0].model.sections.find((section) => section.kind === "root_cause")?.cards[0]).toMatchObject({
      id: "root-cause-card",
      title: "根因",
    });
  });

  it("keeps the same card and bundle model contract across different wrapper aliases", () => {
    const remoteEnvelope = adaptMcpUiPayload({
      source: "remote-mcp",
      mcpServer: "metrics-prod",
      response: {
        body: {
          card: {
            id: "cpu-card",
            uiKind: "readonly_chart",
            title: "CPU 使用率",
            summary: "5 分钟峰值 92%",
            freshness: {
              captured_at: "2026-04-03T10:24:00Z",
              ttl_sec: 60,
            },
            scope: {
              host_id: "web-01",
              service: "nginx",
            },
          },
        },
      },
    });

    const workspaceEnvelope = adaptMcpUiPayload({
      origin: "workspace-agent",
      mcpServer: "ops-console",
      content: {
        ui: {
          mcpBundle: {
            bundleId: "nginx-monitor-1",
            bundleKind: "monitor_bundle",
            summary: "nginx 监控聚合面板",
            subject: "nginx",
            sections: [],
          },
        },
      },
    });

    expect(remoteEnvelope.items[0]).toMatchObject({
      kind: "mcp_ui_card",
      source: "remote",
      mcpServer: "metrics-prod",
      model: {
        id: "cpu-card",
        source: "remote",
        uiKind: "readonly_chart",
        title: "CPU 使用率",
        freshness: {
          ttlSec: 60,
        },
        scope: {
          hostId: "web-01",
          service: "nginx",
        },
      },
    });
    expect(workspaceEnvelope.items[0]).toMatchObject({
      kind: "mcp_bundle",
      source: "workspace",
      mcpServer: "ops-console",
      model: {
        bundleId: "nginx-monitor-1",
        source: "workspace",
        bundleKind: "monitor_bundle",
        summary: "nginx 监控聚合面板",
      },
    });
  });

  it("flattens raw card wrappers into normalized entries with sourceCardId", () => {
    const adapted = adaptMcpUiPayloadFromCard({
      id: "workspace-mcp-card",
      source: "host-agent",
      payload: {
        cards: [
          {
            id: "summary-card",
            ui_kind: "readonly_summary",
            title: "实例摘要",
            summary: "web-08 出现 reload 抖动",
          },
        ],
        bundles: [
          {
            bundle_kind: "monitor_bundle",
            bundle_id: "nginx-monitor-1",
            subject: "nginx",
            sections: [
              {
                kind: "overview",
                cards: [
                  {
                    id: "overview-card",
                    uiKind: "readonly_summary",
                    title: "当前状态",
                  },
                ],
              },
            ],
          },
        ],
      },
    });

    expect(adapted.items).toHaveLength(2);
    expect(adapted.items[0]).toMatchObject({
      sourceCardId: "workspace-mcp-card",
      source: "host",
      kind: "mcp_ui_card",
      model: {
        id: "summary-card",
        source: "host",
      },
    });
    expect(adapted.items[1]).toMatchObject({
      sourceCardId: "workspace-mcp-card",
      source: "host",
      kind: "mcp_bundle",
      model: {
        bundleId: "nginx-monitor-1",
        source: "host",
      },
    });
  });
});
