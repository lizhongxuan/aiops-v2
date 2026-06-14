import { expect, test } from "@playwright/test";

import { resolveUiFixturePreset } from "../../src/lib/uiFixturePresets";
import { openFixturePage } from "../helpers/uiFixtureHarness";

test("shows host agent full runtime trace views from deterministic fixture", async ({ page }) => {
  const fixture = createHostAgentFullRuntimeFixture();
  await page.route("**/api/v1/host-ops/child-agents/*/transcript", (route) => {
    const childAgentId = route.request().url().split("/child-agents/").at(-1)?.split("/transcript")[0] || "";
    return route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(fixture.state.hostOpsTranscripts?.[decodeURIComponent(childAgentId)] || { childAgentId, items: [] }),
    });
  });
  await page.route("**/api/v1/hosts", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        hosts: [
          { id: "host-a", name: "host-a.internal", address: "host-a.internal", status: "online" },
          { id: "host-b", name: "host-b.internal", address: "host-b.internal", status: "online" },
          { id: "host-c", name: "host-c.internal", address: "host-c.internal", status: "online" },
        ],
      }),
    }),
  );

  await openFixturePage(page, "/", fixture);

  await expect(page.getByText("Inspect generic service, process, file, and network state")).toBeVisible();
  await expect(page.getByTestId("host-ops-status-panel")).toBeVisible();
  await expect(page.getByText("共 3 个主机 Agent")).toBeVisible();

  await page.getByTestId("host-subagent-status-row-child-1").click();
  const drawer = page.getByTestId("host-subagent-drawer");
  await expect(drawer).toBeVisible();
  await expect(page.getByTestId("host-subagent-tab-task")).toBeVisible();
  await expect(page.getByTestId("host-subagent-tab-conversation")).toBeVisible();
  await expect(page.getByTestId("host-subagent-tab-prompt")).toBeVisible();
  await expect(page.getByTestId("host-subagent-tab-tools")).toBeVisible();
  await expect(page.getByTestId("host-subagent-tab-mcp-skills")).toBeVisible();
  await expect(page.getByTestId("host-subagent-tab-approval")).toBeVisible();
  await expect(page.getByTestId("host-subagent-tab-evidence")).toBeVisible();
  await expect(page.getByTestId("host-subagent-tab-receipts")).toBeVisible();
  await expect(drawer).toContainText("Trace 摘要");

  await page.getByTestId("host-subagent-tab-prompt").click();
  await expect(drawer).toContainText("Base runtime");
  await expect(drawer).toContainText("Host overlay");
  await expect(drawer).toContainText("Host task context");
  await expect(drawer).toContainText("Skill context");
  await expect(drawer).toContainText("MCP context");
  await expect(drawer).toContainText("host_agent.binding.v1");
  await expect(drawer).toContainText("host_agent.assigned_subtask.v1");
  await expect(drawer).toContainText("P0");
  await expect(drawer).toContainText("compact");
  await expect(drawer).not.toContainText("raw-sensitive-token");
  await expect(drawer).toHaveScreenshot("host-agent-full-runtime-prompt.png");

  await page.getByTestId("host-subagent-tab-tools").click();
  await expect(drawer).toContainText("HostCommandTool");
  await expect(drawer).toContainText("host_agent_tool");
  await expect(drawer).toContainText("operator-terminal");
  await expect(drawer).toContainText("human_terminal");
  await expect(drawer).toHaveScreenshot("host-agent-full-runtime-tools.png");

  await page.getByTestId("host-subagent-tab-evidence").click();
  await expect(drawer).toContainText("artifact://evidence/generic-service");
  await expect(drawer).toContainText("hash:generic-service-evidence");

  await page.getByTestId("host-subagent-tab-receipts").click();
  await expect(drawer).toContainText("report.created");
  await expect(drawer).toContainText("report.sent_to_manager");
  await expect(drawer).toHaveScreenshot("host-agent-full-runtime-evidence-receipts.png");
});

function createHostAgentFullRuntimeFixture() {
  const fixture = JSON.parse(JSON.stringify(resolveUiFixturePreset("host-ops-three-hosts")));
  const state = fixture.state;
  const turnId = state.currentTurnId;

  state.turns[turnId].user.text = "Inspect generic service, process, file, and network state across selected hosts.";
  state.turns[turnId].process[0].steps = [
    { id: "confirm", text: "确认通用多主机检查计划", status: "completed" },
    { id: "host-a-service", text: "检查 host-a.internal 的 generic service 状态", status: "running", childAgentIds: ["child-1"] },
    { id: "host-b-process", text: "检查 host-b.internal 的 generic process 状态", status: "pending", childAgentIds: ["child-2"] },
    { id: "host-c-network", text: "检查 host-c.internal 的 generic network 状态", status: "pending", childAgentIds: ["child-3"] },
  ];
  state.turns[turnId].process[0].text = "Manager generated a generic multi-host plan.";
  state.turns[turnId].process[1].text = "3 个 host-bound 子 Agent 已启动";

  state.hostMissions["mission-1"].mentionedHosts = [
    { tokenId: "mention-a", raw: "@host-a.internal", hostId: "host-a", address: "host-a.internal", displayName: "host-a.internal", source: "inventory", resolved: true },
    { tokenId: "mention-b", raw: "@host-b.internal", hostId: "host-b", address: "host-b.internal", displayName: "host-b.internal", source: "inventory", resolved: true },
    { tokenId: "mention-c", raw: "@host-c.internal", hostId: "host-c", address: "host-c.internal", displayName: "host-c.internal", source: "inventory", resolved: true },
  ];
  state.hostMissions["mission-1"].planSteps = state.turns[turnId].process[0].steps;

  state.childAgents["child-1"] = {
    ...state.childAgents["child-1"],
    hostAddress: "host-a.internal",
    hostDisplayName: "host-a.internal",
    task: "Inspect generic service and file state",
    currentStepTitle: "Inspect generic service and file state",
    runtimeProfile: {
      id: "host-agent-full-runtime",
      capabilities: ["prompt_compiler", "context_governance", "tool_surface_policy", "evidence_gate", "trace"],
    },
    promptSections: promptSections(),
    toolSurfaceSnapshot: toolSurfaceSnapshot(),
    mcpInstructionDeltas: [
      { id: "mcp-delta", server: "generic-docs", summary: "MCP instruction delta", sourceRef: "mcp://generic-docs/delta", redaction: "ref://mcp/delta" },
    ],
    skillActivationTrace: [
      { id: "skill-trace", skill: "generic-log-review", status: "activated", sourceRef: "skill://generic-log-review", redaction: "hash:skill-trace" },
    ],
    approvalTrace: [
      { id: "approval-trace", approvalId: "approval-generic-read", status: "approved", sourceRef: "ref://approval/generic-read", redaction: "hash:approval" },
    ],
    evidenceTrace: [
      {
        id: "evidence-generic-service",
        title: "Generic service evidence",
        source: "host_agent_tool",
        artifactRef: "artifact://evidence/generic-service",
        hash: "hash:generic-service-evidence",
        redaction: "ref://evidence/generic-service",
      },
    ],
    reportTimeline: [
      { id: "report-created", event: "report.created", status: "completed", sourceRef: "ref://report/draft" },
      { id: "report-sent", event: "report.sent_to_manager", status: "completed", sourceRef: "ref://report/final" },
    ],
    subtaskStatus: "queued",
    queueReason: "waiting for host session capacity",
    source: "manager_plan",
  };
  state.childAgents["child-2"] = {
    ...state.childAgents["child-2"],
    hostAddress: "host-b.internal",
    hostDisplayName: "host-b.internal",
    task: "Inspect generic process state",
    currentStepTitle: "Inspect generic process state",
  };
  state.childAgents["child-3"] = {
    ...state.childAgents["child-3"],
    hostAddress: "host-c.internal",
    hostDisplayName: "host-c.internal",
    task: "Inspect generic network state",
    currentStepTitle: "Inspect generic network state",
  };

  state.hostOpsTranscripts = {
    "child-1": {
      childAgentId: "child-1",
      items: [
        { id: "manager-message", type: "manager_message", content: "Inspect generic host resources with redacted evidence refs.", status: "completed" },
        { id: "host-agent-message", type: "assistant_message", content: "Collected generic service, process, file, and network evidence refs.", status: "completed" },
      ],
      agentMessages: [
        { id: "manager-message", role: "manager", content: "Inspect generic host resources." },
        { id: "host-agent-message", role: "host_agent", content: "Collected redacted evidence refs." },
      ],
    },
  };

  fixture.sessions.sessions[0].preview = "Inspect generic service, process, file, and network state.";
  return fixture;
}

function promptSections() {
  return [
    {
      id: "prompt-base",
      title: "Base runtime",
      category: "base_runtime",
      sectionId: "base.runtime.v1",
      retentionRank: "P0",
      compactAction: "keep",
      sourceRef: "ref://prompt/base-runtime",
      redaction: "hash:prompt-base",
    },
    {
      id: "prompt-overlay",
      title: "Host overlay",
      category: "host_overlay",
      sectionId: "host_agent.binding.v1",
      retentionRank: "P0",
      compactAction: "keep",
      sourceRef: "agent-message:generic-task",
      redaction: "redacted",
    },
    {
      id: "prompt-task",
      title: "Host task context",
      category: "host_task_context",
      sectionId: "host_agent.assigned_subtask.v1",
      retentionRank: "P1",
      compactAction: "compact",
      sourceRef: "agent-message:generic-task",
      redaction: "ref://task/context",
    },
    {
      id: "prompt-skill",
      title: "Skill context",
      category: "skill_context",
      sectionId: "skill.generic_log_review.v1",
      retentionRank: "P2",
      compactAction: "summarize",
      sourceRef: "skill://generic-log-review",
      redaction: "hash:skill-context",
    },
    {
      id: "prompt-mcp",
      title: "MCP context",
      category: "mcp_context",
      sectionId: "mcp.generic_docs.instructions.v1",
      retentionRank: "P2",
      compactAction: "delta",
      sourceRef: "mcp://generic-docs",
      redaction: "ref://mcp/context",
    },
  ];
}

function toolSurfaceSnapshot() {
  return [
    {
      id: "tool-host-command",
      name: "HostCommandTool",
      source: "host_agent_tool",
      status: "allowed",
      summary: "Read generic service, process, file, and network state",
      redaction: "ref://tool/host-command",
    },
    {
      id: "tool-human-terminal",
      name: "operator-terminal",
      source: "human_terminal",
      status: "recorded",
      summary: "Manual terminal observation attached by operator",
      redaction: "hash:human-terminal",
    },
  ];
}
