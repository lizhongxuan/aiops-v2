// @ts-check
import { expect, test } from "@playwright/test";

import {
  observeWorkflowPageHealth,
  workflowAgentCapabilitiesFixture,
  workflowStudioFixture,
} from "../helpers/workflowStudioPageFixture.js";

const WORKFLOW_ID = workflowStudioFixture.workflow.name;
const SESSION_ID = "candidate-dialog-session";
const TURN_ID = "candidate-dialog-turn";
const APPROVAL_ID = "candidate-dialog-approval";
const workflowKey = { tenantId: "runner-default", workflowId: WORKFLOW_ID };
const revision = {
  resourceVersion: "fixture-rv-1",
  semanticHash: "sha256:fixture-semantic-1",
  layoutHash: "sha256:fixture-layout-1",
};
const binding = {
  candidateId: "candidate-dialog-candidate",
  changeSetId: "candidate-dialog-change-set",
  previewHash: "sha256:candidate-dialog-preview",
  expectedRevision: revision.resourceVersion,
};

test.describe("Workflow Candidate final confirmation browser contract", () => {
  test("[UX-HDR-007] Workspace Apply owns one modal confirmation and submits one typed approval decision", async ({ page }, testInfo) => {
    /** @type {Array<Record<string, any>>} */
    const transportRequests = [];
    const health = observeWorkflowPageHealth(page);
    const artifact = candidateArtifact();
    const transportState = candidateTransportState(artifact);

    await installCandidateDialogFixture(page, transportState, transportRequests);
    await page.setViewportSize({ width: 1274, height: 964 });
    await page.goto(`/runner/${encodeURIComponent(WORKFLOW_ID)}`, { waitUntil: "networkidle" });
    await expect(page.getByTestId("runner-studio-page")).toBeVisible();

    // Opening the real Workbench mounts the workflow AssistantTransport and
    // reconciles its typed Candidate into the authoritative Workspace.
    await page.getByTestId("workflow-workbench-trigger").click();
    const panel = page.getByTestId("workflow-ai-transport-panel");
    await expect(panel).toBeVisible();
    const approvalCard = panel.getByTestId("workflow-approval-card");
    await expect(approvalCard).toBeVisible();
    await expect(approvalCard.getByTestId("workflow-candidate-approval-owner")).toBeVisible();
    await expect(approvalCard.getByTestId("workflow-approval-accept")).toHaveCount(0);
    await expect(approvalCard.getByTestId("workflow-approval-reject")).toHaveCount(0);

    await page.getByTestId("workflow-context-tab-changes").click();
    const apply = page.getByTestId("workflow-candidate-apply");
    const owner = page.getByTestId("workflow-candidate-workspace-actions");
    await expect(apply).toHaveCount(1);
    await expect(apply).toBeVisible();
    await expect(apply).toBeEnabled();

    await apply.click();
    const dialog = page.getByTestId("workflow-candidate-apply-dialog");
    const confirm = page.getByTestId("workflow-candidate-confirm-apply");
    const cancel = dialog.getByRole("button", { name: "取消", exact: true });
    await expect(dialog).toBeVisible();
    await expect(confirm).toBeFocused();
    await expect(owner).toHaveAttribute("aria-hidden", "true");
    await expect(owner).toHaveAttribute("inert", "");

    // Real keyboard traversal remains trapped inside the modal and never
    // reaches the inert Candidate owner behind it.
    for (const key of ["Shift+Tab", "Tab", "Tab", "Shift+Tab"]) {
      await page.keyboard.press(key);
      await expect(apply).not.toBeFocused();
      await expect(dialog).toHaveCount(1);
    }
    await expect(page.getByTestId("workflow-candidate-apply-dialog")).toHaveCount(1);
    expect(approvalDecisionCommands(transportRequests)).toEqual([]);
    await expect(dialog).toHaveScreenshot("candidate-apply-dialog.png", {
      animations: "disabled",
    });

    await page.keyboard.press("Escape");
    await expect(dialog).toHaveCount(0);
    await expect(page.getByTestId("workflow-context-dock")).toBeVisible();
    await expect(apply).toBeFocused();

    await apply.press("Enter");
    await expect(dialog).toBeVisible();
    await cancel.click();
    await expect(dialog).toHaveCount(0);
    await expect(apply).toBeFocused();

    await apply.click();
    await expect(dialog).toBeVisible();
    await expect(confirm).toBeFocused();
    await testInfo.attach("candidate-final-confirmation-dialog", {
      body: await dialog.screenshot(),
      contentType: "image/png",
    });

    const decisionRequest = page.waitForRequest((request) => {
      if (!request.url().endsWith("/api/v1/assistant/transport") || request.method() !== "POST") return false;
      try {
        return request.postDataJSON()?.commands?.some((command) => (
          command?.type === "aiops.approval-decision"
          && command?.approvalId === APPROVAL_ID
        )) === true;
      } catch {
        return false;
      }
    });
    await confirm.click();
    await decisionRequest;

    await expect(dialog).toHaveCount(0);
    await expect(owner).toHaveAttribute("aria-hidden", "false");
    await expect(owner).not.toHaveAttribute("inert");
    await expect(apply).toBeFocused();
    await expect.poll(() => approvalDecisionCommands(transportRequests)).toHaveLength(1);
    expect(approvalDecisionCommands(transportRequests)[0]).toMatchObject({
      type: "aiops.approval-decision",
      sessionId: SESSION_ID,
      turnId: TURN_ID,
      approvalId: APPROVAL_ID,
      decision: "accept",
    });

    await page.getByTestId("workflow-context-tab-ai").click();
    await expect(approvalCard.getByTestId("workflow-approval-accept")).toHaveCount(0);
    await expect(approvalCard.getByTestId("workflow-approval-reject")).toHaveCount(0);
    await health.assertHealthy();
  });
});

async function installCandidateDialogFixture(page, transportState, transportRequests) {
  const graph = workflowStudioFixture.workflow.graph;
  await page.addInitScript(({ key, value }) => {
    window.sessionStorage.setItem(key, value);
  }, {
    key: `aiops.workflow.transport.session.${encodeURIComponent(WORKFLOW_ID)}`,
    value: SESSION_ID,
  });
  await page.routeWebSocket("**/ws", (socket) => {
    socket.send(JSON.stringify({
      kind: "workflow-candidate-dialog-test-snapshot",
      config: {
        workflowCapabilities: workflowAgentCapabilitiesFixture(),
        workflowWorkspace: {
          schemaVersion: "aiops.workflow.workspace.v1",
          workflowKey,
          committed: {
            workflowKey,
            revision,
            graph,
            sequence: 1,
            loadedAt: "2026-07-23T08:00:00Z",
          },
          runGateFacts: {
            currentBasis: {
              resourceVersion: revision.resourceVersion,
              semanticHash: revision.semanticHash,
              catalogVersion: "fixture-catalog-v1",
              actionSchemaDigest: "sha256:fixture-action-schema-1",
              referenceSnapshotHash: "sha256:fixture-reference-snapshot-1",
              environmentBindingVersion: "sha256:fixture-environment-binding-1",
            },
            environment: {
              name: "staging",
              bindingVersion: "sha256:fixture-environment-binding-1",
            },
          },
          validationArtifacts: [],
          durableHistory: [],
          events: [],
          lastSequence: 1,
        },
      },
    }));
  });
  await page.route("**/api/v1/state", (route) => route.fulfill({
    json: {
      schemaVersion: "aiops.transport.v2",
      sessionId: "workflow-page-shell",
      threadId: "workflow-page-shell",
      status: "idle",
      turns: {},
      turnOrder: [],
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
      updatedAt: "2026-07-23T08:00:00Z",
    },
  }));
  await page.route("**/api/v1/sessions", (route) => route.fulfill({
    json: { activeSessionId: "workflow-page-shell", sessions: [] },
  }));
  await page.route("**/api/runner-studio/actions*", (route) => route.fulfill({
    json: { items: workflowStudioFixture.actions },
  }));
  await page.route("**/api/runner-studio/workflows", (route) => route.fulfill({
    json: { workflows: [workflowStudioFixture.workflow] },
  }));
  await page.route(`**/api/runner-studio/workflows/${WORKFLOW_ID}/graph`, (route) => route.fulfill({ json: graph }));
  await page.route("**/api/v1/assistant/resume", (route) => route.fulfill({
    status: 200,
    contentType: "text/plain; charset=utf-8",
    body: transportStateBody(transportState),
  }));
  await page.route("**/api/v1/assistant/transport", (route) => {
    transportRequests.push(route.request().postDataJSON());
    return route.fulfill({
      status: 200,
      contentType: "text/plain; charset=utf-8",
      body: transportStateBody(transportState),
    });
  });
}

function candidateTransportState(artifact) {
  const artifactBlock = {
    id: "candidate-dialog-artifact-block",
    type: "artifact",
    kind: "assistant",
    status: "completed",
    artifact,
  };
  const approvalBlock = {
    id: "candidate-dialog-approval-block",
    type: "approval",
    kind: "approval",
    status: "blocked",
    approvalId: APPROVAL_ID,
    text: "确认应用当前候选修改",
  };
  return {
    schemaVersion: "aiops.transport.v2",
    sessionId: SESSION_ID,
    threadId: `workflow:${WORKFLOW_ID}`,
    status: "blocked",
    currentTurnId: TURN_ID,
    turns: {
      [TURN_ID]: {
        id: TURN_ID,
        status: "blocked",
        workflowContext: {
          workflowId: WORKFLOW_ID,
          revision,
          selection: { nodeIds: [], edgeIds: [] },
          environment: {
            name: "staging",
            bindingVersion: "sha256:fixture-environment-binding-1",
          },
        },
        blockOrder: [artifactBlock.id, approvalBlock.id],
        blocksById: {
          [artifactBlock.id]: artifactBlock,
          [approvalBlock.id]: approvalBlock,
        },
        updatedAt: "2026-07-23T08:01:00Z",
      },
    },
    turnOrder: [TURN_ID],
    pendingApprovals: {
      [APPROVAL_ID]: {
        id: APPROVAL_ID,
        turnId: TURN_ID,
        status: "pending",
        reason: "应用当前候选修改",
        targetSummary: "容量检查节点",
        expectedEffect: "保存 1 项候选修改",
      },
    },
    mcpSurfaces: {},
    artifacts: {},
    hostMissions: {},
    childAgents: {},
    runtimeLiveness: {
      activeTurns: {},
      activeAgents: {},
      pendingApprovals: { [APPROVAL_ID]: true },
      pendingUserInputs: {},
      activeCommandStreams: {},
    },
    seq: 7,
    updatedAt: "2026-07-23T08:01:00Z",
  };
}

function candidateArtifact() {
  return {
    id: "candidate-dialog-artifact",
    type: "workflow_patch_preview",
    renderer: "workflow.change.preview.v2",
    schemaVersion: "aiops.workflow.edit.v2",
    createdAt: "2026-07-23T08:01:00Z",
    updatedAt: "2026-07-23T08:01:00Z",
    inlineData: {
      schemaVersion: "aiops.workflow.edit.v2",
      isError: false,
      content: [{ type: "text", text: "候选修改已生成" }],
      structuredContent: {
        kind: "preview",
        changeSet: {
          schemaVersion: "aiops.workflow.edit.v2",
          id: binding.changeSetId,
          workflowKey,
          candidateId: binding.candidateId,
          candidateRevision: 1,
          actor: "agent",
          intent: "更新容量检查说明",
          base: revision,
          operations: [{
            op: "update_node",
            nodeId: "check-capacity",
            set: { label: "检查磁盘容量（AI 候选）" },
          }],
          previewHash: binding.previewHash,
          validation: {
            kind: "validation",
            basis: {
              resourceVersion: revision.resourceVersion,
              semanticHash: revision.semanticHash,
              catalogVersion: "fixture-catalog-v1",
              actionSchemaDigest: "sha256:fixture-action-schema-1",
              referenceSnapshotHash: "sha256:fixture-reference-snapshot-1",
              environmentBindingVersion: "sha256:fixture-environment-binding-1",
            },
            valid: true,
            diagnostics: [],
          },
          state: "previewed",
          createdAt: "2026-07-23T08:01:00Z",
          expiresAt: "2030-01-01T00:00:00Z",
        },
        candidate: {
          id: binding.candidateId,
          workflowKey,
          currentChangeSetId: binding.changeSetId,
          currentRevision: 1,
          currentPreviewHash: binding.previewHash,
          state: "ready",
          expiresAt: "2030-01-01T00:00:00Z",
        },
        diff: { touchedPaths: ["nodes/check-capacity/label"] },
      },
    },
    actions: [{
      id: "candidate-dialog-apply-action",
      intent: "workflow.apply_candidate",
      label: "应用修改",
      target: { ...binding, approvalId: APPROVAL_ID },
    }],
  };
}

function transportStateBody(state) {
  return `aui-state:${JSON.stringify([{ type: "set", path: [], value: state }])}\n`;
}

function approvalDecisionCommands(requests) {
  return requests.flatMap((request) => Array.isArray(request?.commands)
    ? request.commands.filter((command) => command?.type === "aiops.approval-decision")
    : []);
}
