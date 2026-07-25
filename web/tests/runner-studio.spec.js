// @ts-check
import { expect, test } from "@playwright/test";

import {
  installWorkflowStudioFixture,
  workflowAgentCapabilitiesFixture,
} from "./helpers/workflowStudioPageFixture.js";

const ACTIONS = {
  items: [
    { action: "script.shell", label: "Shell Script", category: "script", description: "运行 Shell 脚本" },
    { action: "script.python", label: "Python Script", category: "script", description: "运行 Python 脚本" },
    { action: "http.request", label: "HTTP Request", category: "network", description: "发送 HTTP 请求" },
    { action: "builtin.tcp_ping", label: "TCP Ping", category: "network", description: "检查 TCP 端口" },
    { action: "builtin.dns_resolve", label: "DNS Resolve", category: "network", description: "解析 DNS" },
    { action: "condition.evaluate", label: "Condition", category: "control", description: "条件分支" },
  ],
};

const EMPTY_STATE = {
  sessionId: "runner-e2e",
  kind: "single_host",
  selectedHostId: "server-local",
  hosts: [{ id: "server-local", name: "server-local", status: "online" }],
  runtime: { codex: { status: "connected" }, turn: { phase: "idle" } },
  config: { workflowCapabilities: workflowAgentCapabilitiesFixture() },
};

const OPS_MANUAL_CANDIDATE = {
  id: "candidate-runner-blank",
  source_type: "workflow_reverse_generated",
  review_status: "pending",
  proposed_manual: {
    id: "manual-runner-blank-draft",
    title: "runner-blank 运维手册候选",
    status: "draft",
    workflow_ref: { workflow_id: "runner-blank", workflow_digest: "sha256:abc" },
    operation: { target_type: "runner_workflow", action: "review_required", risk_level: "medium" },
    document_markdown: "# runner-blank 运维手册候选",
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
    next_steps: ["先审核候选"],
  },
};

async function installRunnerStudioRoutes(page) {
  const storedGraphs = new Map();
  await page.routeWebSocket("**/ws", (socket) => {
    socket.send(JSON.stringify(EMPTY_STATE));
  });
  await page.route("**/api/v1/state", (route) => route.fulfill({ json: EMPTY_STATE }));
  await page.route("**/api/v1/sessions", (route) => route.fulfill({ json: { items: [] } }));
  await page.route("**/api/runner-studio/actions*", (route) => route.fulfill({ json: ACTIONS }));
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
    const segments = new URL(request.url()).pathname.split("/");
    const name = decodeURIComponent(segments.at(-2) || "runner-blank");
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
    return route.fulfill({ json: { name, status: "draft", graph } });
  });
  await page.route("**/api/v1/ops-manuals/candidates/prepare", (route) => route.abort());
  await page.route("**/api/v1/ops-manuals/candidates/generate-from-workflow", (route) =>
    route.fulfill({
      json: {
        candidate: OPS_MANUAL_CANDIDATE,
        validation_report: OPS_MANUAL_CANDIDATE.structured_validation_report,
        user_summary: OPS_MANUAL_CANDIDATE.user_summary,
      },
    }),
  );
}

async function createBlankWorkflow(page) {
  await page.goto("/runner");
  await page.getByTestId("runner-create-workflow").click();
  await expect(page.getByTestId("runner-studio-topbar")).toContainText("新建工作流");
  await expect(page).toHaveURL(/\/runner\/runner-blank$/);
}

test.describe("Runner Studio", () => {
  test.beforeEach(async ({ page }) => {
    await installRunnerStudioRoutes(page);
  });

  test("opens /runner as the native workflow library", async ({ page }) => {
    await page.goto("/runner");

    await expect(page.getByTestId("runner-studio-page")).toBeVisible();
    await expect(page.getByTestId("runner-workflow-library")).toBeVisible();
    await expect(page.getByTestId("runner-studio-topbar")).toHaveCount(0);
    await expect(page.getByText("打开 Runner UI")).toHaveCount(0);
  });

  test("creates and deletes a local workflow from the library", async ({ page }) => {
    await createBlankWorkflow(page);
    await page.getByTestId("runner-back-to-library").click();

    const library = page.getByTestId("runner-workflow-library");
    await expect(library).toContainText("新建工作流");
    await page.getByTestId("runner-delete-workflow-runner-blank").click();
    const confirm = page.getByTestId("workflow-delete-confirm");
    await expect(confirm).toContainText("新建工作流");
    await confirm.getByTestId("workflow-delete-confirm-submit").click();

    await expect(library).not.toContainText("新建工作流");
    await expect(library).toContainText("暂无工作流");
  });

  test("binds the authoritative workspace after the first graph save", async ({ page }) => {
    await createBlankWorkflow(page);
    const authoritativeGraphRead = page.waitForRequest((request) =>
      request.method() === "GET" && request.url().includes("/api/runner-studio/workflows/runner-blank/graph"),
    );

    await page.getByTestId("runner-toolbar-save").click();

    await authoritativeGraphRead;
    await expect(page.getByTestId("workflow-save-status")).toContainText("已自动保存");
  });

  test("opens a specific workflow editor route and returns to the library", async ({ page }) => {
    await page.unroute("**/api/runner-studio/workflows");
    await page.route("**/api/runner-studio/workflows", (route) =>
      route.fulfill({
        json: {
          workflows: [{
            name: "pg-restore",
            title: "PG Restore",
            status: "draft",
            graph: { version: "v1", workflow: { name: "pg-restore" }, nodes: [], edges: [] },
          }],
        },
      }),
    );

    await page.goto("/runner/pg-restore");
    await expect(page.getByTestId("runner-studio-topbar")).toContainText("PG Restore");
    await expect(page.getByTestId("workflow-context-dock")).toBeHidden();
    await expect(page.getByTestId("workflow-context-drawer-open")).toHaveCount(0);
    await expect(page.getByTestId("runner-workflow-library")).toHaveCount(0);

    await page.getByTestId("runner-back-to-library").click();
    await expect(page).toHaveURL(/\/runner$/);
    await expect(page.getByTestId("runner-workflow-library")).toContainText("PG Restore");
  });

  test("prepares a read-only OpsManual candidate for the selected workflow", async ({ page }) => {
    await createBlankWorkflow(page);
    await page.getByTestId("runner-toolbar-more").click();
    await page.getByTestId("runner-toolbar-ops-manual").click();

    const modal = page.getByTestId("runner-ops-manual-modal");
    await expect(modal).toBeVisible();
    await expect(modal).toContainText("准备运维手册候选");
    await expect(modal).toContainText("新建工作流");
    await expect(modal).toContainText("只绑定 1 个 Runner Workflow");
    await expect(modal).toContainText("生成会读取 Runner Workflow YAML");

    const prepareRequest = page.waitForRequest((request) =>
      request.url().includes("/api/v1/ops-manuals/candidates/generate-from-workflow") && request.method() === "POST",
    );
    await page.getByTestId("runner-ops-manual-prepare").click();
    const request = await prepareRequest;
    expect(request.postDataJSON()).toMatchObject({
      workflow_id: "runner-blank",
      options: { include_recent_run_records: true, use_llm_summary: false },
    });
    await expect(modal).toContainText("已生成候选");
    await expect(modal).toContainText("系统识别到 runner-blank Workflow");
    await expect(modal).toContainText("缺少近期成功闭环记录");
    await expect(modal.getByRole("link", { name: "查看候选" })).toBeVisible();
  });

  test("surfaces workflow type, host snapshots, leases, and OpsManual binding", async ({ page }) => {
    await page.unroute("**/api/runner-studio/workflows");
    await page.route("**/api/runner-studio/workflows", (route) =>
      route.fulfill({
        json: {
          workflows: [{
            name: "pg-pool-fix",
            title: "PG Pool Fix",
            status: "validated",
            workflow_type: "repair",
            host_profile_snapshot: { host_id: "host-db-01", display_name: "db-01", os: "Linux", arch: "x86_64" },
            host_lease: { lease_id: "lease-db-01", status: "acquired", expires_at: "2026-07-22T10:00:00+08:00" },
            experience_pack_binding: { enabled: true, pack_id: "pack-pg-pool", workflow_bindable: true },
            graph: { version: "v1", workflow: { name: "pg-pool-fix", workflow_type: "repair" }, nodes: [], edges: [] },
          }],
        },
      }),
    );

    await page.goto("/runner");
    const library = page.getByTestId("runner-workflow-library");
    await expect(library).toContainText("修复");
    await expect(library).toContainText("HostProfileSnapshot");
    await expect(library).toContainText("HostLease");
    await expect(library).toContainText("可绑定运维手册");
  });

  test("keeps primary actions compact and routes AI into the workflow overlay drawer", async ({ page }) => {
    await createBlankWorkflow(page);

    await expect(page.getByTestId("runner-toolbar-save")).toBeVisible();
    await expect(page.getByTestId("runner-toolbar-run")).toBeVisible();
    await expect(page.getByTestId("runner-toolbar-ai-generate")).toBeVisible();
    await page.getByTestId("runner-toolbar-more").click();
    await expect(page.getByTestId("runner-toolbar-validate")).toBeVisible();
    await expect(page.getByTestId("runner-toolbar-dry-run")).toContainText("发布前检查");
    await expect(page.getByTestId("runner-toolbar-publish")).toBeVisible();

    await page.getByTestId("runner-toolbar-ai-generate").click();
    await expect(page.getByTestId("workflow-context-dock")).toBeVisible();
    await expect(page.getByTestId("workflow-context-dock")).toHaveAttribute("data-scope", "workflow");
    await expect(page.getByTestId("workflow-context-tab-ai")).toHaveAttribute("aria-selected", "true");
    await expect(page.getByTestId("workflow-context-tab-configuration")).toHaveCount(0);
    await expect(page.getByTestId("workflow-ai-transport-panel")).toBeVisible();
  });

  test("uses authoritative AppSnapshot scope with independent node and workflow sidebars", async ({ page }, testInfo) => {
    /** @type {Array<Record<string, any>>} */
    const transportRequests = [];
    /** @type {Array<Record<string, any>>} */
    const dryRunRequests = [];
    /** @type {string[]} */
    const requestedUrls = [];
    page.on("request", (request) => requestedUrls.push(request.url()));

    await installWorkflowStudioFixture(page);
    await page.route("**/api/v1/assistant/transport", async (route) => {
      transportRequests.push(route.request().postDataJSON());
      await route.fulfill({ status: 200, contentType: "text/plain; charset=utf-8", body: "aui-state:[]\n" });
    });
    await page.route("**/api/runner-studio/workflow-dry-run", async (route) => {
      dryRunRequests.push(route.request().postDataJSON());
      await route.fulfill({ status: 200, json: { accepted: true } });
    });
    await page.goto("/runner/capacity-inspection", { waitUntil: "networkidle" });

    await expect(page.getByTestId("runner-studio-canvas")).toBeVisible();
    await expect(page.getByTestId("workflow-context-dock")).toBeHidden();
    await page.getByTestId("canvas-node-check-capacity").click();
    const nodeSidebar = page.getByTestId("workflow-node-sidebar");
    await expect(nodeSidebar).toBeVisible();
    await expect(page.getByTestId("workflow-context-tab-configuration")).toHaveCount(0);
    await expect(page.getByTestId("runner-node-panel")).toBeVisible();
    const canvasBeforeWorkflowDrawer = await page.getByTestId("workflow-context-canvas-pane").boundingBox();

    await page.getByTestId("runner-toolbar-ai-generate").click();
    await expect(page.getByTestId("workflow-context-dock")).toBeVisible();
    await expect(nodeSidebar).toHaveAttribute("aria-hidden", "true");
    expectSameCanvasGeometry(await page.getByTestId("workflow-context-canvas-pane").boundingBox(), canvasBeforeWorkflowDrawer);
    const panel = page.getByTestId("workflow-ai-transport-panel");
    await expect(panel).toBeVisible();
    await expect(panel.getByTestId("workflow-scope-workflow")).toHaveCount(0);
    await expect(panel.getByTestId("workflow-scope-selection")).toHaveCount(0);
    await expect(panel.getByLabel("本次发送范围")).toHaveCount(0);
    const prompt = "只修改已选容量检查步骤，并先给我看变更";
    await panel.getByTestId("workflow-ai-input").fill(prompt);
    await panel.getByTestId("workflow-ai-send").click();
    await expect.poll(() => transportRequests.length).toBe(1);
    expect(transportRequests[0]?.commands?.[0]).toMatchObject({
      type: "add-message",
      message: {
        role: "user",
        parts: [{ type: "text", text: prompt }],
        workflowContext: {
          workflowId: "capacity-inspection",
          revision: {
            resourceVersion: "fixture-rv-1",
            semanticHash: "sha256:fixture-semantic-1",
            layoutHash: "sha256:fixture-layout-1",
          },
          selection: { nodeIds: ["check-capacity"], edgeIds: [] },
        },
      },
    });

    await page.getByTestId("workflow-context-tab-review").click();
    await expect(page.getByTestId("workflow-validation-panel")).toBeVisible();
    await expect(page.getByTestId("workflow-validation-panel")).toContainText("已通过");

    await page.getByTestId("workflow-context-tab-run").click();
    const runPanel = page.getByTestId("workflow-run-panel");
    await expect(runPanel).toBeVisible();
    await expect(runPanel).toHaveAttribute("data-gate-state", "current");
    await expect(runPanel.getByTestId("workflow-run-gate-status")).toContainText("检查和试运行已通过");
    await expect(runPanel.getByTestId("workflow-draft-dry-run")).toBeEnabled();
    await runPanel.getByTestId("workflow-draft-dry-run").click();
    await expect.poll(() => dryRunRequests.length).toBe(1);
    expect(dryRunRequests[0]).toMatchObject({
      schemaVersion: "aiops.workflow.edit.v2",
      workflowId: "capacity-inspection",
      expectedBasis: {
        resourceVersion: "fixture-rv-1",
        semanticHash: "sha256:fixture-semantic-1",
        catalogVersion: "fixture-catalog-v1",
        actionSchemaDigest: "sha256:fixture-action-schema-1",
        referenceSnapshotHash: "sha256:fixture-reference-snapshot-1",
        environmentBindingVersion: "sha256:fixture-environment-binding-1",
      },
      environment: "staging",
    });
    expect(dryRunRequests[0].transactionId).toBeUndefined();

    await page.getByTestId("workflow-context-tab-history").click();
    await expect(page.getByTestId("workflow-history-panel")).toBeVisible();
    await expect(page.getByTestId("workflow-history-panel")).toContainText("更新容量检查阈值");

    await page.getByTestId("workflow-context-drawer-close").click();
    await expect(page.getByTestId("workflow-context-dock")).toBeHidden();
    await expect(nodeSidebar).toBeVisible();
    expectSameCanvasGeometry(await page.getByTestId("workflow-context-canvas-pane").boundingBox(), canvasBeforeWorkflowDrawer);

    await testInfo.attach("workflow-split-sidebars-current-path", {
      body: await page.screenshot({ fullPage: true, animations: "disabled" }),
      contentType: "image/png",
    });
    expect(requestedUrls.some((url) => /\/api\/runner-studio\/(?:workflow-ai(?:\/|\?|$)|ai\/(?:draft|result|event)(?:\/|\?|$))/.test(url))).toBe(false);
  });

  test("keeps the committed Canvas unchanged and exposes recovery after AssistantTransport 500", async ({ page }) => {
    await installWorkflowStudioFixture(page);
    await page.route("**/api/v1/assistant/transport", (route) =>
      route.fulfill({ status: 500, contentType: "text/plain; charset=utf-8", body: "internal server error" }),
    );
    await page.goto("/runner/capacity-inspection", { waitUntil: "networkidle" });

    const committedNodes = page.locator('[data-testid^="canvas-node-"]:not([data-testid$="-candidate"])');
    const committedNodeCount = await committedNodes.count();
    expect(committedNodeCount).toBeGreaterThan(0);
    await page.getByTestId("canvas-node-check-capacity").click();
    await page.getByTestId("runner-toolbar-ai-generate").click();
    const panel = page.getByTestId("workflow-ai-transport-panel");
    await panel.getByTestId("workflow-ai-input").fill("把已选步骤改成危险命令并立即应用");
    await panel.getByTestId("workflow-ai-send").click();

    await expect(panel).toContainText("服务异常,请稍后重试");
    await expect(panel.getByTestId("workflow-ai-retry")).toBeVisible();
    await expect(committedNodes).toHaveCount(committedNodeCount);
    await expect(page.getByTestId("runner-candidate-canvas-status")).toHaveCount(0);
    await page.getByTestId("workflow-context-tab-review").click();
    await expect(page.getByTestId("workflow-change-review")).toHaveCount(0);
    await expect(page.getByTestId("workflow-validation-panel")).toContainText("已通过");
  });

  test("settles a rejected manual save into a visible recoverable error", async ({ page }) => {
    const pageErrors = [];
    page.on("pageerror", (error) => pageErrors.push(error.message));
    await page.unroute("**/api/runner-studio/workflows/graph");
    await page.route("**/api/runner-studio/workflows/graph", (route) =>
      route.fulfill({ status: 422, json: { error: "graph must contain at least one executable node" } }),
    );

    await createBlankWorkflow(page);
    await page.getByTestId("runner-toolbar-save").click();

    const saveError = page.getByTestId("workflow-command-error-toast");
    await expect(saveError).toContainText("修改未保存");
    await expect(saveError).toContainText("graph must contain at least one executable node");
    await expect(saveError).toHaveAttribute("role", "alert");
    await expect(page.getByTestId("workflow-save-status")).toContainText("未保存");
    await saveError.getByRole("button", { name: "关闭错误提示" }).click();
    await expect(saveError).toHaveCount(0);
    expect(pageErrors).toEqual([]);
  });

  test("allows dismissing the local orchestration API notice", async ({ page }) => {
    await page.unroute("**/api/runner-studio/actions*");
    await page.route("**/api/runner-studio/actions*", (route) =>
      route.fulfill({ status: 503, json: { error: "runner studio unavailable" } }),
    );

    await page.goto("/runner");
    const notice = page.getByTestId("runner-studio-api-notice");
    await expect(notice).toBeVisible();
    await expect(notice).toContainText("内置 Runner API");
    await expect(notice).not.toContainText("Runner API upstream");
    await page.getByTestId("runner-api-notice-close").click();
    await expect(notice).toHaveCount(0);
    await expect(page.getByTestId("runner-workflow-library")).toBeVisible();
  });
});

function expectSameCanvasGeometry(actual, expected) {
  expect(actual).not.toBeNull();
  expect(expected).not.toBeNull();
  expect(Math.abs((actual?.x || 0) - (expected?.x || 0))).toBeLessThanOrEqual(1);
  expect(Math.abs((actual?.width || 0) - (expected?.width || 0))).toBeLessThanOrEqual(1);
}
