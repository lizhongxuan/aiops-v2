// @ts-check
import { expect, test } from "@playwright/test";

import {
  createChatFixtureSessions,
  createChatFixtureState,
  createProtocolFixtureSessions,
  createProtocolFixtureState,
  installUiFixture,
  waitForFixtureStable,
} from "../helpers/uiFixtureHarness.js";
import { resolveUiFixturePreset } from "../../src/lib/uiFixturePresets";

test.describe("host ops comprehensive user flow", () => {
  test("simulates configuring LLM, checking hosts, sending @hosts, and opening child transcript", async ({ page }) => {
    const hostOpsFixture = withDetailedHostOpsTranscript(resolveUiFixturePreset("host-ops-three-hosts"));
    const idleFixture = createIdleChatFixture();
    const transportRequests = [];
    const llmUpdates = [];

    await mockLLMConfig(page, llmUpdates);
    await mockHostInventory(page);
    await installHostOpsTranscriptRoute(page, hostOpsFixture);
    await installAssistantTransportRoute(page, hostOpsFixture, transportRequests);
    await installUiFixture(page, idleFixture);

    await page.goto("/settings/llm", { waitUntil: "networkidle" });
    await waitForFixtureStable(page);
    await expect(page.getByText("LLM 配置").first()).toBeVisible();
    await page.getByLabel("Provider", { exact: true }).selectOption("openai");
    await page.getByLabel("Model", { exact: true }).fill("gpt-5.4");
    await page.locator("label").filter({ hasText: "Base URL" }).locator("input").fill("https://www.aicodexcn.com/v1");
    await page.locator("label").filter({ hasText: "API Key" }).locator("input").fill("test-api-key-not-real");
    await page.getByTestId("llm-context-tokens-input").fill("200000");
    await page.getByRole("button", { name: "保存并重启 Runtime" }).click();
    await expect(page.getByText("配置已保存")).toBeVisible();
    expect(llmUpdates.at(-1)).toMatchObject({
      provider: "openai",
      baseURL: "https://www.aicodexcn.com/v1",
      model: "gpt-5.4",
      maxContextTokens: 200000,
    });
    expect(String(llmUpdates.at(-1)?.apiKey || "")).toBe("test-api-key-not-real");

    await page.goto("/settings/hosts", { waitUntil: "networkidle" });
    await waitForFixtureStable(page);
    await page.getByRole("button", { name: "接入配置" }).click();
    const hostTable = page.locator(".hosts-table-shell");
    await expect(hostTable).toContainText("1.1.1.1 / root");
    await expect(hostTable).toContainText("1.1.1.2 / root");
    await expect(hostTable).toContainText("1.1.1.3 / root");
    await expect(hostTable).toContainText("在线");
    await expect(hostTable).toContainText("可 SSH");

    await page.goto("/", { waitUntil: "networkidle" });
    await waitForFixtureStable(page);
    const requestText = "@1.1.1.1 和 @1.1.1.2 执行通用运维变更，@1.1.1.3 执行结果验证。";
    await expect(page.getByTestId("omnibar-input")).toBeVisible();
    await page.getByTestId("omnibar-input").fill(requestText);
    await page.getByTestId("omnibar-primary-action").click();

    await expect.poll(() => transportRequests.length, { timeout: 5000 }).toBe(1);
    const command = transportRequests[0]?.commands?.[0];
    expect(command?.type).toBe("add-message");
    expect(command?.message?.parts?.[0]?.text).toBe(requestText);
    const mentions = JSON.parse(command?.message?.metadata?.["aiops.hostops.mentions"] || "[]");
    expect(mentions.map((item) => item.raw)).toEqual(["@1.1.1.1", "@1.1.1.2", "@1.1.1.3"]);
    expect(command?.message?.metadata?.["aiops.hostops.clientDetectedMultiHost"]).toBe("true");

    const panel = page.getByTestId("host-ops-status-panel");
    await expect(panel).toBeVisible({ timeout: 10000 });
    await expect(panel).toContainText("共 5 个步骤，已经完成 0 个");
    await expect(panel).toContainText("共 3 个主机 Agent");
    await expect(panel).toContainText("@1.1.1.1(@1.1.1.1)");
    await expect(panel).toContainText("@1.1.1.2(@1.1.1.2)");
    await expect(panel).toContainText("@1.1.1.3(@1.1.1.3)");

    await page.getByTestId("host-subagent-status-row-child-1").click();
    const drawer = page.getByTestId("host-subagent-drawer");
    await expect(drawer).toBeVisible();
    await expect(page.getByText("主机 Agent 详情")).toBeVisible();
    await expect(drawer).toContainText("host-child:mission-1:host-a");
    await expect(drawer).toContainText("Manager 输入");
    await expect(drawer).toContainText("工具调用");
    await expect(drawer).toContainText("check_host_state");
    await expect(drawer).toContainText("工具结果");
    await expect(drawer).toContainText("host_state=ok");
    await expect(drawer).toContainText("Assistant 返回");

    await page.getByTestId("host-subagent-drawer-close").click();
    await expect(page.getByTestId("host-subagent-drawer")).toHaveCount(0);
  });

  test("simulates a high-risk approval decision from the composer", async ({ page }) => {
    await installUiFixture(page, {
      name: "hostops-approval-user-flow",
      state: createProtocolFixtureState(),
      sessions: createProtocolFixtureSessions(),
    });
    await page.goto("/", { waitUntil: "networkidle" });
    await waitForFixtureStable(page);

    const approvalComposer = page.getByTestId("codex-approval-inline");
    await expect(approvalComposer).toContainText("等待审批");
    await expect(approvalComposer).toContainText("systemctl reload nginx");
    await approvalComposer.getByRole("button", { name: "提交" }).click();
    await expect(approvalComposer).toContainText("已提交确认，正在继续执行");
    await expect(approvalComposer.getByRole("button", { name: "提交中" })).toBeDisabled();
  });
});

function createIdleChatFixture() {
  const state = createChatFixtureState({
    sessionId: "hostops-user-flow",
    threadId: "hostops-user-flow",
    status: "idle",
    cards: [
      {
        id: "user-before-hostops",
        type: "UserMessageCard",
        role: "user",
        text: "Hello there",
        createdAt: "2026-06-04T09:59:00Z",
        updatedAt: "2026-06-04T09:59:00Z",
      },
      {
        id: "assistant-before-hostops",
        type: "AssistantMessageCard",
        text: "输入排障、巡检或变更任务，消息会进入当前主机会话。",
        createdAt: "2026-06-04T09:59:01Z",
        updatedAt: "2026-06-04T09:59:01Z",
      },
    ],
    runtime: {
      turn: { active: false, phase: "completed", hostId: "workspace" },
      codex: { status: "connected", retryAttempt: 0, retryMax: 5 },
      activity: { viewedFiles: [], searchedWebQueries: [], searchedContentQueries: [] },
    },
    finalText: "输入排障、巡检或变更任务，消息会进入当前主机会话。",
  });
  state.status = "idle";
  state.runtimeLiveness = {
    activeTurns: {},
    activeAgents: {},
    pendingApprovals: {},
    pendingUserInputs: {},
    activeCommandStreams: {},
  };
  state.hosts = createManagedHostRows();
  state.selectedHostId = "workspace";

  return {
    name: "hostops-comprehensive-idle-chat",
    state,
    sessions: createChatFixtureSessions({
      activeSessionId: "hostops-user-flow",
      sessions: [
        {
          id: "hostops-user-flow",
          kind: "single_host",
          title: "HostOps 综合测试",
          status: "idle",
          messageCount: 1,
          preview: "Hello there",
          selectedHostId: "workspace",
          lastActivityAt: "2026-06-04T09:59:01Z",
        },
      ],
    }),
  };
}

function createManagedHostRows() {
  return [
    {
      id: "host-a",
      name: "host-a",
      address: "1.1.1.1",
      sshUser: "root",
      sshPort: 22,
      status: "online",
      installState: "installed",
      controlMode: "managed",
      transport: "agent_http",
      executable: true,
      terminalCapable: true,
      agentUrl: "http://1.1.1.1:7072",
      labels: { role: "worker-a", env: "test" },
    },
    {
      id: "host-b",
      name: "host-b",
      address: "1.1.1.2",
      sshUser: "root",
      sshPort: 22,
      status: "online",
      installState: "installed",
      controlMode: "managed",
      transport: "agent_http",
      executable: true,
      terminalCapable: true,
      agentUrl: "http://1.1.1.2:7072",
      labels: { role: "worker-b", env: "test" },
    },
    {
      id: "host-c",
      name: "host-c",
      address: "1.1.1.3",
      sshUser: "root",
      sshPort: 22,
      status: "online",
      installState: "installed",
      controlMode: "managed",
      transport: "agent_http",
      executable: true,
      terminalCapable: true,
      agentUrl: "http://1.1.1.3:7072",
      labels: { role: "verifier", env: "test" },
    },
  ];
}

function withDetailedHostOpsTranscript(fixture) {
  const cloned = JSON.parse(JSON.stringify(fixture));
  cloned.state.hosts = createManagedHostRows();
  cloned.state.hostOpsTranscripts = {
    ...(cloned.state.hostOpsTranscripts || {}),
    "child-1": {
      childAgentId: "child-1",
      items: [
        {
          id: "child-1-manager",
          type: "manager_message",
          content: "在绑定主机 @1.1.1.1 上检查主机状态，不要操作其他主机。",
          status: "completed",
          createdAt: "2026-06-04T10:00:03Z",
        },
        {
          id: "child-1-tool-call",
          type: "tool_call",
          toolName: "check_host_state",
          content: '{"hostId":"host-a","reason":"host state check"}',
          status: "running",
          createdAt: "2026-06-04T10:00:04Z",
        },
        {
          id: "child-1-tool-result",
          type: "tool_result",
          toolName: "check_host_state",
          content: '{"status":"completed","hostId":"host-a","result":"host_state=ok","source":"host.agent"}',
          status: "completed",
          createdAt: "2026-06-04T10:00:05Z",
        },
        {
          id: "child-1-assistant",
          type: "assistant_message",
          content: "绑定主机 @1.1.1.1 状态正常，结果为 host_state=ok。",
          status: "completed",
          createdAt: "2026-06-04T10:00:06Z",
        },
      ],
    },
  };
  return cloned;
}

async function mockLLMConfig(page, updates) {
  let current = {
    provider: "openai",
    model: "gpt-5.4",
    baseURL: "https://www.aicodexcn.com/v1",
    apiKeySet: false,
    apiKeyMasked: "",
    maxContextTokens: 200000,
    bifrostActive: false,
  };
  await page.route("**/api/v1/llm-config", async (route) => {
    if (route.request().method() === "PUT") {
      const payload = route.request().postDataJSON();
      updates.push(payload);
      current = {
        ...current,
        provider: payload.provider,
        model: payload.model,
        baseURL: payload.baseURL,
        maxContextTokens: Number(payload.maxContextTokens || 200000),
        apiKeySet: Boolean(payload.apiKey),
        apiKeyMasked: payload.apiKey ? "sk-***" : current.apiKeyMasked,
        bifrostActive: true,
      };
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ ok: true, message: "配置已保存" }),
      });
    }
    return route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(current),
    });
  });
}

async function mockHostInventory(page) {
  await page.route("**/api/v1/hosts", async (route) => {
    if (route.request().method() === "GET") {
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ items: createManagedHostRows() }),
      });
    }
    return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ ok: true }) });
  });
  await page.route("**/api/v1/hosts/*/install", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ ok: true, installRunId: "run-hostops-e2e" }) }),
  );
  await page.route("**/api/v1/hosts/*/ssh/test", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ ok: true, status: "ok" }) }),
  );
  await page.route("**/api/v1/terminal/sessions", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ sessions: [] }) }),
  );
  await page.route("**/api/v1/host-profiles**", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ items: [] }) }),
  );
  await page.route("**/api/v1/host-leases**", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ items: [] }) }),
  );
}

async function installHostOpsTranscriptRoute(page, fixture) {
  await page.route("**/api/v1/host-ops/child-agents/*/transcript", (route) => {
    const childAgentId = route.request().url().split("/child-agents/").at(-1)?.split("/transcript")[0] || "";
    const transcript = fixture.state.hostOpsTranscripts?.[decodeURIComponent(childAgentId)] || {
      childAgentId,
      items: [],
    };
    return route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(transcript),
    });
  });
}

async function installAssistantTransportRoute(page, hostOpsFixture, requests) {
  await page.route("**/api/v1/assistant/transport", async (route) => {
    requests.push(route.request().postDataJSON());
    return route.fulfill({
      status: 200,
      contentType: "text/plain; charset=utf-8",
      body: `aui-state:${JSON.stringify([{ type: "set", path: [], value: hostOpsFixture.state }])}\n`,
    });
  });
}
