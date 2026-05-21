import { expect, test } from "@playwright/test";

const transportState = {
  schemaVersion: "aiops.transport.v2",
  sessionId: "browser-flow-session",
  threadId: "browser-flow-session",
  status: "idle",
  currentTurnId: "turn-browser-flow",
  turns: {
    "turn-browser-flow": {
      id: "turn-browser-flow",
      status: "completed",
      startedAt: "2026-05-08T02:00:00.000Z",
      completedAt: "2026-05-08T02:00:12.000Z",
      user: {
        id: "user-browser-flow",
        text: "测试一下 aiops-v2 对话时，工具跟对话的顺序是否正确。",
        createdAt: "2026-05-08T02:00:00.000Z",
      },
      process: [
        {
          id: "assistant-next",
          kind: "assistant",
          status: "completed",
          text: "接下来我要检查运行环境和最近任务状态。",
          updatedAt: "2026-05-08T02:00:01.000Z",
        },
        {
          id: "cmd-order-1",
          kind: "command",
          status: "completed",
          text: "pwd",
          command: "pwd",
          outputPreview: "/Users/lizhongxuan/Desktop/aiops-v2",
          updatedAt: "2026-05-08T02:00:03.000Z",
        },
        {
          id: "cmd-order-2",
          kind: "command",
          status: "completed",
          text: "git status --short",
          command: "git status --short",
          outputPreview: "",
          updatedAt: "2026-05-08T02:00:04.000Z",
        },
        {
          id: "assistant-after-commands",
          kind: "assistant",
          status: "completed",
          text: "命令结果已经拿到，我会继续核对相关页面信息。",
          updatedAt: "2026-05-08T02:00:05.000Z",
        },
        {
          id: "search-order-1",
          kind: "tool",
          displayKind: "web_search",
          status: "completed",
          text: "web_search",
          inputSummary: "aiops-v2 AssistantTransport 顺序",
          queries: ["aiops-v2 AssistantTransport 顺序"],
          updatedAt: "2026-05-08T02:00:07.000Z",
        },
        {
          id: "search-order-2",
          kind: "tool",
          displayKind: "browse_url",
          status: "completed",
          text: "browse_url",
          inputSummary: "https://example.com/aiops-v2-order",
          updatedAt: "2026-05-08T02:00:08.000Z",
        },
        {
          id: "assistant-after-search",
          kind: "assistant",
          status: "completed",
          text: "页面也确认过了，最终回答会基于上面的命令和搜索结果。",
          updatedAt: "2026-05-08T02:00:10.000Z",
        },
      ],
      final: {
        id: "final-browser-flow",
        text: "流程验证完成：对话、命令组、对话、搜索组和后续对话按预期顺序显示。",
        status: "completed",
      },
    },
  },
  turnOrder: ["turn-browser-flow"],
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
  updatedAt: "2026-05-08T02:00:12.000Z",
};

const sessionsPayload = {
  activeSessionId: "browser-flow-session",
  sessions: [
    {
      id: "browser-flow-session",
      kind: "single_host",
      title: "Browser flow",
      status: "completed",
      messageCount: 1,
      preview: "测试一下 aiops-v2 对话时，工具跟对话的顺序是否正确。",
      selectedHostId: "server-local",
      lastActivityAt: "2026-05-08T02:00:12.000Z",
    },
  ],
};

const markdownFinalText = [
  "本机资源总览如下：",
  "",
  "- CPU",
  "  - 型号：Apple M5",
  "  - 当前使用率：user 10.99%，sys 15.54%，idle 73.45%",
  "  - Load Average：2.88 / 2.84 / 2.92",
  "- 内存",
  "  - 总内存：32 GB",
  "  - 当前物理内存：31 GB 已用，552 MB 未用",
  "  - Compressor：约 7.3 GB",
  "- 磁盘",
  "  - 系统盘容量：460 Gi",
  "  - 使用率：49%",
  "",
  "数据为实时快照。",
].join("\n");

const runningPreludeText = "我先复查主机当前的 CPU、内存、磁盘和负载情况，再给你一个最新快照。";
const runningPreludeStartedAt = "2026-05-08T02:00:00.000Z";
const runningPreludeRenderedAt = "2026-05-08T02:00:01.000Z";

function finalMarkdownState(status) {
  const running = status === "working";
  return {
    schemaVersion: "aiops.transport.v2",
    sessionId: `markdown-final-${status}`,
    threadId: `markdown-final-${status}`,
    status: running ? "working" : "idle",
    currentTurnId: `turn-markdown-final-${status}`,
    turns: {
      [`turn-markdown-final-${status}`]: {
        id: `turn-markdown-final-${status}`,
        status,
        startedAt: "2026-05-08T02:00:00.000Z",
        completedAt: running ? undefined : "2026-05-08T02:00:12.000Z",
        updatedAt: "2026-05-08T02:00:12.000Z",
        user: {
          id: `user-markdown-final-${status}`,
          text: "看下本机的资源情况",
          createdAt: "2026-05-08T02:00:00.000Z",
        },
        process: [
          {
            id: `cmd-markdown-final-${status}`,
            kind: "command",
            status: running ? "running" : "completed",
            text: "top -l 1",
            command: "top -l 1",
            outputPreview: "CPU usage: 10.99% user, 15.54% sys, 73.45% idle",
            updatedAt: "2026-05-08T02:00:05.000Z",
          },
          {
            id: `assistant-markdown-final-${status}`,
            kind: "assistant",
            displayKind: "assistant.final",
            status: running ? "running" : "completed",
            text: markdownFinalText,
            updatedAt: "2026-05-08T02:00:12.000Z",
          },
        ],
        final: {
          id: `final-markdown-${status}`,
          text: markdownFinalText,
          status: running ? "running" : "completed",
        },
      },
    },
    turnOrder: [`turn-markdown-final-${status}`],
    pendingApprovals: {},
    mcpSurfaces: {},
    artifacts: {},
    runtimeLiveness: {
      activeTurns: running ? { [`turn-markdown-final-${status}`]: true } : {},
      activeAgents: {},
      pendingApprovals: {},
      pendingUserInputs: {},
      activeCommandStreams: {},
    },
    seq: running ? 4 : 8,
    updatedAt: "2026-05-08T02:00:12.000Z",
  };
}

function runningPreludeBeforeToolsState() {
  return {
    schemaVersion: "aiops.transport.v2",
    sessionId: "running-prelude-before-tools",
    threadId: "running-prelude-before-tools",
    status: "working",
    currentTurnId: "turn-running-prelude-before-tools",
    turns: {
      "turn-running-prelude-before-tools": {
        id: "turn-running-prelude-before-tools",
        status: "working",
        startedAt: runningPreludeStartedAt,
        updatedAt: runningPreludeStartedAt,
        user: {
          id: "user-running-prelude-before-tools",
          text: "再看下主机资源",
          createdAt: runningPreludeStartedAt,
        },
        process: [],
        final: {
          id: "final-running-prelude-before-tools",
          text: runningPreludeText,
          status: "running",
        },
      },
    },
    turnOrder: ["turn-running-prelude-before-tools"],
    pendingApprovals: {},
    mcpSurfaces: {},
    artifacts: {},
    runtimeLiveness: {
      activeTurns: { "turn-running-prelude-before-tools": true },
      activeAgents: {},
      pendingApprovals: {},
      pendingUserInputs: {},
      activeCommandStreams: {},
    },
    seq: 3,
    updatedAt: "2026-05-08T02:00:04.000Z",
  };
}

function longTerminalOutputState() {
  const outputPreview = [
    "RSS   PID COMM",
    ...Array.from({ length: 80 }, (_, index) => {
      const line = index + 1;
      return `${String(100000 + line).padStart(6, " ")} ${String(64000 + line).padStart(5, " ")} process-${line}`;
    }),
  ].join("\n");
  return {
    schemaVersion: "aiops.transport.v2",
    sessionId: "long-terminal-output",
    threadId: "long-terminal-output",
    status: "idle",
    currentTurnId: "turn-long-terminal-output",
    turns: {
      "turn-long-terminal-output": {
        id: "turn-long-terminal-output",
        status: "completed",
        startedAt: "2026-05-08T02:00:00.000Z",
        completedAt: "2026-05-08T02:00:12.000Z",
        user: {
          id: "user-long-terminal-output",
          text: "看下进程内存",
          createdAt: "2026-05-08T02:00:00.000Z",
        },
        process: [
          {
            id: "cmd-long-terminal-output",
            kind: "command",
            status: "completed",
            text: "ps -arc -o rss,pid,comm",
            command: "ps -arc -o rss,pid,comm",
            outputPreview,
            updatedAt: "2026-05-08T02:00:05.000Z",
          },
        ],
        final: {
          id: "final-long-terminal-output",
          text: "进程列表已获取。",
          status: "completed",
        },
      },
    },
    turnOrder: ["turn-long-terminal-output"],
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
    updatedAt: "2026-05-08T02:00:12.000Z",
  };
}

const rcaReportTransportState = {
  schemaVersion: "aiops.transport.v2",
  sessionId: "rca-report-session",
  threadId: "rca-report-session",
  status: "idle",
  currentTurnId: "turn-rca",
  turns: {
    "turn-rca": {
      id: "turn-rca",
      status: "completed",
      startedAt: "2026-05-15T02:00:00.000Z",
      completedAt: "2026-05-15T02:00:12.000Z",
      user: {
        id: "user-rca",
        text: "分析 checkout 服务最近 30 分钟延迟升高的根因",
        createdAt: "2026-05-15T02:00:00.000Z",
      },
      process: [
        {
          id: "tool-coroot-context",
          kind: "tool",
          displayKind: "coroot.collect_rca_context",
          status: "completed",
          text: "coroot.collect_rca_context",
          updatedAt: "2026-05-15T02:00:04.000Z",
        },
        {
          id: "tool-artifact",
          kind: "tool",
          displayKind: "rca_report",
          status: "completed",
          text: "aiops.ui_artifact_emit",
          updatedAt: "2026-05-15T02:00:08.000Z",
        },
      ],
      agentUiArtifacts: [
        {
          id: "artifact-rca-report",
          type: "rca_report",
          titleZh: "checkout 根因分析",
          summaryZh: "checkout 延迟升高最可能来自 catalog 依赖。",
          status: "ok",
          severity: "high",
          source: "coroot",
          permissionScope: "read",
          redactionStatus: "redacted",
          inlineData: {
            schemaVersion: "aiops.rca_report/v1",
            source: "coroot",
            status: "ok",
            target: { service: "checkout" },
            window: { timeRange: "30m" },
            conclusion: {
              summaryZh: "checkout 延迟升高最可能来自 catalog 依赖。",
              rootCauseEntity: "catalog",
              confidence: 0.72,
            },
            hypotheses: [
              {
                id: "hyp-1",
                titleZh: "catalog 依赖延迟",
                confidence: 0.72,
                supportingEvidenceRefs: ["ev-coroot-latency"],
                contradictingEvidenceRefs: [],
                missingEvidence: [],
              },
            ],
            sections: [
              {
                id: "propagation",
                kind: "propagation_map",
                titleZh: "传播路径",
                evidenceRefs: ["ev-coroot-latency"],
                payload: {
                  nodes: [{ id: "checkout" }, { id: "catalog" }],
                  edges: [{ source: "checkout", target: "catalog" }],
                },
              },
              {
                id: "metrics",
                kind: "timeseries_grid",
                titleZh: "关键指标",
                evidenceRefs: ["ev-coroot-latency"],
                payload: {
                  metrics: [
                    {
                      name: "latency_p99",
                      entity: "checkout->catalog",
                      valueSummary: "p99 rose to 1.8s",
                      status: "critical",
                    },
                  ],
                },
              },
            ],
            evidenceRefs: ["ev-coroot-latency"],
            rawRefs: [{ source: "coroot", uri: "coroot://project/default/checkout" }],
            limitations: [],
          },
        },
      ],
      final: {
        id: "final-rca",
        text: "RCA 初步完成，最强假设是 catalog 依赖延迟传播到 checkout。",
        status: "completed",
      },
    },
  },
  turnOrder: ["turn-rca"],
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
  seq: 1,
  updatedAt: "2026-05-15T02:00:12.000Z",
};

function artifactTransportState(sessionId, userText, artifact) {
  const artifacts = Array.isArray(artifact) ? artifact : [artifact];
  const turnId = `turn-${sessionId}`;
  return {
    schemaVersion: "aiops.transport.v2",
    sessionId,
    threadId: sessionId,
    status: "idle",
    currentTurnId: turnId,
    turns: {
      [turnId]: {
        id: turnId,
        status: "completed",
        startedAt: "2026-05-19T10:00:00.000Z",
        completedAt: "2026-05-19T10:00:08.000Z",
        updatedAt: "2026-05-19T10:00:08.000Z",
        user: {
          id: `user-${sessionId}`,
          text: userText,
          createdAt: "2026-05-19T10:00:00.000Z",
        },
        process: [],
        agentUiArtifacts: artifacts,
        final: {
          id: `final-${sessionId}`,
          text: "已完成运维手册判定。",
          status: "completed",
        },
      },
    },
    turnOrder: [turnId],
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
    seq: 1,
    updatedAt: "2026-05-19T10:00:08.000Z",
  };
}

function opsManualMergedParamConfirmationState() {
  return artifactTransportState(
    "ops-manual-merged-param-confirmation",
    "按运维手册判断：对 MySQL 做备份",
    [
      {
        id: "artifact-ops-manual-mysql-search",
        type: "ops_manual_search_result",
        titleZh: "运维手册检索",
        source: "tool:search_ops_manuals",
        redactionStatus: "redacted",
        inlineData: {
          decision: "need_info",
          summary: "信息不足，不能直接使用工作流。",
          ops_manual_flow_id: "flow-mysql-search",
          operation_frame: {
            target: { type: "mysql" },
            operation: { action: "backup" },
            target_scope: { hosts: ["server-local"] },
          },
          manuals: [
            {
              manual: {
                id: "manual-mysql-backup-ssh",
                title: "MySQL SSH 备份运维手册",
              },
              bound_workflow_id: "workflow-mysql-backup-ssh",
              usable_mode: "need_info",
              matched_fields: ["object_type", "operation_type"],
              workflow_preview: {
                title: "MySQL SSH 备份工作流",
                nodes: [
                  { id: "collect", title: "检查实例", command: "docker ps --filter name=mysql" },
                  { id: "backup", title: "生成备份", command: "mysqldump --single-transaction" },
                ],
              },
            },
          ],
        },
      },
      {
        id: "artifact-ops-manual-mysql-params",
        type: "ops_manual_param_resolution",
        titleZh: "运维手册参数解析",
        source: "tool:resolve_ops_manual_params",
        redactionStatus: "redacted",
        inlineData: {
          status: "need_user_input",
          ops_manual_flow_id: "flow-regenerated-param",
          manual_id: "manual-mysql-backup-ssh",
          workflow_id: "flow-mysql-search",
          resolved_params: [
            { id: "target_host", value: "server-local", source: "user_form", evidence: "context fact: target_host" },
            { id: "target_instance", value: "docker:aiops-mysql", source: "docker", evidence: "docker ps: image=mysql:8.0 ports=33306 status=Up" },
          ],
          fields: [
            {
              id: "backup_path",
              label: "备份路径",
              type: "path",
              ui_control: "text",
              required: true,
              placeholder: "例如 /data/backups",
            },
          ],
        },
      },
    ],
  );
}

function opsManualDirectActionsState() {
  return artifactTransportState(
    "ops-manual-direct-actions",
    "排查 redis-01 内存上涨",
    {
      id: "artifact-ops-manual-direct-actions",
      type: "ops_manual_search_result",
      titleZh: "运维手册检索",
      source: "tool:search_ops_manuals",
      redactionStatus: "redacted",
      inlineData: {
        decision: "direct_execute",
        summary: "找到可进入预检的 Redis 运维手册。",
        ops_manual_flow_id: "flow-redis-direct-actions",
        operation_frame: {
          target: { type: "redis", name: "redis-01" },
          operation: { action: "rca_or_repair" },
          target_scope: { hosts: ["redis-01"] },
        },
        manuals: [
          {
            manual: {
              id: "manual-redis-rca-ssh",
              title: "Redis SSH 排障运维手册",
            },
            bound_workflow_id: "workflow-redis-rca-ssh",
            usable_mode: "direct_execute",
            matched_fields: ["object_type", "operation_type", "execution_surface"],
          },
        ],
        recommended_next_action: "运行只读预检，通过后确认或审批执行。",
      },
    },
  );
}

function opsManualDynamicCandidatesState() {
  return artifactTransportState(
    "ops-manual-dynamic-candidates",
    "补齐 Redis 运维手册参数",
    {
      id: "artifact-ops-manual-dynamic-candidates",
      type: "ops_manual_param_resolution",
      titleZh: "运维手册参数解析",
      source: "tool:resolve_ops_manual_params",
      redactionStatus: "redacted",
      inlineData: {
        status: "ambiguous",
        manual_id: "manual-redis-rca-ssh",
        workflow_id: "workflow-redis-rca-ssh",
        ops_manual_flow_id: "flow-redis-dynamic-candidates",
        fields: [
          {
            id: "target_instance",
            label: "实例/服务",
            type: "resource_ref",
            ui_control: "select",
            required: true,
            candidates: [
              {
                value: "docker:redis-prod-a",
                label: "redis-prod-a | image redis:7.2 | ports 6379/tcp | health healthy",
              },
              {
                value: "k8s:redis-cache-0",
                label: "redis-cache-0 | namespace prod-cache | image redis:7.2.4 | phase Running",
              },
            ],
          },
          {
            id: "execution_surface",
            label: "访问/执行入口",
            type: "enum",
            ui_control: "select",
            required: true,
            candidates: [
              { value: "ssh:redis-01", label: "ssh redis-01 | service redis-server | port 6379" },
              { value: "kubectl:prod-cache", label: "kubectl prod-cache | deployment redis | health ready" },
            ],
          },
        ],
      },
    },
  );
}

function dataStreamForState(state) {
  return `aui-state:${JSON.stringify([{ type: "set", path: [], value: state }])}\n`;
}

async function routeShellApis(page, stateOrGetState) {
  await page.route("**/api/v1/sessions", async (route) => {
    await route.fulfill({ json: sessionsPayload });
  });
  await page.route("**/api/v1/hosts", async (route) => {
    await route.fulfill({
      json: {
        items: [
          {
            id: "server-local",
            name: "server-local",
            status: "online",
            executable: true,
            terminalCapable: true,
          },
        ],
      },
    });
  });
  await page.route("**/api/v1/llm-config", async (route) => {
    await route.fulfill({
      json: {
        provider: "mock",
        model: "browser-flow",
        apiKeySet: true,
        bifrostActive: true,
      },
    });
  });
  await page.route("**/api/v1/assistant/resume", async (route) => {
    const state = typeof stateOrGetState === "function" ? stateOrGetState() : stateOrGetState;
    await route.fulfill({
      status: 200,
      contentType: "text/plain; charset=utf-8",
      body: dataStreamForState(state),
    });
  });
}

test("process transcript keeps narration and expanded search details aligned", async ({ page }) => {
  await routeShellApis(page, transportState);
  await page.goto("/");
  await expect(page.getByTestId("aiops-process-header")).toBeVisible();
  await page.getByTestId("aiops-process-header").click();
  await page.getByTestId("aiops-search-toggle").click();

  const transcript = page.getByTestId("aiops-process-transcript-body");
  await expect(transcript).toContainText("接下来我要检查运行环境和最近任务状态。");
  await expect(transcript).toContainText("网页检索 2 项");
  await expect(transcript).toContainText("https://example.com/aiops-v2-order");
  await expect(transcript).toHaveScreenshot("process-transcript-order-alignment.png");
});

test("assistant final markdown keeps the same layout while running and after completion", async ({ page }) => {
  let resumeState = finalMarkdownState("working");
  await routeShellApis(page, () => resumeState);

  await page.goto("/");
  const runningFinal = page.getByTestId("aiops-final-text");
  await expect(runningFinal).toBeVisible();
  await expect(runningFinal).toHaveScreenshot("assistant-final-markdown-running.png");

  resumeState = finalMarkdownState("completed");
  await page.reload();
  const completedFinal = page.getByTestId("aiops-final-text");
  await expect(completedFinal).toBeVisible();
  await expect(completedFinal).toHaveScreenshot("assistant-final-markdown-completed.png");
});

test("running assistant text keeps the process header before tool blocks arrive", async ({ page }) => {
  await page.clock.setFixedTime(runningPreludeRenderedAt);
  await routeShellApis(page, runningPreludeBeforeToolsState());

  await page.goto("/");
  const transcript = page.getByTestId("aiops-process-transcript");
  await expect(page.getByTestId("aiops-process-header")).toContainText("处理中 1s");
  await expect(transcript).toContainText(runningPreludeText);
  await expect(page.getByTestId("aiops-process-transcript-body")).toHaveCount(0);
  await expect(transcript).toHaveScreenshot("assistant-running-prelude-with-process-header.png");
});

test("long terminal output stays inside a scrollable output box", async ({ page }) => {
  await routeShellApis(page, longTerminalOutputState());

  await page.goto("/");
  await page.getByTestId("aiops-process-header").click();
  await page.getByTestId("aiops-command-row-cmd-long-terminal-output").click();

  const terminalCard = page.getByTestId("aiops-terminal-card-cmd-long-terminal-output");
  const terminalOutput = page.getByTestId("aiops-command-output-cmd-long-terminal-output");
  await expect(terminalCard).toHaveClass(/max-h-72/);
  await expect(terminalCard).toHaveClass(/overflow-hidden/);
  await expect(terminalOutput).toHaveClass(/max-h-48/);
  await expect(terminalOutput).toHaveClass(/overflow-y-auto/);

  const sizes = await terminalOutput.evaluate((element) => ({
    clientHeight: element.clientHeight,
    scrollHeight: element.scrollHeight,
  }));
  expect(sizes.clientHeight).toBeGreaterThan(0);
  expect(sizes.clientHeight).toBeLessThanOrEqual(192);
  expect(sizes.scrollHeight).toBeGreaterThan(sizes.clientHeight);
});

test("chat renders rca report artifact", async ({ page }) => {
  await routeShellApis(page, rcaReportTransportState);

  await page.goto("/");
  await expect(page.getByTestId("rca-report-artifact")).toBeVisible();
  await expect(page).toHaveScreenshot("chat-rca-report-artifact.png", { fullPage: true });
});

test("ops manual direct hit shows distinct skip reference and preflight actions", async ({ page }) => {
  await routeShellApis(page, opsManualDirectActionsState());

  await page.goto("/");
  const card = page.getByTestId("ops-manual-search-result-card");
  await expect(card).toContainText("运行预检");
  await expect(card).toContainText("仅参考手册");
  await expect(card).toContainText("不使用");
  await expect(card).toHaveScreenshot("ops-manual-direct-three-actions.png");

  await card.getByRole("button", { name: "仅参考手册" }).click();
  await expect(card.getByTestId("ops-manual-reference-submitted")).toContainText(
    "不进入 Workflow 预检",
  );
  await expect(card.getByTestId("ops-manual-preflight-running")).toHaveCount(0);
  await expect(card).toHaveScreenshot("ops-manual-reference-only-selected.png");
});

test("ops manual dynamic candidates keep long environment labels inside the card", async ({ page }) => {
  await routeShellApis(page, opsManualDynamicCandidatesState());

  await page.goto("/");
  const card = page.getByTestId("ops-manual-param-resolution-card");
  await expect(card).toContainText("image redis:7.2");
  await expect(card).toContainText("namespace prod-cache");
  await expect(card).toContainText("health ready");
  await expect(card).toHaveScreenshot("ops-manual-dynamic-candidates.png");
});

test("ops manual search and parameter confirmation merge into one card", async ({ page }) => {
  await routeShellApis(page, opsManualMergedParamConfirmationState());

  await page.goto("/");
  const card = page.getByTestId("ops-manual-progress-card");
  await expect(card).toBeVisible();
  await expect(page.getByTestId("ops-manual-search-result-card")).toHaveCount(0);
  await expect(page.getByTestId("ops-manual-param-resolution-card")).toHaveCount(0);
  await expect(card.getByRole("button", { name: "不使用" })).toHaveCount(1);
  await expect(card.getByText("备份路径")).toHaveCount(0);
  await expect(card.getByText("目标主机")).toBeVisible();
  await expect(card.getByText("目标实例")).toBeVisible();
  await expect(card.getByText("server-local")).toHaveCount(0);
  await expect(card.getByText("docker:aiops-mysql")).toHaveCount(0);

  await card.getByRole("button", { name: "查看详细参数" }).click();
  await expect(card.getByText("server-local")).toBeVisible();
  await expect(card.getByText("docker:aiops-mysql")).toBeVisible();
  await expect(card).toHaveScreenshot("ops-manual-merged-param-confirmation.png");
});
