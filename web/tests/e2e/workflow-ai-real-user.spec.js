// @ts-check
import { expect, test } from "@playwright/test";

const ACTIONS = {
  items: [
    { action: "script.python", label: "Python Script", category: "script", description: "Run Python script content.", defaults: { script: "print('ok')" } },
    { action: "script.shell", label: "Shell Script", category: "script", description: "Run shell script content.", defaults: { script: "set -e\necho ok" } },
    { action: "manual.approval", label: "Manual Approval", category: "control", description: "Pause until an operator approves." },
    { action: "notify.send", label: "Notify", category: "control", description: "Send a notification." },
  ],
};

const EMPTY_STATE = {
  sessionId: "workflow-ai-real-user",
  kind: "single_host",
  selectedHostId: "pg-primary",
  hosts: [{ id: "pg-primary", name: "pg-primary", status: "online" }],
  runtime: { codex: { status: "connected" }, turn: { phase: "idle" } },
};

const PG_BACKUP_PLAN = {
  id: "plan-pg-backup",
  workflowId: "runner-blank",
  message: "生成一个 pg 备份工作流",
  items: [
    {
      id: "collect-pg-context",
      title: "确认 PostgreSQL 实例与连接方式",
      description: "确认实例地址、端口、数据库名和凭据引用。",
      status: "pending",
    },
    {
      id: "dump-pg-database",
      title: "执行 pg_dump 备份",
      description: "生成带时间戳的压缩备份文件。",
      status: "pending",
    },
    {
      id: "upload-backup-artifact",
      title: "上传备份到对象存储",
      description: "把备份文件上传到指定 bucket/prefix。",
      status: "pending",
    },
    {
      id: "verify-retention",
      title: "校验备份并执行保留策略",
      description: "检查备份可读性并清理过期备份。",
      status: "pending",
    },
  ],
};

test.describe("Workflow AI real-user journeys", () => {
  test("answers a read-only explanation request without creating a plan", async ({ page }) => {
    const planRequests = [];
    await installRunnerStudioWorkflowAiHarness(page, { planRequests });

    await page.goto("/runner", { waitUntil: "networkidle" });
    await page.getByTestId("runner-create-workflow").click();
    await page.getByTestId("runner-toolbar-ai-generate").click();

    const initialNodeCount = await page.locator("[data-testid^='canvas-node-']").count();
    const drawer = page.getByTestId("workflow-ai-drawer");
    await drawer.getByPlaceholder(/Workflow AI/).fill("解释一下当前工作流做了什么");
    await drawer.getByRole("button", { name: "Send" }).click();

    await expect(page.getByTestId("workflow-ai-readonly-answer")).toBeVisible();
    await expect(page.getByTestId("workflow-ai-plan-card")).toHaveCount(0);
    expect(planRequests).toHaveLength(0);
    await expect.poll(() => page.locator("[data-testid^='canvas-node-']").count()).toBe(initialNodeCount);
  });

  test("handles a greeting as normal chat instead of generating an edit plan", async ({ page }) => {
    const planRequests = [];
    const chatRequests = [];
    await installRunnerStudioWorkflowAiHarness(page, { planRequests, chatRequests });

    await page.goto("/runner", { waitUntil: "networkidle" });
    await page.getByTestId("runner-create-workflow").click();
    await page.getByTestId("runner-toolbar-ai-generate").click();

    const drawer = page.getByTestId("workflow-ai-drawer");
    await drawer.getByPlaceholder(/Workflow AI/).fill("你好");
    await drawer.getByRole("button", { name: "Send" }).click();

    await expect(page.getByTestId("workflow-ai-readonly-answer")).not.toContainText("Workflow AI 回复");
    await expect(page.getByTestId("workflow-ai-readonly-answer")).toContainText("你好，我是 Workflow AI");
    await expect(page.getByTestId("workflow-ai-plan-card")).toHaveCount(0);
    await expect(drawer).not.toContainText("正在生成修改计划");
    expect(planRequests).toHaveLength(0);
    expect(chatRequests).toHaveLength(1);
    expect(chatRequests[0]?.metadata?.source).toBe("workflow_ai_chat");
  });

  test("simulates an operator generating a PostgreSQL backup workflow from Workflow AI", async ({ page }) => {
    const planRequests = [];
    await installRunnerStudioWorkflowAiHarness(page, { planRequests });

    await page.goto("/runner", { waitUntil: "networkidle" });
    await page.getByTestId("runner-create-workflow").click();

    const toolbarButtons = await page
      .locator(".runner-studio-toolbar-actions > button, [data-testid='runner-toolbar-more-container'] > button")
      .allTextContents();
    expect(toolbarButtons.map((text) => text.trim())).toEqual(["保存", "运行", "运行详情", "AI", "更多"]);

    await page.getByTestId("runner-toolbar-more").click();
    await expect(page.getByTestId("runner-toolbar-more-menu")).not.toContainText("AI");
    await page.keyboard.press("Escape");

    await page.getByTestId("runner-toolbar-ai-generate").click();
    await expect(page.getByTestId("workflow-ai-drawer")).toBeVisible();
    await expect(page.getByTestId("workflow-ai-context-card")).toHaveCount(0);
    await expect(page.getByTestId("workflow-ai-updated-label")).toContainText("修改");

    const operatorPrompt = [
      "生成一个 pg 备份的工作流",
      "目标主机使用已保存的 pg-primary 凭据引用",
      "每天 02:00 执行，保留 7 天",
      "备份后需要校验并通知",
    ].join("；");
    await page.getByPlaceholder(/Workflow AI/).fill(operatorPrompt);
    await page.getByRole("button", { name: "Send" }).click();

    await expect.poll(() => planRequests.length).toBe(1);
    expect(planRequests[0]?.message).toContain("pg 备份");

    const drawer = page.getByTestId("workflow-ai-drawer");
    await expect(page.getByTestId("workflow-ai-plan-card")).toContainText("确认 PostgreSQL 实例与连接方式");
    await expect(page.getByTestId("workflow-ai-plan-card")).toContainText("执行 pg_dump 备份");
    await expect(page.getByTestId("workflow-ai-plan-card")).toContainText("上传备份到对象存储");
    await expect(page.getByTestId("workflow-ai-plan-card")).toContainText("校验备份并执行保留策略");
    await expect(drawer.getByRole("button", { name: "Start" })).toHaveCount(0);
    await expect(drawer.getByRole("button", { name: "Apply" })).toHaveCount(0);
    await expect(drawer.getByRole("button", { name: "确认计划并开始修改" })).toHaveCount(0);

    await drawer.getByPlaceholder(/Workflow AI/).fill("确认");
    await drawer.getByRole("button", { name: "Send" }).click();

    await expect(page.getByTestId("workflow-ai-step-history-card").first()).toContainText("完成步骤");
    await expect(page.getByTestId("canvas-node-ai-step-collect-pg-context")).toContainText("确认 PostgreSQL 实例与连接方式");
    await expect(page.getByTestId("canvas-node-ai-step-dump-pg-database")).toContainText("执行 pg_dump 备份");
    await expect(page.getByTestId("canvas-node-ai-step-upload-backup-artifact")).toContainText("上传备份到对象存储");
    await expect(page.getByTestId("canvas-node-ai-step-verify-retention")).toContainText("校验备份并执行保留策略");

    await drawer.getByRole("button", { name: "事件" }).click();
    const eventDrawer = page.getByTestId("workflow-event-drawer");
    await expect(eventDrawer).toBeVisible();
    await expect(eventDrawer).toContainText("workflow.ai.plan.confirmed");
    await expect(eventDrawer).toContainText("workflow.graph.node.added");
    await expect(eventDrawer).toContainText("workflow.graph.edge.added");
    await expect(eventDrawer).toContainText("workflow.node.script.generated");
    await expect(eventDrawer).toContainText("workflow.ai.step.completed");
    await expect(eventDrawer.getByText("workflow.graph.node.added")).toHaveCount(PG_BACKUP_PLAN.items.length);

    const dumpGraphEvent = eventDrawer
      .getByTestId("workflow-event-row")
      .filter({ hasText: "workflow.graph.node.added" })
      .filter({ hasText: "ai-step-dump-pg-database" });
    await expect(dumpGraphEvent).toHaveCount(1);
    await dumpGraphEvent.click();
    await expect(page.getByTestId("canvas-node-ai-step-dump-pg-database")).toHaveClass(/selected|ai-highlighted/);

    await expect(page.getByTestId("runner-node-panel")).toBeVisible();
    await page.getByTestId("runner-node-panel-tab-script").click();
    await expect(page.getByTestId("runner-node-script-editor")).toHaveValue(/Workflow AI generated step/);
    await page.getByTestId("runner-node-panel-tab-io").click();
    await expect(page.getByTestId("runner-node-io-tab")).toContainText("输入变量");
  });

  test("revises a pending plan through the composer without changing the canvas", async ({ page }) => {
    const planRequests = [];
    await installRunnerStudioWorkflowAiHarness(page, { planRequests });

    await page.goto("/runner", { waitUntil: "networkidle" });
    await page.getByTestId("runner-create-workflow").click();
    const initialNodeCount = await page.locator("[data-testid^='canvas-node-']").count();
    await page.getByTestId("runner-toolbar-ai-generate").click();
    const drawer = page.getByTestId("workflow-ai-drawer");

    await drawer.getByPlaceholder(/Workflow AI/).fill("生成一个 pg 备份的工作流，使用 pgBackRest");
    await drawer.getByRole("button", { name: "Send" }).click();
    await expect(page.getByTestId("workflow-ai-plan-card")).toContainText("执行 pg_dump 备份");
    await expect.poll(() => page.locator("[data-testid^='canvas-node-']").count()).toBe(initialNodeCount);

    await drawer.getByPlaceholder(/Workflow AI/).fill("把第 2 步改成先检查磁盘空间");
    await drawer.getByRole("button", { name: "Send" }).click();

    await expect.poll(() => planRequests.length).toBe(2);
    await expect(page.getByTestId("workflow-ai-plan-card")).toContainText("检查磁盘空间");
    await expect.poll(() => page.locator("[data-testid^='canvas-node-']").count()).toBe(initialNodeCount);
  });

  test("allows the operator to rename the workflow before asking Workflow AI", async ({ page }) => {
    await installRunnerStudioWorkflowAiHarness(page);

    await page.goto("/runner", { waitUntil: "networkidle" });
    await page.getByTestId("runner-create-workflow").click();

    await page.getByTestId("runner-workflow-title-display").click();
    await page.getByTestId("runner-workflow-title-input").fill("PG 每日备份工作流");
    await page.keyboard.press("Enter");

    await expect(page.getByTestId("runner-studio-topbar")).toContainText("PG 每日备份工作流");
    await page.getByTestId("runner-toolbar-ai-generate").click();
    await expect(page.getByTestId("workflow-ai-updated-label")).toContainText("PG 每日备份工作流");
  });
});

test.describe("Workflow AI optional live LLM UI journey", () => {
  test("uses the configured LLM to generate the plan shown in Workflow AI", async ({ page, request }) => {
    test.skip(process.env.AIOPS_LIVE_LLM_E2E !== "1", "Set AIOPS_LIVE_LLM_E2E=1 and provider env vars to run the live LLM UI journey.");

    const hostLabel = process.env.AIOPS_TEST_HOST || "pg-primary";
    const sshUser = process.env.AIOPS_TEST_USER || "root";
    const sshPort = process.env.AIOPS_TEST_SSH_PORT || "22";
    const operatorPrompt = [
      "生成一个 PostgreSQL 备份工作流",
      `目标主机 ${hostLabel}，SSH 用户 ${sshUser}，端口 ${sshPort}`,
      "使用已保存的凭据引用，不要使用或输出明文密码",
      "每天 02:00 执行，保留 7 天",
      "备份后需要校验、上传对象存储并通知",
    ].join("；");
    const livePlan = await createLiveWorkflowPlan(request, operatorPrompt);
    const planRequests = [];

    await installRunnerStudioWorkflowAiHarness(page, { planRequests, plan: livePlan });
    await page.goto("/runner", { waitUntil: "networkidle" });
    await page.getByTestId("runner-create-workflow").click();
    await page.getByTestId("runner-toolbar-ai-generate").click();
    await expect(page.getByTestId("workflow-ai-drawer")).toBeVisible();

    await page.getByPlaceholder(/Workflow AI/).fill(operatorPrompt);
    await page.getByRole("button", { name: "Send" }).click();

    await expect.poll(() => planRequests.length, { timeout: 10000 }).toBe(1);
    const planCard = page.getByTestId("workflow-ai-plan-card");
    for (const item of livePlan.items) {
      await expect(planCard).toContainText(item.title);
    }

    const drawer = page.getByTestId("workflow-ai-drawer");
    await expect(drawer.getByRole("button", { name: "Start" })).toHaveCount(0);
    await expect(drawer.getByRole("button", { name: "Apply" })).toHaveCount(0);
    await expect(drawer.getByRole("button", { name: "确认计划并开始修改" })).toHaveCount(0);
    await drawer.getByPlaceholder(/Workflow AI/).fill("确认");
    await drawer.getByRole("button", { name: "Send" }).click();

    for (const item of livePlan.items) {
      await expect(page.getByTestId(`canvas-node-ai-step-${item.id}`)).toContainText(item.title);
    }

    await drawer.getByRole("button", { name: "事件" }).click();
    const eventDrawer = page.getByTestId("workflow-event-drawer");
    await expect(eventDrawer).toContainText("workflow.ai.plan.confirmed");
    await expect(eventDrawer.getByText("workflow.graph.node.added")).toHaveCount(livePlan.items.length);
    await expect(eventDrawer.getByText("workflow.ai.step.completed")).toHaveCount(livePlan.items.length);
  });
});

async function installRunnerStudioWorkflowAiHarness(page, { planRequests = [], chatRequests = [], plan = PG_BACKUP_PLAN } = {}) {
  const storedGraphs = new Map();
  let latestWorkflowAiChat = null;

  await page.route("**/api/v1/state", (route) =>
    route.fulfill({
      json: latestWorkflowAiChat
        ? {
            ...EMPTY_STATE,
            schemaVersion: "aiops.transport.v2",
            sessionId: latestWorkflowAiChat.sessionId,
            threadId: latestWorkflowAiChat.sessionId,
            status: "idle",
            currentTurnId: latestWorkflowAiChat.turnId,
            turns: {
              [latestWorkflowAiChat.turnId]: {
                id: latestWorkflowAiChat.turnId,
                user: { clientTurnId: latestWorkflowAiChat.clientTurnId, text: latestWorkflowAiChat.content },
                status: "completed",
                final: {
                  id: `${latestWorkflowAiChat.turnId}-final`,
                  text: "你好，我是 Workflow AI。你可以直接和我对话；只有你明确要求创建或修改工作流时，我才会先生成计划。",
                  status: "completed",
                },
              },
            },
            turnOrder: [latestWorkflowAiChat.turnId],
          }
        : EMPTY_STATE,
    }),
  );
  await page.route("**/api/v1/sessions", (route) => route.fulfill({ json: { items: [] } }));
  await page.route("**/api/v1/chat/message", (route) => {
    const body = route.request().postDataJSON();
    chatRequests.push(body);
    latestWorkflowAiChat = {
      sessionId: body?.sessionId || "workflow-ai-chat",
      clientTurnId: body?.clientTurnId || "",
      content: body?.content || "",
      turnId: "workflow-ai-chat-turn",
    };
    return route.fulfill({
      json: {
        accepted: true,
        sessionId: latestWorkflowAiChat.sessionId,
        turnId: "workflow-ai-chat-turn",
        status: "accepted",
      },
    });
  });
  await page.route("**/api/runner-studio/actions*", (route) => route.fulfill({ json: ACTIONS }));
  await page.route("**/api/runner-studio/workflows", (route) => route.fulfill({ json: { workflows: [] } }));
  await page.route("**/api/runner-studio/workflows/graph/validate", (route) =>
    route.fulfill({
      json: {
        valid: true,
        status: "validated",
        validated_graph_hash: "workflow-ai-e2e-graph-hash",
        warnings: [],
      },
    }),
  );
  await page.route("**/api/runner-studio/workflows/graph/dry-run", (route) =>
    route.fulfill({
      json: {
        run_id: "workflow-ai-e2e-dry-run",
        status: "dry_run_passed",
        validated_graph_hash: "workflow-ai-e2e-graph-hash",
        dry_run_graph_hash: "workflow-ai-e2e-graph-hash",
      },
    }),
  );
  await page.route("**/api/runner-studio/workflows/graph", (route) => {
    const body = route.request().postDataJSON();
    const name = body?.graph?.workflow?.name || "runner-blank";
    const graph = {
      ...(body?.graph || {}),
      ui: { ...(body?.graph?.ui || {}), resource_version: "workflow-ai-e2e-created" },
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
            workflow: { name, title: name },
            ui: { resource_version: "workflow-ai-e2e-loaded" },
            nodes: [],
            edges: [],
          },
      });
    }
    const body = request.postDataJSON();
    const graph = {
      ...(body?.graph || {}),
      ui: { ...(body?.graph?.ui || {}), resource_version: "workflow-ai-e2e-saved" },
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
    route.fulfill({ json: { run_id: "workflow-ai-e2e-run", status: "running" } }),
  );
  await page.route("**/api/runner-studio/runs/workflow-ai-e2e-run/events/history", (route) =>
    route.fulfill({ json: [{ type: "run_start", run_id: "workflow-ai-e2e-run", status: "running" }] }),
  );
  await page.route("**/api/runner-studio/workflow-ai/sessions", (route) =>
    route.fulfill({
      json: {
        schemaVersion: "workflow.ai.session.v1",
        id: "workflow-ai-e2e-session",
        workflowId: "runner-blank",
        status: "active",
        toolLogRef: "workflow-ai-tool-log/e2e",
      },
    }),
  );
  await page.route("**/api/runner-studio/workflow-ai/plan", (route) => {
    const body = route.request().postDataJSON();
    planRequests.push(body);
    const revisedPlan = String(body?.message || "").includes("磁盘空间")
      ? {
          ...plan,
          items: plan.items.map((item, index) =>
            index === 1
              ? { ...item, id: "check-disk-space", title: "检查磁盘空间", description: "先检查备份目录和 pgBackRest repo 的可用空间。" }
              : item,
          ),
        }
      : plan;
    return route.fulfill({
      json: {
        ...revisedPlan,
        workflowId: body?.workflowId || revisedPlan.workflowId || PG_BACKUP_PLAN.workflowId,
        message: body?.message || revisedPlan.message || PG_BACKUP_PLAN.message,
      },
    });
  });
}

async function createLiveWorkflowPlan(request, operatorPrompt) {
  const baseURL = requiredEnv("AIOPS_LLM_BASE_URL").replace(/\/+$/, "");
  const apiKey = requiredEnv("AIOPS_LLM_API_KEY");
  const model = process.env.AIOPS_LLM_MODEL || "glm-5.1";
  const response = await request.post(`${baseURL}/chat/completions`, {
    headers: {
      Authorization: `Bearer ${apiKey}`,
      "Content-Type": "application/json",
    },
    data: {
      model,
      messages: [
        {
          role: "system",
          content: [
            "你是 AIOps Workflow AI planner。",
            "只返回 JSON，不要 markdown。",
            "JSON schema: {\"items\":[{\"title\":\"...\",\"description\":\"...\"}]}。",
            "生成 4 个步骤，必须覆盖 PostgreSQL 连接确认、pg_dump 备份、备份上传、备份校验/保留/通知。",
            "不要请求、输出或复述明文密码，只能说使用已保存凭据引用。",
          ].join("\n"),
        },
        { role: "user", content: operatorPrompt },
      ],
      temperature: 0.2,
    },
    timeout: 45000,
  });
  expect(response.ok()).toBeTruthy();
  const payload = await response.json();
  const content = String(payload?.choices?.[0]?.message?.content || "");
  expect(content.toLowerCase()).not.toContain("password");
  const parsed = extractJsonObject(content);
  const items = Array.isArray(parsed?.items) ? parsed.items : [];
  expect(items.length).toBeGreaterThanOrEqual(3);
  const normalizedItems = items.slice(0, 4).map((item, index) => ({
    id: `live-step-${index + 1}`,
    title: String(item?.title || `LLM 生成步骤 ${index + 1}`).slice(0, 64),
    description: String(item?.description || "").slice(0, 240),
    status: "pending",
  }));
  const joined = normalizedItems.map((item) => `${item.title} ${item.description}`).join(" ").toLowerCase();
  expect(joined).toContain("pg");
  return {
    id: `live-plan-${Date.now()}`,
    workflowId: "runner-blank",
    message: operatorPrompt,
    items: normalizedItems,
  };
}

function extractJsonObject(content) {
  const trimmed = String(content || "").trim();
  try {
    return JSON.parse(trimmed);
  } catch {
    const fenced = trimmed.match(/```(?:json)?\s*([\s\S]*?)```/i);
    if (fenced?.[1]) {
      return JSON.parse(fenced[1].trim());
    }
    const start = trimmed.indexOf("{");
    const end = trimmed.lastIndexOf("}");
    if (start >= 0 && end > start) {
      return JSON.parse(trimmed.slice(start, end + 1));
    }
    throw new Error("LLM response did not contain a JSON object");
  }
}

function requiredEnv(name) {
  const value = process.env[name];
  if (!value) {
    throw new Error(`${name} is required for live Workflow AI LLM smoke testing`);
  }
  return value;
}
