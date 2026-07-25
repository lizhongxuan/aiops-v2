// @ts-check
import { readFileSync } from "node:fs";

export const workflowStudioFixture = JSON.parse(readFileSync(new URL("../fixtures/workflow-studio.snapshot.fixture.json", import.meta.url), "utf8"));

const fixtureBasis = Object.freeze({
  resourceVersion: "fixture-rv-1",
  semanticHash: "sha256:fixture-semantic-1",
  catalogVersion: "fixture-catalog-v1",
  actionSchemaDigest: "sha256:fixture-action-schema-1",
  referenceSnapshotHash: "sha256:fixture-reference-snapshot-1",
  environmentBindingVersion: "sha256:fixture-environment-binding-1",
});

export async function installWorkflowStudioFixture(page) {
  const graph = workflowStudioFixture.workflow.graph;
  await page.routeWebSocket("**/ws", (socket) => {
    socket.send(JSON.stringify({
      kind: "workflow-test-snapshot",
      config: {
        workflowCapabilities: workflowAgentCapabilitiesFixture(),
        workflowWorkspace: {
          schemaVersion: "aiops.workflow.workspace.v1",
          workflowKey: { tenantId: "runner-default", workflowId: workflowStudioFixture.workflow.name },
          committed: {
            workflowKey: { tenantId: "runner-default", workflowId: workflowStudioFixture.workflow.name },
            revision: {
              resourceVersion: fixtureBasis.resourceVersion,
              semanticHash: fixtureBasis.semanticHash,
              layoutHash: "sha256:fixture-layout-1",
            },
            graph,
            sequence: 1,
            loadedAt: "2026-07-21T08:00:00Z",
          },
          runGateFacts: {
            currentBasis: fixtureBasis,
            environment: {
              name: "staging",
              bindingVersion: fixtureBasis.environmentBindingVersion,
            },
            postCheck: {
              basis: fixtureBasis,
              mode: "draft_non_production",
              status: "dry_run_passed",
              checkedAt: "2026-07-21T08:02:00Z",
              diagnostics: [],
            },
          },
          validationArtifacts: [{
            artifactId: "fixture-validation-1",
            basis: fixtureBasis,
            status: "valid",
            diagnostics: [],
            issuedAt: "2026-07-21T08:01:00Z",
          }],
          durableHistory: [{
            source: "durable_transaction",
            transactionId: "fixture-transaction-1",
            businessLabel: "更新容量检查阈值",
            status: "completed",
            occurredAt: "2026-07-21T08:00:30Z",
            actorLabel: "测试操作员",
            touchedObjectCount: 1,
            reversible: true,
          }],
          events: [],
          lastSequence: 1,
        },
      },
    }));
  });
  await page.route("**/api/v1/state", (route) => route.fulfill({
    json: {
      schemaVersion: "aiops.transport.v2",
      sessionId: "workflow-snapshot",
      threadId: "workflow-snapshot",
      status: "idle",
      turns: {},
      turnOrder: [],
      pendingApprovals: {},
      mcpSurfaces: {},
      artifacts: {},
      runtimeLiveness: { activeTurns: {}, activeAgents: {}, pendingApprovals: {}, pendingUserInputs: {}, activeCommandStreams: {} },
      seq: 1,
      updatedAt: "2026-07-21T08:00:00Z",
    },
  }));
  await page.route("**/api/v1/sessions", (route) => route.fulfill({ json: { activeSessionId: "workflow-snapshot", sessions: [] } }));
  await page.route("**/api/runner-studio/actions*", (route) => route.fulfill({ json: { items: workflowStudioFixture.actions } }));
  await page.route("**/api/runner-studio/workflows", (route) => route.fulfill({ json: { workflows: [workflowStudioFixture.workflow] } }));
  await page.route("**/api/runner-studio/workflows/capacity-inspection/graph", (route) => route.fulfill({ json: graph }));
}

export function workflowAgentCapabilitiesFixture() {
  return {
    schemaVersion: "aiops.workflow.rollout.v1",
    stage: "internal_apply",
    saltVersion: "workflow-fixture-v1",
    rolloutPercentage: 100,
    bucketBasisPoints: 0,
    decision: "percentage",
    capabilities: {
      workflow_agent_ui: true,
      workflow_mcp_read: true,
      workflow_mcp_preview: true,
      workflow_mcp_mutation: true,
      workflow_delete: false,
      workflow_publish: false,
      workflow_execute: false,
      workflow_domain_events: true,
      workflow_plan_ir: false,
      workflow_elk_layout: false,
      workflow_edit_grants: false,
      workflow_external_mcp: false,
      workflow_mcp_apps: false,
    },
  };
}

export function observeWorkflowPageHealth(page) {
  /** @type {string[]} */
  const consoleErrors = [];
  /** @type {string[]} */
  const failedRequests = [];
  page.on("console", (message) => {
    if (message.type() === "error") consoleErrors.push(message.text());
  });
  page.on("requestfailed", (request) => {
    if (!request.url().includes("/ws")) failedRequests.push(`${request.method()} ${request.url()} ${request.failure()?.errorText || "failed"}`);
  });
  return {
    async assertHealthy() {
      const frameworkError = await page.locator("vite-error-overlay, nextjs-portal, [data-framework-error], #webpack-dev-server-client-overlay").count();
      if (frameworkError) throw new Error(`framework error layer count=${frameworkError}`);
      if (consoleErrors.length) throw new Error(`console errors:\n${consoleErrors.join("\n")}`);
      if (failedRequests.length) throw new Error(`failed requests:\n${failedRequests.join("\n")}`);
    },
  };
}

export async function openWorkflowStudioFixture(page, viewport) {
  await page.setViewportSize(viewport);
  await installWorkflowStudioFixture(page);
  const health = observeWorkflowPageHealth(page);
  await page.goto("/runner/capacity-inspection", { waitUntil: "networkidle" });
  return health;
}
