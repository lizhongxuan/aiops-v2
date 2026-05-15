// @ts-check
import { test, expect } from "@playwright/test";
import {
  createChatFixtureSessions,
  createChatFixtureState,
  openFixturePage,
} from "../helpers/uiFixtureHarness";

function idleRuntime() {
  return {
    turn: { active: false, phase: "idle", hostId: "web-01" },
    codex: { status: "connected", retryAttempt: 0, retryMax: 5 },
  };
}

function stateWithArtifact({ text, artifact }) {
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
  state.turns[state.currentTurnId].agentUiArtifacts = [artifact];
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
      recommendedNextActions: ["fill_parameters", "run_precheck", "start_dry_run"],
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
      steps: [
        { id: "precheck", title: "环境预检查", status: "passed", summary: "确认 Redis 实例、权限和指标可读。" },
        { id: "dry_run", title: "Dry Run", status: "waiting", summary: "输出命令计划，不修改目标环境。" },
        { id: "recovery", title: "恢复验证", status: "waiting", summary: "验证 used_memory_rss 和 p95 恢复。" },
      ],
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

test.describe("Ops Manual workflow UX", () => {
  test("short Redis troubleshooting input does not show typing precheck", async ({ page }) => {
    await openFixturePage(page, "/", {
      state: createChatFixtureState({ cards: [], runtime: idleRuntime() }),
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
    await expect(card).toContainText("需补充信息");
    await expect(card).toContainText("目标 Redis 实例是哪一个？");
    await expect(card).toContainText("部署方式是 Kubernetes、Docker 还是物理机？");
    await expect(page.getByText(/命中\s*\d+\s*%/)).toHaveCount(0);
    await expect(card).not.toContainText("可直接执行");
    await expect(page.getByText("Runner 已执行")).toHaveCount(0);
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
    await expect(card).toContainText("os；package_manager");
    await expect(card).toContainText("生成适配工作流");
    await expect(card).toContainText("workflow targets ubuntu apt/systemd");
    await expect(card).not.toContainText("开始前置检查");

    await page.getByRole("button", { name: "生成适配工作流" }).click();
    await expect(page.getByTestId("ops-manual-generation-confirmation")).toContainText("生成适配工作流");
    await expect(page.getByTestId("omnibar-input")).toHaveCount(0);
  });

  test("Ubuntu PostgreSQL backup shows direct_execute with bound workflow and Dry Run", async ({ page }) => {
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
              recommended_action: "run_bound_workflow",
              run_record_summary: { success_count: 6, failure_count: 0, recent_result: "passed" },
            },
          ],
        }),
      }),
      sessions: createChatFixtureSessions(),
    });

    const card = page.getByTestId("ops-manual-search-result-card");
    await expect(card).toContainText("可直接执行");
    await expect(card).toContainText("PostgreSQL 备份 Ubuntu 运维手册");
    await expect(card).toContainText("workflow-pg-backup-ubuntu");
    await expect(card).toContainText("Dry Run");
    await expect(card).toContainText("用户确认前不会执行 Runner Workflow");
    await expect(page.getByText("Runner 已执行")).toHaveCount(0);
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

    const card = page.getByTestId("ops-manual-search-result-card");
    await expect(card).toContainText("无可用手册");
    await expect(card).toContainText("继续普通排查");
    await expect(card).not.toContainText("PostgreSQL 备份 Ubuntu 运维手册");
    await expect(card).not.toContainText("可直接执行");
    await expect(page.getByText(/命中\s*\d+\s*%/)).toHaveCount(0);
  });

  test("workflow generation artifact shows step-by-step nodes and read-only preview modal", async ({ page }) => {
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

    await page.getByRole("button", { name: "预览 Runner 草稿" }).click();
    await expect(page.getByRole("dialog", { name: "Runner Workflow 只读预览" })).toBeVisible();
    await expect(page.getByRole("dialog", { name: "Runner Workflow 只读预览" })).toContainText("只读");
    await expect(page).toHaveURL(startURL);
  });

  test("ops manual generation uses bottom confirmation panel and restores composer", async ({ page }) => {
    await openFixturePage(page, "/", {
      state: stateWithArtifact({
        text: "Redis 排障已闭环，请生成运维手册",
        artifact: opsManualArtifact("reference_only"),
      }),
      sessions: createChatFixtureSessions(),
    });

    await page.getByRole("button", { name: "生成运维手册" }).click();
    await expect(page.getByTestId("ops-manual-generation-confirmation")).toContainText("生成运维手册候选");
    await expect(page.getByTestId("omnibar-input")).toHaveCount(0);

    await page.getByRole("button", { name: "取消" }).click();
    await expect(page.getByTestId("ops-manual-generation-confirmation")).toHaveCount(0);
    await expect(page.getByTestId("omnibar-input")).toBeVisible();
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
    await expect(page.getByText("Dry Run：通过")).toBeVisible();
  });
});
