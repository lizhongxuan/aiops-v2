#!/usr/bin/env node
import fs from "node:fs/promises";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";
import { hasActiveRuntimeWork } from "./agent-output-stability-sample.mjs";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(scriptDir, "..");
const baseURL = process.env.AIOPS_AGENT_OUTPUT_STABILITY_URL || "http://127.0.0.1:18083/";
const outDir =
  process.env.AIOPS_AGENT_OUTPUT_STABILITY_OUT_DIR ||
  path.join(repoRoot, "output/playwright/agent-runtime-output-stability-20260625");
const timeoutMs = Number.parseInt(process.env.AIOPS_AGENT_OUTPUT_STABILITY_TIMEOUT_MS || "240000", 10);
const sampleIntervalMs = Number.parseInt(process.env.AIOPS_AGENT_OUTPUT_STABILITY_SAMPLE_MS || "5000", 10);
const configureLLM = process.env.AIOPS_AGENT_OUTPUT_STABILITY_CONFIGURE_LLM === "1";
const prompt = await loadPrompt();

const knownToolPreludes = [
  "I'll search for relevant documentation",
  "Let me try browsing",
  "Let me check",
  "我会先搜索",
  "我先查阅",
  "我先查看",
  "让我深入查看",
  "让我进一步",
  "让我先",
  "正在等待模型返回",
];

await fs.mkdir(outDir, { recursive: true });
await cleanRunArtifacts(outDir);
await maybeConfigureLLM();

const { chromium } = await import("../web/node_modules/playwright/index.mjs");
const browser = await chromium.launch({ headless: true });
const context = await browser.newContext({
  baseURL,
  viewport: { width: 1440, height: 960 },
  ignoreHTTPSErrors: true,
});
const page = await context.newPage();

const report = {
  baseURL: redactURL(baseURL),
  outDir,
  startedAt: new Date().toISOString(),
  promptChars: prompt.length,
  configuredLLMFromEnv: configureLLM,
  samples: [],
  failures: [],
};

try {
  await context.addInitScript(() => {
    window.localStorage.setItem("aiops.debugTranscript", "1");
  });
  const url = withDebugTranscript(baseURL);
  await page.goto(url, { waitUntil: "domcontentloaded", timeout: 30_000 });
  await waitForComposer(page);
  const sessionMode = await createFreshSession(page);
  report.sessionMode = sessionMode;
  if (sessionMode === "api") {
    await page.reload({ waitUntil: "domcontentloaded" });
    await waitForComposer(page);
  }
  await savePageArtifact(page, "00-initial");

  await submitMessage(page, prompt);
  await savePageArtifact(page, "01-submitted");

  const started = Date.now();
  let finalSeenAt = null;
  let finalTextWhenSeen = "";
  let postFinalActivity = false;
  let previousBodyLength = 0;

  while (Date.now() - started <= timeoutMs) {
    await page.waitForTimeout(report.samples.length === 0 ? 1000 : sampleIntervalMs);
    await expandProcessSections(page);
    const elapsedMs = Date.now() - started;
    const sample = await collectSample(page, elapsedMs);
    report.samples.push(sample);
    await savePageArtifact(page, `sample-${String(report.samples.length).padStart(2, "0")}`);

    if (sample.finalText && finalSeenAt == null) {
      finalSeenAt = elapsedMs;
      finalTextWhenSeen = sample.finalText;
    } else if (sample.finalText && sample.finalText !== finalTextWhenSeen) {
      report.failures.push("final text changed after first appearance");
      break;
    }
    if (finalSeenAt != null && sample.hasActiveModelOrToolAfterFinal) {
      postFinalActivity = true;
    }
    if (isTerminalSample(sample)) {
      break;
    }
    if (sample.bodyLength < previousBodyLength * 0.65 && previousBodyLength > 1000) {
      report.failures.push("visible transcript body shrank sharply during processing");
      break;
    }
    previousBodyLength = Math.max(previousBodyLength, sample.bodyLength);
  }

  const last = report.samples.at(-1) || {};
  const finalText = String(last.finalText || "");
  if (!finalText.trim()) {
    report.failures.push("no final answer observed before timeout");
  }
  if (knownToolPreludes.some((needle) => finalText.includes(needle))) {
    report.failures.push("final answer looks like a tool prelude");
  }
  if (postFinalActivity) {
    report.failures.push("model/tool activity continued after final appeared");
  }
  if (/failed to receive stream chunk|context deadline exceeded|unexpected eof|stream chunk|upstream request timeout/i.test(finalText)) {
    report.failures.push("raw stream error became final answer");
  }
  if (/final contract|non_substantive_final_answer|kinds=|signals=|Official-domain fallback results|\{"content"/i.test(finalText)) {
    report.failures.push("final answer leaked internal evidence/debug text");
  }
  if (/pgbackrest|pg_autoctl|timeline/i.test(prompt)) {
    if (/还不能给最终结论/.test(finalText)) {
      report.failures.push("pgBackRest longcase fell back to incomplete final instead of RCA");
    }
    if (!/(根因|原因|timeline|pgBackRest|pg_autoctl|recovery_target_timeline|PGDATA)/i.test(finalText)) {
      report.failures.push("pgBackRest longcase final lacks RCA content");
    }
  }
  if (last.rawStreamErrorVisible) {
    report.failures.push("raw stream error is visible in transcript");
  }
  if (/网页检索\s+1[0-9]\s*(项|次)/.test(String(last.bodyText || ""))) {
    report.failures.push("search card shows excessive 10+ item count");
  }

  report.finishedAt = new Date().toISOString();
  await writeReport(report);
  if (report.failures.length) {
    throw new Error(report.failures.join("; "));
  }
  console.log(`PASS agent output stability: ${report.samples.length} samples`);
} finally {
  report.finishedAt ||= new Date().toISOString();
  await writeReport(report).catch(() => null);
  await browser.close();
}

async function maybeConfigureLLM() {
  if (!configureLLM) {
    return;
  }
  const baseURLFromEnv = process.env.AIOPS_TEST_LLM_BASE_URL || "";
  const apiKey = process.env.AIOPS_TEST_LLM_API_KEY || "";
  const model = process.env.AIOPS_TEST_LLM_MODEL || "";
  if (!baseURLFromEnv || !apiKey || !model) {
    throw new Error("AIOPS_AGENT_OUTPUT_STABILITY_CONFIGURE_LLM=1 requires AIOPS_TEST_LLM_BASE_URL, AIOPS_TEST_LLM_API_KEY, AIOPS_TEST_LLM_MODEL");
  }
  const endpoint = new URL("/api/v1/llm-config", baseURL);
  const response = await fetch(endpoint, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      provider: "zhipu",
      baseURL: baseURLFromEnv,
      apiKey,
      model,
      maxContextTokens: 120000,
    }),
  });
  if (!response.ok) {
    throw new Error(`failed to configure LLM: HTTP ${response.status}`);
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

async function createFreshSession(page) {
  const newSession = page.locator('button:has-text("新建会话")').first();
  if (await newSession.isVisible().catch(() => false)) {
    await newSession.click();
    await page.waitForTimeout(750);
    await waitForComposer(page);
    return "ui";
  }
  await page.evaluate(async () => {
    await fetch("/api/v1/sessions", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ kind: "workspace" }),
    }).catch(() => null);
  });
  return "api";
}

async function submitMessage(page, message) {
  const composer = page.locator('[data-testid="omnibar-input"], textarea').first();
  await composer.fill("");
  await composer.fill(message.trim());
  const primary = page.locator('[data-testid="omnibar-primary-action"]').first();
  if (await primary.isVisible().catch(() => false)) {
    await primary.click();
    return;
  }
  await composer.press(process.platform === "darwin" ? "Meta+Enter" : "Control+Enter");
}

async function expandProcessSections(page) {
  const buttons = page.locator('button:has-text("已处理"), button:has-text("处理中"), button:has-text("网页检索")');
  const count = await buttons.count().catch(() => 0);
  for (let i = 0; i < Math.min(count, 8); i++) {
    const button = buttons.nth(i);
    const expanded = await button.getAttribute("aria-expanded").catch(() => null);
    if (expanded === "false") {
      await button.click().catch(() => null);
    }
  }
}

async function collectSample(page, elapsedMs) {
  const bodyText = await page.locator("body").innerText({ timeout: 10_000 }).catch(() => "");
  const finalText = await page
    .locator('[data-testid="aiops-final-text"]')
    .last()
    .innerText({ timeout: 1000 })
    .catch(() => "");
  const statusText = await page
    .locator('button:has-text("处理中"), button:has-text("已处理"), button:has-text("等待审核")')
    .last()
    .innerText({ timeout: 1000 })
    .catch(() => "");
  const processBlocks = await page
    .locator('[data-testid^="aiops-"], .aiops-message-markdown')
    .evaluateAll((nodes) =>
      nodes.slice(-40).map((node) => ({
        testId: node.getAttribute("data-testid") || "",
        text: (node.textContent || "").trim().slice(0, 500),
      })),
    )
    .catch(() => []);
  const lower = bodyText.toLowerCase();
  const hasActive = hasActiveRuntimeWork({ statusText, bodyText, processBlocks });
  return {
    elapsedMs,
    statusText,
    bodyLength: bodyText.length,
    finalText: finalText.trim(),
    bodyText: bodyText.slice(0, 5000),
    hasActiveModelOrToolAfterFinal: Boolean(finalText.trim() && hasActive),
    rawStreamErrorVisible: /failed to receive stream chunk|context deadline exceeded|unexpected eof|stream chunk|upstream request timeout/.test(lower),
    processBlocks,
  };
}

function isTerminalSample(sample) {
	const text = `${sample.statusText}\n${sample.bodyText}`;
	if (hasActiveRuntimeWork(sample)) {
		return false;
	}
  return Boolean(sample.finalText && (/已处理/.test(sample.statusText) || /根因|结论|下一步|证据/.test(sample.finalText)));
}

async function savePageArtifact(page, label) {
  const text = await page.locator("body").innerText({ timeout: 10_000 }).catch(() => "");
  await fs.writeFile(path.join(outDir, `${label}-page-text.txt`), `${text}\n`);
  await page.screenshot({ path: path.join(outDir, `${label}.png`), fullPage: true }).catch(() => null);
  return text;
}

async function writeReport(value) {
  const redacted = JSON.parse(JSON.stringify(value));
  await fs.writeFile(path.join(outDir, "agent-output-stability-report.json"), `${JSON.stringify(redacted, null, 2)}\n`);
}

async function loadPrompt() {
  const promptFile = process.env.AIOPS_AGENT_OUTPUT_STABILITY_PROMPT_FILE || "";
  if (promptFile) {
    return (await fs.readFile(promptFile, "utf8")).trim();
  }
  return (
    process.env.AIOPS_AGENT_OUTPUT_STABILITY_PROMPT ||
    "我用pgbackrest对主机做了几次备份,然后选择某个的备份记录恢复了主机A,现在想把主机A加入主机C的pg_mon中,并且把主机B当做从节点加入集群,为什么从节点执行命令 pg_autoctl create postgres 会失败?"
  );
}

async function cleanRunArtifacts(directory) {
  const entries = await fs.readdir(directory).catch(() => []);
  await Promise.all(
    entries
      .filter((entry) =>
        /^(?:00-|01-|sample-\d+|agent-output-stability-report\.json$|browser-console-events\.json$)/.test(entry),
      )
      .map((entry) => fs.rm(path.join(directory, entry), { force: true, recursive: true })),
  );
}

function withDebugTranscript(rawURL) {
  const url = new URL(rawURL);
  url.searchParams.set("debugTranscript", "1");
  return url.toString();
}

function redactURL(value) {
  try {
    const url = new URL(value);
    url.username = "";
    url.password = "";
    return url.toString();
  } catch {
    return value;
  }
}
