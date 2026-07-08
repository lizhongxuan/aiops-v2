// @ts-check
import { test, expect } from "@playwright/test";
import {
  createChatFixtureSessions,
  createChatFixtureState,
  openBrowserFixturePage,
  openFixturePage,
} from "../helpers/uiFixtureHarness";

function idleRuntime() {
  return {
    turn: { active: false, phase: "idle", hostId: "web-01" },
    codex: { status: "connected", retryAttempt: 0, retryMax: 5 },
  };
}

const RUNNER_ACTIONS = {
  items: [
    { action: "script.shell", label: "Shell Script", category: "script", description: "Run shell script content.", defaults: { script: "set -euo pipefail\necho ok" } },
    { action: "manual.approval", label: "Manual Approval", category: "control", description: "Pause until an operator approves." },
    { action: "builtin.http_check", label: "HTTP Check", category: "network", description: "Run a read-only HTTP check." },
  ],
};

const RUNNER_EMPTY_STATE = {
  sessionId: "runner-e2e",
  kind: "single_host",
  selectedHostId: "server-local",
  hosts: [{ id: "server-local", name: "server-local", status: "online" }],
  runtime: { codex: { status: "connected" }, turn: { phase: "idle" } },
};

function stateWithArtifact({ text, artifact }) {
  return stateWithArtifacts({ text, artifacts: [artifact] });
}

function stateWithArtifacts({ text, artifacts }) {
  const state = createChatFixtureState({
    cards: [
      {
        id: "user-ops-manual",
        type: "UserMessageCard",
        role: "user",
        text,
        createdAt: "2026-05-14T10:00:00Z",
        updatedAt: "2026-05-14T10:00:00Z",
      },
      {
        id: "assistant-ops-manual",
        type: "AssistantMessageCard",
        role: "assistant",
        text: "已完成运维手册判定。",
        createdAt: "2026-05-14T10:00:10Z",
        updatedAt: "2026-05-14T10:00:10Z",
      },
    ],
    runtime: idleRuntime(),
  });
  state.turns[state.currentTurnId].agentUiArtifacts = artifacts;
  return state;
}

function stateWithProcess({ text, process, finalText = "" }) {
  const state = createChatFixtureState({
    cards: [
      {
        id: "user-redis-triage",
        type: "UserMessageCard",
        role: "user",
        text,
        createdAt: "2026-05-15T10:00:00Z",
        updatedAt: "2026-05-15T10:00:00Z",
      },
      {
        id: "assistant-redis-triage",
        type: "AssistantMessageCard",
        role: "assistant",
        text: finalText,
        createdAt: "2026-05-15T10:00:16Z",
        updatedAt: "2026-05-15T10:00:16Z",
      },
    ],
    runtime: idleRuntime(),
    finalText,
  });
  const turn = state.turns[state.currentTurnId];
  turn.process = process;
  turn.startedAt = "2026-05-15T10:00:00Z";
  turn.completedAt = "2026-05-15T10:00:16Z";
  turn.updatedAt = "2026-05-15T10:00:16Z";
  return state;
}

function opsManualArtifact(state, overrides = {}) {
  return {
    id: `artifact-${state}`,
    type: "ops_manual_match",
    titleZh: "运维手册判定",
    summaryZh: "按结构化条件完成判定。",
    redactionStatus: "redacted",
    source: "ai-chat",
    inlineData: {
      state,
      manualId: "manual-redis-memory",
      manualTitle: "Redis 内存压力排障手册",
      workflowRef: { workflowId: "workflow-redis-memory" },
      reasons: ["中间件匹配：redis", "操作类型匹配：rca_or_repair"],
      missingContext: [],
      compatibilityGaps: [],
      recommendedNextActions: ["fill_parameters", "run_precheck", "confirm_execution"],
      runRecordSummary: { successCount: 3, failureCount: 0, recentResult: "passed" },
      ...overrides,
    },
  };
}

function opsManualSearchArtifact(decision, overrides = {}) {
  return {
    id: `artifact-search-${decision}`,
    type: "ops_manual_search_result",
    titleZh: "运维手册检索结果",
    summaryZh: "按结构化条件完成检索判定。",
    redactionStatus: "redacted",
    source: "tool:search_ops_manuals",
    inlineData: {
      decision,
      summary: "已完成运维手册检索。",
      operation_frame: {
        target: { type: "postgresql" },
        operation: { action: "backup" },
      },
      manuals: [],
      searched_fields: ["object_type", "operation_type", "execution_surface"],
      ...overrides,
    },
  };
}

function runnerGenerationArtifact() {
  return {
    id: "artifact-runner-generation",
    type: "runner_workflow_generation",
    titleZh: "Runner Workflow 生成进度",
    summaryZh: "正在生成可审核的 Runner Workflow 草稿。",
    source: "ai-chat",
    redactionStatus: "redacted",
    inlineData: {
      workflowTitle: "Redis 内存压力排障 Workflow",
      previewMode: "readonly_modal",
      generationAvailable: true,
      validationProvider: "docker",
      validationScenario: "redis-memory-dry-run",
      steps: [
        { id: "precheck", title: "环境预检查", status: "passed", summary: "确认 Redis 实例、权限和指标可读。" },
        { id: "dry_run", title: "Dry Run", status: "waiting", summary: "输出命令计划，不修改目标环境。" },
        { id: "recovery", title: "恢复验证", status: "waiting", summary: "验证 used_memory_rss 和 p95 恢复。" },
      ],
    },
  };
}

function opsManualPreflightArtifact(status, overrides = {}) {
  return {
    id: `artifact-preflight-${status}`,
    type: "ops_manual_preflight_result",
    titleZh: "运维手册预检",
    summaryZh: "Node 0 预检完成。",
    source: "tool:run_ops_manual_preflight",
    redactionStatus: "redacted",
    inlineData: {
      status,
      ready: status === "passed",
      reason: status === "passed" ? "只读探针通过，可以确认或审批后执行。" : "预检未通过，不能执行 Runner Workflow。",
      manual_id: "manual-pg-backup-ubuntu",
      workflow_id: "workflow-pg-backup-ubuntu",
      probe_id: "probe-pg-backup-readonly",
      next_action: status === "passed" ? "confirm_execution" : "fallback_guide",
      evidence: [
        { name: "ssh_access", status: status === "passed" ? "passed" : "failed", note: "只读连接检查" },
        { name: "pg_isready", status: status === "passed" ? "passed" : "failed", note: "PostgreSQL 可用性检查" },
      ],
      ...overrides,
    },
  };
}

function opsManualFallbackGuideArtifact() {
  return {
    id: "artifact-fallback-guide",
    type: "ops_manual_fallback_guide",
    titleZh: "运维手册降级步骤",
    summaryZh: "预检失败后只能按手册逐步确认。",
    source: "tool:run_ops_manual_preflight",
    redactionStatus: "redacted",
    inlineData: {
      title: "PostgreSQL 备份降级步骤",
      reason: "目标不可达，不能运行绑定 Workflow。",
      steps: ["确认主机名和网络可达性。", "确认 ssh 授权。", "确认 pg_isready 后再重新预检。"],
    },
  };
}

async function routeOpsManualApis(page) {
  const redisManual = {
    id: "manual-redis-memory",
    title: "Redis 内存压力排障手册",
    status: "verified",
    workflow_ref: { workflow_id: "workflow-redis-memory", workflow_version: "v3" },
    operation: { target_type: "redis", action: "rca_or_repair" },
    applicability: { middleware: "redis", os: ["ubuntu"], execution_surface: ["ssh"] },
    required_context: { required_inputs: ["target_instance"], required_evidence: ["used_memory_rss", "p95"] },
    preconditions: ["确认目标 Redis 实例", "确认可读取 Coroot 指标"],
    validation: ["used_memory_rss 不再持续上涨", "payment-api p95 回落"],
    cannot_use_when: ["目标实例未知"],
    document_markdown: "用于 Redis 内存压力排障，必须先完成前置检查和 Dry Run。",
    run_record_summary: { success_count: 2, failure_count: 0, recent_result: "passed" },
  };
  await page.route("**/api/v1/ops-manuals?**", (route) =>
    route.fulfill({ json: { items: [redisManual], total: 1 } }),
  );
  await page.route("**/api/v1/ops-manuals/candidates?**", (route) =>
    route.fulfill({
      json: {
        items: [
          {
            id: "candidate-redis-memory",
            review_status: "pending",
            source_type: "ai_chat",
            proposed_manual: {
              ...redisManual,
              id: "manual-redis-memory-draft",
              title: "Redis 内存压力排障手册候选",
              status: "draft",
            },
            validation_report: ["已绑定 Workflow 草稿", "需要复核适用环境"],
          },
        ],
        total: 1,
      },
    }),
  );
  await page.route("**/api/v1/ops-manuals/run-records?**", (route) =>
    route.fulfill({
      json: {
        items: [
          {
            id: "run-redis-001",
            manual_id: "manual-redis-memory",
            workflow_id: "workflow-redis-memory",
            dry_run_status: "passed",
            execution_status: "success",
            validation_status: "passed",
            operator: "sre-a",
            completed_at: "2026-05-14T10:30:00+08:00",
          },
        ],
        total: 1,
      },
    }),
  );
}

async function routeRunnerWorkflowManualApis(page) {
  const storedGraphs = new Map();
  await page.route("**/api/v1/state", (route) => route.fulfill({ json: RUNNER_EMPTY_STATE }));
  await page.route("**/api/v1/sessions", (route) => route.fulfill({ json: { items: [] } }));
  await page.route("**/api/runner-studio/actions*", (route) => route.fulfill({ json: RUNNER_ACTIONS }));
  await page.route("**/api/runner-studio/workflows", (route) => route.fulfill({ json: { workflows: [] } }));
  await page.route("**/api/runner-studio/workflows/graph", (route) => {
    const body = route.request().postDataJSON();
    const name = body?.graph?.workflow?.name || "runner-blank";
    const graph = {
      ...(body?.graph || {}),
      ui: { ...(body?.graph?.ui || {}), resource_version: "rv-created-e2e" },
    };
    storedGraphs.set(name, graph);
    return route.fulfill({ json: { name, status: "draft", graph } });
  });
  await page.route("**/api/runner-studio/workflows/*/graph", (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const name = decodeURIComponent(url.pathname.split("/").at(-2) || "runner-blank");
    if (request.method() === "GET") {
      return route.fulfill({
        json: storedGraphs.get(name) || {
          version: "v1",
          workflow: { name },
          ui: { resource_version: "rv-loaded-e2e" },
          nodes: [],
          edges: [],
        },
      });
    }
    const body = request.postDataJSON();
    const graph = {
      ...(body?.graph || {}),
      ui: { ...(body?.graph?.ui || {}), resource_version: "rv-saved-e2e" },
    };
    storedGraphs.set(name, graph);
    return route.fulfill({ json: { name: body?.graph?.workflow?.name || name, status: "draft", graph } });
  });
  await page.route("**/api/runner-studio/workflows/*/validate", (route) =>
    route.fulfill({
      json: {
        valid: true,
        status: "validated",
        validated_graph_hash: "graph-hash-e2e",
        validated_layout_hash: "layout-hash-e2e",
        warnings: [],
      },
    }),
  );
  await page.route("**/api/runner-studio/runs", (route) =>
    route.fulfill({ json: { items: [] } }),
  );
  await page.route("**/api/v1/ops-manuals/candidates/prepare", (route) => route.abort());
  await page.route("**/api/v1/ops-manuals/candidates/generate-from-workflow", (route) =>
    route.fulfill({
      json: {
        candidate: workflowReverseCandidate(),
        validation_report: workflowReverseCandidate().structured_validation_report,
        user_summary: workflowReverseCandidate().user_summary,
      },
    }),
  );
  await page.route(/\/api\/v1\/ops-manuals(\?.*)?$/, (route) =>
    route.fulfill({ json: { items: [], total: 0 } }),
  );
  await page.route(/\/api\/v1\/ops-manuals\/candidates(\?.*)?$/, (route) =>
    route.fulfill({ json: { items: [workflowReverseCandidate()], total: 1 } }),
  );
  await page.route(/\/api\/v1\/ops-manuals\/run-records(\?.*)?$/, (route) =>
    route.fulfill({ json: { items: [], total: 0 } }),
  );
}

function workflowReverseCandidate() {
  return {
    id: "candidate-runner-blank",
    source_type: "workflow_reverse_generated",
    review_status: "pending",
    proposed_manual: {
      id: "manual-runner-blank-draft",
      title: "runner-blank 运维手册候选",
      status: "draft",
      workflow_ref: {
        workflow_id: "runner-blank",
        workflow_version: "v1",
        workflow_digest: "sha256:abc",
        storage_uri: "runner://workflows/runner-blank",
      },
      operation: { target_type: "runner_workflow", action: "review_required", risk_level: "medium" },
      document_markdown: "# runner-blank 运维手册候选\n\n## 适用范围\n- runner-blank\n\n## 缺口检查\n- 缺少近期成功闭环记录",
    },
    structured_validation_report: {
      status: "warning",
      warnings: [{ code: "missing_recent_successful_run", message: "缺少近期成功闭环记录" }],
      blocking: [],
      passed: [{ code: "workflow_ref_present", message: "已绑定 Workflow" }],
    },
    user_summary: {
      understood: ["系统识别到 runner-blank Workflow"],
      missing: ["缺少近期成功闭环记录"],
      next_steps: ["先审核候选，再决定是否补充闭环记录"],
    },
  };
}

test.describe("Ops Manual workflow UX", () => {
  test("redis memory triage shows evidence not plan meta", async ({ page }) => {
    await openFixturePage(page, "/", {
      state: stateWithProcess({
        text: "redis-local-01 prod vm ssh Redis used_memory_rss rising symptom metrics medium readonly no restart no write",
        finalText: "Redis 内存排查已完成：used_memory_rss 正在上升，建议继续核对慢查询与连接数。",
        process: [
          {
            id: "redis-plan",
            kind: "plan",
            displayKind: "plan",
            status: "completed",
            text: "plan updated: active (1/4 in_progress)",
            steps: [
              { id: "metrics", text: "查询 Redis RSS 与 used_memory 指标", status: "completed" },
              { id: "events", text: "读取最近 30 分钟 Redis 相关事件", status: "completed" },
            ],
          },
          {
            id: "redis-coroot-metrics",
            kind: "tool",
            displayKind: "coroot.service_metrics",
            status: "completed",
            text: "Coroot Redis 指标显示 used_memory_rss / used_memory 比值升高",
            inputSummary: "redis-local-01 used_memory_rss p95",
            outputPreview: "used_memory_rss=1.8GiB used_memory=1.0GiB p95=420ms",
            mock: true,
            evidenceRefs: ["evidence:redis:rss", "evidence:redis:p95"],
          },
          {
            id: "redis-k8s-events",
            kind: "tool",
            displayKind: "k8s.get_events",
            status: "completed",
            text: "未发现 Redis Pod OOMKilled 事件",
            inputSummary: "redis-local-01 events",
            mock: true,
            evidenceRefs: ["evidence:redis:events"],
          },
        ],
      }),
      sessions: createChatFixtureSessions(),
    });

    await expect(page.getByText(/plan updated: active/)).toHaveCount(0);
    await expect(page.getByText("Redis 内存排查已完成")).toBeVisible();

    await page.getByTestId("aiops-process-header").click();
    await expect(page.getByText("查询 Redis RSS 与 used_memory 指标")).toBeVisible();
    await expect(page.getByText("读取最近 30 分钟 Redis 相关事件")).toBeVisible();
    await expect(page.getByText(/plan updated: active/)).toHaveCount(0);

    await page.getByRole("button", { name: /已调用 2 个工具/ }).click();
    await expect(page.getByText("Redis 内存排查已完成")).toBeVisible();
  });

  test("short Redis troubleshooting input does not show typing precheck", async ({ page }) => {
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
    await openFixturePage(page, "/", {
      state: null,
      sessions: createChatFixtureSessions(),
    });

    await page.getByTestId("omnibar-input").fill("排查 Redis");

    await expect(page.getByTestId("ops-manual-precheck-card")).toHaveCount(0);
    await expect(page.getByText(/命中\s*\d+\s*%/)).toHaveCount(0);
    await expect(page.getByText("命中手册")).toHaveCount(0);
    await expect(page.getByText("开始执行")).toHaveCount(0);
  });

  test("search_ops_manuals need_info result asks for context without fake percentage", async ({ page }) => {
    await openFixturePage(page, "/", {
      state: stateWithArtifact({
        text: "排查 Redis",
        artifact: opsManualSearchArtifact("need_info", {
          summary: "信息不足，不能直接使用工作流。",
          operation_frame: {
            target: { type: "redis" },
            operation: { action: "rca_or_repair" },
          },
          next_questions: ["目标 Redis 实例是哪一个？", "部署方式是 Kubernetes、Docker 还是物理机？", "当前现象和指标是什么？"],
        }),
      }),
      sessions: createChatFixtureSessions(),
    });

    const card = page.getByTestId("ops-manual-search-result-card");
    await expect(card).not.toContainText("手册缺上下文");
    await expect(card).not.toContainText("信息不足，不能直接使用工作流。");
    await expect(card).not.toContainText("请在底部补充");
    await expect(card).not.toContainText("打开补充表单");
    await expect(page.getByTestId("ops-manual-context-prompt")).toHaveCount(0);
    await expect(page.getByTestId("ops-manual-context-composer")).toHaveCount(0);
    await expect(page.getByTestId("omnibar-input")).toBeVisible();
    await expect(card).not.toContainText("补充上下文");
    await expect(card).not.toContainText("目标 Redis 实例是哪一个？");
    await expect(card).not.toContainText("部署方式是 Kubernetes、Docker 还是物理机？");
    await expect(card).not.toContainText("当前现象和指标是什么？");
    await expect(page.getByText(/命中\s*\d+\s*%/)).toHaveCount(0);
    await expect(card).not.toContainText("可直接执行");
    await expect(card).not.toContainText("立即执行");
    await expect(card).not.toContainText("进入 Dry Run");
    await expect(card).not.toContainText("manual-redis");
    await expect(card).not.toContainText("Kubernetes Pod");
    await expect(card).not.toContainText("绑定 Workflow");
    await expect(card).not.toContainText("匹配字段");
    await expect(card).not.toContainText("已检索字段");
    await expect(card).not.toContainText("授权读取 Coroot");
    await expect(page.getByText("Runner 已执行")).toHaveCount(0);
  });

  test("need_info result shows fallback next questions when tool omits them", async ({ page }) => {
    await openFixturePage(page, "/", {
      state: stateWithArtifact({
        text: "帮我排查 Redis",
        artifact: opsManualSearchArtifact("need_info", {
          summary: "信息不足，不能直接使用工作流。",
          operation_frame: {
            target: { type: "redis" },
            operation: { action: "rca_or_repair" },
          },
        }),
      }),
      sessions: createChatFixtureSessions(),
    });

    const card = page.getByTestId("ops-manual-search-result-card");
    await expect(card).not.toContainText("手册缺上下文");
    await expect(card).not.toContainText("信息不足，不能直接使用工作流。");
    await expect(card).not.toContainText("请在底部补充");
    await expect(page.getByTestId("ops-manual-context-prompt")).toHaveCount(0);
    await expect(page.getByTestId("ops-manual-context-composer")).toHaveCount(0);
    await expect(page.getByTestId("omnibar-input")).toBeVisible();
    await expect(card).not.toContainText("请确认 redis / rca_or_repair 的目标实例或服务名称。");
    await expect(card).not.toContainText("请补充部署形态、访问方式和必要只读证据。");
    await expect(card).not.toContainText("立即执行");
    await expect(card).not.toContainText("进入 Dry Run");
    await expect(card).not.toContainText("选择目标实例");
    await expect(card).not.toContainText("选择部署形态");
  });

  test("4-field need_info fixture keeps context in the manual card without opening a fixed bottom form", async ({ page }) => {
    await openFixturePage(page, "/", "ops-manual-4field-form");

    await expect(page.getByTestId("ops-manual-context-composer")).toHaveCount(0);
    await expect(page.getByTestId("omnibar-input")).toBeVisible();
    const card = page.getByTestId("ops-manual-search-result-card");
    await expect(card).toContainText("Redis SSH 排障运维手册");
    await expect(card).not.toContainText("请在底部补充");
    await expect(card.getByRole("button", { name: "不使用" })).toBeVisible();
    await expect(card.getByRole("button", { name: "查看工作流" })).toBeVisible();
    await expect(card.getByRole("button", { name: "查看手册" })).toBeVisible();
  });

  test("4-field browser fixture does not fabricate a duplicate bottom prompt", async ({ page }) => {
    await openBrowserFixturePage(page, "/", "ops-manual-4field-form");

    await expect(page.getByTestId("ops-manual-context-composer")).toHaveCount(0);
    await expect(page.getByTestId("ops-manual-context-prompt")).toHaveCount(0);
    await expect(page.getByTestId("omnibar-input")).toBeVisible();
    const card = page.getByTestId("ops-manual-search-result-card");
    await expect(card).toContainText("Redis SSH 排障运维手册");
    await expect(card).not.toContainText("打开补充表单");
  });

  test("CentOS PostgreSQL backup shows search adapt result instead of direct execution", async ({ page }) => {
    await openFixturePage(page, "/", {
      state: stateWithArtifact({
        text: "在 CentOS 主机 pg-centos-01 上通过 ssh 做 PostgreSQL 备份，备份到 /data/backups，已确认 ssh_access 和 pg_isready 正常",
        artifact: opsManualSearchArtifact("adapt", {
          summary: "找到 PostgreSQL 备份手册，但当前环境需要适配。",
          manuals: [
            {
              manual: { id: "manual-pg-backup-ubuntu", title: "PostgreSQL 备份 Ubuntu 运维手册" },
              bound_workflow_id: "workflow-pg-backup-ubuntu",
              usable_mode: "adapt",
              matched_fields: ["object_type", "operation_type", "execution_surface"],
              environment_diffs: ["os", "package_manager"],
              blocked_reasons: ["workflow targets ubuntu apt/systemd but current host is centos/yum/systemd"],
              recommended_action: "generate_workflow_variant",
            },
          ],
        }),
      }),
      sessions: createChatFixtureSessions(),
    });

    const card = page.getByTestId("ops-manual-search-result-card");
    await expect(card).toContainText("需适配");
    await expect(card).toContainText("PostgreSQL 备份 Ubuntu 运维手册");
    await expect(card).toContainText("操作系统；包管理器");
    await expect(card).toContainText("生成适配工作流");
    await expect(card).toContainText("workflow targets ubuntu apt/systemd");
    await expect(card).not.toContainText("开始前置检查");

    await page.getByRole("button", { name: "生成适配工作流" }).click();
    await expect(page.getByTestId("ops-manual-generation-confirmation")).toContainText("生成适配工作流");
    await expect(page.getByTestId("omnibar-input")).toHaveCount(0);
  });

  test("Ubuntu PostgreSQL backup shows direct_execute with bound workflow and preflight first", async ({ page }) => {
    await openFixturePage(page, "/", {
      state: stateWithArtifact({
        text: "在 Ubuntu 主机 pg-ubuntu-01 上通过 ssh 做 PostgreSQL 备份，备份到 /data/backups，已确认 ssh_access 和 pg_isready 正常",
        artifact: opsManualSearchArtifact("direct_execute", {
          summary: "找到可直接使用的运维手册，用户确认前不会执行 Runner Workflow。",
          manuals: [
            {
              manual: { id: "manual-pg-backup-ubuntu", title: "PostgreSQL 备份 Ubuntu 运维手册" },
              bound_workflow_id: "workflow-pg-backup-ubuntu",
              usable_mode: "direct_execute",
              matched_fields: ["object_type", "operation_type", "environment", "required_context"],
              recommended_action: "run_preflight_probe",
              preflight_status: "not_run",
              run_record_summary: { success_count: 6, failure_count: 0, recent_result: "passed" },
            },
          ],
          recommended_next_action: "运行 Node 0 预检，通过后确认或审批执行。",
        }),
      }),
      sessions: createChatFixtureSessions(),
    });

    const card = page.getByTestId("ops-manual-search-result-card");
    await expect(card).toContainText("可进入预检");
    await expect(card).toContainText("PostgreSQL 备份 Ubuntu 运维手册");
    await expect(card).toContainText("workflow-pg-backup-ubuntu");
    await expect(card).toContainText("下一步：AI 会先运行只读预检");
    await expect(card).not.toContainText("立即执行");
    await expect(page.getByText("Runner 已执行")).toHaveCount(0);
  });

  test("direct manual match and passed preflight merge into one confirmation path", async ({ page }) => {
    await openFixturePage(page, "/", {
      state: stateWithArtifacts({
        text: "运行 PostgreSQL Ubuntu 备份预检",
        artifacts: [
          opsManualSearchArtifact("direct_execute", {
            summary: "找到可直接使用的运维手册，用户确认前不会执行 Runner Workflow。",
            manuals: [
              {
                manual: { id: "manual-pg-backup-ubuntu", title: "PostgreSQL 备份 Ubuntu 运维手册" },
                bound_workflow_id: "workflow-pg-backup-ubuntu",
                usable_mode: "direct_execute",
                recommended_action: "run_preflight_probe",
              },
            ],
          }),
          opsManualPreflightArtifact("passed"),
        ],
      }),
      sessions: createChatFixtureSessions(),
    });

    const card = page.getByTestId("ops-manual-search-result-card");
    await expect(card).toContainText("PostgreSQL 备份 Ubuntu 运维手册");
    await expect(card).toContainText("workflow-pg-backup-ubuntu");
    await expect(card).toContainText("Workflow 预检");
    await expect(card).toContainText("预检通过");
    await expect(card).toContainText("ssh_access");
    await expect(card).toContainText("确认执行");
    await expect(page.getByTestId("ops-manual-merged-preflight")).toHaveCount(1);
    await expect(page.getByTestId("ops-manual-preflight-result-card")).toHaveCount(0);
    await expect(page.getByText("立即执行")).toHaveCount(0);
    await expect(page.getByText(/命中\s*\d+\s*%/)).toHaveCount(0);

    await page.getByRole("button", { name: "确认执行" }).click();
    const confirmation = page.getByTestId("ops-manual-generation-confirmation");
    await expect(confirmation).toContainText("确认执行");
    await expect(confirmation).toContainText("确认执行绑定 Workflow");
    await expect(page.getByTestId("omnibar-input")).toHaveCount(0);

    await confirmation.getByRole("button", { name: "取消" }).click();
    await expect(page.getByTestId("ops-manual-generation-confirmation")).toHaveCount(0);
    await expect(page.getByTestId("omnibar-input")).toBeVisible();
  });

  test("preflight failed routes to fallback guide without workflow execution", async ({ page }) => {
    await openFixturePage(page, "/", {
      state: stateWithArtifacts({
        text: "运行 PostgreSQL Ubuntu 备份预检",
        artifacts: [opsManualPreflightArtifact("failed"), opsManualFallbackGuideArtifact()],
      }),
      sessions: createChatFixtureSessions(),
    });

    const preflight = page.getByTestId("ops-manual-preflight-result-card");
    await expect(preflight).toContainText("预检失败");
    await expect(preflight).toContainText("查看降级步骤");
    await expect(preflight).not.toContainText("进入 Dry Run");
    await expect(preflight).not.toContainText("立即执行");
    const fallback = page.getByTestId("ops-manual-fallback-guide-card");
    await expect(fallback).toContainText("PostgreSQL 备份降级步骤");
    await expect(fallback).toContainText("确认主机名和网络可达性");
    await expect(fallback).not.toContainText("进入 Dry Run");
    await expect(fallback).not.toContainText("立即执行");
    await expect(page.getByText("Runner 已执行")).toHaveCount(0);
  });

  test("need_info search and blocked preflight use distinct stages", async ({ page }) => {
    await openFixturePage(page, "/", {
      state: stateWithArtifacts({
        text: "排查 Redis",
        artifacts: [
          opsManualSearchArtifact("need_info", {
            summary: "信息不足，不能直接使用工作流。",
            manuals: [{ manual: { id: "manual-redis-rca-ssh", title: "Redis SSH 排障运维手册" } }],
          }),
          opsManualPreflightArtifact("blocked"),
        ],
      }),
      sessions: createChatFixtureSessions(),
    });

    const searchCard = page.getByTestId("ops-manual-search-result-card");
    await expect(searchCard).toContainText("运维手册检索");
    await expect(searchCard).not.toContainText("手册缺上下文");
    await expect(searchCard).toContainText("暂未进入 Workflow 预检");
    await expect(searchCard).not.toContainText("Workflow 预检阻断");

    const preflight = page.getByTestId("ops-manual-preflight-result-card");
    await expect(preflight).toContainText("Workflow 预检");
    await expect(preflight).toContainText("Workflow 预检阻断");
    await expect(preflight).not.toContainText("运维手册检索");
  });

  test("reference_only search result stays as guidance without executable workflow entry", async ({ page }) => {
    await openFixturePage(page, "/", {
      state: stateWithArtifact({
        text: "参考 PostgreSQL 备份步骤，不要运行 Workflow",
        artifact: opsManualSearchArtifact("reference_only", {
          summary: "找到可参考手册，但不能直接执行绑定工作流。",
          manuals: [
            {
              manual: { id: "manual-pg-backup-reference", title: "PostgreSQL 备份参考手册" },
              usable_mode: "reference_only",
              blocked_reasons: ["当前环境未绑定可安全执行的 Workflow"],
            },
          ],
          recommended_next_action: "AI 会继续自动只读排查；如果缺目标、时间范围、权限或观测数据，会先让你补齐必要信息。",
        }),
      }),
      sessions: createChatFixtureSessions(),
    });

    const card = page.getByTestId("ops-manual-search-result-card");
    await expect(card).toContainText("仅参考");
    await expect(card).toContainText("PostgreSQL 备份参考手册");
    await expect(card).toContainText("没有可直接运行的 Workflow");
    await expect(card).toContainText("AI 会继续自动只读排查");
    await expect(card).toContainText("先让你补齐必要信息");
    await expect(card).toContainText("参考关系");
    await expect(card).not.toContainText("按步骤执行");
    await expect(card).not.toContainText("运行预检");
    await expect(card).not.toContainText("进入 Dry Run");
    await expect(card).not.toContainText("立即执行");
  });

  test("stale cross-object reference_only search result is hidden for Kafka", async ({ page }) => {
    await openFixturePage(page, "/", {
      state: stateWithArtifact({
        text: "Kafka consumer group checkout-prod lag 持续升高，先只读分析",
        artifact: opsManualSearchArtifact("reference_only", {
          operation_frame: {
            object_type: "kafka",
            operation_type: "rca_or_repair",
          },
          manuals: [
            {
              manual: {
                id: "manual-k8s-pod-crashloop-rca",
                title: "Kubernetes Pod CrashLoop/OOM 排障运维手册",
                operation: { target_type: "kubernetes_pod", action: "rca_or_repair" },
              },
              bound_workflow_id: "workflow-k8s-pod-crashloop-rca",
              usable_mode: "reference_only",
              blocked_reasons: ["object_type differs"],
            },
          ],
        }),
      }),
      sessions: createChatFixtureSessions(),
    });

    await expect(page.getByTestId("ops-manual-search-result-card")).toHaveCount(0);
    await expect(page.getByText("未找到适用手册，AI 将继续只读排查")).toHaveCount(0);
    await expect(page.getByText("没有找到适用于 Kafka 的可用运维手册。")).toHaveCount(0);
    await expect(page.getByText("AI 不使用不匹配的手册")).toHaveCount(0);
    await expect(page.getByText("请在底部补充")).toHaveCount(0);
    await expect(page.getByTestId("ops-manual-context-prompt")).toHaveCount(0);
    await expect(page.getByTestId("ops-manual-context-composer")).toHaveCount(0);
    await expect(page.getByText("manual-k8s-pod-crashloop-rca")).toHaveCount(0);
    await expect(page.getByText("Kubernetes Pod CrashLoop/OOM")).toHaveCount(0);
    await expect(page.getByText("对象类型不匹配")).toHaveCount(0);
    await expect(page.getByText("object_type differs")).toHaveCount(0);
    await expect(page.getByText("进入 Dry Run")).toHaveCount(0);
    await expect(page.getByText("立即执行")).toHaveCount(0);
  });

  test("MySQL backup does not expose PostgreSQL workflow as executable", async ({ page }) => {
    await openFixturePage(page, "/", {
      state: stateWithArtifact({
        text: "在 Ubuntu 主机 mysql-01 上通过 ssh 对 MySQL 做备份，备份到 /data/backups",
        artifact: opsManualSearchArtifact("no_match", {
          summary: "没有找到合适的运维手册。",
          operation_frame: {
            target: { type: "mysql" },
            operation: { action: "backup" },
          },
          recommended_next_action: "继续普通 Agent 运维流程。",
        }),
      }),
      sessions: createChatFixtureSessions(),
    });

    await expect(page.getByTestId("ops-manual-search-result-card")).toHaveCount(0);
    await expect(page.getByText("无可用手册")).toHaveCount(0);
    await expect(page.getByText("AI 不使用不匹配的手册")).toHaveCount(0);
    await expect(page.getByText("请在底部补充")).toHaveCount(0);
    await expect(page.getByTestId("ops-manual-context-prompt")).toHaveCount(0);
    await expect(page.getByTestId("ops-manual-context-composer")).toHaveCount(0);
    await expect(page.getByText("PostgreSQL 备份 Ubuntu 运维手册")).toHaveCount(0);
    await expect(page.getByText("可直接执行")).toHaveCount(0);
    await expect(page.getByText("运行预检")).toHaveCount(0);
    await expect(page.getByText("进入 Dry Run")).toHaveCount(0);
    await expect(page.getByText("立即执行")).toHaveCount(0);
    await expect(page.getByText(/命中\s*\d+\s*%/)).toHaveCount(0);
  });

  test("workflow generation artifact shows step-by-step nodes, read-only drawer and generate confirmation", async ({ page }) => {
    await openFixturePage(page, "/", {
      state: stateWithArtifact({
        text: "确认生成 Redis 排障工作流",
        artifact: runnerGenerationArtifact(),
      }),
      sessions: createChatFixtureSessions(),
    });
    const startURL = page.url();

    const card = page.getByTestId("runner-workflow-generation-card");
    await expect(card).toContainText("环境预检查");
    await expect(card).not.toContainText("人工审批");
    await expect(card).toContainText("Dry Run");
    await expect(card).toContainText("恢复验证");

    await card.getByRole("button", { name: "查看详情" }).click();
    const drawer = page.getByRole("dialog", { name: "Runner Workflow 生成详情" });
    await expect(drawer).toBeVisible();
    await expect(drawer).toContainText("只读");
    await expect(drawer).toContainText("Provider：docker");
    await expect(drawer).toContainText("redis-memory-dry-run");
    await page.keyboard.press("Escape");

    await card.getByRole("button", { name: "生成" }).click();
    await expect(page.getByRole("dialog", { name: "Runner Workflow 生成详情" })).toBeVisible();
    await expect(page.getByTestId("ops-manual-generation-confirmation")).toContainText("生成 Runner Workflow 草稿");
    await expect(page).toHaveURL(startURL);
  });

  test("reference-only match card does not expose unsupported generation button", async ({ page }) => {
    await openFixturePage(page, "/", {
      state: stateWithArtifact({
        text: "Redis 排障已闭环，请生成运维手册",
        artifact: opsManualArtifact("reference_only"),
      }),
      sessions: createChatFixtureSessions(),
    });

    await expect(page.getByTestId("ops-manual-generation-confirmation")).toHaveCount(0);
    await expect(page.getByRole("button", { name: "生成运维手册" })).toHaveCount(0);
    await expect(page.getByTestId("omnibar-input")).toBeVisible();
  });

  test("Runner Studio generates a workflow reverse candidate and opens the review card", async ({ page }) => {
    await routeRunnerWorkflowManualApis(page);
    await page.goto("/runner");
    await page.getByTestId("runner-create-workflow").click();
    await expect(page.getByTestId("runner-studio-topbar")).toContainText("新建工作流");

    await page.getByTestId("runner-toolbar-more").click();
    await page.getByTestId("runner-toolbar-ops-manual").click();

    const modal = page.getByTestId("runner-ops-manual-modal");
    await expect(modal).toBeVisible();
    await expect(modal).toContainText("准备运维手册候选");
    await expect(modal).toContainText("生成会读取 Runner Workflow YAML");
    await expect(modal).toContainText("手册预览");

    const generateRequest = page.waitForRequest((req) =>
      req.url().includes("/api/v1/ops-manuals/candidates/generate-from-workflow") && req.method() === "POST",
    );
    await page.getByTestId("runner-ops-manual-prepare").click();
    const request = await generateRequest;
    expect(request.postDataJSON()).toMatchObject({
      workflow_id: "runner-blank",
      options: { include_recent_run_records: true, use_llm_summary: false },
    });
    expect(JSON.stringify(request.postDataJSON())).not.toContain("draft_manual");

    await expect(modal).toContainText("已生成候选");
    await expect(modal).toContainText("系统识别到 runner-blank Workflow");
    await expect(modal).toContainText("缺少近期成功闭环记录");
    await modal.getByRole("link", { name: "查看候选" }).click();

    await expect(page).toHaveURL(/\/settings\/ops-manuals\?candidate=candidate-runner-blank$/);
    await page.getByRole("tab", { name: "待审核手册" }).click();
    await expect(page.getByText("由 Workflow 反向生成")).toBeVisible();
    await expect(page.getByText("Workflow ID：runner-blank")).toBeVisible();
    await expect(page.getByText("sha256:abc")).toBeVisible();
    await expect(page.getByText("系统识别到 runner-blank Workflow")).toBeVisible();
    await expect(page.getByText("缺少近期成功闭环记录").first()).toBeVisible();
    await expect(page.getByText("审核通过后，该手册会变为 verified，并参与 AI Chat 的 search_ops_manuals 检索。")).toBeVisible();
  });

  test("ops manual management keeps simple tabs, detail modal and read-only workflow preview", async ({ page }) => {
    await routeOpsManualApis(page);
    await page.goto("/settings/ops-manuals");

    await expect(page.getByRole("tab", { name: "已验证手册" })).toHaveAttribute("aria-selected", "true");
    await expect(page.getByTestId("ops-manual-card-manual-redis-memory")).toContainText("Redis 内存压力排障手册");

    await page.getByTestId("ops-manual-card-manual-redis-memory").click();
    await expect(page.getByRole("dialog", { name: "Redis 内存压力排障手册" })).toContainText("绑定 Workflow");
    await page.getByRole("button", { name: "Close" }).click();

    await page.getByRole("tab", { name: "待审核手册" }).click();
    await expect(page.getByText("Redis 内存压力排障手册候选")).toBeVisible();
    await page.getByRole("button", { name: "只读预览" }).click();
    await expect(page.getByRole("dialog", { name: "绑定 Workflow 只读预览" })).toContainText("workflow-redis-memory");

    await page.getByRole("button", { name: "Close" }).click();
    await page.getByRole("tab", { name: "执行记录" }).click();
    await expect(page.getByText("run-redis-001")).toBeVisible();
    await expect(page.getByText("历史发布前检查：通过")).toBeVisible();
  });
});
