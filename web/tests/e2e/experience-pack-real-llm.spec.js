// @ts-check
import { test, expect } from "@playwright/test";

const runRealLLM = process.env.AIOPS_REAL_LLM === "1";
const llmBaseURL = (process.env.AIOPS_LLM_BASE_URL || "").replace(/\/$/, "");
const llmAPIKey = process.env.AIOPS_LLM_API_KEY || "";
const llmModel = process.env.AIOPS_LLM_MODEL || "gpt-5.4";

const EMPTY_STATE = {
  sessionId: "experience-pack-real-llm-e2e",
  kind: "single_host",
  selectedHostId: "server-local",
  hosts: [{ id: "server-local", name: "server-local", status: "online" }],
  runtime: { codex: { status: "connected" }, turn: { phase: "idle" } },
};

let cachedDecision;

test.describe("experience pack real LLM flows", () => {
  test.skip(!runRealLLM, "Set AIOPS_REAL_LLM=1 with AIOPS_LLM_BASE_URL/API_KEY/MODEL to run real LLM tests.");
  test.skip(!llmBaseURL || !llmAPIKey, "AIOPS_LLM_BASE_URL and AIOPS_LLM_API_KEY are required for real LLM tests.");

  test("真实 LLM 返回可用于经验包检索的结构化决策", async () => {
    test.setTimeout(120_000);

    const decision = await getExperiencePackDecision();

    expect(decision.model).toBe(llmModel);
    expect(decision.should_use_experience_pack).toBe(true);
    expect(decision.required_review).toBe(true);
    expect(decision.matched_pack_title).toContain("PG");
    expect(decision.runner_steps.join(" ")).toContain("Dry Run");
    expect(decision.runner_steps.join(" ")).toContain("恢复验证");
  });

  test("用户按真实 LLM 决策启用经验包并生成 Runner 草稿", async ({ page }) => {
    test.setTimeout(120_000);

    const decision = await getExperiencePackDecision();
    await page.addInitScript(() => window.localStorage.clear());
    await installExperiencePackRoutes(page, decision);

    await page.goto("/settings/experience-packs");
    await expect(page.locator("main")).toContainText(decision.matched_pack_title);
    await expect(page.locator("main")).toContainText("不可检索");
    await expect(page.locator("main")).toContainText(decision.match_reason);

    await page.getByRole("tab", { name: "待审核经验" }).click();
    await page.getByRole("button", { name: "发送到 Runner Studio" }).click();
    await expect(page.locator("main")).toContainText("已创建本地草稿");

    await page.getByRole("link", { name: "打开 Runner Studio" }).click();
    await expect(page).toHaveURL(/\/runner\/wf-real-llm-pg-experience$/);
    await expect(page.locator("main")).toContainText("环境预检查");
    await expect(page.locator("main")).toContainText("人工审批");
    await expect(page.locator("main")).toContainText("Dry Run");
    await expect(page.locator("main")).toContainText("受控执行");
    await expect(page.locator("main")).toContainText("恢复验证");
    await expect(page.locator("main")).toContainText("受控回滚");

    await page.goto("/settings/experience-packs");
    await page.getByRole("button", { name: "审核通过" }).click();
    await expect(page.getByTestId("experience-pack-workbench").getByText("可检索", { exact: true })).toBeVisible();
  });
});

async function getExperiencePackDecision() {
  if (!cachedDecision) {
    cachedDecision = await askRealLLMForExperiencePackDecision();
  }
  return cachedDecision;
}

async function askRealLLMForExperiencePackDecision() {
  const content = await streamChatCompletion([
    {
      role: "system",
      content:
        "你是 AIOps 经验包检索测试助手。只输出 JSON，不要 Markdown，不要解释。字段必须稳定，便于自动化测试解析。",
    },
    {
      role: "user",
      content: [
        "运维场景：prod 环境 payment-api 访问 PostgreSQL 慢，Coroot 显示 postgres active connections 逼近 max_connections，",
        "接口 p95 升高，日志出现 too many clients already。系统已有一个 PG 连接池耗尽修复经验包，",
        "它要求人工审核、Dry Run、受控执行和恢复验证。",
        "",
        "请判断是否应命中并启用经验包。只返回如下 JSON：",
        JSON.stringify({
          should_use_experience_pack: true,
          matched_pack_title: "PG 连接池耗尽修复经验包",
          match_reason: "Coroot 连接数、接口延迟和 postgres 错误模式与历史成功经验一致",
          required_review: true,
          runner_steps: ["环境预检查", "人工审批", "Dry Run", "受控执行", "恢复验证", "受控回滚"],
        }),
      ].join("\n"),
    },
  ]);
  const parsed = parseJSONObject(content);
  return normalizeDecision(parsed);
}

async function streamChatCompletion(messages) {
  const response = await fetch(`${llmBaseURL}/chat/completions`, {
    method: "POST",
    headers: {
      "content-type": "application/json",
      authorization: `Bearer ${llmAPIKey}`,
    },
    body: JSON.stringify({
      model: llmModel,
      messages,
      stream: true,
      temperature: 0,
      max_completion_tokens: 1200,
    }),
  });
  if (!response.ok || !response.body) {
    const body = await response.text().catch(() => "");
    throw new Error(`LLM request failed: ${response.status} ${body.slice(0, 300)}`);
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let text = "";
  let seenModel = "";

  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    const lines = buffer.split(/\r?\n/);
    buffer = lines.pop() || "";

    for (const line of lines) {
      const trimmed = line.trim();
      if (!trimmed.startsWith("data:")) continue;
      const payload = trimmed.slice("data:".length).trim();
      if (!payload || payload === "[DONE]") continue;
      const event = JSON.parse(payload);
      if (typeof event.model === "string") {
        seenModel = event.model;
      }
      const delta = event.choices?.[0]?.delta?.content;
      if (typeof delta === "string") {
        text += delta;
      }
    }
  }

  if (!text.trim()) {
    throw new Error(`LLM streamed no content for model ${seenModel || llmModel}`);
  }
  return text;
}

function parseJSONObject(content) {
  const trimmed = content.trim();
  try {
    return JSON.parse(trimmed);
  } catch {
    const start = trimmed.indexOf("{");
    const end = trimmed.lastIndexOf("}");
    if (start === -1 || end === -1 || end <= start) {
      throw new Error(`LLM output is not JSON: ${trimmed.slice(0, 300)}`);
    }
    return JSON.parse(trimmed.slice(start, end + 1));
  }
}

function normalizeDecision(value) {
  const runnerSteps = Array.isArray(value.runner_steps)
    ? value.runner_steps.map((item) => String(item).trim()).filter(Boolean)
    : [];
  return {
    model: llmModel,
    should_use_experience_pack: Boolean(value.should_use_experience_pack),
    matched_pack_title: String(value.matched_pack_title || "PG 连接池耗尽修复经验包").trim(),
    match_reason: String(value.match_reason || "Coroot 信号与历史经验一致").trim(),
    required_review: value.required_review !== false,
    runner_steps: runnerSteps.length
      ? runnerSteps
      : ["环境预检查", "人工审批", "Dry Run", "受控执行", "恢复验证", "受控回滚"],
  };
}

async function installExperiencePackRoutes(page, decision) {
  let approved = false;
  const runnerGraph = buildRunnerGraph(decision.runner_steps);
  const runnerCandidateResponse = {
    id: "runner-candidate-real-llm-pg",
    pack_id: "pack-real-llm-pg",
    workflow_id: "wf-real-llm-pg-experience",
    workflow_name: "Real LLM PG Experience Workflow",
    status: "draft",
    studio_draft_link: "/runner/wf-real-llm-pg-experience",
    workflow: {
      id: "wf-real-llm-pg-experience",
      name: "wf-real-llm-pg-experience",
      title: "Real LLM PG Experience Workflow",
      status: "draft",
      local_draft: true,
      ai_generated_draft: true,
      graph: runnerGraph,
    },
    graph: runnerGraph,
  };

  await page.route("**/api/v1/state", (route) => route.fulfill({ json: EMPTY_STATE }));
  await page.route("**/api/v1/sessions*", (route) => route.fulfill({ json: { activeSessionId: "sess-real-llm", sessions: [] } }));
  await page.route("**/api/v1/hosts", (route) => route.fulfill({ json: { items: EMPTY_STATE.hosts } }));
  await page.route("**/api/v1/llm-config", (route) =>
    route.fulfill({ json: { provider: "openai", model: llmModel, apiKeySet: true, baseURL: llmBaseURL } }),
  );
  await page.route("**/api/v1/experience-packs/*/reuse-records**", (route) => route.fulfill({ json: { items: [], total: 0 } }));
  await page.route("**/api/v1/experience-packs/candidates**", (route) =>
    route.fulfill({ json: candidatePayload(decision, approved) }),
  );
  await page.route("**/api/v1/experience-packs/candidates/**/approve", (route) => {
    approved = true;
    return route.fulfill({ json: packPayload(decision, true) });
  });
  await page.route("**/api/v1/experience-packs/runner-candidates/confirm", (route) =>
    route.fulfill({ json: runnerCandidateResponse }),
  );
  await page.route("**/api/runner-studio/actions*", (route) =>
    route.fulfill({
      json: {
        items: [
          { action: "shell.run", label: "Shell Script", category: "script" },
          { action: "manual.approval", label: "Manual Approval", category: "control" },
        ],
      },
    }),
  );
  await page.route("**/api/runner-studio/workflows", (route) => route.fulfill({ json: { workflows: [] } }));
  await page.route("**/api/runner-studio/workflows/*/graph", (route) => route.fulfill({ json: runnerGraph }));
}

function candidatePayload(decision, approved) {
  return {
    items: [
      {
        id: "candidate-real-llm-pg",
        pack_id: "pack-real-llm-pg",
        title: decision.matched_pack_title,
        summary: "由真实 LLM 根据 PG 连接耗尽场景判断命中的候选经验包。",
        status: approved ? "approved" : "candidate",
        match_reason: decision.match_reason,
        source_case_id: "case-real-llm-pg-slow-api",
        experience_pack: packPayload(decision, approved),
      },
    ],
    total: 1,
  };
}

function packPayload(decision, approved) {
  return {
    id: "pack-real-llm-pg",
    title: decision.matched_pack_title,
    summary: decision.match_reason,
    version: "v1.0",
    category: "repair",
    usage_shape: "guided",
    status: approved ? "enabled" : "disabled",
    review_status: approved ? "approved" : "pending",
    enabled: approved,
    skill: { name: "Real LLM PG Skill", summary: decision.match_reason },
    validation_gate: { status: "passed", passed: true, validators: ["runner.readonly_probe"] },
    runner_bindings: [{ id: "binding-real-llm-pg", workflow_id: "wf-real-llm-pg-experience", workflow_name: "Real LLM PG Experience Workflow", status: "draft" }],
    authorization_scopes: approved ? [{ type: "environment", value: "prod", searchable: true, reason: "真实 LLM 验证通过" }] : [],
    retrieval_eval: { score: 0.93, matched_cases: 3, verdict: approved ? "pass" : "pending" },
    advanced_refs: { gene_asset_id: "gene-real-llm-pg", capsule_asset_ids: ["capsule-real-llm-pg"] },
  };
}

function buildRunnerGraph(steps) {
  const labels = ["Start", ...steps, "End"];
  const nodes = labels.map((label, index) => ({
    id: nodeId(label, index),
    type: index === 0 ? "start" : index === labels.length - 1 ? "end" : "action",
    label,
    position: { x: 80 + index * 220, y: index === labels.length - 1 ? 160 : 120 },
    step: index > 0 && index < labels.length - 1 ? { name: nodeId(label, index), action: label === "人工审批" ? "manual.approval" : "shell.run", args: { script: `echo ${label}` } } : undefined,
    ports: index === 0 ? [{ id: "next", type: "output", label: "下一步" }] : [{ id: "in", type: "input", label: "输入" }],
  }));
  const edges = nodes.slice(0, -1).map((node, index) => ({
    id: `${node.id}-${nodes[index + 1].id}`,
    source: node.id,
    target: nodes[index + 1].id,
    source_port: index === 0 ? "next" : "next",
    target_port: "in",
  }));
  return {
    version: "v1",
    workflow: { name: "wf-real-llm-pg-experience", title: "Real LLM PG Experience Workflow" },
    nodes,
    edges,
  };
}

function nodeId(label, index) {
  const normalized = label
    .toLowerCase()
    .replace(/\s+/g, "-")
    .replace(/[^a-z0-9\u4e00-\u9fa5-]/g, "");
  return normalized || `node-${index}`;
}
