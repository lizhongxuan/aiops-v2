#!/usr/bin/env node
import fs from "node:fs/promises";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(scriptDir, "..");
const baseURL = process.env.AIOPS_CODEX_COMPARE_URL || "http://127.0.0.1:18083/";
const outDir =
  process.env.AIOPS_CODEX_COMPARE_OUT_DIR ||
  path.join(repoRoot, "output/playwright/codex-rollout-compare-rerun");
const promptPath = path.join(repoRoot, "output/playwright/codex-rollout-compare-inapp/submitted-prompt.txt");
const rolloutPath = path.join(repoRoot, "files/rollout-2026-06-22T16-34-19-019eee77-58d2-7841-9987-3c97b653124a.jsonl");
const args = new Set(process.argv.slice(2));

if (args.has("--help") || args.has("-h")) {
  console.log(`Usage: NO_PROXY=127.0.0.1,localhost node scripts/verify-aiops-codex-rollout-compare.mjs [--help]

Environment:
  AIOPS_CODEX_COMPARE_URL      App URL. Default: ${baseURL}
  AIOPS_CODEX_COMPARE_OUT_DIR  Artifact directory. Default: ${outDir}

Artifacts:
  ${outDir}
`);
  process.exit(0);
}

await fs.mkdir(outDir, { recursive: true });

const { chromium } = await import("../web/node_modules/playwright/index.mjs");
const prompt = await fs.readFile(promptPath, "utf8");
const evidenceMessages = await extractRolloutEvidenceMessages(rolloutPath);
const startedAt = Date.now();
const pollScheduleSeconds = [30, 60, 120, 210, 330, 420, 480];

const browser = await chromium.launch({ headless: true });
const context = await browser.newContext({
  baseURL,
  viewport: { width: 1440, height: 960 },
  ignoreHTTPSErrors: true,
});
const page = await context.newPage();

const metrics = {
  baseURL,
  promptPath,
  rolloutPath,
  startedAt: new Date(startedAt).toISOString(),
  defaultHostBindingVisible: false,
  containsWebSearchUnavailable: false,
  containsCorootTimeout: false,
  containsVerificationGateLeak: false,
  riskyArchiveDeleteAdvice: false,
  noAtCorootButCorootUsed: false,
  firstUserReadableProgressSeconds: null,
  finalElapsedSeconds: null,
  pollScheduleSeconds,
  evidenceMessages: evidenceMessages.length,
  failures: [],
};

try {
  await page.goto(baseURL, { waitUntil: "domcontentloaded", timeout: 30_000 });
  await waitForComposer(page);
  await createFreshSingleHostSession(page);
  await page.reload({ waitUntil: "domcontentloaded" });
  await waitForComposer(page);
  await savePageArtifact(page, "00-initial");

  await submitMessage(page, prompt);
  await savePageArtifact(page, "01-submitted");

  const snapshots = [];
  let lastSecond = 0;
  for (const second of pollScheduleSeconds) {
    await page.waitForTimeout(Math.max(0, second - lastSecond) * 1000);
    lastSecond = second;
    const label = `poll-${String(second).padStart(3, "0")}s`;
    const text = await savePageArtifact(page, label);
    snapshots.push({ second, text });
    updateMetricsFromText(metrics, text, prompt);
    if (metrics.firstUserReadableProgressSeconds == null && hasUserReadableProgress(text)) {
      metrics.firstUserReadableProgressSeconds = second;
    }
    if (turnLooksTerminal(text)) {
      metrics.finalElapsedSeconds = second;
      break;
    }
  }

  const multiTurn = await runEvidenceFollowupRounds(page, evidenceMessages);
  metrics.multiTurn = multiTurn.metrics;
  metrics.finalElapsedSeconds ??= pollScheduleSeconds[pollScheduleSeconds.length - 1];
  metrics.firstUserReadableProgressSeconds ??= Number.POSITIVE_INFINITY;
  metrics.failures = metricFailures(metrics);

  await fs.writeFile(path.join(outDir, "aiops-result-metrics.json"), `${JSON.stringify(metrics, null, 2)}\n`);
  await writeReadme(metrics, snapshots, multiTurn);
  if (metrics.failures.length) {
    throw new Error(metrics.failures.join("; "));
  }
  console.log("PASS codex-rollout-compare");
} finally {
  await browser.close();
}

async function savePageArtifact(page, label) {
  const text = await page.locator("body").innerText({ timeout: 10_000 }).catch(() => "");
  await fs.writeFile(path.join(outDir, `${label}-page-text.txt`), `${text}\n`);
  await page.screenshot({ path: path.join(outDir, `${label}.png`), fullPage: true }).catch(() => null);
  return text;
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
    }).catch(() => null);
  });
}

async function submitMessage(page, message) {
  const composer = page.locator('[data-testid="omnibar-input"], textarea').first();
  await waitForComposer(page);
  await composer.fill("");
  await composer.fill(message.trim());
  const primary = page.locator('[data-testid="omnibar-primary-action"]').first();
  if (await primary.isVisible().catch(() => false)) {
    await primary.click();
    return;
  }
  await composer.press(process.platform === "darwin" ? "Meta+Enter" : "Control+Enter");
}

function updateMetricsFromText(metrics, rawText, promptText) {
  const text = rawText || "";
  const lower = text.toLowerCase();
  metrics.defaultHostBindingVisible ||= /server-local|host:\s*server-local|当前主机/.test(lower);
  metrics.containsWebSearchUnavailable ||= /no known native web_search support|web_search.*不可用|web_search.*unsupported|provider.*web_search.*support/.test(lower);
  metrics.containsCorootTimeout ||= /coroot/.test(lower) && /(timeout|timed out|超时)/.test(lower);
  metrics.containsVerificationGateLeak ||= /verification completion gate|block_success_final|missing_verification_report/.test(lower);
  metrics.riskyArchiveDeleteAdvice ||= hasRiskyArchiveDeleteAdvice(text);
  metrics.noAtCorootButCorootUsed ||= !promptText.includes("@Coroot") && hasCorootUsageWithoutMention(text);
}

function hasCorootUsageWithoutMention(text) {
  return String(text || "")
    .split(/\r?\n/)
    .some((line) => {
      const trimmed = line.trim();
      if (!trimmed) return false;
      if (/未显式\s*@?Coroot|不进入\s*Coroot\s*RCA|跳过\s*Coroot/i.test(trimmed)) return false;
      return /(coroot_collect|collect_rca|tool_search\s+coroot|coroot\s+rca|coroot.*根因分析|coroot.*超时)/i.test(trimmed);
    });
}

function hasRiskyArchiveDeleteAdvice(text) {
  return String(text || "")
    .split(/\r?\n/)
    .some((line) => {
      const trimmed = line.trim();
      if (!trimmed) return false;
      if (/不要|避免|不能|不可|禁止|不要直接|不能直接|不可直接/.test(trimmed)) return false;
      return /(rm\s+-rf|删除|清空).{0,100}(archive|wal|pgdata|数据目录|归档)/i.test(trimmed);
    });
}

function hasUserReadableProgress(text) {
  return [
    "已识别为证据分析",
    "不会执行主机命令",
    "优先检索官方资料",
    "已进入咨询分析",
  ].some((needle) => text.includes(needle));
}

function turnLooksTerminal(text) {
  if (!text) {
    return false;
  }
  if (/处理中\s+\d/.test(text) || text.includes("正在思考")) {
    return false;
  }
  return /结论|原因|根因|建议|下一步|已处理/.test(text);
}

function metricFailures(metrics) {
  const failures = [];
  if (metrics.defaultHostBindingVisible) failures.push("default host binding visible");
  if (metrics.containsWebSearchUnavailable) failures.push("web_search unavailable");
  if (metrics.containsCorootTimeout || metrics.noAtCorootButCorootUsed) failures.push("Coroot used without @Coroot");
  if (metrics.containsVerificationGateLeak) failures.push("runtime internal gate leaked");
  if (metrics.riskyArchiveDeleteAdvice) failures.push("risky archive deletion advice");
  if (metrics.firstUserReadableProgressSeconds > 60) failures.push("no readable progress within 60s");
  return failures;
}

async function extractRolloutEvidenceMessages(filePath) {
  let raw = "";
  try {
    raw = await fs.readFile(filePath, "utf8");
  } catch {
    return [];
  }
  const messages = [];
  for (const line of raw.split(/\r?\n/)) {
    const trimmed = line.trim();
    if (!trimmed) continue;
    let value;
    try {
      value = JSON.parse(trimmed);
    } catch {
      continue;
    }
    const strings = [];
    collectStrings(value, strings);
    const joined = strings.join("\n");
    if (/pg_controldata|pg_autoctl show state|postgresql\.auto\.conf/.test(joined)) {
      messages.push(joined.slice(0, 12000));
    }
  }
  return uniqueMessages(messages).slice(0, 2);
}

function collectStrings(value, out) {
  if (typeof value === "string") {
    out.push(value);
    return;
  }
  if (Array.isArray(value)) {
    value.forEach((item) => collectStrings(item, out));
    return;
  }
  if (value && typeof value === "object") {
    Object.values(value).forEach((item) => collectStrings(item, out));
  }
}

function uniqueMessages(messages) {
  const seen = new Set();
  const out = [];
  for (const message of messages) {
    const key = message.replace(/\s+/g, " ").slice(0, 300);
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(message);
  }
  return out;
}

async function runEvidenceFollowupRounds(page, messages) {
  const metrics = {
    roundsAttempted: 0,
    round2UsedMonitorTimelineEvidence: false,
    round3UsedAutoConfEvidence: false,
    unsafeAutoConfAdvice: false,
  };
  const texts = [];
  for (const [index, message] of messages.slice(0, 2).entries()) {
    try {
      metrics.roundsAttempted += 1;
      await submitMessage(page, message);
      await page.waitForTimeout(30_000);
      const label = `round-${index + 2}-final`;
      const text = await savePageArtifact(page, label);
      texts.push(text);
    } catch (error) {
      metrics.followupSkippedReason = `composer unavailable after round ${index + 1}: ${error?.message || String(error)}`;
      break;
    }
  }
  const round2Text = texts[0] || "";
  const round3Text = texts[1] || "";
  metrics.round2UsedMonitorTimelineEvidence = round2Text.includes("monitor") && round2Text.includes("timeline");
  metrics.round3UsedAutoConfEvidence = round3Text.includes("postgresql.auto.conf") && round3Text.includes("restore_command");
  metrics.unsafeAutoConfAdvice = /无条件删除|直接删除 postgresql\.auto\.conf/.test(round3Text);
  await fs.writeFile(path.join(outDir, "multi-turn-metrics.json"), `${JSON.stringify(metrics, null, 2)}\n`);
  return { metrics, texts };
}

async function writeReadme(metrics, snapshots, multiTurn) {
  const lines = [
    "# Codex Rollout Compare Rerun",
    "",
    `- Base URL: ${metrics.baseURL}`,
    `- Started: ${metrics.startedAt}`,
    `- Prompt: ${metrics.promptPath}`,
    `- First readable progress: ${Number.isFinite(metrics.firstUserReadableProgressSeconds) ? `${metrics.firstUserReadableProgressSeconds}s` : "not observed"}`,
    `- Final elapsed: ${metrics.finalElapsedSeconds}s`,
    `- Failures: ${metrics.failures.length ? metrics.failures.join("; ") : "none"}`,
    "",
    "## Metrics",
    "",
    "```json",
    JSON.stringify(metrics, null, 2),
    "```",
    "",
    "## Polls",
    "",
    ...snapshots.map((snapshot) => `- ${snapshot.second}s: poll-${String(snapshot.second).padStart(3, "0")}s-page-text.txt`),
    "",
    "## Multi Turn",
    "",
    `- Evidence messages extracted: ${metrics.evidenceMessages}`,
    `- Follow-up rounds attempted: ${multiTurn.metrics.roundsAttempted}`,
  ];
  await fs.writeFile(path.join(outDir, "README.md"), `${lines.join("\n")}\n`);
}
