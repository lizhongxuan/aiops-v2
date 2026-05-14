// @ts-check
import { test, expect } from "@playwright/test";

const EMPTY_STATE = {
  sessionId: "experience-pack-e2e",
  kind: "single_host",
  selectedHostId: "server-local",
  hosts: [{ id: "server-local", name: "server-local", status: "online" }],
  runtime: { codex: { status: "connected" }, turn: { phase: "idle" } },
};

const pendingPack = {
  id: "pack-pg-pool",
  title: "PG 连接池修复经验包",
  summary: "诊断连接池耗尽，执行参数调整并验证恢复。",
  version: "v1.0",
  category: "repair",
  usage_shape: "guided",
  status: "disabled",
  review_status: "pending",
  enabled: false,
  skill: { name: "PG 连接池修复 Skill", summary: "从 case-pg-fix 提炼出的连接池耗尽处理经验。" },
  validation_gate: { status: "passed", passed: true, validators: ["runner.readonly_probe"] },
  runner_bindings: [{ id: "binding-pg-draft", workflow_id: "wf-pg-pool-draft", workflow_name: "PG Pool Draft", status: "draft" }],
  history: { success_count: 4, failure_count: 1, recent_result: "success" },
  advanced_refs: { gene_asset_id: "gene-pg-pool", capsule_asset_ids: ["capsule-pg-1"] },
  workflow_binding: { workflow_id: "wf-pg-pool-draft", workflow_name: "PG Pool Draft", status: "draft", version: "v1" },
  retrieval_eval: { score: 0.91, matched_cases: 4, verdict: "pending" },
  authorization_scopes: [],
};

const approvedSearchablePack = {
  ...pendingPack,
  status: "enabled",
  review_status: "approved",
  enabled: true,
  authorization_scopes: [{ type: "environment", value: "prod", searchable: true, reason: "生产 PG 集群" }],
  retrieval_eval: { score: 0.94, matched_cases: 5, verdict: "pass" },
};

const runnerDraftGraph = {
  version: "v1",
  workflow: { name: "wf-pg-pool-experience-draft", title: "PG Pool Experience Workflow" },
  nodes: [
    { id: "start", type: "start", label: "Start", position: { x: 80, y: 160 }, ports: [{ id: "next", type: "output", label: "下一步" }] },
    { id: "precheck", type: "action", label: "环境预检查", position: { x: 300, y: 120 }, step: { name: "precheck", action: "shell.run", args: { script: "echo precheck" } } },
    { id: "approval", type: "action", label: "人工审批", position: { x: 520, y: 120 }, step: { name: "approval", action: "manual.approval", args: { risk_reason: "需要人工确认参数" } } },
    { id: "dry_run", type: "action", label: "Dry Run", position: { x: 740, y: 120 }, step: { name: "dry-run", action: "shell.run", args: { script: "echo dry run" } } },
    { id: "execute", type: "action", label: "受控执行", position: { x: 960, y: 120 }, step: { name: "execute", action: "shell.run", args: { script: "echo execute" } } },
    { id: "validate", type: "action", label: "恢复验证", position: { x: 1180, y: 120 }, step: { name: "validate", action: "shell.run", args: { script: "echo validate" } } },
    { id: "rollback", type: "action", label: "受控回滚", position: { x: 960, y: 300 }, step: { name: "rollback", action: "shell.run", args: { script: "echo rollback" } } },
    { id: "end", type: "end", label: "End", position: { x: 1400, y: 160 }, ports: [{ id: "in", type: "input", label: "输入" }] },
  ],
  edges: [
    { id: "start-precheck", source: "start", target: "precheck", source_port: "next", target_port: "in" },
    { id: "precheck-approval", source: "precheck", target: "approval", source_port: "next", target_port: "in" },
    { id: "approval-dry-run", source: "approval", target: "dry_run", source_port: "approved", target_port: "in" },
    { id: "dry-run-execute", source: "dry_run", target: "execute", source_port: "next", target_port: "in" },
    { id: "execute-validate", source: "execute", target: "validate", source_port: "next", target_port: "in" },
    { id: "execute-rollback", source: "execute", target: "rollback", source_port: "failure", target_port: "in" },
  ],
};

const runnerCandidateResponse = {
  id: "runner-candidate-pg-pool",
  pack_id: "pack-pg-pool",
  workflow_id: "wf-pg-pool-experience-draft",
  workflow_name: "PG Pool Experience Workflow",
  status: "draft",
  studio_draft_link: "/runner/wf-pg-pool-experience-draft",
  workflow: {
    id: "wf-pg-pool-experience-draft",
    name: "wf-pg-pool-experience-draft",
    title: "PG Pool Experience Workflow",
    status: "draft",
    local_draft: true,
    ai_generated_draft: true,
    graph: runnerDraftGraph,
  },
  graph: runnerDraftGraph,
  runner_binding: { id: "binding-pg-local", workflow_id: "wf-pg-pool-experience-draft", workflow_name: "PG Pool Experience Workflow", status: "draft", review_status: "pending" },
};

const runnerActions = {
  items: [
    { action: "shell.run", label: "Shell Script", category: "script" },
    { action: "manual.approval", label: "Manual Approval", category: "control" },
  ],
};

function candidatePayload(pack) {
  return {
    items: [
      {
        id: "candidate-pg-pool",
        pack_id: "pack-pg-pool",
        title: "PG 连接池修复候选经验包",
        summary: "从 case-pg-fix 提炼出的连接池耗尽处理经验，等待审核启用。",
        status: pack.review_status === "approved" ? "approved" : "candidate",
        match_reason: "中间件类型、错误模式和 HostProfile 标签一致",
        source_case_id: "case-pg-fix",
        experience_pack: pack,
      },
    ],
    total: 1,
  };
}

async function installExperiencePackRoutes(page) {
  let currentPack = { ...pendingPack };

  await page.route("**/api/v1/state", (route) => route.fulfill({ json: EMPTY_STATE }));
  await page.route("**/api/v1/sessions*", (route) => route.fulfill({ json: { activeSessionId: "sess-exp", sessions: [] } }));
  await page.route("**/api/v1/hosts", (route) => route.fulfill({ json: { items: EMPTY_STATE.hosts } }));
  await page.route("**/api/v1/llm-config", (route) => route.fulfill({ json: { provider: "mock", model: "experience-pack-e2e", apiKeySet: true } }));
  await page.route("**/api/v1/experience-packs/*/reuse-records**", (route) => route.fulfill({ json: { items: [], total: 0 } }));
  await page.route("**/api/v1/experience-packs/candidates**", (route) => route.fulfill({ json: candidatePayload(currentPack) }));
  await page.route("**/api/v1/experience-packs/runner-candidates/confirm", (route) => route.fulfill({ json: runnerCandidateResponse }));
  await page.route("**/api/v1/experience-packs/candidates/**/approve", (route) => {
    currentPack = { ...approvedSearchablePack };
    return route.fulfill({ json: currentPack });
  });
  await page.route("**/api/runner-studio/actions*", (route) => route.fulfill({ json: runnerActions }));
  await page.route("**/api/runner-studio/workflows", (route) => route.fulfill({ json: { workflows: [] } }));
  await page.route("**/api/runner-studio/workflows/*/graph", (route) => route.fulfill({ json: runnerDraftGraph }));
}

async function openExperiencePackReview(page) {
  await page.goto("/settings/experience-packs");
  await expect(page.locator("main")).toContainText("PG 连接池修复候选经验包");
  await page.getByRole("tab", { name: "待审核经验" }).click();
  await expect(page.getByRole("button", { name: "发送到 Runner Studio" })).toBeVisible();
}

test.describe("experience pack user flows", () => {
  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => window.localStorage.clear());
    await installExperiencePackRoutes(page);
  });

  test("用户审核通过经验包后，经验包变为可检索", async ({ page }) => {
    await page.goto("/settings/experience-packs");

    await expect(page.locator("main")).toContainText("PG 连接池修复候选经验包");
    await expect(page.locator("main")).toContainText("不可检索");
    await page.getByRole("button", { name: "审核通过" }).click();

    await expect(page.getByTestId("experience-pack-workbench").getByText("可检索", { exact: true })).toBeVisible();
    await expect(page.locator("main")).toContainText("已审核启用，且已配置可检索授权范围");
  });

  test("用户从待审核经验生成 Runner Studio 本地草稿", async ({ page }) => {
    await openExperiencePackReview(page);

    await page.getByRole("button", { name: "发送到 Runner Studio" }).click();

    await expect(page.locator("main")).toContainText("已创建本地草稿");
    await expect(page.getByRole("link", { name: "打开 Runner Studio" })).toHaveAttribute("href", "/runner/wf-pg-pool-experience-draft");
    const drafts = JSON.parse(await page.evaluate(() => window.localStorage.getItem("runner.studio.localDrafts") || "{}"));
    expect(drafts["wf-pg-pool-experience-draft"].graph.nodes).toHaveLength(8);
    expect(drafts["wf-pg-pool-experience-draft"].graph.edges).toHaveLength(6);
  });

  test("用户打开经验包生成的 Runner 草稿后，可以看到受控执行流程", async ({ page }) => {
    await openExperiencePackReview(page);
    await page.getByRole("button", { name: "发送到 Runner Studio" }).click();
    await page.getByRole("link", { name: "打开 Runner Studio" }).click();

    await expect(page).toHaveURL(/\/runner\/wf-pg-pool-experience-draft$/);
    await expect(page.getByRole("heading", { name: "wf-pg-pool-experience-draft" })).toBeVisible();
    await expect(page.locator("main")).toContainText("环境预检查");
    await expect(page.locator("main")).toContainText("人工审批");
    await expect(page.locator("main")).toContainText("Dry Run");
    await expect(page.locator("main")).toContainText("受控执行");
    await expect(page.locator("main")).toContainText("恢复验证");
    await expect(page.locator("main")).toContainText("受控回滚");
  });
});
