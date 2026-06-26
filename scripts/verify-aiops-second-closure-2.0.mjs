#!/usr/bin/env node
import fs from "node:fs/promises";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(scriptDir, "..");
const defaultBaseURL = "http://127.0.0.1:18083/";
const baseURL =
  argValue("--base-url") ||
  process.env.AIOPS_SECOND_CLOSURE_URL ||
  defaultBaseURL;
const screenshotDir =
  argValue("--screenshot-dir") ||
  process.env.AIOPS_SECOND_CLOSURE_SCREENSHOT_DIR ||
  path.join(repoRoot, "artifacts", "second-closure-2.0");
const args = new Set(process.argv.slice(2));

const scenarios = [
  {
    id: "plain-chat-no-default-local",
    title: "Plain Chat ops question does not bind server-local",
    mode: "submit",
    message:
      "Nginx upstream 502，先分析原因和证据缺口，不要连接或执行任何主机命令。",
    mustNotText: ["server-local", "host_bound_ops", "exec_command"],
    mustTextAny: ["Nginx", "502", "证据", "原因", "排查"],
  },
  {
    id: "local-mention-binds-target",
    title: "@local selects local host target",
    mode: "submit",
    message: "@local 检查 systemd 服务为什么失败，先只读分析，不要修改。",
    mustTextAny: ["@local", "server-local", "local", "systemd"],
  },
  {
    id: "coroot-healthy-enters-rca",
    title: "@Coroot with healthy MCP shows RCA context",
    mode: "fixture",
    fixture: fixtureState({
      finalText:
        "@Coroot checkout 服务异常 RCA：Coroot MCP healthy，已进入根因分析，先看依赖链、网络错误、实例健康和最近变更。",
      mcpHealthy: true,
      opsRun: {
        id: "opsrun-coroot-healthy",
        status: "completed",
        title: "@Coroot 分析 checkout 服务异常",
        targetSummary: "Coroot service:checkout",
      },
    }),
    mustText: ["Coroot", "RCA", "根因分析", "checkout"],
  },
  {
    id: "coroot-unhealthy-falls-back",
    title: "@Coroot unhealthy still allows normal Chat analysis",
    mode: "fixture",
    fixture: fixtureState({
      finalText:
        "@Coroot 当前不可用，未进入 Coroot RCA；继续按普通 Chat 基于用户证据分析，不阻塞排障。",
      mcpHealthy: false,
      opsRun: {
        id: "opsrun-coroot-unhealthy",
        status: "completed",
        title: "@Coroot 分析 checkout 服务异常",
        targetSummary: "Coroot unavailable",
      },
    }),
    mustText: ["Coroot", "不可用", "普通 Chat", "不阻塞"],
  },
  {
    id: "high-confidence-opsmanual-asks-user",
    title: "High confidence OpsManual recommendation asks before use",
    mode: "fixture",
    fixture: fixtureState({
      finalText:
        "检索到高置信运维手册：Redis 主从复制恢复。边界：同一对象、同一操作、Redis 7、sentinel 拓扑。历史使用 4 次，成功 3 次，成功率 75%。是否使用？也可以跳过并继续普通 AI Chat。",
      opsRun: {
        id: "opsrun-manual",
        status: "completed",
        title: "Redis 主从复制异常",
      },
    }),
    mustText: ["高置信", "运维手册", "成功率", "是否使用", "跳过"],
  },
  {
    id: "post-run-suggestions-after-useful-run",
    title: "Useful terminal run shows post-run asset suggestions",
    mode: "fixture",
    fixture: fixtureState({
      finalText:
        "已完成 Redis 主从复制异常处理记录候选，包含证据、用户确认、执行记录和最终结论。",
      opsRun: {
        id: "opsrun-post-run",
        status: "completed",
        title: "修复 Redis 主从复制异常",
        currentStep: "已整理证据和执行记录",
        postRunSuggestions: [
          { type: "run_record", label: "生成 Run Record" },
          { type: "processing_record", label: "生成处理记录" },
          { type: "experience_candidate", label: "生成经验候选" },
          { type: "case", label: "生成 Case" },
        ],
      },
    }),
    mustText: ["生成 Run Record", "生成处理记录", "生成经验候选", "生成 Case"],
  },
  {
    id: "approval-checkpoints-visible",
    title: "Approval flow shows checkpoints around approval",
    mode: "fixture",
    expandProcess: true,
    skipComposerWait: true,
    fixture: fixtureState({
      finalText: "等待用户审批；审批后将从 checkpoint-after-approval-request 继续。",
      opsRun: {
        id: "opsrun-approval-checkpoints",
        status: "blocked",
        title: "@web-02 修复 nginx reload 失败",
        currentStep: "等待审批",
        checkpointId: "checkpoint-after-approval-request",
      },
      pendingApprovals: {
        "approval-second-closure-reload": {
          id: "approval-second-closure-reload",
          status: "pending",
          command: "systemctl reload nginx",
          reason: "需要审批后才能执行变更命令。",
        },
      },
      process: [
        {
          id: "checkpoint-before-approval",
          kind: "system",
          displayKind: "checkpoint.before_approval",
          status: "completed",
          text: "checkpoint before approval: observed facts and target refs captured before requesting approval",
          checkpointId: "checkpoint-before-approval",
        },
        {
          id: "approval-second-closure-reload",
          kind: "approval",
          displayKind: "approval.command",
          status: "blocked",
          text: "approval required: systemctl reload nginx",
          command: "systemctl reload nginx",
          approvalId: "approval-second-closure-reload",
          checkpointId: "checkpoint-before-approval",
        },
        {
          id: "checkpoint-after-approval-request",
          kind: "system",
          displayKind: "checkpoint.after_approval_request",
          status: "blocked",
          text: "checkpoint after approval request: waiting to resume with approved command or fallback analysis",
          checkpointId: "checkpoint-after-approval-request",
        },
      ],
    }),
    mustText: ["checkpoint before approval", "要执行这个命令", "checkpoint after approval request", "systemctl reload nginx"],
  },
  {
    id: "error-recovery-checkpoint-visible",
    title: "Tool failure shows error recovery checkpoint",
    mode: "fixture",
    expandProcess: true,
    skipComposerWait: true,
    fixture: fixtureState({
      finalText: "工具失败后已进入 error recovery checkpoint，继续基于已有证据和可用工具分析。",
      opsRun: {
        id: "opsrun-error-recovery",
        status: "completed",
        title: "Redis 复制延迟排查",
        currentStep: "继续普通分析",
        checkpointId: "checkpoint-error-recovery",
        targetSummary: "service:redis",
      },
      process: [
        {
          id: "tool-redis-info",
          kind: "tool",
          displayKind: "redis.info",
          status: "failed",
          text: "redis.info failed: connection timeout",
          outputPreview: "connection timeout while reading replication info",
        },
        {
          id: "checkpoint-error-recovery",
          kind: "system",
          displayKind: "checkpoint.error_recovery",
          status: "completed",
          text: "error recovery checkpoint: captured failed tool output, preserved evidence, and continued ordinary AI Chat analysis",
          checkpointId: "checkpoint-error-recovery",
        },
        {
          id: "assistant-error-recovery-commentary",
          kind: "assistant",
          displayKind: "assistant.message",
          phase: "commentary",
          streamState: "complete",
          status: "completed",
          text: "我会基于已有证据继续普通分析。",
        },
      ],
    }),
    mustText: ["redis.info failed", "error recovery checkpoint", "continued ordinary AI Chat analysis", "继续普通分析"],
  },
];

if (args.has("--help") || args.has("-h")) {
  console.log(`Usage: node scripts/verify-aiops-second-closure-2.0.mjs [--base-url URL] [--screenshot-dir DIR] [--dry-run]

Environment:
  AIOPS_SECOND_CLOSURE_URL             Base URL. Default: ${defaultBaseURL}
  AIOPS_SECOND_CLOSURE_SCREENSHOT_DIR  Screenshot directory. Default: ${screenshotDir}
`);
  process.exit(0);
}

await fs.mkdir(screenshotDir, { recursive: true });

if (args.has("--dry-run")) {
  const manifestPath = path.join(screenshotDir, "scenario-manifest.json");
  await fs.writeFile(
    manifestPath,
    `${JSON.stringify({ baseURL, screenshotDir, scenarios: scenarios.map(stripFixture) }, null, 2)}\n`,
  );
  console.log(`dry-run ok: wrote ${manifestPath}`);
  process.exit(0);
}

const { chromium } = await import("../web/node_modules/playwright/index.mjs");
const browser = await chromium.launch({ headless: true });
const report = {
  baseURL,
  screenshotDir,
  startedAt: new Date().toISOString(),
  scenarios: [],
};

try {
  for (const [index, scenario] of scenarios.entries()) {
    const result = await runScenario(browser, scenario, index + 1);
    report.scenarios.push(result);
    console.log(`${result.ok ? "PASS" : "FAIL"} ${scenario.id}`);
    if (!result.ok) {
      throw new Error(`${scenario.id}: ${result.errors.join("; ")}`);
    }
  }
} finally {
  report.finishedAt = new Date().toISOString();
  await fs.writeFile(
    path.join(screenshotDir, "report.json"),
    `${JSON.stringify(report, null, 2)}\n`,
  );
  await browser.close();
}

async function runScenario(browser, scenario, ordinal) {
  const prefix = `${String(ordinal).padStart(2, "0")}-${scenario.id}`;
  const errors = [];
  const context = await browser.newContext({
    baseURL,
    viewport: { width: 1440, height: 960 },
    ignoreHTTPSErrors: true,
  });
  if (scenario.fixture) {
    await context.addInitScript((fixture) => {
      window.__CODEX_UI_FIXTURE__ = fixture;
    }, scenario.fixture);
  }
  const page = await context.newPage();
  try {
    await page.goto(baseURL, { waitUntil: "domcontentloaded", timeout: 30_000 });
    if (scenario.mode === "submit") {
      await createFreshSingleHostSession(page);
      await page.reload({ waitUntil: "domcontentloaded", timeout: 30_000 });
    }
    if (!scenario.skipComposerWait) {
      await waitForComposer(page);
    }
    if (scenario.mode === "submit") {
      await submitMessage(page, scenario.message);
      await page.waitForTimeout(2_000);
    }
    if (scenario.expandProcess) {
      await expandProcessTranscript(page);
    }
    await page.screenshot({
      path: path.join(screenshotDir, `${prefix}.png`),
      fullPage: true,
    });
    const text = await page.locator("body").innerText().catch(() => "");
    await fs.writeFile(path.join(screenshotDir, `${prefix}.txt`), `${text}\n`);
    checkText(scenario, text, errors);
  } catch (error) {
    errors.push(error instanceof Error ? error.message : String(error));
    await fs
      .writeFile(path.join(screenshotDir, `${prefix}-error.txt`), `${errors.join("\n")}\n`)
      .catch(() => null);
  } finally {
    await context.close();
  }
  return {
    id: scenario.id,
    title: scenario.title,
    ok: errors.length === 0,
    errors,
  };
}

async function expandProcessTranscript(page) {
  const header = page.locator('[data-testid="aiops-process-header"]').first();
  if (await header.isVisible({ timeout: 2_000 }).catch(() => false)) {
    const expanded = await header.getAttribute("aria-expanded").catch(() => "");
    if (expanded !== "true") {
      await header.click();
    }
    await page.locator('[data-testid="aiops-process-transcript-body"]').first().waitFor({ state: "visible", timeout: 3_000 }).catch(() => {});
  }
}

async function waitForComposer(page) {
  const composer = page.locator('[data-testid="omnibar-input"], textarea').first();
  await composer.waitFor({ state: "visible", timeout: 30_000 });
  await page.waitForFunction(() => {
    const el = document.querySelector('[data-testid="omnibar-input"], textarea');
    return Boolean(el && !el.disabled && !el.getAttribute("aria-disabled"));
  }, null, { timeout: 30_000 });
}

async function createFreshSingleHostSession(page) {
  await page.evaluate(async () => {
    await fetch("/api/v1/sessions", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ kind: "single_host" }),
    });
  });
}

async function submitMessage(page, message) {
  const composer = page.locator('[data-testid="omnibar-input"], textarea').first();
  await waitForComposer(page);
  await composer.fill("");
  await composer.fill(message);
  const primary = page.locator('[data-testid="omnibar-primary-action"]').first();
  if (await primary.isVisible().catch(() => false)) {
    await primary.click();
    return;
  }
  await composer.press(process.platform === "darwin" ? "Meta+Enter" : "Control+Enter");
}

function checkText(scenario, text, errors) {
  for (const needle of scenario.mustText || []) {
    if (!text.includes(needle)) {
      errors.push(`missing text ${JSON.stringify(needle)}`);
    }
  }
  if (scenario.mustTextAny && !scenario.mustTextAny.some((needle) => text.includes(needle))) {
    errors.push(`missing any text from ${scenario.mustTextAny.map((item) => JSON.stringify(item)).join(", ")}`);
  }
  for (const needle of scenario.mustNotText || []) {
    if (text.includes(needle)) {
      errors.push(`unexpected text ${JSON.stringify(needle)}`);
    }
  }
}

function fixtureState({ finalText, mcpHealthy, opsRun, process = [], pendingApprovals = {} }) {
  const now = "2026-06-23T10:00:00Z";
  const turnId = `${opsRun.id}-turn`;
  const terminal = opsRun.status === "completed" || opsRun.status === "failed" || opsRun.status === "canceled";
  const agentRunStatus = opsRun.status === "blocked" ? "running" : opsRun.status || "completed";
  const agentSteps = process.length
    ? process.map((block) => ({
        id: block.id,
        kind:
          block.kind === "approval"
            ? "approval"
            : block.kind === "system" && String(block.displayKind || "").includes("checkpoint")
              ? "checkpoint"
              : block.kind === "tool" || block.kind === "command"
                ? "tool_call"
                : block.kind === "assistant"
                  ? "final_response"
                  : block.kind === "search"
                    ? "tool_search"
                    : "reasoning",
        status: block.status === "blocked" ? "waiting_approval" : block.status,
        title: block.text,
        toolName: block.displayKind || block.source,
        approvalId: block.approvalId,
        checkpointId: block.checkpointId,
        outputSummary: block.outputPreview,
      }))
    : [
        { id: "evidence-1", kind: "evidence", status: "completed", title: "证据整理", outputSummary: "observed facts" },
        { id: "final-1", kind: "final_response", status: "completed", title: "最终结论", outputSummary: finalText },
      ];
  return {
    state: {
      schemaVersion: "aiops.transport.v2",
      sessionId: "fixture-second-closure",
      threadId: "fixture-second-closure",
      status: opsRun.status === "blocked" ? "blocked" : "idle",
      currentTurnId: turnId,
      opsRun: {
        source: "chat",
        evidenceCount: 3,
        agentRun: {
          id: opsRun.id,
          userGoal: opsRun.title,
          status: agentRunStatus,
          targetSummary: opsRun.targetSummary,
          currentStep: opsRun.currentStep,
          currentStepId: opsRun.currentStepId,
          checkpointId: opsRun.checkpointId,
          steps: agentSteps,
        },
        ...opsRun,
      },
      turns: {
        [turnId]: {
          id: turnId,
          status: opsRun.status === "blocked" ? "blocked" : "completed",
          startedAt: now,
          completedAt: terminal ? now : "",
          user: { id: `${turnId}:user`, text: opsRun.title, createdAt: now },
          process,
          final: { id: `${turnId}:final`, text: finalText, status: "completed" },
        },
      },
      turnOrder: [turnId],
      pendingApprovals,
      mcpSurfaces:
        mcpHealthy === undefined
          ? {}
          : {
              coroot: {
                id: "coroot",
                name: "Coroot",
                status: mcpHealthy ? "healthy" : "unhealthy",
                summary: mcpHealthy ? "Coroot MCP connected" : "Coroot MCP unavailable",
              },
            },
      artifacts: {},
      runtimeLiveness: {
        activeTurns: opsRun.status === "blocked" ? { [turnId]: true } : {},
        activeAgents: {},
        pendingApprovals: Object.fromEntries(Object.keys(pendingApprovals).map((id) => [id, true])),
        pendingUserInputs: {},
        activeCommandStreams: {},
      },
      hostMissions: {},
      childAgents: {},
      seq: 1,
      updatedAt: now,
    },
  };
}

function stripFixture(scenario) {
  const { fixture, ...rest } = scenario;
  return fixture ? { ...rest, fixture: true } : rest;
}

function argValue(name) {
  const index = process.argv.indexOf(name);
  if (index < 0 || index + 1 >= process.argv.length) {
    return "";
  }
  return process.argv[index + 1];
}
