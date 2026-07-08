// @ts-check
import { test, expect } from "@playwright/test";
import { readFileSync } from "node:fs";

const ACTIONS = {
  items: [
    { action: "cmd.run", label: "Command", category: "command", description: "Run a shell command through /bin/sh -c on each target." },
    { action: "shell.run", label: "Shell Script", category: "script", description: "Run inline shell script content through /bin/sh.", defaults: { script: "set -e\necho ok" } },
    { action: "script.shell", label: "Shell Script", category: "script", description: "Run shell script content resolved by the script service or supplied inline.", defaults: { script: "set -euo pipefail\necho ok" } },
    { action: "script.python", label: "Python Script", category: "script", description: "Run Python script content resolved by the script service or supplied inline.", defaults: { script: "print('ok')" } },
    { action: "http.request", label: "HTTP Request", category: "network", description: "Send a governed HTTP request and validate the response status.", defaults: { method: "GET", url: "https://example.com/healthz", expected_status: [200], timeout: "10s" } },
    { action: "builtin.tcp_ping", label: "TCP Ping", category: "network", description: "Check whether a TCP host and port are reachable.", defaults: { host: "example.com", port: 443, timeout: "3s" } },
    { action: "builtin.dns_resolve", label: "DNS Resolve", category: "network", description: "Resolve DNS records using the runner host resolver.", defaults: { name: "example.com", record_type: "A", timeout: "3s" } },
    { action: "condition.evaluate", label: "Condition", category: "control", description: "Evaluate a graph condition node or condition edge." },
    { action: "manual.approval", label: "Manual Approval", category: "control", description: "Pause a graph run until an operator approves or rejects the node." },
    { action: "notify.send", label: "Notify", category: "control", description: "Send a notification or trigger an external notification channel." },
    { action: "variable.aggregate", label: "Variable Aggregator", category: "control", description: "Aggregate upstream variables into a stable graph output." },
  ],
};

const EMPTY_STATE = {
  sessionId: "runner-e2e",
  kind: "single_host",
  selectedHostId: "server-local",
  hosts: [{ id: "server-local", name: "server-local", status: "online" }],
  runtime: { codex: { status: "connected" }, turn: { phase: "idle" } },
};

async function createBlankWorkflow(page) {
  await page.goto("/runner");
  await page.getByTestId("runner-create-workflow").click();
  await expect(page.getByTestId("runner-studio-topbar")).toContainText("新建工作流");
}

async function openCanvasContextMenu(page) {
  await page.getByTestId("runner-canvas-dropzone").click({ button: "right", position: { x: 360, y: 260 } });
  await expect(page.getByTestId("runner-canvas-context-menu")).toBeVisible();
}

async function expectWithinViewport(page, locator) {
  const box = await locator.boundingBox();
  expect(box).not.toBeNull();
  const viewport = page.viewportSize();
  expect(box.x).toBeGreaterThanOrEqual(0);
  expect(box.y).toBeGreaterThanOrEqual(0);
  expect(box.x + box.width).toBeLessThanOrEqual(viewport.width + 2);
  expect(box.y + box.height).toBeLessThanOrEqual(viewport.height + 2);
}

async function clickToolbar(page, key) {
  const directAction = page.getByTestId(`runner-toolbar-${key}`);
  if (await directAction.count()) {
    await directAction.click();
    return;
  }
  await page.getByTestId("runner-toolbar-more").click();
  await page.getByTestId(`runner-toolbar-${key}`).click();
}

async function currentCanvasZoom(page) {
  return page.locator(".react-flow__viewport").evaluate((node) => {
    const transform = window.getComputedStyle(node).transform;
    if (!transform || transform === "none") return 1;
    const matrix = transform.match(/matrix\(([^)]+)\)/);
    if (matrix) return Number(matrix[1].split(",")[0]) || 1;
    const matrix3d = transform.match(/matrix3d\(([^)]+)\)/);
    if (matrix3d) return Number(matrix3d[1].split(",")[0]) || 1;
    return 1;
  });
}

test.describe("Runner Studio", () => {
  test.beforeEach(async ({ page }) => {
    const storedGraphs = new Map();
    await page.route("**/api/v1/state", (route) => route.fulfill({ json: EMPTY_STATE }));
    await page.route("**/api/v1/sessions", (route) => route.fulfill({ json: { items: [] } }));
    await page.route("**/api/runner-studio/actions*", (route) => route.fulfill({ json: ACTIONS }));
    await page.route("**/api/runner-studio/workflows", (route) => route.fulfill({ json: { workflows: [] } }));
    await page.route("**/api/runner-studio/workflows/graph/validate", async (route) => {
      const body = route.request().postDataJSON();
      const hasInvalidAiNode = (body?.graph?.nodes || []).some((node) => node.id === "ai-invalid");
      if (hasInvalidAiNode) {
        return route.fulfill({
          json: { valid: false, errors: [{ message: "AI patch validation failed" }] },
        });
      }
      return route.fulfill({
        json: {
          valid: true,
          status: "validated",
          validated_graph_hash: "graph-hash-e2e",
          warnings: [],
        },
      });
    });
    await page.route("**/api/runner-studio/workflows/runner-blank/validate", (route) =>
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
    await page.route("**/api/runner-studio/workflows/graph/dry-run", (route) =>
      route.fulfill({
        json: {
          run_id: "dry-run-e2e",
          status: "dry_run_passed",
          validated_graph_hash: "graph-hash-e2e",
          dry_run_graph_hash: "graph-hash-e2e",
          dry_run_at: "2026-05-05T00:00:00Z",
        },
      }),
    );
    await page.route("**/api/runner-studio/workflows/graph", (route) => {
      const body = route.request().postDataJSON();
      const name = body?.graph?.workflow?.name || "runner-blank";
      const graph = {
        ...(body?.graph || {}),
        ui: { ...(body?.graph?.ui || {}), resource_version: "rv-created-e2e" },
      };
      storedGraphs.set(name, graph);
      return route.fulfill({
        json: {
          name,
          status: "draft",
          graph,
        },
      });
    });
    await page.route("**/api/runner-studio/workflows/*/graph", (route) => {
      const request = route.request();
      const url = new URL(request.url());
      const segments = url.pathname.split("/");
      const name = decodeURIComponent(segments.at(-2) || "runner-blank");
      if (request.method() === "GET") {
        return route.fulfill({
          json:
            storedGraphs.get(name) || {
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
      return route.fulfill({
        json: {
          name: body?.graph?.workflow?.name || name,
          status: "draft",
          graph,
        },
      });
    });
    await page.route("**/api/runner-studio/runs", (route) =>
      route.fulfill({ json: { run_id: "run-e2e", status: "running" } }),
    );
    await page.route("**/api/runner-studio/runs/run-e2e/events/history", (route) =>
      route.fulfill({
        json: [
          { type: "run_start", run_id: "run-e2e", status: "running" },
          {
            type: "host_result",
            run_id: "run-e2e",
            step: "script-shell",
            host: "server-local",
            status: "success",
            output: { stdout: "ok", exit_code: 0 },
          },
          { type: "step_finish", run_id: "run-e2e", step: "script-shell", status: "success" },
          { type: "run_finish", run_id: "run-e2e", status: "success" },
        ],
      }),
    );
    await page.route("**/api/runner-studio/ai/draft", (route) =>
      route.fulfill({
        json: {
          graph_patch: {
            operations: [{ op: "add_node", node_id: "ai-invalid" }],
            graph: {
              version: "v1",
              workflow: { name: "runner-blank" },
              nodes: [{ id: "ai-invalid", type: "action", position: { x: 440, y: 140 }, step: { name: "bad", action: "shell.run" } }],
              edges: [],
            },
          },
          diff_summary: {
            semantic_changes: [{ title: "AI bad node", detail: "invalid shell.run patch" }],
          },
        },
      }),
    );
    await page.route("**/api/runner-studio/workflow-ai/sessions", (route) =>
      route.fulfill({ json: { id: "drawer-runner-blank-0", workflowId: "runner-blank", baseRevision: "rev-e2e", activeRevision: "rev-e2e", status: "active" } }),
    );
    await page.route("**/api/runner-studio/workflow-ai/plan", (route) =>
      route.fulfill({
        json: {
          id: "plan-e2e",
          workflowId: "runner-blank",
          message: "添加验证步骤",
          items: [{
            id: "item-1",
            title: "添加验证步骤",
            description: "在 Start 后添加一个验证节点。",
            goal: "验证工作流输入。",
            environment: "读取 workflow_context。",
            nodeLabel: "添加验证步骤",
            nodeType: "action",
            nodeAction: "script.python",
            scriptSummary: "检查上下文并输出 validation_result。",
            validationSummary: "确认 validation_result.ok 为 true。",
            inputVariables: [{ name: "workflow_context", type: "object" }],
            outputVariables: [{ name: "validation_result", type: "object" }],
            script: "print({'validation_result': {'ok': True}})",
            status: "pending",
          }],
        },
      }),
    );
    await page.route("**/api/runner-studio/workflows/*/publish", (route) =>
      route.fulfill({ json: { name: "runner-blank", status: "published", published_graph_hash: "graph-hash-e2e" } }),
    );
    await page.route("**/api/v1/ops-manuals/candidates/prepare", (route) => route.abort());
    await page.route("**/api/v1/ops-manuals/candidates/generate-from-workflow", (route) =>
      route.fulfill({
        json: {
          candidate: {
            id: "candidate-runner-blank",
            source_type: "workflow_reverse_generated",
            review_status: "pending",
            proposed_manual: {
              id: "manual-runner-blank-draft",
              title: "runner-blank 运维手册候选",
              status: "draft",
              workflow_ref: { workflow_id: "runner-blank", workflow_digest: "sha256:abc" },
              operation: { target_type: "runner_workflow", action: "review_required", risk_level: "medium" },
              document_markdown: "# runner-blank 运维手册候选\n\n## 适用范围\n- runner-blank",
            },
            structured_validation_report: {
              status: "warning",
              warnings: [{ code: "missing_recent_successful_run", message: "缺少近期成功闭环记录" }],
              blocking: [],
              passed: [{ code: "workflow_ref_present", message: "已绑定 Workflow" }],
            },
            user_summary: { understood: ["系统识别到 runner-blank Workflow"], missing: ["缺少近期成功闭环记录"], next_steps: ["先审核候选"] },
          },
          validation_report: {
            status: "warning",
            warnings: [{ code: "missing_recent_successful_run", message: "缺少近期成功闭环记录" }],
            blocking: [],
            passed: [{ code: "workflow_ref_present", message: "已绑定 Workflow" }],
          },
          user_summary: { understood: ["系统识别到 runner-blank Workflow"], missing: ["缺少近期成功闭环记录"], next_steps: ["先审核候选"] },
        },
      }),
    );
  });

  test("opens /runner as native Runner Studio instead of a legacy entry page", async ({ page }) => {
    await page.goto("/runner");

    await expect(page.getByTestId("runner-studio-page")).toBeVisible();
    await expect(page.getByTestId("runner-workflow-library")).toBeVisible();
    await expect(page.getByTestId("runner-studio-topbar")).toHaveCount(0);
    await expect(page.getByTestId("runner-studio-bottom-drawer")).toHaveCount(0);
    await expect(page.getByTestId("runner-run-drawer")).toHaveCount(0);
    await expect(page.getByText("打开 Runner UI")).toHaveCount(0);
  });

  test("deletes a local workflow from the workflow library", async ({ page }) => {
    page.on("dialog", (dialog) => dialog.accept());

    await createBlankWorkflow(page);
    await page.getByTestId("runner-back-to-library").click();

    await expect(page.getByTestId("runner-workflow-library")).toContainText("runner-blank");
    await page.getByTestId("runner-delete-workflow-runner-blank").click();

    await expect(page.getByTestId("runner-workflow-library")).not.toContainText("runner-blank");
    await expect(page.getByTestId("runner-workflow-library")).toContainText("暂无工作流");
  });

  test("opens a specific workflow editor from /runner/:workflowName and returns to the list route", async ({ page }) => {
    await page.unroute("**/api/runner-studio/workflows");
    await page.route("**/api/runner-studio/workflows", (route) =>
      route.fulfill({
        json: {
          workflows: [
            {
              name: "pg-restore",
              title: "PG Restore",
              status: "draft",
              graph: { version: "v1", workflow: { name: "pg-restore" }, nodes: [], edges: [] },
            },
          ],
        },
      }),
    );

    await page.goto("/runner/pg-restore");

    await expect(page.getByTestId("runner-studio-topbar")).toContainText("PG Restore");
    await expect(page.getByTestId("app-shell-header").getByTestId("runner-studio-topbar")).toContainText("PG Restore");
    await expect(page.locator(".runner-studio-shell > .runner-studio-topbar")).toHaveCount(0);
    await expect(page.getByTestId("runner-workflow-library")).toHaveCount(0);

    await page.getByTestId("runner-back-to-library").click();

    await expect(page).toHaveURL(/\/runner$/);
    await expect(page.getByTestId("runner-workflow-library")).toContainText("PG Restore");
    await expect(page.getByTestId("runner-studio-topbar")).toHaveCount(0);
  });

  test("opens a read-only OpsManual candidate preview for the selected workflow", async ({ page }) => {
    await createBlankWorkflow(page);

    await page.getByTestId("runner-toolbar-more").click();
    await page.getByTestId("runner-toolbar-ops-manual").click();

    const modal = page.getByTestId("runner-ops-manual-modal");
    await expect(modal).toBeVisible();
    await expect(modal).toContainText("准备运维手册候选");
    await expect(modal).toContainText("runner-blank");
    await expect(modal).toContainText("draft");
    await expect(modal).toContainText("只绑定 1 个 Runner Workflow");
    await expect(modal).toContainText("生成会读取 Runner Workflow YAML");
    await expect(modal).toContainText("手册预览");
    await expect(page).toHaveURL(/\/runner\/runner-blank$/);

    const prepareRequest = page.waitForRequest((req) =>
      req.url().includes("/api/v1/ops-manuals/candidates/generate-from-workflow") && req.method() === "POST",
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
    await expect(modal).toContainText("查看候选");
    await expect(page).toHaveURL(/\/runner\/runner-blank$/);

    await page.route(/\/api\/v1\/ops-manuals(\?.*)?$/, (route) =>
      route.fulfill({ json: { items: [] } }),
    );
    await page.route(/\/api\/v1\/ops-manuals\/candidates(\?.*)?$/, (route) =>
      route.fulfill({
        json: {
          items: [
            {
              id: "candidate-runner-blank",
              source_type: "workflow_reverse_generated",
              review_status: "pending",
              proposed_manual: {
                id: "manual-runner-blank-draft",
                title: "runner-blank 运维手册候选",
                status: "draft",
                workflow_ref: { workflow_id: "runner-blank", workflow_digest: "sha256:abc" },
                operation: { target_type: "runner_workflow", action: "review_required", risk_level: "medium" },
                document_markdown: "# runner-blank 运维手册候选\n\n## 适用范围\n- runner-blank",
              },
              structured_validation_report: {
                status: "warning",
                warnings: [{ code: "missing_recent_successful_run", message: "缺少近期成功闭环记录" }],
                blocking: [],
                passed: [{ code: "workflow_ref_present", message: "已绑定 Workflow" }],
              },
              user_summary: { understood: ["系统识别到 runner-blank Workflow"], missing: ["缺少近期成功闭环记录"], next_steps: ["先审核候选"] },
            },
          ],
        },
      }),
    );
    await page.route(/\/api\/v1\/ops-manuals\/run-records(\?.*)?$/, (route) =>
      route.fulfill({ json: { items: [] } }),
    );

    await modal.getByRole("link", { name: "查看候选" }).click();
    await expect(page).toHaveURL(/\/settings\/ops-manuals\?candidate=candidate-runner-blank$/);
    await page.getByRole("tab", { name: "待审核手册" }).click();
    await expect(page.getByText("由 Workflow 反向生成")).toBeVisible();
    await expect(page.getByText("Workflow ID：runner-blank")).toBeVisible();
    await expect(page.getByText("sha256:abc")).toBeVisible();
    await expect(page.getByText("系统识别到 runner-blank Workflow")).toBeVisible();
    await expect(page.getByText("缺少近期成功闭环记录").first()).toBeVisible();
  });

  test("surfaces Workflow type, HostProfileSnapshot, HostLease and ops manual binding context", async ({ page }) => {
    await page.unroute("**/api/runner-studio/workflows");
    await page.route("**/api/runner-studio/workflows", (route) =>
      route.fulfill({
        json: {
          workflows: [
            {
              name: "pg-pool-fix",
              title: "PG Pool Fix",
              status: "validated",
              workflow_type: "repair",
              case_id: "case-pg-fix",
              host_profile_snapshot: { host_id: "host-db-01", display_name: "db-01", os: "Linux", arch: "x86_64" },
              host_lease: { lease_id: "lease-db-01", status: "acquired", expires_at: "2026-05-12T10:00:00+08:00" },
              experience_pack_binding: { enabled: true, pack_id: "pack-pg-pool", workflow_bindable: true },
              graph: {
                version: "v1",
                workflow: {
                  name: "pg-pool-fix",
                  workflow_type: "repair",
                  case_id: "case-pg-fix",
                  host_profile_snapshot: { host_id: "host-db-01", display_name: "db-01", os: "Linux", arch: "x86_64" },
                  host_lease: { lease_id: "lease-db-01", status: "acquired" },
                  experience_pack_binding: { enabled: true, pack_id: "pack-pg-pool", workflow_bindable: true },
                },
                nodes: [],
                edges: [],
              },
            },
          ],
        },
      }),
    );

    await page.goto("/runner");

    await expect(page.getByTestId("runner-workflow-library")).toContainText("修复");
    await expect(page.getByTestId("runner-workflow-library")).toContainText("HostProfileSnapshot");
    await expect(page.getByTestId("runner-workflow-library")).toContainText("HostLease");
    await expect(page.getByTestId("runner-workflow-library")).toContainText("可绑定运维手册");
    await expect(page.getByTestId("runner-workflow-library")).not.toContainText("Runbook");

    await page.getByText("PG Pool Fix").click();
    await expect(page.getByTestId("runner-workflow-context")).toHaveCount(0);
  });

  test("creates a blank workflow, drags nodes, configures I/O, validates, and runs publish precheck", async ({ page }) => {
    await page.goto("/runner");
    await page.getByTestId("runner-create-workflow").click();
    await expect(page.getByTestId("runner-studio-topbar")).toContainText("新建工作流");
    await expect(page.getByTestId("app-shell-header").getByTestId("runner-studio-topbar")).toContainText("新建工作流");
    await expect(page.locator(".runner-studio-shell > .runner-studio-topbar")).toHaveCount(0);
    await expect.poll(async () => currentCanvasZoom(page)).toBeLessThanOrEqual(0.9);
    await expect(page).toHaveURL(/\/runner\/runner-blank$/);
    await expect(page.locator(".runner-studio-canvas-head")).toHaveCount(0);
    await expect(page.getByRole("button", { name: "在连线上插入节点" })).toHaveCount(0);
    await expect(page.locator(".runner-flow-edge-hover-path")).toHaveCount(0);

    await expect(page.locator(".runner-canvas-toolbar")).toHaveCount(0);
    await expect(page.locator(".react-flow__controls").getByTestId("runner-node-picker-toggle")).toBeVisible();
    await page.getByTestId("runner-node-picker-toggle").click();
    await expect(page.getByTestId("runner-node-picker")).toBeVisible();
    await expect(page.getByTestId("catalog-action-cmd-run")).toHaveCount(0);
    await expect(page.getByTestId("catalog-action-shell-run")).toHaveCount(0);
    await expect(page.getByTestId("catalog-action-notify-send")).toHaveCount(0);
    await expect(page.getByTestId("catalog-action-variable-aggregate")).toHaveCount(0);
    await expect(page.getByTestId("catalog-action-script-shell")).toBeVisible();
    await expect(page.getByTestId("catalog-action-script-python")).toBeVisible();
    await expect(page.getByTestId("catalog-action-http-request")).toBeVisible();
    await expect(page.getByTestId("catalog-action-builtin-tcp_ping")).toBeVisible();
    await expect(page.getByTestId("catalog-action-builtin-dns_resolve")).toBeVisible();
    await expect(page.getByTestId("catalog-action-condition-evaluate")).toBeVisible();
    await expect(page.getByTestId("catalog-action-manual-approval")).toHaveCount(0);
    const dropzoneBox = await page.getByTestId("runner-canvas-dropzone").boundingBox();
    const flowBox = await page.locator(".runner-canvas-react .react-flow").boundingBox();
    expect(dropzoneBox).not.toBeNull();
    expect(flowBox).not.toBeNull();
    expect(Math.abs(Math.round(flowBox.x) - Math.round(dropzoneBox.x))).toBeLessThanOrEqual(2);
    expect(Math.abs(Math.round(flowBox.width) - Math.round(dropzoneBox.width))).toBeLessThanOrEqual(2);

    await page.getByTestId("catalog-action-script-shell").dragTo(page.getByTestId("runner-canvas-dropzone"));
    await page.getByTestId("catalog-action-condition-evaluate").dragTo(page.getByTestId("runner-canvas-dropzone"));
    await expect(page.getByTestId("canvas-node-script-shell")).toBeVisible();
    await expect(page.getByTestId("canvas-node-condition-evaluate")).toBeVisible();
    await expect(page.getByTestId("canvas-node-manual-approval")).toHaveCount(0);
    await expect(page.getByTestId("canvas-node-script-shell")).not.toContainText("script.shell");
    await expect(page.getByTestId("canvas-node-script-shell")).not.toContainText("执行 shell 脚本内容，可配置输入、输出、重试和超时。");
    await expect(page.getByTestId("canvas-node-script-shell")).not.toContainText("1 in · 2 out");

    await page.getByTestId("canvas-node-script-shell").click();
    await expect(page.getByTestId("runner-node-panel")).toBeVisible();
    await expect(page.getByTestId("runner-node-panel-tab-input")).toHaveCount(0);
    await expect(page.getByTestId("runner-node-panel-tab-output")).toHaveCount(0);
    await page.getByTestId("runner-code-input-add").click();
    await page.getByLabel("输入变量名").fill("backup_id");
    const inputKeyBox = await page.getByLabel("输入变量名").boundingBox();
    const inputValueBox = await page.getByTestId("runner-code-input-value-0").boundingBox();
    expect(inputKeyBox).not.toBeNull();
    expect(inputValueBox).not.toBeNull();
    expect(Math.abs(Math.round(inputKeyBox.width) - Math.round(inputValueBox.width))).toBeLessThanOrEqual(4);
    await page.getByTestId("runner-code-output-add").click();
    await page.getByLabel("输出变量名").fill("restore_lsn");
    await page.getByTestId("runner-node-panel-apply").click();

    await clickToolbar(page, "validate");
    await expect(page.getByTestId("runner-studio-topbar")).toContainText("validated");
    await page.getByTestId("runner-toolbar-more").click();
    await expect(page.getByTestId("runner-toolbar-dry-run")).toContainText("发布前检查");
    await expect(page.getByRole("menuitem", { name: "Dry Run" })).toHaveCount(0);

    const dryRunRequest = page.waitForRequest((req) =>
      req.url().includes("/api/runner-studio/workflows/graph/dry-run") && req.method() === "POST",
    );
    await clickToolbar(page, "dry-run");
    await dryRunRequest;

    await page.reload();
    await expect(page.getByTestId("runner-studio-topbar")).toContainText("新建工作流");
    await expect(page.getByTestId("canvas-node-script-shell")).toBeVisible();
    await expect(page.getByTestId("canvas-node-condition-evaluate")).toBeVisible();
    await expect(page.getByTestId("canvas-node-manual-approval")).toHaveCount(0);
    await page.getByTestId("canvas-node-script-shell").click();
    await expect(page.getByLabel("输入变量名")).toHaveValue("backup_id");
    await expect(page.getByLabel("输出变量名")).toHaveValue("restore_lsn");
    await page.getByTestId("runner-node-panel-close").click();

    await expect(page.locator(".runner-canvas-toolbar").getByTestId("runner-canvas-fullscreen-toggle")).toHaveCount(0);
    await expect(page.locator(".react-flow__controls").getByTestId("runner-canvas-fullscreen-toggle")).toBeVisible();
    await page.getByTestId("runner-canvas-fullscreen-toggle").click();
    await expect(page.locator(".runner-studio-shell.fullscreen")).toBeVisible();
    await page.getByTestId("runner-canvas-fullscreen-toggle").click();
    await expect(page.locator(".runner-studio-shell.fullscreen")).toHaveCount(0);

    await page.getByTestId("runner-toolbar-run-details").click();
    await expect(page.getByTestId("runner-run-drawer")).toBeVisible();
    await page.getByTestId("runner-run-drawer-close").click();
    await expect(page.getByTestId("runner-run-drawer")).toHaveCount(0);

    const runRequest = page.waitForRequest((req) =>
      req.url().includes("/api/runner-studio/runs") && req.method() === "POST",
    );
    await page.getByTestId("runner-toolbar-run").click();
    await runRequest;
    await expect(page.getByTestId("canvas-node-start")).toHaveClass(/run-(running|success)/);
    await expect(page.getByTestId("canvas-node-start")).toContainText(/运行中|成功/);
    await expect(page.getByTestId("runner-run-drawer")).toHaveCount(0);
    await page.getByTestId("runner-toolbar-run-details").click();
    await expect(page.getByTestId("runner-run-drawer")).toBeVisible();
    await expect(page.getByTestId("runner-run-history-panel")).toContainText("运行记录");
    await expect(page.getByTestId("runner-run-history-panel")).toContainText("run-e2e");
    await expect(page.getByTestId("runner-run-panel")).toHaveCount(0);
    await page.getByTestId("runner-run-history-row-run-e2e").click();
    await expect(page.getByTestId("runner-run-panel")).toContainText("run-e2e");
    await expect(page.getByTestId("runner-run-panel")).toContainText("script-shell");
    await expect(page.getByTestId("runner-run-panel")).toContainText("ok");
    await page.getByTestId("runner-run-history-back").click();
    await expect(page.getByTestId("runner-run-panel")).toHaveCount(0);
    await expect(page.getByTestId("canvas-node-start")).toHaveClass(/run-success/);
    await expect(page.getByTestId("canvas-node-script-shell")).toHaveClass(/run-success/);
    await expect(page.getByTestId("canvas-node-end")).toHaveCount(0);

    await page.getByTestId("runner-run-drawer-close").click();
    await page.getByTestId("canvas-node-script-shell").click();
    await expect(page.getByTestId("runner-node-panel-apply")).toContainText("保存");
    await expect(page.getByTestId("runner-node-panel-apply")).not.toContainText("应用");
    await page.getByTestId("runner-node-panel-open-run").click();
    await expect(page.getByTestId("runner-run-drawer")).toHaveCount(0);
    await expect(page.getByTestId("runner-node-panel-tab-last-run")).toHaveClass(/active/);
    await expect(page.getByTestId("runner-node-last-run-view")).toBeVisible();
    await expect(page.getByTestId("runner-node-last-run-view")).not.toContainText("失败原因");
    await expect(page.getByTestId("runner-node-last-run-view")).not.toContainText("step script-shell finished with status=success");
    await expect(page.getByTestId("runner-node-run-details")).toHaveCount(0);
  });

  test("reflows canvas nodes from the bottom-left fit control", async ({ page }) => {
    const messyGraph = {
      version: "v1",
      workflow: { name: "messy-layout" },
      nodes: [
        { id: "start", type: "start", label: "Start", position: { x: 680, y: 360 }, ports: [{ id: "next", type: "output" }] },
        { id: "shell-a", type: "action", label: "Shell A", position: { x: 60, y: 60 }, step: { name: "shell-a", action: "shell.run", args: { script: "echo a" } } },
        { id: "shell-b", type: "action", label: "Shell B", position: { x: 70, y: 440 }, step: { name: "shell-b", action: "shell.run", args: { script: "echo b" } } },
        { id: "end", type: "end", label: "End", position: { x: 120, y: 250 }, ports: [{ id: "in", type: "input" }] },
      ],
      edges: [
        { id: "start-shell-a", source: "start", target: "shell-a", kind: "next" },
        { id: "shell-a-shell-b", source: "shell-a", target: "shell-b", kind: "next" },
        { id: "shell-b-end", source: "shell-b", target: "end", kind: "next" },
      ],
    };
    await page.route("**/api/runner-studio/workflows/messy-layout/graph", (route) => {
      if (route.request().method() === "GET") return route.fulfill({ json: messyGraph });
      const body = route.request().postDataJSON();
      return route.fulfill({ json: { name: "messy-layout", status: "draft", graph: body.graph } });
    });
    await page.goto("/runner/messy-layout");
    await expect(page.getByTestId("runner-studio-topbar")).toContainText("messy-layout");
    await expect(page.locator(".react-flow__controls .react-flow__controls-fitview")).toBeVisible();

    await page.locator(".react-flow__controls .react-flow__controls-fitview").click();
    await expect.poll(async () => currentCanvasZoom(page)).toBeLessThanOrEqual(0.9);
    const saveRequest = page.waitForRequest((req) =>
      req.url().includes("/api/runner-studio/workflows/messy-layout/graph") && ["POST", "PUT"].includes(req.method()),
    );
    await page.getByTestId("runner-toolbar-save").click();
    const payload = (await saveRequest).postDataJSON();
    const byId = Object.fromEntries(payload.graph.nodes.map((node) => [node.id, node.position]));

    expect(byId.start.x).toBeLessThan(byId["shell-a"].x);
    expect(byId["shell-a"].x).toBeLessThan(byId["shell-b"].x);
    expect(byId["shell-b"].x).toBeLessThan(byId.end.x);
    expect(byId.start.y).toBe(byId["shell-a"].y);
    expect(byId["shell-a"].y).toBe(byId["shell-b"].y);
    expect(byId["shell-b"].y).toBe(byId.end.y);
  });

  test("keeps primary workflow actions compact and moves secondary actions into more menu", async ({ page }) => {
    await createBlankWorkflow(page);

    await expect(page.getByTestId("runner-toolbar-save")).toBeVisible();
    await expect(page.getByTestId("runner-toolbar-run")).toBeVisible();
    await expect(page.getByTestId("runner-toolbar-run-details")).toBeVisible();
    await expect(page.getByTestId("runner-toolbar-events")).toHaveCount(0);
    await expect(page.getByTestId("runner-toolbar-ai-generate")).toBeVisible();
    await expect(page.getByTestId("runner-toolbar-more")).toBeVisible();
    await expect(page.getByTestId("runner-toolbar-validate")).toHaveCount(0);
    await expect(page.getByTestId("runner-toolbar-dry-run")).toHaveCount(0);
    await expect(page.getByTestId("runner-toolbar-variables")).toHaveCount(0);
    await expect(page.getByTestId("runner-toolbar-publish")).toHaveCount(0);

    await page.getByTestId("runner-toolbar-more").click();
    await expect(page.getByTestId("runner-toolbar-more-menu")).toBeVisible();
    await expect(page.getByTestId("runner-toolbar-validate")).toBeVisible();
    await expect(page.getByTestId("runner-toolbar-dry-run")).toContainText("发布前检查");
    await expect(page.getByTestId("runner-toolbar-variables")).toBeVisible();
    await expect(page.getByTestId("runner-toolbar-publish")).toBeVisible();
    await expect(page.getByTestId("runner-toolbar-more-menu").getByText("AI")).toHaveCount(0);
    await expect(page.getByTestId("runner-toolbar-import")).toBeVisible();
    await expect(page.getByTestId("runner-toolbar-export")).toBeVisible();
    await page.getByTestId("runner-studio-canvas").click({ position: { x: 20, y: 20 } });
    await expect(page.getByTestId("runner-toolbar-more-menu")).toHaveCount(0);

    await page.getByTestId("runner-toolbar-ai-generate").click();
    const workflowAiDrawer = page.getByTestId("workflow-ai-drawer");
    await expect(workflowAiDrawer.getByRole("button", { name: "新会话" })).toBeVisible();
    await expect(workflowAiDrawer.getByRole("button", { name: "事件" })).toBeVisible();
  });

  test("imports and exports compact workflow JSON without layout metadata", async ({ page }) => {
    await createBlankWorkflow(page);

    const importedWorkflow = {
      kind: "aiops.runner.workflow",
      version: 1,
      workflow: {
        name: "external-name-ignored",
        title: "Imported repair flow",
        workflow_type: "repair",
        status: "validated",
      },
      nodes: [
        {
          id: "start",
          type: "start",
          label: "Start",
          position: { x: 5000, y: 6000 },
          ui: { host_groups: [{ label: "db", hosts: ["db-01"] }] },
          state: { status: "running" },
        },
        {
          id: "inspect-pg",
          type: "action",
          label: "Inspect PG",
          position: { x: 9000, y: 9000 },
          step: { action: "shell.run", targets: ["db"], args: { script: "pg_isready" } },
          inputs: [{ key: "cluster", value_source: { type: "literal", value: "pg-main" } }],
          outputs: [{ key: "pg_status", type: "string" }],
          measured: { width: 999 },
        },
        {
          id: "repair-pg",
          type: "action",
          label: "Repair PG",
          position: { x: 12000, y: 9000 },
          step: { action: "shell.run", targets: ["db"], args: { script: "echo repair" } },
        },
      ],
      edges: [
        { id: "old-start-inspect", source: "start", source_port: "next", target: "inspect-pg", target_port: "in", kind: "next", state: { status: "running" } },
        { source: "inspect-pg", source_port: "next", target: "repair-pg", target_port: "in", kind: "next" },
      ],
      dry_run_graph_hash: "should-not-import",
      layout: { stale: true },
    };

    await page.getByTestId("runner-workflow-import-input").setInputFiles({
      name: "workflow.json",
      mimeType: "application/json",
      buffer: Buffer.from(JSON.stringify(importedWorkflow)),
    });

    await expect(page.getByTestId("runner-save-state")).toContainText("已导入，未保存");
    await expect(page.getByTestId("canvas-node-inspect-pg")).toBeVisible();
    await expect(page.getByTestId("canvas-node-repair-pg")).toBeVisible();

    const saveRequest = page.waitForRequest((req) =>
      req.url().includes("/api/runner-studio/workflows") && req.url().endsWith("/graph") && ["POST", "PUT"].includes(req.method()),
    );
    await page.getByTestId("runner-toolbar-save").click();
    const savedGraph = (await saveRequest).postDataJSON().graph;
    const inspectNode = savedGraph.nodes.find((node) => node.id === "inspect-pg");
    expect(inspectNode.position.x).toBeLessThan(1000);
    expect(inspectNode.position.y).toBeLessThan(1000);
    expect(inspectNode.position).not.toEqual({ x: 9000, y: 9000 });
    expect(inspectNode.step.args.script).toBe("pg_isready");
    expect(inspectNode.measured).toBeUndefined();
    expect(savedGraph.dry_run_graph_hash).toBeUndefined();

    await page.getByTestId("runner-toolbar-more").click();
    const downloadPromise = page.waitForEvent("download");
    await page.getByTestId("runner-toolbar-export").click();
    const download = await downloadPromise;
    const exportedPath = await download.path();
    const exported = JSON.parse(readFileSync(exportedPath, "utf8"));

    expect(exported.kind).toBe("aiops.runner.workflow");
    expect(exported.workflow.name).toBe("runner-blank");
    expect(exported.workflow.title).toBe("Imported repair flow");
    expect(exported.nodes.find((node) => node.id === "inspect-pg").step.args.script).toBe("pg_isready");
    expect(exported.nodes.some((node) => node.position)).toBe(false);
    expect(exported.nodes.some((node) => node.state || node.measured)).toBe(false);
    expect(exported.edges.some((edge) => edge.id || edge.state)).toBe(false);
    expect(exported.layout).toBeUndefined();
    expect(exported.dry_run_graph_hash).toBeUndefined();
  });

  test("uses Dify-like node settings for system nodes and script actions", async ({ page }) => {
    await createBlankWorkflow(page);
    await expect(page.getByTestId("canvas-node-end")).toHaveCount(0);

    await page.getByTestId("canvas-node-start").click();
    await expect(page.getByTestId("runner-node-panel")).toBeVisible();
    await expect(page.getByTestId("runner-node-system-card")).toContainText("开始节点");
    await expect(page.getByTestId("runner-node-system-card")).toContainText("由工作流自动维护");
    await expect(page.getByTestId("runner-node-action-readonly")).toHaveCount(0);
    await expect(page.getByTestId("runner-node-script-editor")).toHaveCount(0);

    await expect(page.locator(".runner-canvas-toolbar")).toHaveCount(0);
    await expect(page.locator(".react-flow__controls").getByTestId("runner-node-picker-toggle")).toBeVisible();
    await page.getByTestId("runner-node-picker-toggle").click();
    await page.getByTestId("catalog-action-script-shell").click();
    const canvasBoxBeforePanel = await page.getByTestId("runner-canvas-dropzone").boundingBox();
    await page.getByTestId("canvas-node-script-shell").click();
    const canvasBoxAfterPanel = await page.getByTestId("runner-canvas-dropzone").boundingBox();
    expect(canvasBoxBeforePanel).not.toBeNull();
    expect(canvasBoxAfterPanel).not.toBeNull();
    expect(Math.abs(Math.round(canvasBoxAfterPanel.width) - Math.round(canvasBoxBeforePanel.width))).toBeLessThanOrEqual(2);
    await expect(page.getByTestId("runner-node-panel-modal")).toBeVisible();
    await expect(page.getByTestId("runner-node-panel-title")).toHaveText("Shell Script");
    await expect(page.getByTestId("runner-node-name-input")).toHaveCount(0);
    await expect(page.getByTestId("runner-node-action-readonly")).toHaveCount(0);
    await expect(page.getByTestId("runner-node-targets")).not.toContainText("运行主机标签");
    await expect(page.getByTestId("runner-node-targets")).not.toContainText("步骤会按标签展开目标主机");
    await expect(page.getByTestId("runner-node-script-editor")).toBeVisible();
    await expect(page.getByTestId("runner-node-script-editor")).toHaveValue("set -euo pipefail\necho ok");
    await page.getByTestId("runner-node-script-expand").click();
    await expect(page.getByTestId("runner-script-editor-modal")).toBeVisible();
    await expect(page.getByTestId("runner-script-editor-modal-textarea")).toHaveValue("set -euo pipefail\necho ok");
    await expect(page.getByTestId("runner-script-editor-modal-textarea")).toHaveCSS("background-color", "rgb(15, 23, 42)");
    await expect(page.getByTestId("runner-script-editor-modal-textarea")).toHaveCSS("color", "rgb(226, 232, 240)");
    await page.getByTestId("runner-script-editor-modal-close").click();
    await expect(page.getByTestId("runner-script-editor-modal")).toHaveCount(0);
    await expect(page.getByTestId("runner-code-input-variables")).toBeVisible();
    await page.getByTestId("runner-code-input-add").click();
    await page.getByTestId("runner-code-input-value-0").click();
    await expect(page.getByTestId("runner-variable-picker")).toContainText("sys.run_id");
    await expect(page.getByTestId("runner-code-output-variables")).toBeVisible();
    await expect(page.getByTestId("runner-node-action-input")).toHaveCount(0);
    await expect(page.locator(".runner-next-step-editor")).toHaveCount(0);
  });

  test("configures runner host groups and step target labels without exposing a terminal end node", async ({ page }) => {
    await createBlankWorkflow(page);
    await expect(page.getByTestId("canvas-node-end")).toHaveCount(0);

    await page.getByTestId("canvas-node-start").click();
    await expect(page.getByTestId("runner-start-host-groups")).toBeVisible();
    await page.getByTestId("runner-host-group-label-0").fill("web");
    await page.getByTestId("runner-host-group-hosts-0").fill("web-01\nweb-02");
    await page.getByTestId("runner-host-group-add").click();
    await page.getByTestId("runner-host-group-label-1").fill("db");
    await page.getByTestId("runner-host-group-hosts-1").fill("db-01");
    await page.getByTestId("runner-node-panel-apply").click();

    await page.getByTestId("runner-node-picker-toggle").click();
    await page.getByTestId("catalog-action-script-shell").click();
    await page.getByTestId("canvas-node-script-shell").click();
    await expect(page.getByTestId("runner-node-targets")).toBeVisible();
    await expect(page.getByTestId("runner-node-target-options")).toContainText("web");
    await expect(page.getByTestId("runner-node-target-options")).toContainText("db");
    await page.getByTestId("runner-node-target-labels-input").fill("web, db");
    await expect(page.getByTestId("runner-node-target-summary")).toHaveCount(0);
    await page.getByTestId("runner-node-panel-apply").click();
    await page.getByTestId("runner-node-panel-close").click();

    const saveRequest = page.waitForRequest((req) =>
      req.url().includes("/api/runner-studio/workflows/graph") && req.method() === "POST",
    );
    await page.getByTestId("runner-toolbar-save").click();
    const request = await saveRequest;
    const payload = request.postDataJSON();
    const graph = payload.graph;
    expect(graph.workflow.inventory.groups.web.hosts).toEqual(["web-01", "web-02"]);
    expect(graph.workflow.inventory.groups.db.hosts).toEqual(["db-01"]);
    expect(graph.workflow.inventory.hosts["web-01"].address).toBe("web-01");
    expect(graph.nodes.find((node) => node.id === "script-shell").step.targets).toEqual(["web", "db"]);
    expect(graph.nodes.find((node) => node.id === "end")).toBeTruthy();
  });

  test("opens the node library from a Dify-like canvas context menu", async ({ page }) => {
    await createBlankWorkflow(page);

    await openCanvasContextMenu(page);
    await expect(page.getByTestId("runner-canvas-context-menu")).toContainText("添加节点");
    await page.getByTestId("runner-context-add-node").click();

    await expect(page.getByTestId("runner-node-picker")).toBeVisible();
    await expect(page.getByTestId("runner-canvas-context-menu")).toHaveCount(0);
  });

  test("deletes selected workflow nodes from the keyboard and node panel", async ({ page }) => {
    await createBlankWorkflow(page);

    await page.getByTestId("runner-node-picker-toggle").click();
    await page.getByTestId("catalog-action-script-shell").click();
    await page.getByTestId("canvas-node-script-shell").click();
    await expect(page.getByTestId("runner-node-panel")).toBeVisible();
    await page.keyboard.press("Delete");
    await expect(page.getByTestId("canvas-node-script-shell")).toHaveCount(0);
    await expect(page.getByTestId("runner-node-panel")).toHaveCount(0);

    await page.getByTestId("runner-node-picker-toggle").click();
    await page.getByTestId("catalog-action-builtin-tcp_ping").click();
    await page.getByTestId("canvas-node-builtin-tcp-ping").click();
    await expect(page.getByTestId("runner-node-panel-delete")).toBeVisible();
    await page.getByTestId("runner-node-panel-delete").click();
    await expect(page.getByTestId("canvas-node-builtin-tcp-ping")).toHaveCount(0);
    await expect(page.getByTestId("runner-node-panel")).toHaveCount(0);
  });

  test("adds nodes at the canvas context position and persists manual edge changes", async ({ page }) => {
    await createBlankWorkflow(page);
    const canvas = page.getByTestId("runner-canvas-dropzone");
    const canvasBox = await canvas.boundingBox();
    expect(canvasBox).not.toBeNull();
    const clickPoint = { x: Math.round(canvasBox.width * 0.82), y: Math.round(canvasBox.height * 0.68) };

    await page.mouse.click(canvasBox.x + clickPoint.x, canvasBox.y + clickPoint.y, { button: "right" });
    await expect(page.getByTestId("runner-canvas-context-menu")).toBeVisible();
    await page.getByTestId("runner-context-add-node").click();
    await page.getByTestId("catalog-action-script-shell").click();
    const firstNodeBox = await page.getByTestId("canvas-node-script-shell").boundingBox();
    expect(firstNodeBox).not.toBeNull();
    expect(Math.abs(firstNodeBox.x + firstNodeBox.width / 2 - (canvasBox.x + clickPoint.x))).toBeLessThan(130);
    expect(Math.abs(firstNodeBox.y + firstNodeBox.height / 2 - (canvasBox.y + clickPoint.y))).toBeLessThan(110);

    const secondClickPoint = {
      x: Math.round(firstNodeBox.x - canvasBox.x + firstNodeBox.width / 2 - 230),
      y: Math.round(firstNodeBox.y - canvasBox.y + firstNodeBox.height / 2 + 90),
    };
    await page.mouse.click(canvasBox.x + secondClickPoint.x, canvasBox.y + secondClickPoint.y, { button: "right" });
    await expect(page.getByTestId("runner-canvas-context-menu")).toBeVisible();
    await page.getByTestId("runner-context-add-node").click();
    await page.getByTestId("catalog-action-script-shell").click();
    await expect(page.getByTestId("canvas-node-script-shell-2")).toBeVisible();
    const secondNodeBox = await page.getByTestId("canvas-node-script-shell-2").boundingBox();
    expect(secondNodeBox).not.toBeNull();
    expect(Math.abs(secondNodeBox.x + secondNodeBox.width / 2 - (canvasBox.x + secondClickPoint.x))).toBeLessThan(40);
    expect(Math.abs(secondNodeBox.y + secondNodeBox.height / 2 - (canvasBox.y + secondClickPoint.y))).toBeLessThan(40);

    const sourceHandle = await page.getByTestId("canvas-node-script-shell").getByTitle("下一步").boundingBox();
    const targetHandle = await page.getByTestId("canvas-node-script-shell-2").locator(".runner-canvas-handle.input").boundingBox();
    expect(sourceHandle).not.toBeNull();
    expect(targetHandle).not.toBeNull();
    await page.mouse.move(sourceHandle.x + sourceHandle.width / 2, sourceHandle.y + sourceHandle.height / 2);
    await page.mouse.down();
    await page.mouse.move(targetHandle.x + targetHandle.width / 2, targetHandle.y + targetHandle.height / 2, { steps: 8 });
    await page.mouse.up();

    const saveManualEdge = page.waitForRequest((req) =>
      req.url().includes("/api/runner-studio/workflows/graph") && req.method() === "POST",
    );
    await page.getByTestId("runner-toolbar-save").click();
    const graphWithManualEdge = (await saveManualEdge).postDataJSON().graph;
    expect(graphWithManualEdge.edges).toEqual(
      expect.arrayContaining([expect.objectContaining({ source: "script-shell", target: "script-shell-2" })]),
    );

    const firstEdge = page.locator(".runner-flow-edge-hover-path").first();
    await firstEdge.hover({ force: true });
    await expect(page.getByRole("button", { name: "在连线上插入节点" })).toBeVisible();
    await expect(page.getByTestId("runner-edge-delete-start-script-shell-next")).toHaveCount(0);
    const startHandle = await page.getByTestId("canvas-node-start").getByTitle("下一步").boundingBox();
    const firstNodeInput = await page.getByTestId("canvas-node-script-shell").locator(".runner-canvas-handle.input").boundingBox();
    expect(startHandle).not.toBeNull();
    expect(firstNodeInput).not.toBeNull();
    const edgeClickX = startHandle.x + startHandle.width / 2 + (firstNodeInput.x + firstNodeInput.width / 2 - (startHandle.x + startHandle.width / 2)) * 0.25;
    const edgeClickY = startHandle.y + startHandle.height / 2 + (firstNodeInput.y + firstNodeInput.height / 2 - (startHandle.y + startHandle.height / 2)) * 0.25;
    await page.mouse.click(edgeClickX, edgeClickY);
    await expect(page.locator(".react-flow__edge.selected")).toHaveCount(1);
    await expect(page.locator(".react-flow__edge.selected .runner-flow-edge-path")).toHaveCSS("stroke", "rgb(37, 99, 235)");
    await page.keyboard.press("Delete");
    const saveAfterDelete = page.waitForRequest((req) =>
      req.url().includes("/api/runner-studio/workflows/") && req.url().endsWith("/graph") && ["POST", "PUT"].includes(req.method()),
    );
    await page.getByTestId("runner-toolbar-save").click();
    const graphAfterDelete = (await saveAfterDelete).postDataJSON().graph;
    expect(graphAfterDelete.edges.some((edge) => edge.source === "start" && edge.target === "script-shell")).toBe(false);
  });

  test("keeps run details closed on failed run submission and surfaces the failure reason", async ({ page }) => {
    await page.unroute("**/api/runner-studio/runs");
    await page.route("**/api/runner-studio/runs", (route) =>
      route.fulfill({
        status: 500,
        json: { error: "script.shell requires args.script" },
      }),
    );

    await createBlankWorkflow(page);
    await page.getByTestId("runner-node-picker-toggle").click();
    await page.getByTestId("catalog-action-script-shell").click();

    const runRequest = page.waitForRequest((req) =>
      req.url().includes("/api/runner-studio/runs") && req.method() === "POST",
    );
    await page.getByTestId("runner-toolbar-run").click();
    await runRequest;
    await expect(page.getByTestId("runner-toolbar-run")).toBeDisabled();
    await expect(page.getByTestId("runner-toolbar-run")).toHaveAttribute("title", /8 秒/);

    await expect(page.getByTestId("runner-run-drawer")).toHaveCount(0);
    await expect(page.getByTestId("runner-studio-topbar")).toContainText("运行失败");
    await expect(page.getByTestId("canvas-node-start")).toHaveClass(/run-failed/);
    await expect(page.getByTestId("canvas-node-start")).toContainText("失败");

    await page.getByTestId("runner-toolbar-run-details").click();
    await expect(page.getByTestId("runner-run-drawer")).toBeVisible();
    await expect(page.getByTestId("runner-run-history-panel")).toContainText("运行记录");
    await expect(page.getByTestId("runner-run-panel")).toHaveCount(0);
    await page.locator(".runner-run-history-row").click();
    await expect(page.getByTestId("runner-run-panel")).toContainText("运行提交失败");
    await expect(page.getByTestId("runner-run-panel")).toContainText("script.shell requires args.script");
    await expect(page.getByTestId("runner-run-panel")).not.toContainText("暂无日志。");
  });

  test("labels runtime failures separately from submission failures and shows stderr", async ({ page }) => {
    await page.unroute("**/api/runner-studio/runs/run-e2e/events/history");
    await page.route("**/api/runner-studio/runs/run-e2e/events/history", (route) =>
      route.fulfill({
        json: [
          { type: "run_start", run_id: "run-e2e", status: "running" },
          {
            type: "host_result",
            run_id: "run-e2e",
            step: "script-shell",
            host: "server-local",
            status: "failed",
            message: "script.shell failed: exit status 9",
            output: {
              stdout: "about-to-fail stdout\n",
              stderr: "intentional failure: missing deployment token\n",
            },
          },
          { type: "node_finished", run_id: "run-e2e", status: "failed", message: "script.shell failed: exit status 9", output: { node_id: "script-shell" } },
          { type: "run_finish", run_id: "run-e2e", status: "failed", message: "script.shell failed: exit status 9" },
        ],
      }),
    );

    await createBlankWorkflow(page);
    await page.getByTestId("runner-node-picker-toggle").click();
    await page.getByTestId("catalog-action-script-shell").click();

    const runRequest = page.waitForRequest((req) =>
      req.url().includes("/api/runner-studio/runs") && req.method() === "POST",
    );
    await page.getByTestId("runner-toolbar-run").click();
    await runRequest;

    await expect(page.getByTestId("runner-run-drawer")).toHaveCount(0);
    await expect(page.getByTestId("runner-studio-topbar")).toContainText("运行失败");
    await expect(page.getByTestId("canvas-node-script-shell")).toHaveClass(/run-failed/);

    await page.getByTestId("runner-toolbar-run-details").click();
    await expect(page.getByTestId("runner-run-history-panel")).toBeVisible();
    const historyListStyle = await page.locator(".runner-run-history-list").evaluate((node) => {
      const style = window.getComputedStyle(node);
      return { display: style.display, gap: style.gap };
    });
    expect(historyListStyle).toEqual({ display: "grid", gap: "10px" });
    await expect(page.getByTestId("runner-run-history-panel")).toContainText("运行记录");
    await expect(page.getByTestId("runner-run-history-panel")).toContainText("run-e2e");
    await expect(page.getByTestId("runner-run-history-panel")).toContainText("failed");
    await expect(page.getByTestId("runner-run-history-panel")).toContainText("script-shell");
    await expect(page.getByTestId("runner-run-panel")).toHaveCount(0);
    await page.getByTestId("runner-run-history-row-run-e2e").click();
    await expect(page.getByTestId("runner-run-detail-panel")).toBeVisible();
    await expect(page.getByTestId("runner-run-panel")).toContainText("运行失败：script.shell failed: exit status 9");
    await expect(page.getByTestId("runner-run-panel")).toContainText("intentional failure: missing deployment token");
    await expect(page.getByTestId("runner-run-panel")).not.toContainText("运行提交失败：script.shell failed");
    const hasHorizontalOverflow = await page.getByTestId("runner-run-drawer").locator(".runner-studio-run-drawer-body").evaluate((node) => node.scrollWidth > node.clientWidth + 1);
    expect(hasHorizontalOverflow).toBe(false);

    await page.getByTestId("runner-run-drawer-close").click();
    await page.getByTestId("canvas-node-script-shell").click();
    await page.getByTestId("runner-node-panel-open-run").click();
    await expect(page.getByTestId("runner-run-drawer")).toHaveCount(0);
    await expect(page.getByTestId("runner-node-panel-tab-last-run")).toHaveClass(/active/);
    await expect(page.getByTestId("runner-node-last-run-view")).toBeVisible();
    await expect(page.getByTestId("runner-node-last-run-view")).toContainText("失败原因");
    await expect(page.getByTestId("runner-node-last-run-view")).toContainText("intentional failure: missing deployment token");
    await expect(page.getByTestId("runner-node-last-run-view")).not.toContainText("运行记录");
  });

  test("disables rapid reruns and explains network submission failures in node details", async ({ page }) => {
    await page.unroute("**/api/runner-studio/runs");
    let runSubmissions = 0;
    await page.route("**/api/runner-studio/runs", (route) => {
      runSubmissions += 1;
      return route.abort("failed");
    });

    await createBlankWorkflow(page);
    await page.getByTestId("runner-node-picker-toggle").click();
    await page.getByTestId("catalog-action-script-shell").click();

    const runRequest = page.waitForRequest((req) =>
      req.url().includes("/api/runner-studio/runs") && req.method() === "POST",
    );
    await page.getByTestId("runner-toolbar-run").click();
    await runRequest;
    await expect(page.getByTestId("runner-toolbar-run")).toBeDisabled();
    await expect(page.getByTestId("runner-toolbar-run")).toHaveAttribute("title", /8 秒/);
    expect(runSubmissions).toBe(1);

    await expect(page.getByTestId("runner-run-drawer")).toHaveCount(0);
    await expect(page.getByTestId("runner-studio-topbar")).toContainText("运行失败");
    await page.getByTestId("canvas-node-script-shell").click();
    await page.getByTestId("runner-node-panel-open-run").click();

    await expect(page.getByTestId("runner-run-drawer")).toHaveCount(0);
    await expect(page.getByTestId("runner-node-panel-tab-last-run")).toHaveClass(/active/);
    await expect(page.getByTestId("runner-node-last-run-view")).toContainText("运行提交失败");
    await expect(page.getByTestId("runner-node-last-run-view")).toContainText("Failed to fetch");
    await expect(page.getByTestId("runner-node-last-run-view")).toContainText("网络请求失败");
    await expect(page.getByTestId("runner-node-last-run-view")).toContainText("Runner 服务");
  });

  test("shows localized filtered edge insert picker cards", async ({ page }) => {
    await createBlankWorkflow(page);
    await page.getByTestId("runner-node-picker-toggle").click();
    await page.getByTestId("catalog-action-script-shell").click();

    const initialEdge = page.locator(".runner-flow-edge-hover-path").first();
    await expect(initialEdge).toBeVisible();
    await initialEdge.hover({ force: true });
    await page.getByRole("button", { name: "在连线上插入节点" }).click();

    const picker = page.getByTestId("runner-edge-node-picker");
    await expect(picker).toBeVisible();
    await expect(picker).toContainText("执行 shell 脚本内容，可配置输入、输出、重试和超时。");
    await expect(picker).toContainText("根据变量或表达式选择 IF / ELSE 后续路径。");
    await expect(picker).toContainText("检查 TCP 主机端口是否可达。");
    await expect(picker).not.toContainText("在高风险步骤前暂停，等待人工确认后继续。");
    await expect(picker).not.toContainText("Command");
    await expect(picker).not.toContainText("Notify");
    await expect(picker).not.toContainText("Variable Aggregator");
    await expect(picker).not.toContainText("Run a shell command");
  });

  test("Workflow AI confirms a plan once and adds a visible workflow node", async ({ page }) => {
    await page.goto("/runner");
    await page.getByTestId("runner-create-workflow").click();

    await clickToolbar(page, "ai-generate");
    await expect(page.getByTestId("workflow-ai-drawer")).toBeVisible();
    await expect(page.getByTestId("workflow-ai-context-card")).toHaveCount(0);
    await expect(page.getByTestId("workflow-ai-updated-label")).toContainText("修改");
    await page.getByPlaceholder(/Workflow AI/).fill("添加验证步骤");
    await page.getByRole("button", { name: "Send" }).click();
    const drawer = page.getByTestId("workflow-ai-drawer");
    await expect(page.getByTestId("workflow-ai-plan-card")).toContainText("添加验证步骤");
    await expect(drawer.getByRole("button", { name: "Start" })).toHaveCount(0);
    await expect(drawer.getByRole("button", { name: "Apply" })).toHaveCount(0);
    await drawer.getByPlaceholder(/Workflow AI/).fill("确认");
    await drawer.getByRole("button", { name: "Send" }).click();

    await expect(page.getByTestId("canvas-node-ai-step-item-1")).toContainText("添加验证步骤");
    await drawer.getByRole("button", { name: "事件" }).click();
    await expect(page.getByTestId("workflow-event-drawer")).toContainText("workflow.ai.plan.confirmed");
    await expect(page.getByTestId("workflow-event-drawer")).toContainText("workflow.graph.node.added");
  });

  test("publish review requires graph hash, publish precheck hash, and publish note", async ({ page }) => {
    await page.goto("/runner");
    await page.getByTestId("runner-create-workflow").click();

    await clickToolbar(page, "publish");
    await expect(page.getByText("缺少当前 validated_graph_hash")).toBeVisible();
    await expect(page.getByTestId("publish-confirm")).toBeDisabled();
    await page.getByRole("button", { name: "取消" }).click();

    await clickToolbar(page, "validate");
    await clickToolbar(page, "publish");
    await expect(page.getByText("发布前检查未通过或已过期")).toBeVisible();
    await expect(page.getByTestId("publish-confirm")).toBeDisabled();
    await page.getByRole("button", { name: "取消" }).click();

    await clickToolbar(page, "dry-run");
    await expect(page.getByTestId("runner-studio-topbar")).toContainText("dry_run_passed");
    await clickToolbar(page, "publish");
    await expect(page.getByTestId("publish-confirm")).toBeDisabled();
    await page.getByTestId("publish-note").fill("change window approved");
    await expect(page.getByTestId("publish-confirm")).toBeEnabled();
  });

  test("allows dismissing local orchestration API notice", async ({ page }) => {
    await page.unroute("**/api/runner-studio/actions*");
    await page.route("**/api/runner-studio/actions*", (route) =>
      route.fulfill({ status: 503, json: { error: "runner studio unavailable" } }),
    );

    await page.goto("/runner");

    await expect(page.getByTestId("runner-studio-api-notice")).toBeVisible();
    await expect(page.getByTestId("runner-studio-api-notice")).toContainText("内置 Runner API");
    await expect(page.getByTestId("runner-studio-api-notice")).not.toContainText("Runner API upstream");
    await expect(page.getByTestId("runner-studio-api-notice")).not.toContainText("设置 Runner API upstream");
    await expect(page.getByTestId("runner-studio-api-notice")).not.toContainText("AIOPS_RUNNER_" + "DISABLED");
    await page.getByTestId("runner-api-notice-close").click();
    await expect(page.getByTestId("runner-studio-api-notice")).toHaveCount(0);
    await expect(page.getByTestId("runner-workflow-library")).toBeVisible();
  });

  test("keeps canvas fullscreen and run drawer within responsive viewports", async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
    await createBlankWorkflow(page);

    for (const viewport of [
      { width: 1103, height: 862 },
      { width: 1440, height: 900 },
      { width: 1920, height: 1080 },
    ]) {
      await page.setViewportSize(viewport);
      await expect(page.getByTestId("runner-canvas-dropzone")).toBeVisible();

      await page.getByTestId("runner-canvas-fullscreen-toggle").click();
      await expect(page.locator(".runner-studio-shell.fullscreen")).toBeVisible();
      await expectWithinViewport(page, page.getByTestId("runner-canvas-dropzone"));
      await page.getByTestId("runner-canvas-fullscreen-toggle").click();
      await expect(page.locator(".runner-studio-shell.fullscreen")).toHaveCount(0);

      await page.getByTestId("runner-toolbar-run-details").click();
      await expect(page.getByTestId("runner-run-drawer")).toBeVisible();
      await expectWithinViewport(page, page.getByTestId("runner-run-drawer"));
      await page.getByTestId("runner-run-drawer-close").click();
    }

    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto("/runner");
    await expect(page.getByTestId("runner-workflow-library")).toBeVisible();
    await expect(page.getByTestId("runner-create-workflow")).toBeVisible();
  });
});
