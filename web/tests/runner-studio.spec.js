// @ts-check
import { test, expect } from "@playwright/test";

const ACTIONS = {
  items: [
    { action: "cmd.run", label: "Command" },
    { action: "shell.run", label: "Shell Script" },
    { action: "script.shell", label: "Stored Script" },
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
  await page.getByTestId("runner-open-manager").click();
  await page.getByTestId("workflow-create-blank").click();
  await expect(page.getByTestId("runner-studio-topbar")).toContainText("runner-blank");
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
            step: "cmd-run",
            host: "server-local",
            status: "success",
            output: { stdout: "ok", exit_code: 0 },
          },
          { type: "step_finish", run_id: "run-e2e", step: "cmd-run", status: "success" },
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
    await page.route("**/api/runner-studio/workflows/*/publish", (route) =>
      route.fulfill({ json: { name: "runner-blank", status: "published", published_graph_hash: "graph-hash-e2e" } }),
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
    await expect(page.getByTestId("runner-workflow-library")).toHaveCount(0);

    await page.getByTestId("runner-back-to-library").click();

    await expect(page).toHaveURL(/\/runner$/);
    await expect(page.getByTestId("runner-workflow-library")).toContainText("PG Restore");
    await expect(page.getByTestId("runner-studio-topbar")).toHaveCount(0);
  });

  test("creates a blank workflow, drags nodes, configures I/O, validates, and dry-runs", async ({ page }) => {
    await page.goto("/runner");
    await page.getByTestId("runner-open-manager").click();
    await page.getByTestId("workflow-create-blank").click();
    await expect(page.getByTestId("runner-studio-topbar")).toContainText("runner-blank");
    await expect(page).toHaveURL(/\/runner\/runner-blank$/);
    await expect(page.locator(".runner-studio-canvas-head")).toHaveCount(0);

    await page.getByTestId("runner-node-picker-toggle").click();
    await page.getByTestId("catalog-action-cmd-run").dragTo(page.getByTestId("runner-canvas-dropzone"));
    await page.getByTestId("catalog-action-shell-run").dragTo(page.getByTestId("runner-canvas-dropzone"));
    await page.getByTestId("catalog-action-script-shell").dragTo(page.getByTestId("runner-canvas-dropzone"));
    await expect(page.getByTestId("canvas-node-cmd-run")).toBeVisible();
    await expect(page.getByTestId("canvas-node-shell-run")).toBeVisible();
    await expect(page.getByTestId("canvas-node-script-shell")).toBeVisible();

    await page.getByTestId("canvas-node-cmd-run").click();
    await expect(page.getByTestId("runner-node-panel")).toBeVisible();
    await page.getByTestId("runner-node-panel-tab-input").click();
    await page.getByTestId("input-add").click();
    await page.getByTestId("input-key-input_1").fill("backup_id");
    await page.getByTestId("runner-node-panel-tab-output").click();
    await page.getByTestId("output-add").click();
    await page.getByTestId("output-key-output_1").fill("restore_lsn");
    await page.getByTestId("runner-node-panel-apply").click();

    await page.getByTestId("runner-toolbar-validate").click();
    await expect(page.getByTestId("runner-studio-topbar")).toContainText("validated");

    const dryRunRequest = page.waitForRequest((req) =>
      req.url().includes("/api/runner-studio/workflows/graph/dry-run") && req.method() === "POST",
    );
    await page.getByTestId("runner-toolbar-dry-run").click();
    await dryRunRequest;

    await page.reload();
    await expect(page.getByTestId("runner-studio-topbar")).toContainText("runner-blank");
    await expect(page.getByTestId("canvas-node-cmd-run")).toBeVisible();
    await expect(page.getByTestId("canvas-node-shell-run")).toBeVisible();
    await expect(page.getByTestId("canvas-node-script-shell")).toBeVisible();
    await page.getByTestId("canvas-node-cmd-run").click();
    await page.getByTestId("runner-node-panel-tab-input").click();
    await expect(page.getByTestId("input-key-backup_id")).toHaveValue("backup_id");
    await page.getByTestId("runner-node-panel-tab-output").click();
    await expect(page.getByTestId("output-key-restore_lsn")).toHaveValue("restore_lsn");
    await page.getByTestId("runner-node-panel-close").click();

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
    await page.getByTestId("runner-toolbar-run-details").click();
    await expect(page.getByTestId("runner-run-panel")).toContainText("run-e2e");
    await expect(page.getByTestId("runner-run-panel")).toContainText("cmd-run");
    await expect(page.getByTestId("runner-run-panel")).toContainText("ok");
  });

  test("does not overwrite the graph when AI draft validation fails", async ({ page }) => {
    await page.goto("/runner");
    await page.getByTestId("runner-open-manager").click();
    await page.getByTestId("workflow-create-blank").click();

    await page.getByTestId("runner-toolbar-ai-generate").click();
    await page.getByTestId("runner-ai-instruction").fill("生成一个非法 AI patch");
    await page.getByTestId("runner-ai-generate").click();
    await expect(page.getByText("AI bad node")).toBeVisible();
    await page.getByTestId("runner-ai-apply").click();

    await expect(page.getByText("AI patch validation failed")).toBeVisible();
    await expect(page.getByTestId("canvas-node-ai-invalid")).toHaveCount(0);
  });

  test("publish review requires graph hash, dry run hash, and publish note", async ({ page }) => {
    await page.goto("/runner");
    await page.getByTestId("runner-open-manager").click();
    await page.getByTestId("workflow-create-blank").click();

    await page.getByTestId("runner-toolbar-publish").click();
    await expect(page.getByText("缺少当前 validated_graph_hash")).toBeVisible();
    await expect(page.getByTestId("publish-confirm")).toBeDisabled();
    await page.getByRole("button", { name: "取消" }).click();

    await page.getByTestId("runner-toolbar-validate").click();
    await page.getByTestId("runner-toolbar-publish").click();
    await expect(page.getByText("Dry Run 未通过或已过期")).toBeVisible();
    await expect(page.getByTestId("publish-confirm")).toBeDisabled();
    await page.getByRole("button", { name: "取消" }).click();

    await page.getByTestId("runner-toolbar-dry-run").click();
    await expect(page.getByTestId("runner-studio-topbar")).toContainText("dry_run_passed");
    await page.getByTestId("runner-toolbar-publish").click();
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
    await expect(page.getByTestId("runner-open-manager")).toBeVisible();
  });
});
