// @ts-check
import { expect, test } from "@playwright/test";

const mobileViewport = { width: 390, height: 820 };

const sessionPayload = {
  activeSessionId: "mobile-chat-session",
  sessions: [
    {
      id: "mobile-chat-session",
      kind: "single_host",
      title: "移动端 Debug 会话",
      status: "completed",
      messageCount: 1,
      preview: "页面按钮很慢，帮我定位原因",
      selectedHostId: "server-local",
      lastActivityAt: "2026-05-12T09:12:00+08:00",
    },
  ],
};

const traceJson = {
  schemaVersion: 1,
  kind: "runtime_model_input",
  sessionId: "mobile-chat-session",
  turnId: "turn-mobile-1",
  caseId: "case-mobile-slow-button",
  iteration: 1,
  createdAt: "2026-05-12T09:12:00+08:00",
  visibleTools: ["coroot.query_latency"],
  promptFingerprint: {
    stableHash: "stable-mobile-hash",
    developerHash: "developer-mobile-hash",
    toolRegistryHash: "tools-mobile-hash",
  },
  modelInput: [
    { index: 0, providerRole: "system", semanticRole: "system", promptLayer: "system", content: "System prompt" },
    { index: 1, providerRole: "system", semanticRole: "developer", promptLayer: "developer", content: "Developer prompt" },
    {
      index: 2,
      providerRole: "user",
      semanticRole: "user",
      promptLayer: "conversation",
      content: "页面按钮很慢，帮我定位原因",
    },
    {
      index: 3,
      providerRole: "assistant",
      semanticRole: "assistant",
      promptLayer: "conversation",
      content: "我会查询 Coroot 指标并生成图表。",
      toolCalls: [
        {
          id: "tool-call-coroot-mobile",
          type: "function",
          function: { name: "coroot.query_latency" },
          llmRequestId: "llm-request-mobile-1",
        },
      ],
    },
  ],
  llmRequests: [
    {
      id: "llm-request-mobile-1",
      request_body: {
        messages: [
          { role: "system", content: "System prompt" },
          { role: "developer", content: "Developer prompt" },
          { role: "user", content: "页面按钮很慢，帮我定位原因" },
        ],
      },
      retrieval_context: "Coroot latency context",
      output: "图表已生成",
      usage: { prompt_tokens: 18, completion_tokens: 7, total_tokens: 25 },
      duration_ms: 420,
      tool_messages: [{ content: "checkout p95=2800ms" }],
    },
  ],
  artifacts: {
    "coroot-mobile-latency-chart": {
      artifact_id: "coroot-mobile-latency-chart",
      type: "coroot_chart",
      title: "Checkout p95 延迟图",
    },
  },
  agentUiArtifacts: [
    {
      artifact_id: "coroot-mobile-latency-chart",
      metadata: {
        llmRequestId: "llm-request-mobile-1",
        toolCallId: "tool-call-coroot-mobile",
        evidence_ref: "ev-mobile-latency",
        case_id: "case-mobile-slow-button",
        redactionStatus: "redacted",
      },
    },
  ],
};

const incidentPayload = {
  id: "case-mobile-slow-button",
  title: "页面按钮很慢",
  status: "active",
  severity: "medium",
  source: "debug_mode",
  environment: "prod",
  businessCapability: "checkout",
  entityId: "svc-checkout",
  summary: "Debug Mode 采集到点击按钮后的全链路 trace，Coroot 指向库存服务锁等待。",
  hypotheses: [{ id: "hyp-mobile-1", title: "库存锁等待", summary: "inventory.reserve span 占用主要耗时" }],
  evidence: [
    {
      id: "ev-mobile-latency",
      evidence_ref: "ev-mobile-latency",
      artifact_id: "coroot-mobile-latency-chart",
      title: "Coroot latency",
      summary: "p95 高于基线",
      source: "coroot",
      trace_id: "trace-mobile-slow-button",
    },
  ],
  host_profile_snapshots: [
    {
      host_id: "host-prod-07",
      display_name: "web-07",
      status: "online",
      os: "linux",
      arch: "x86_64",
      labels: { env: "prod", role: "web" },
    },
  ],
  host_leases: [
    {
      lease_id: "lease-mobile-1",
      host_id: "host-prod-07",
      status: "active",
      mission_id: "case-mobile-slow-button",
      owner_session_id: "mobile-chat-session",
      expires_at: "2026-05-12T09:40:00+08:00",
    },
  ],
  workflow_runs: [
    {
      run_id: "run-mobile-1",
      workflow_id: "wf-mobile-cache-verify",
      status: "succeeded",
      verification_refs: ["verify-mobile-1"],
    },
  ],
  verifications: [{ id: "verify-mobile-1", title: "Latency recovered", status: "passed", summary: "p95 回到基线" }],
  experience_candidates: [{ pack_id: "pack-mobile-lock", title: "库存锁等待经验包", status: "candidate" }],
};

const hostProfilesPayload = {
  items: [
    {
      host_id: "host-prod-07",
      display_name: "web-07",
      status: "online",
      os: "linux",
      arch: "x86_64",
      labels: { env: "prod", role: "web" },
      agent_version: "1.8.4",
      agent_id: "agent-prod-07",
      runtime: { os_release: "Ubuntu 24.04", kernel: "6.8.0" },
      service_runtime: { supervisor: "systemd", unit: "aiops-agent.service" },
      last_case_id: "case-mobile-slow-button",
      last_heartbeat_at: "2026-05-12T09:20:00+08:00",
      profile_expires_at: "2026-05-12T09:50:00+08:00",
    },
  ],
};

const hostLeasesPayload = {
  items: [
    {
      lease_id: "lease-mobile-1",
      host_id: "host-prod-07",
      status: "active",
      mission_id: "case-mobile-slow-button",
      owner_session_id: "mobile-chat-session",
      acquired_at: "2026-05-12T09:10:00+08:00",
      expires_at: "2026-05-12T09:40:00+08:00",
    },
  ],
};

function chatState() {
  return {
    schemaVersion: "aiops.transport.v2",
    sessionId: "mobile-chat-session",
    threadId: "mobile-chat-session",
    status: "idle",
    currentTurnId: "turn-mobile-1",
    turns: {
      "turn-mobile-1": {
        id: "turn-mobile-1",
        status: "completed",
        startedAt: "2026-05-12T09:12:00+08:00",
        completedAt: "2026-05-12T09:12:12+08:00",
        user: {
          id: "user-mobile-1",
          text: "页面按钮很慢，帮我定位原因",
          createdAt: "2026-05-12T09:12:00+08:00",
        },
        final: {
          id: "final-mobile-1",
          text: "慢请求主要耗时集中在库存服务锁等待，下面是 Coroot 证据图表。",
          status: "completed",
        },
        agentUiArtifacts: [
          {
            id: "coroot-mobile-latency-chart",
            type: "coroot_chart",
            titleZh: "Coroot 延迟趋势",
            summaryZh: "Checkout p95 延迟在 DebugEvent 后升高，峰值 2800ms。",
            caseId: "case-mobile-slow-button",
            evidenceRef: "ev-mobile-latency",
            promptTraceId: "trace-mobile-1",
            source: "coroot",
            redactionStatus: "redacted",
            inlineData: {
              mcpCard: {
                uiKind: "readonly_chart",
                title: "Checkout p95 延迟",
                visual: {
                  kind: "timeseries",
                  series: [
                    {
                      name: "p95_latency_ms",
                      data: [
                        { timestamp: 1778547600, value: 420 },
                        { timestamp: 1778548500, value: 960 },
                        { timestamp: 1778549400, value: 2800 },
                      ],
                    },
                  ],
                },
              },
            },
          },
        ],
      },
    },
    turnOrder: ["turn-mobile-1"],
    pendingApprovals: {},
    mcpSurfaces: {},
    artifacts: {},
    runtimeLiveness: {
      activeTurns: {},
      activeAgents: {},
      pendingApprovals: {},
      pendingUserInputs: {},
      activeCommandStreams: {},
    },
    seq: 8,
    updatedAt: "2026-05-12T09:12:12+08:00",
  };
}

function dataStreamForState(state) {
  return `aui-state:${JSON.stringify([{ type: "set", path: [], value: state }])}\n`;
}

async function routeSharedApis(page) {
  await page.route("**/api/v1/sessions", (route) => route.fulfill({ json: sessionPayload }));
  await page.route("**/api/v1/hosts", (route) =>
    route.fulfill({
      json: {
        items: [
          {
            id: "server-local",
            name: "server-local",
            status: "online",
            executable: true,
            terminalCapable: true,
          },
          {
            id: "host-prod-07",
            name: "web-07",
            address: "10.10.4.27",
            status: "online",
            executable: true,
            terminalCapable: true,
            agentVersion: "1.8.4",
            labels: { env: "prod", role: "web" },
          },
        ],
      },
    }),
  );
  await page.route("**/api/v1/llm-config", (route) =>
    route.fulfill({ json: { provider: "mock", model: "mobile-e2e", apiKeySet: true, bifrostActive: true } }),
  );
  await page.route("**/api/v1/terminal/sessions", (route) => route.fulfill({ json: { items: [] } }));
  await page.route("**/api/v1/assistant/resume", (route) =>
    route.fulfill({
      status: 200,
      contentType: "text/plain; charset=utf-8",
      body: dataStreamForState(chatState()),
    }),
  );
  await page.route("**/api/v1/assistant/transport", (route) =>
    route.fulfill({ status: 200, contentType: "text/plain; charset=utf-8", body: dataStreamForState(chatState()) }),
  );
  await page.route("**/api/v1/incidents/case-mobile-slow-button", (route) => route.fulfill({ json: incidentPayload }));
  await page.route("**/api/v1/opsgraph/entities/**/neighborhood**", (route) =>
    route.fulfill({
      json: {
        entity: { id: "svc-checkout", name: "Checkout", type: "service" },
        neighbors: [{ id: "svc-inventory", name: "Inventory", type: "service", status: "warning" }],
        entities: [],
        relationships: [],
        depth: 2,
      },
    }),
  );
  await page.route("**/api/v1/opsgraph/entities/**/business-impact", (route) =>
    route.fulfill({ json: { capabilities: [{ name: "checkout", impact: "下单变慢" }], tenants: [] } }),
  );
  await page.route("**/api/v1/debug/model-input-traces/file**", (route) =>
    route.fulfill({ json: { content: JSON.stringify(traceJson) } }),
  );
  await page.route("**/api/v1/debug/model-input-traces?limit=2000", (route) =>
    route.fulfill({
      json: {
        rootDir: ".data/model-input-traces",
        selectedId: "trace-mobile-1",
        traces: [
          {
            id: "trace-mobile-1",
            sessionId: "mobile-chat-session",
            turnId: "turn-mobile-1",
            caseId: "case-mobile-slow-button",
            iteration: 1,
            jsonPath: ".data/model-input-traces/mobile-chat-session/turn-mobile-1/iteration-001.json",
            markdownPath: ".data/model-input-traces/mobile-chat-session/turn-mobile-1/iteration-001.md",
            diffPath: ".data/model-input-traces/mobile-chat-session/turn-mobile-1/iteration-001.diff",
            relativePath: "mobile-chat-session/turn-mobile-1/iteration-001.json",
            promptFingerprint: traceJson.promptFingerprint,
          },
        ],
      },
    }),
  );
  await page.route("**/api/v1/host-profiles?**", (route) => route.fulfill({ json: hostProfilesPayload }));
  await page.route("**/api/v1/host-profiles/host-prod-07/report-history", (route) =>
    route.fulfill({
      json: {
        items: [
          {
            report_id: "report-mobile-1",
            host_id: "host-prod-07",
            status: "accepted",
            reported_at: "2026-05-12T09:20:00+08:00",
            summary: "CPU 8C / Memory 32GiB / Disk 400GiB",
          },
        ],
      },
    }),
  );
  await page.route("**/api/v1/host-leases?**", (route) => route.fulfill({ json: hostLeasesPayload }));
}

async function expectWithinViewport(page, locator) {
  await expect(locator).toBeVisible();
  const box = await locator.boundingBox();
  const viewport = page.viewportSize();
  expect(box).not.toBeNull();
  expect(viewport).not.toBeNull();
  if (!box || !viewport) return;
  expect(box.x).toBeGreaterThanOrEqual(-1);
  expect(box.x + box.width).toBeLessThanOrEqual(viewport.width + 1);
}

test.describe("企业级智能运维助手移动端关键流程", () => {
  test.beforeEach(async ({ page }) => {
    await page.setViewportSize(mobileViewport);
    await routeSharedApis(page);
  });

  test("AI Chat Agent-to-UI artifact 卡片不溢出", async ({ page }) => {
    await page.goto("/");

    const artifact = page.getByTestId("agent-ui-artifact").first();
    await expect(artifact).toContainText("Coroot 延迟趋势");
    await expect(artifact).toContainText("p95_latency_ms");
    await expectWithinViewport(page, artifact);
  });

  test("Case 工作台 tabs 在移动端可切换", async ({ page }) => {
    await page.goto("/incidents/case-mobile-slow-button");

    const tabs = page.getByTestId("case-stage-tabs");
    await expect(tabs).toBeVisible();
    await expect(page.getByTestId("case-header")).toContainText("Case 总览");
    await page.getByRole("tab", { name: /证据/ }).click();
    await expect(page.getByRole("tab", { name: /证据/ })).toHaveAttribute("data-state", "active");
    await page.getByRole("tab", { name: /主机环境/ }).click();
    await expect(page.getByRole("tab", { name: /主机环境/ })).toHaveAttribute("data-state", "active");
    await expectWithinViewport(page, tabs);
  });

  test("Prompt Trace 三级结构在移动端可逐级进入", async ({ page }) => {
    await page.goto("/debug/prompts");

    const traceScroller = page.getByTestId("prompt-trace-scroll");
    await expect(traceScroller).toBeVisible();
    await expect(page.getByText("历史会话")).toBeVisible();
    await page.getByText("mobile-chat-session").first().click();
    await traceScroller.evaluate((node) => { node.scrollLeft = 300; });
    await expect(page.getByText("用户请求列表")).toBeVisible();
    await page.getByRole("button", { name: /页面按钮很慢/ }).click();
    await traceScroller.evaluate((node) => { node.scrollLeft = 620; });
    await expect(page.getByText("LLM 请求列表")).toBeVisible();
    await page.getByTestId("prompt-trace-llm-card").click();
    await expect(page.getByRole("dialog", { name: "LLM 请求详情" })).toBeVisible();
    await expect(page.getByRole("heading", { name: "LLM 返回内容" })).toBeVisible();
    await expect(page.getByText("llm-request-mobile-1", { exact: true })).toBeVisible();
    await expect(page.getByText("图表已生成")).toBeVisible();
  });

  test("主机列表基础字段在移动端不重叠", async ({ page }) => {
    await page.goto("/settings/hosts");

    await expect(page.locator("main").getByText("主机列表", { exact: true })).toBeVisible();
    const table = page.locator(".hosts-table-shell").first();
    await expect(table).toContainText("主机 IP / 用户名");
    await expect(table).toContainText("基础信息");
    await expectWithinViewport(page, table);
  });
});
