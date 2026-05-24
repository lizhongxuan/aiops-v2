// @ts-check
import { test, expect } from "@playwright/test";
import { createChatFixtureSessions, createChatFixtureState, openFixturePage } from "./helpers/uiFixtureHarness";

function idleRuntime() {
  return {
    turn: { active: false, phase: "idle", hostId: "server-local" },
    codex: { status: "connected", retryAttempt: 0, retryMax: 5 },
  };
}

function fixtureWithArtifacts(artifacts) {
  const state = createChatFixtureState({
    sessionId: "ops-manual-param-resolution",
    threadId: "ops-manual-param-resolution",
    cards: [
      {
        id: "user-ops-manual-param",
        type: "UserMessageCard",
        role: "user",
        text: "排查 Redis",
        createdAt: "2026-05-17T10:00:00Z",
        updatedAt: "2026-05-17T10:00:00Z",
      },
      {
        id: "assistant-ops-manual-param",
        type: "AssistantMessageCard",
        role: "assistant",
        text: "已检索运维手册，并开始解析必要参数。",
        createdAt: "2026-05-17T10:00:10Z",
        updatedAt: "2026-05-17T10:00:10Z",
      },
    ],
    runtime: idleRuntime(),
  });
  state.turns[state.currentTurnId].agentUiArtifacts = artifacts;
  return {
    state,
    sessions: createChatFixtureSessions({
      activeSessionId: "ops-manual-param-resolution",
      sessions: [
        {
          id: "ops-manual-param-resolution",
          kind: "single_host",
          title: "Ops Manual Param Resolution",
          status: "running",
          messageCount: 2,
          preview: "参数解析测试",
          selectedHostId: "server-local",
          lastActivityAt: "2026-05-17T10:00:10Z",
        },
      ],
    }),
  };
}

function searchArtifact(overrides = {}) {
  return {
    id: "artifact-search-redis",
    type: "ops_manual_search_result",
    titleZh: "运维手册检索结果",
    summaryZh: "按结构化条件完成检索判定。",
    redactionStatus: "redacted",
    source: "tool:search_ops_manuals",
    inlineData: {
      decision: "need_info",
      summary: "命中 Redis 排障手册，等待参数解析。",
      operation_frame: { target: { type: "redis" }, operation: { action: "rca_or_repair" } },
      manuals: [
        {
          manual: {
            id: "manual-redis-rca-ssh",
            title: "Redis SSH 排障运维手册",
            description: "用于 Redis SSH 场景的只读排障和恢复前验证。",
            content: "适用场景：Redis 内存压力、慢查询、连接异常。验证方式：检查 INFO memory、slowlog 和业务 p95。",
          },
          bound_workflow_id: "workflow-redis-rca-ssh",
          workflow_preview: {
            title: "Redis SSH 排障工作流",
            nodes: [
              { id: "collect", title: "采集只读指标", command: "redis-cli INFO memory" },
              { id: "verify", title: "校验延迟", command: "redis-cli SLOWLOG GET 10" },
            ],
          },
        },
      ],
      ...overrides,
    },
  };
}

function paramResolutionArtifact(inlineData) {
  return {
    id: `artifact-param-${inlineData.status}`,
    type: "ops_manual_param_resolution",
    titleZh: "运维手册参数解析",
    summaryZh: "已解析运维手册必要参数。",
    redactionStatus: "redacted",
    source: "tool:resolve_ops_manual_params",
    inlineData: {
      manual_id: "manual-redis-rca-ssh",
      workflow_id: "workflow-redis-rca-ssh",
      artifact_type: "ops_manual_param_resolution",
      ...inlineData,
    },
  };
}

test("Redis 参数自动补齐后不弹补信息表单，可直接运行预检", async ({ page }) => {
  await openFixturePage(page, "/", fixtureWithArtifacts([
    searchArtifact({ decision: "direct_execute", summary: "Redis 手册已匹配。" }),
    paramResolutionArtifact({
      status: "resolved",
      resolved_params: [
        { id: "target_location", value: "server-local", source: "selected_host", confidence: 1 },
        { id: "target_instance", value: "docker:aiops-redis", source: "docker_resource_resolver", confidence: 0.98 },
        { id: "execution_surface", value: "docker exec aiops-redis", source: "docker_resource_resolver", confidence: 0.98 },
      ],
      next_action: "run_preflight",
    }),
  ]));

  await expect(page.locator("main").getByText("参数已补齐，下一步运行预检")).toBeVisible();
  await expect(page.getByRole("button", { name: "运行预检" })).toBeVisible();
  await expect(page.getByText("补充运维手册参数")).toHaveCount(0);
});

test("Redis 多实例时只让用户选择实例，不询问固定四字段", async ({ page }) => {
  await openFixturePage(page, "/", fixtureWithArtifacts([
    searchArtifact(),
    paramResolutionArtifact({
      status: "ambiguous",
      resolved_params: [
        { id: "target_location", value: "server-local", source: "selected_host", confidence: 1 },
        { id: "execution_surface", value: "docker exec", source: "docker_resource_resolver", confidence: 0.9 },
      ],
      fields: [
        {
          id: "target_instance",
          label: "Redis 实例",
          type: "resource_ref",
          required: true,
          ui_control: "select",
          candidates: [
            { value: "docker:redis-a", label: "redis-a", source: "docker", confidence: 0.91 },
            { value: "docker:redis-b", label: "redis-b", source: "docker", confidence: 0.88 },
            { value: "__manual__", label: "其他，手动填写", source: "user" },
          ],
        },
      ],
      next_action: "await_user_input",
    }),
  ]));

  const form = page.locator("form").filter({ hasText: "补充运维手册参数" });
  await expect(form).toBeVisible();
  const redisSelect = form.getByLabel("Redis 实例");
  await expect(redisSelect).toBeVisible();
  await expect.poll(async () => redisSelect.locator("option").allTextContents()).toEqual([
    "redis-a",
    "redis-b",
    "其他，手动填写",
  ]);
  await expect(form.getByText("目标位置（可选）")).toHaveCount(0);
  await expect(form.getByText("访问/执行入口")).toHaveCount(0);
  await expect(form.getByText("现象/证据")).toHaveCount(0);
});

test("PostgreSQL 备份路径缺失时只询问 backup_path", async ({ page }) => {
  await openFixturePage(page, "/", fixtureWithArtifacts([
    paramResolutionArtifact({
      manual_id: "manual-pg-backup-ssh",
      workflow_id: "workflow-pg-backup-ssh",
      status: "need_user_input",
      resolved_params: [
        { id: "target_host", value: "pg-01", source: "conversation_resolver", confidence: 0.95 },
        { id: "execution_surface", value: "ssh", source: "manual_default_resolver", confidence: 0.8 },
      ],
      fields: [
        {
          id: "backup_path",
          label: "备份路径",
          type: "path",
          required: true,
          ui_control: "text",
          placeholder: "/data/backups",
        },
      ],
      next_action: "await_user_input",
    }),
  ]));

  await expect(page.getByText("pg-01")).toBeVisible();
  const form = page.locator("form").filter({ hasText: "备份路径" });
  await expect(form).toBeVisible();
  await expect(form.getByLabel("备份路径")).toBeVisible();
  await expect(form.getByText("目标位置（可选）")).toHaveCount(0);
  await expect(form.getByText("访问/执行入口")).toHaveCount(0);
});

test("MySQL Docker 备份路径缺失时只询问 backup_path", async ({ page }) => {
  await openFixturePage(page, "/", fixtureWithArtifacts([
    paramResolutionArtifact({
      manual_id: "manual-mysql-backup-ssh",
      workflow_id: "workflow-mysql-backup-ssh",
      status: "need_user_input",
      resolved_params: [
        { id: "target_host", value: "server-local", source: "selected_host", confidence: 0.95 },
        { id: "target_instance", value: "docker:aiops-mysql", source: "docker", confidence: 0.92 },
        { id: "execution_surface", value: "docker exec aiops-mysql", source: "docker", confidence: 0.92 },
      ],
      fields: [
        {
          id: "backup_path",
          label: "备份路径",
          type: "path",
          required: true,
          ui_control: "text",
          placeholder: "/data/backups",
        },
      ],
      next_action: "await_user_input",
    }),
  ]));

  await expect(page.getByText("docker:aiops-mysql")).toBeVisible();
  const form = page.locator("form").filter({ hasText: "备份路径" });
  await expect(form).toBeVisible();
  await expect(form.getByLabel("备份路径")).toBeVisible();
  await expect(form.getByText("目标位置（可选）")).toHaveCount(0);
  await expect(form.getByText("访问/执行入口")).toHaveCount(0);
  await expect(form.getByText("实例/服务")).toHaveCount(0);
});

test("点击运行预检后立即显示运行中状态并禁止重复点击", async ({ page }) => {
  await openFixturePage(page, "/", fixtureWithArtifacts([
    searchArtifact({ decision: "direct_execute", summary: "Redis 手册已匹配。" }),
    paramResolutionArtifact({
      status: "resolved",
      resolved_params: [
        { id: "target_location", value: "server-local", source: "selected_host", confidence: 1 },
        { id: "target_instance", value: "docker:aiops-redis", source: "docker_resource_resolver", confidence: 0.98 },
        { id: "execution_surface", value: "docker exec aiops-redis", source: "docker_resource_resolver", confidence: 0.98 },
      ],
      next_action: "run_preflight",
    }),
  ]));

  const preflightButton = page.getByRole("button", { name: "运行预检" });
  await expect(preflightButton).toBeVisible();
  await preflightButton.click();

  await expect(page.getByRole("button", { name: "预检中" })).toBeDisabled();
  await expect(page.getByTestId("ops-manual-preflight-running")).toContainText("预检请求已提交");
  await expect(page.getByRole("button", { name: "运行预检" })).toHaveCount(0);
});

test("刷新页面后参数解析卡不重复且表单保持单份", async ({ page }) => {
  await openFixturePage(page, "/", fixtureWithArtifacts([
    searchArtifact(),
    paramResolutionArtifact({
      status: "ambiguous",
      resolved_params: [
        { id: "target_location", value: "server-local", source: "selected_host", confidence: 1 },
      ],
      fields: [
        {
          id: "target_instance",
          label: "Redis 实例",
          type: "resource_ref",
          required: true,
          ui_control: "select",
          candidates: [
            { value: "docker:redis-a", label: "redis-a", source: "docker", confidence: 0.91 },
            { value: "docker:redis-b", label: "redis-b", source: "docker", confidence: 0.88 },
          ],
        },
      ],
      next_action: "await_user_input",
    }),
  ]));

  await expect(page.locator("main").getByText("补充运维手册参数")).toHaveCount(1);
  await expect(page.locator("form").filter({ hasText: "补充运维手册参数" })).toHaveCount(1);

  await page.reload({ waitUntil: "networkidle" });

  await expect(page.locator("main").getByText("补充运维手册参数")).toHaveCount(1);
  await expect(page.locator("form").filter({ hasText: "补充运维手册参数" })).toHaveCount(1);
});

test("用户选择不使用手册后进入普通运维文本且无乱码", async ({ page }) => {
  await openFixturePage(page, "/", fixtureWithArtifacts([searchArtifact()]));

  await page.getByRole("button", { name: "不使用" }).click();

  await expect(page.getByRole("button", { name: "已切换" })).toBeDisabled();
  await expect(page.getByTestId("ops-manual-skip-submitted")).toContainText("已切换为普通只读排查");
  await expect(page.getByText("请先做��")).toHaveCount(0);
  await expect(page.locator("form").filter({ hasText: "补充运维手册参数" })).toHaveCount(0);
});

test("手册命中卡可以只读查看工作流和手册", async ({ page }) => {
  await openFixturePage(page, "/", fixtureWithArtifacts([searchArtifact()]));

  await page.getByRole("button", { name: "查看工作流" }).click();
  await expect(page.getByRole("dialog")).toContainText("工作流只读预览");
  await expect(page.getByRole("dialog")).toContainText("Redis SSH 排障工作流");
  await expect(page.getByRole("dialog")).toContainText("redis-cli INFO memory");
  await page.keyboard.press("Escape");

  await page.getByRole("button", { name: "查看手册" }).click();
  await expect(page.getByRole("dialog")).toContainText("运维手册只读预览");
  await expect(page.getByRole("dialog")).toContainText("Redis SSH 排障运维手册");
  await expect(page.getByRole("dialog")).toContainText("适用场景：Redis 内存压力");
});
