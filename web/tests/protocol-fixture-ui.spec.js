// @ts-check
import { test, expect } from "@playwright/test";
import {
  createProtocolFixtureSessions,
  createProtocolFixtureState,
  openFixturePage,
} from "./helpers/uiFixtureHarness";

const SCREENSHOT_DIR = "tests/screenshots";

async function setGlobalMcpDrawerState(page, open) {
  const drawer = page.locator(".app-mcp-drawer");
  const isOpen = await drawer.evaluate((element) => element.classList.contains("is-open"));
  if (isOpen !== open) {
    await page.locator('.header-icon-btn[title="Skills & MCP"]').click();
  }

  if (open) {
    await expect(page.locator(".app-mcp-drawer.is-open")).toBeVisible();
  } else {
    await expect(page.locator(".app-mcp-drawer.is-open")).toHaveCount(0);
  }
}

async function returnToChatFromProtocol(page) {
  await page.locator(".nav-item").first().click();
  await expect(page.locator(".chat-composer-dock")).toBeVisible();
}

async function pasteTransferOnly(locator, text) {
  await locator.evaluate((element, payload) => {
    const pasteEvent = new Event("paste", { bubbles: true, cancelable: true });
    Object.defineProperty(pasteEvent, "clipboardData", {
      configurable: true,
      value: {
        getData: () => payload,
        files: [],
      },
    });
    element.dispatchEvent(pasteEvent);
  }, text);
}

async function dropImage(locator, filename) {
  await locator.evaluate((element, name) => {
    const file = new File(["img"], name, { type: "image/png" });
    const dropEvent = new Event("drop", { bubbles: true, cancelable: true });
    Object.defineProperty(dropEvent, "dataTransfer", {
      configurable: true,
      value: {
        files: [file],
        getData: () => "",
      },
    });
    element.dispatchEvent(dropEvent);
  }, filename);
}

function createSyntheticMcpSurfaceCards() {
  return [
    {
      id: "user-mcp-1",
      type: "UserMessageCard",
      role: "user",
      text: "请给我 nginx 的监控面板，并提供一个可审批的控制动作。",
      createdAt: "2026-04-03T12:30:00Z",
      updatedAt: "2026-04-03T12:30:00Z",
    },
    {
      id: "assistant-mcp-1",
      type: "AssistantMessageCard",
      role: "assistant",
      text: "我已为你聚合了监控面板，也准备了一个需要审批的控制动作。",
      payload: {
        actionSurfaces: [
          {
            id: "mcp-action-surface-1",
            placement: "inline_action",
            uiKind: "action_panel",
            source: "workspace",
            mcpServer: "metrics-prod",
            title: "MCP 控制面板",
            summary: "对 nginx 执行受控重启前，先进入右侧审批栏。",
            scope: {
              service: "nginx",
              env: "prod",
              hostId: "web-02",
            },
            freshness: {
              label: "刚拉取",
              capturedAt: "2026-04-03T12:30:05Z",
            },
            actions: [
              {
                id: "restart-nginx",
                label: "重启 nginx",
                intent: "restart_service",
                mutation: true,
                approvalMode: "required",
                confirmText: "确认后将把重启申请加入右侧审批栏。",
                permissionPath: "mcp.ops.service.restart",
                target: {
                  label: "web-02 / nginx",
                },
                params: {
                  service: "nginx",
                  host: "web-02",
                },
              },
            ],
          },
        ],
        resultBundles: [
          {
            id: "mcp-monitor-bundle-1",
            placement: "inline_final",
            bundleKind: "monitor_bundle",
            source: "workspace",
            mcpServer: "metrics-prod",
            summary: "nginx 监控聚合面板",
            subject: {
              type: "service",
              name: "nginx",
              env: "prod",
            },
            freshness: {
              label: "刚拉取",
              capturedAt: "2026-04-03T12:30:05Z",
            },
            sections: [
              {
                id: "overview-1",
                kind: "overview",
                title: "概览",
                cards: [
                  {
                    id: "overview-card-1",
                    uiKind: "readonly_summary",
                    title: "当前状态",
                    summary: "nginx 当前处于可观察状态。",
                  },
                ],
              },
              {
                id: "trends-1",
                kind: "trends",
                title: "趋势",
                cards: [
                  {
                    id: "trend-card-1",
                    uiKind: "readonly_chart",
                    title: "请求趋势",
                    summary: "请求量最近 5 分钟保持平稳。",
                  },
                ],
              },
            ],
          },
        ],
      },
      createdAt: "2026-04-03T12:30:10Z",
      updatedAt: "2026-04-03T12:30:10Z",
    },
  ];
}

function createSyntheticRemediationMcpSurfaceCards() {
  return [
    {
      id: "user-remediate-1",
      type: "UserMessageCard",
      role: "user",
      text: "redis 最近抖动，给我一个根因定位后的修复工作台。",
      createdAt: "2026-04-03T13:00:00Z",
      updatedAt: "2026-04-03T13:00:00Z",
    },
    {
      id: "assistant-remediate-1",
      type: "AssistantMessageCard",
      role: "assistant",
      text: "我整理了 remediation bundle、审批控制卡和验证面板。",
      payload: {
        resultBundles: [
          {
            id: "redis-remediation-1",
            bundleKind: "remediation_bundle",
            source: "protocol",
            mcpServer: "ops-console",
            summary: "redis 缓存命中率抖动修复面板",
            rootCause: "连接池抖动导致请求重试放大",
            confidence: "0.91",
            subject: {
              type: "service",
              name: "redis",
              env: "prod",
            },
            freshness: {
              label: "刚拉取",
              capturedAt: "2026-04-03T13:00:05Z",
            },
            recentActivities: [
              { id: "act-1", label: "已收集最近 5 分钟错误率", detail: "上升 3%" },
              { id: "act-2", label: "已核对连接池配置", detail: "超出安全阈值" },
              { id: "act-3", label: "已定位慢查询", detail: "峰值出现在 12:58" },
              { id: "act-4", label: "已生成修复建议", detail: "建议先降载再验证" },
              { id: "act-5", label: "已准备验证面板", detail: "等待执行后刷新" },
              { id: "act-6", label: "最终进度", detail: "等待你确认审批" },
            ],
            sections: [
              {
                kind: "root_cause",
                title: "根因",
                cards: [
                  {
                    id: "root-cause-card-1",
                    uiKind: "readonly_summary",
                    title: "根因说明",
                    summary: "连接池抖动放大了请求重试。",
                  },
                ],
              },
              {
                kind: "recommended_actions",
                title: "推荐操作",
                cards: [
                  {
                    id: "recommend-card-1",
                    uiKind: "action_panel",
                    title: "推荐控制卡",
                    summary: "先进行受控重启。",
                    scope: {
                      service: "redis",
                      env: "prod",
                    },
                    actions: [
                      {
                        id: "restart-redis",
                        label: "重启 redis",
                        intent: "restart_service",
                        mutation: true,
                        approvalMode: "required",
                        confirmText: "确认后将进入审批并执行 redis 重启。",
                        permissionPath: "mcp.ops.service.restart",
                        target: {
                          label: "web-02 / redis",
                        },
                      },
                    ],
                  },
                ],
              },
              {
                kind: "control_panels",
                title: "控制面板",
                cards: [
                  {
                    id: "control-card-1",
                    uiKind: "action_panel",
                    title: "重启控制面板",
                    summary: "这张卡会直接触发审批路径。",
                    scope: {
                      service: "redis",
                      hostId: "web-02",
                    },
                    action: {
                      id: "restart-redis-control",
                      label: "重启 redis",
                      intent: "restart_service",
                      mutation: true,
                      approvalMode: "required",
                      confirmText: "确认后将把重启申请加入右侧审批栏。",
                      permissionPath: "mcp.ops.service.restart",
                      target: {
                        label: "web-02 / redis",
                      },
                    },
                  },
                ],
              },
              {
                kind: "validation_panels",
                title: "验证面板",
                cards: [
                  {
                    id: "validation-card-1",
                    uiKind: "readonly_chart",
                    title: "验证结果",
                    summary: "检查结果会在刷新后更新。",
                    freshness: {
                      label: "刚拉取",
                      capturedAt: "2026-04-03T13:00:05Z",
                    },
                    visual: {
                      kind: "table",
                      columns: ["指标", "当前值", "阈值"],
                      rows: [
                        ["错误率", "1.2%", "< 2%"],
                        ["P95", "180ms", "< 220ms"],
                      ],
                    },
                  },
                ],
              },
            ],
          },
        ],
      },
      createdAt: "2026-04-03T13:00:10Z",
      updatedAt: "2026-04-03T13:00:10Z",
    },
  ];
}

test.describe("Protocol fixture UI smoke", () => {
  test("approval details stay in the side rail while the main thread stays compact", async ({ page }) => {
    await openFixturePage(page, "/protocol", {
      state: createProtocolFixtureState(),
      sessions: createProtocolFixtureSessions(),
    });

    const processFold = page.getByTestId("protocol-process-fold-turn-user-1");
    await expect(processFold).toContainText("审批详情已收进右侧审批面板");
    await expect(processFold).toContainText("web-02 正在等待审批");
    await expect(processFold).not.toContainText("等待 reload 审批");
    await expect(processFold).not.toContainText("执行 systemctl reload nginx");
    await expect(page.getByTestId("protocol-approval-approval-card-1")).toContainText("systemctl reload nginx");

    await page.screenshot({
      path: `${SCREENSHOT_DIR}/protocol-fixture-approval-boundary.png`,
      fullPage: false,
    });
  });

  test("background agents stay docked above the composer instead of flowing back into the main thread", async ({ page }) => {
    await openFixturePage(page, "/protocol", {
      state: createProtocolFixtureState(),
      sessions: createProtocolFixtureSessions(),
    });

    const backgroundCard = page.locator(".protocol-composer-widgets .protocol-background-agents-card");
    const thread = page.locator(".protocol-turn-stream");

    await expect(backgroundCard).toContainText("后台 Agent");
    await expect(backgroundCard).toContainText("web-01");
    await expect(backgroundCard).toContainText("web-02");
    await expect(thread).not.toContainText("采集错误日志并回传摘要");
    await expect(thread).not.toContainText("执行 systemctl reload nginx");

    await page.screenshot({
      path: `${SCREENSHOT_DIR}/protocol-fixture-background-agent-boundary.png`,
      fullPage: false,
    });
  });

  test("background agent opens an agent-centric detail modal instead of host execution detail", async ({ page }) => {
    await openFixturePage(page, "/protocol", {
      state: createProtocolFixtureState(),
      sessions: createProtocolFixtureSessions(),
    });

    const backgroundCard = page.locator(".protocol-composer-widgets .protocol-background-agents-card");
    const firstAgent = backgroundCard.locator(".background-agent").first();
    await expect(firstAgent).toBeVisible();
    await firstAgent.click();

    const modal = page.locator(".protocol-agent-detail-modal");
    await expect(modal).toBeVisible();
    await expect(modal).toContainText("BACKGROUND AGENT");
    await expect(modal).toContainText("分配任务信息");
    await expect(modal).toContainText("与 AI 的对话信息");
    await expect(modal).toContainText("审核信息");
    await expect(modal).toContainText("当前状态 / 最近活动");
    await expect(modal).toContainText(/采集 nginx 错误日志|执行 systemctl reload nginx/);
    await expect(modal).not.toContainText("执行详情 · agent-local");
    await expect(modal).not.toContainText("执行详情 · web-01");
    await expect(modal).not.toContainText("命令执行详情 · web-01");

    await page.screenshot({
      path: `${SCREENSHOT_DIR}/protocol-fixture-background-agent-detail.png`,
      fullPage: false,
    });
  });

  test("synthetic MCP action surfaces project into the rail and timeline", async ({ page }) => {
    const stateRequests = [];
    page.on("request", (request) => {
      if (request.url().includes("/api/v1/state")) {
        stateRequests.push(request.url());
      }
    });

    await openFixturePage(page, "/protocol", {
      state: createProtocolFixtureState({
        approvals: [],
        cards: createSyntheticMcpSurfaceCards(),
        runtime: {
          turn: { active: true, phase: "waiting_input", hostId: "server-local" },
          codex: { status: "connected", retryAttempt: 0, retryMax: 5 },
          activity: {},
        },
      }),
      sessions: createProtocolFixtureSessions({
        sessions: [
          {
            id: "workspace-1",
            kind: "workspace",
            title: "Nginx workspace",
            status: "running",
            messageCount: 5,
            preview: "我想知道 nginx 中间件的情况",
            selectedHostId: "server-local",
            lastActivityAt: "2026-04-03T11:00:40Z",
          },
          {
            id: "workspace-0",
            kind: "workspace",
            title: "旧工作台",
            status: "completed",
            messageCount: 7,
            preview: "上一次多主机协作排障",
            selectedHostId: "server-local",
            lastActivityAt: "2026-04-03T10:40:00Z",
          },
          {
            id: "single-1",
            kind: "single_host",
            title: "web-01 单机会话",
            status: "completed",
            messageCount: 4,
            preview: "这是一条单机历史，不该出现在工作台历史里",
            selectedHostId: "web-01",
            lastActivityAt: "2026-04-03T10:20:00Z",
          },
        ],
      }),
    });

    const initialStateRequestCount = stateRequests.length;

    await page.evaluate(() => {
      window.__mcpDrawerEvents = [];
      window.addEventListener("codex:open-mcp-drawer", (event) => {
        window.__mcpDrawerEvents.push(event.detail);
      });
    });

    const turn = page.getByTestId("protocol-turn-turn-user-mcp-1");
    await expect(turn).toContainText("MCP 控制面板");
    await expect(turn).toContainText("nginx 监控聚合面板");
    await expect(page.getByTestId("mcp-control-panel-card")).toBeVisible();
    await expect(page.getByTestId("mcp-bundle-subject")).toContainText("nginx / prod");
    await expect(page.getByTestId("mcp-control-panel-action")).toContainText("重启 nginx");

    await turn.getByTestId("mcp-bundle-action").click();
    await expect.poll(() => stateRequests.length).toBeGreaterThan(initialStateRequestCount);

    await turn.getByTestId("mcp-bundle-pin").click();
    await expect.poll(async () => page.evaluate(() => window.__mcpDrawerEvents.length)).toBe(1);
    await expect.poll(async () => page.evaluate(() => window.__mcpDrawerEvents[0])).toMatchObject({
      source: "protocol-mcp-surface",
      pin: true,
      surface: {
        kind: "bundle",
      },
    });
    await expect(page.locator(".app-mcp-drawer.is-open")).toBeVisible();
    await expect(page.getByTestId("app-mcp-active-surface")).toContainText("nginx 监控聚合面板");

    await turn.getByTestId("mcp-bundle-open-detail").click();
    await expect(page.locator(".protocol-evidence-modal")).toContainText("MCP 面板");
    await expect(page.locator(".protocol-evidence-modal")).toContainText("nginx / prod");
    await expect(page.locator(".modal-tab.active")).toContainText("MCP 面板");
    await page.locator(".protocol-evidence-modal .close-btn").click();

    await page.getByTestId("mcp-control-panel-action").click();

    const approvalRail = page.getByTestId("protocol-approval-rail");
    const eventTimeline = page.getByTestId("protocol-event-timeline");

    await expect(approvalRail).toContainText("重启 nginx");
    await expect(approvalRail).toContainText("待处理");
    await expect(page.locator(".approval-card")).toHaveCount(1);
    await expect(eventTimeline).toContainText("重启 nginx 已进入审批队列");
  });

  test("remediation bundles keep recent activities visible and refresh validation panels in place", async ({ page }) => {
    const stateRequests = [];
    page.on("request", (request) => {
      if (request.url().includes("/api/v1/state")) {
        stateRequests.push(request.url());
      }
    });

    await openFixturePage(page, "/protocol", {
      state: createProtocolFixtureState({
        approvals: [],
        cards: createSyntheticRemediationMcpSurfaceCards(),
        runtime: {
          turn: { active: true, phase: "waiting_input", hostId: "server-local" },
          codex: { status: "connected", retryAttempt: 0, retryMax: 5 },
          activity: {},
        },
      }),
      sessions: createProtocolFixtureSessions(),
    });

    await expect(page.locator(".mcp-bundle-host")).toHaveCount(1);
    await expect(page.locator(".mcp-remediation-bundle-card")).toHaveCount(1);
    await expect(page.getByTestId("mcp-bundle-subject")).toContainText("redis / prod");
    await expect(page.getByTestId("mcp-bundle-recent-activity-strip")).toBeVisible();
    await expect(page.getByTestId("mcp-bundle-recent-activity-strip")).toContainText("最终进度");
    await expect(page.getByTestId("mcp-bundle-section-validation_panels")).toHaveCount(0);
    await expect(page.getByTestId("mcp-bundle-expand-more")).toContainText("展开剩余 2 个分区");

    const initialStateRequestCount = stateRequests.length;

    await page.getByTestId("mcp-bundle-expand-more").click();
    await expect(page.getByTestId("mcp-bundle-section-control_panels")).toBeVisible();
    await expect(page.getByTestId("mcp-bundle-section-validation_panels")).toBeVisible();
    await expect(page.getByTestId("mcp-bundle-section-control_panels").getByTestId("mcp-control-panel-card")).toBeVisible();

    await page.getByTestId("mcp-bundle-section-control_panels").getByTestId("mcp-control-panel-action").click();
    await expect(page.getByTestId("protocol-approval-rail")).toContainText("待处理");
    await expect(page.locator(".approval-card")).toHaveCount(1);
    await expect(page.getByTestId("protocol-event-timeline")).toContainText("已进入审批队列");
    // TODO: current page rendering keeps the approval rail copy generic here.
    // Once the approval-card title is threaded through the page layer, assert the action label.
    await expect.poll(() => stateRequests.length).toBe(initialStateRequestCount);

    await page
      .getByTestId("mcp-bundle-section-validation_panels")
      .getByTestId("mcp-card-refresh")
      .click();
    await expect.poll(() => stateRequests.length).toBeGreaterThan(initialStateRequestCount);
  });

  test("plan projection does not duplicate host chips before structured mapping is ready", async ({ page }) => {
    const state = createProtocolFixtureState();
    state.cards = state.cards.map((card) => {
      if (card.type !== "PlanCard") return card;
      return {
        ...card,
        items: [],
        detail: {
          ...card.detail,
          structured_process: [],
          task_host_bindings: [],
        },
      };
    });

    await openFixturePage(page, "/protocol", {
      state,
      sessions: createProtocolFixtureSessions(),
    });

    await expect(page.locator(".protocol-inline-plan-widget")).toContainText("step -> host-agent 映射");
    await expect(page.locator(".protocol-inline-plan-widget .plan-host-pill")).toHaveCount(0);

    await page.screenshot({
      path: `${SCREENSHOT_DIR}/protocol-fixture-plan-projection.png`,
      fullPage: false,
    });
  });

  test("completed turns stay collapsed by default until the user expands them", async ({ page }) => {
    const state = createProtocolFixtureState({
      approvals: [],
      cards: [
        {
          id: "user-1",
          type: "UserMessageCard",
          role: "user",
          text: "帮我汇总上一轮 nginx 巡检结果",
          createdAt: "2026-04-03T12:00:00Z",
          updatedAt: "2026-04-03T12:00:00Z",
        },
        {
          id: "assistant-1a",
          type: "AssistantMessageCard",
          role: "assistant",
          text: "我先整理刚才收集到的证据。",
          createdAt: "2026-04-03T12:00:10Z",
          updatedAt: "2026-04-03T12:00:10Z",
        },
        {
          id: "assistant-1b",
          type: "AssistantMessageCard",
          role: "assistant",
          text: "结论是 service-a 的 upstream timeout 导致告警抖动。",
          createdAt: "2026-04-03T12:00:30Z",
          updatedAt: "2026-04-03T12:00:30Z",
        },
      ],
      runtime: {
        turn: { active: false, phase: "completed", hostId: "server-local" },
        codex: { status: "connected", retryAttempt: 0, retryMax: 5 },
        activity: {},
      },
    });

    await openFixturePage(page, "/protocol", {
      state,
      sessions: createProtocolFixtureSessions(),
    });

    const toggle = page.locator('[data-testid="protocol-process-fold-turn-user-1"] .protocol-process-toggle');
    await expect(page.getByTestId("protocol-turn-turn-user-1")).toContainText("结论是 service-a 的 upstream timeout 导致告警抖动。");
    await expect(toggle).toHaveAttribute("aria-expanded", "false");
    await expect(page.locator('[data-testid="protocol-process-item-assistant-1a-process-0"]')).toHaveCount(0);

    await page.screenshot({
      path: `${SCREENSHOT_DIR}/protocol-fixture-completed-turn-collapsed.png`,
      fullPage: false,
    });

    await toggle.click();
    await expect(toggle).toHaveAttribute("aria-expanded", "true");
    await expect(page.getByTestId("protocol-process-item-assistant-1a-process-0")).toContainText("我先整理刚才收集到的证据");
  });

  test("history pagination shows a compact boundary, loads older turns, and exposes a full-history entry", async ({ page }) => {
    const cards = [];
    for (let index = 1; index <= 10; index += 1) {
      cards.push(
        {
          id: `user-${index}`,
          type: "UserMessageCard",
          role: "user",
          text: `历史问题 ${index}`,
          createdAt: `2026-04-03T08:0${index}:00Z`,
          updatedAt: `2026-04-03T08:0${index}:00Z`,
        },
        {
          id: `assistant-${index}`,
          type: "AssistantMessageCard",
          role: "assistant",
          text: `历史结果 ${index}`,
          createdAt: `2026-04-03T08:0${index}:30Z`,
          updatedAt: `2026-04-03T08:0${index}:30Z`,
        },
      );
    }

    await openFixturePage(page, "/protocol", {
      state: createProtocolFixtureState({
        approvals: [],
        cards,
        runtime: {
          turn: { active: false, phase: "completed", hostId: "server-local" },
          codex: { status: "connected", retryAttempt: 0, retryMax: 5 },
          activity: {},
        },
      }),
      sessions: createProtocolFixtureSessions({
        sessions: [
          {
            id: "workspace-1",
            kind: "workspace",
            title: "Nginx workspace",
            status: "running",
            messageCount: 5,
            preview: "我想知道 nginx 中间件的情况",
            selectedHostId: "server-local",
            lastActivityAt: "2026-04-03T11:00:40Z",
          },
          {
            id: "workspace-0",
            kind: "workspace",
            title: "旧工作台",
            status: "completed",
            messageCount: 7,
            preview: "上一次多主机协作排障",
            selectedHostId: "server-local",
            lastActivityAt: "2026-04-03T10:40:00Z",
          },
          {
            id: "single-1",
            kind: "single_host",
            title: "web-01 单机会话",
            status: "completed",
            messageCount: 4,
            preview: "这是一条单机历史，不该出现在工作台历史里",
            selectedHostId: "web-01",
            lastActivityAt: "2026-04-03T10:20:00Z",
          },
        ],
      }),
    });

    await page.getByTestId("protocol-history-sentinel").waitFor({ state: "visible" });
    await expect(page.getByTestId("protocol-history-sentinel")).toContainText("更早上下文已折叠");
    await expect(page.getByTestId("protocol-history-load-older")).toContainText("加载更早消息");
    await expect(page.getByTestId("protocol-history-open")).toContainText("查看完整历史");
    await expect(page.getByTestId("protocol-turn-turn-user-9")).toBeVisible();
    await expect(page.getByTestId("protocol-turn-turn-user-1")).toHaveCount(0);

    await page.screenshot({
      path: `${SCREENSHOT_DIR}/protocol-fixture-history-boundary.png`,
      fullPage: false,
    });

    await page.getByTestId("protocol-history-load-older").click();
    await expect(page.getByTestId("protocol-history-sentinel")).toContainText("已到会话开头");
    await expect(page.getByTestId("protocol-turn-turn-user-1")).toBeVisible();
    await page.getByTestId("protocol-history-open").click();
    const drawer = page.locator(".session-history-drawer");
    await expect(drawer).toBeVisible();
    await expect(drawer).toContainText("历史工作台");
    await expect(drawer).toContainText("主 Agent 会话");
    await expect(drawer).toContainText("Nginx workspace");
    await expect(drawer).toContainText("旧工作台");
    await expect(drawer).not.toContainText("web-01 单机会话");
  });

  test("long protocol threads virtualize turn groups without dropping the compact boundary", async ({ page }) => {
    const cards = [];
    for (let index = 1; index <= 30; index += 1) {
      cards.push(
        {
          id: `user-${index}`,
          type: "UserMessageCard",
          role: "user",
          text: `虚拟滚动问题 ${index}`,
          createdAt: `2026-04-03T09:${String(index).padStart(2, "0")}:00Z`,
          updatedAt: `2026-04-03T09:${String(index).padStart(2, "0")}:00Z`,
        },
        {
          id: `assistant-${index}`,
          type: "AssistantMessageCard",
          role: "assistant",
          text: `虚拟滚动结果 ${index}`,
          createdAt: `2026-04-03T09:${String(index).padStart(2, "0")}:30Z`,
          updatedAt: `2026-04-03T09:${String(index).padStart(2, "0")}:30Z`,
        },
      );
    }

    await openFixturePage(page, "/protocol", {
      state: createProtocolFixtureState({
        approvals: [],
        cards,
        runtime: {
          turn: { active: false, phase: "completed", hostId: "server-local" },
          codex: { status: "connected", retryAttempt: 0, retryMax: 5 },
          activity: {},
        },
      }),
      sessions: createProtocolFixtureSessions(),
    });

    const renderedTurns = page.locator('[data-testid^="protocol-turn-turn-"]');
    await expect(page.getByTestId("protocol-history-sentinel")).toContainText("更早上下文已折叠");
    await expect(page.getByTestId("protocol-history-load-older")).toBeVisible();

    for (let loadCount = 0; loadCount < 3; loadCount += 1) {
      await page.getByTestId("protocol-history-load-older").click();
      await page.waitForTimeout(50);
    }

    await page.locator(".protocol-chat-container").evaluate((element) => {
      element.scrollTop = element.scrollHeight;
      element.dispatchEvent(new Event("scroll", { bubbles: true }));
    });

    await expect(page.getByTestId("protocol-turn-turn-1")).toHaveCount(0);
    await expect(page.getByTestId("protocol-history-sentinel")).toContainText("更早上下文已折叠");
    expect(await renderedTurns.count()).toBeLessThan(15);
  });

  test("path-like paste keeps the omnibar recovery hint visible after focus returns", async ({ page }) => {
    await openFixturePage(page, "/protocol", {
      state: createProtocolFixtureState({
        approvals: [],
        runtime: {
          turn: { active: false, phase: "completed", hostId: "server-local" },
          codex: { status: "connected", retryAttempt: 0, retryMax: 5 },
          activity: {},
        },
      }),
      sessions: createProtocolFixtureSessions(),
    });

    const input = page.getByTestId("omnibar-input");

    await pasteTransferOnly(
      input,
      [
        "/Users/lizhongxuan/Desktop/logs/nginx/error.log",
        "/Users/lizhongxuan/Desktop/logs/nginx/access.log",
      ].join("\n"),
    );

    await expect(page.getByTestId("omnibar-attachment-indicator")).toContainText("2 个路径");
    await expect(page.getByTestId("omnibar-artifact-pill")).toContainText("路径 2");
    await expect(input).toHaveValue("");

    await input.blur();
    await input.click();

    await expect(page.getByTestId("omnibar-focus-hint")).toContainText("已恢复输入焦点");
    await expect(page.getByTestId("omnibar-focus-hint")).toContainText("路径仍待处理");
  });

  test("image paste keeps the lightweight artifact hint visible after focus returns", async ({ page }) => {
    await openFixturePage(page, "/protocol", {
      state: createProtocolFixtureState({
        approvals: [],
        runtime: {
          turn: { active: false, phase: "completed", hostId: "server-local" },
          codex: { status: "connected", retryAttempt: 0, retryMax: 5 },
          activity: {},
        },
      }),
      sessions: createProtocolFixtureSessions(),
    });

    const input = page.getByTestId("omnibar-input");

    await dropImage(input, "nginx-error.png");

    await expect(page.getByTestId("omnibar-attachment-indicator")).toContainText("1 张图片");
    await expect(page.getByTestId("omnibar-artifact-pill")).toContainText("图片 1");

    await input.blur();
    await input.click();

    await expect(page.getByTestId("omnibar-focus-hint")).toContainText("已恢复输入焦点");
    await expect(page.getByTestId("omnibar-focus-hint")).toContainText("图片仍待处理");
    await expect(page.getByTestId("omnibar-primary-action")).toBeDisabled();
  });

  test("pinned MCP surfaces survive chat navigation and drawer reopen", async ({ page }) => {
    await openFixturePage(page, "/protocol", {
      state: createProtocolFixtureState({
        cards: createSyntheticMcpSurfaceCards(),
        approvals: [],
        runtime: {
          turn: { active: true, phase: "waiting_input", hostId: "server-local" },
          codex: { status: "connected", retryAttempt: 0, retryMax: 5 },
          activity: {},
        },
      }),
      sessions: createProtocolFixtureSessions(),
    });

    const turn = page.getByTestId("protocol-turn-turn-user-mcp-1");
    await turn.getByTestId("mcp-bundle-pin").click();

    await expect(page.locator(".app-mcp-drawer.is-open")).toBeVisible();
    await expect(page.getByTestId("app-mcp-active-surface")).toContainText("nginx 监控聚合面板");
    await expect(page.getByTestId("app-mcp-pinned-list")).toContainText("常驻面板");
    await expect(page.locator(".mcp-pinned-item.active")).toContainText("nginx 监控聚合面板");

    await returnToChatFromProtocol(page);

    await setGlobalMcpDrawerState(page, false);
    await setGlobalMcpDrawerState(page, true);
    await expect(page.getByTestId("app-mcp-active-surface")).toContainText("nginx 监控聚合面板");
    await expect(page.locator(".mcp-pinned-item.active")).toContainText("nginx 监控聚合面板");
  });
});
