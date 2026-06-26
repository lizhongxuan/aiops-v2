#!/usr/bin/env node
import fs from "node:fs/promises";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

import { resolveUiFixturePreset } from "../web/src/lib/uiFixturePresets.js";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(scriptDir, "..");
const defaultBaseURL = "http://127.0.0.1:18083/";
const baseURL =
  process.env.AIOPS_CODEX_RUNTIME_CONTRACT_V3_URL || defaultBaseURL;
const outDir =
  process.env.AIOPS_CODEX_RUNTIME_CONTRACT_V3_OUT_DIR ||
  path.join(repoRoot, "output/playwright/codex-runtime-contract-v3");
const timeoutMs = Number.parseInt(
  process.env.AIOPS_CODEX_RUNTIME_CONTRACT_V3_TIMEOUT_MS || "120000",
  10,
);
const useFixture =
  process.env.AIOPS_CODEX_RUNTIME_CONTRACT_V3_USE_FIXTURE !== "0";
const args = new Set(process.argv.slice(2));

const timelineMarkers = [
  {
    id: "approval-pause",
    label: "approval pause",
    requiredText: ["approval pause marker"],
  },
  {
    id: "approval-denied-continuation",
    label: "approval denied continuation",
    requiredText: ["approval denied continuation marker"],
  },
  {
    id: "multi-host-child-agent-timeline",
    label: "multi-host child agent timeline",
    requiredText: ["multi-host child agent timeline marker"],
  },
  {
    id: "context-compacted",
    label: "context compacted marker",
    requiredText: ["context compacted marker"],
  },
  {
    id: "pending-input-steer",
    label: "pending input accepted / steer marker",
    requiredText: ["pending input accepted / steer marker"],
  },
  {
    id: "turn-cancelled-aborted-tool",
    label: "turn cancelled / aborted tool marker",
    requiredText: ["turn cancelled / aborted tool marker"],
  },
  {
    id: "resource-lock-conflict",
    label: "resource lock conflict marker",
    requiredText: ["resource lock conflict marker"],
  },
];

const defaultPrompt = [
  "synthetic_codex_runtime_contract_v3_smoke:",
  "Render a UI-visible timeline that includes approval pause, approval denied continuation,",
  "multi-host child agent timeline, context compacted marker, pending input accepted / steer marker,",
  "turn cancelled / aborted tool marker, and resource lock conflict marker.",
  "Use the literal marker phrases so browser smoke can verify projection.",
].join(" ");

if (args.has("--help") || args.has("-h")) {
  console.log(`Usage: node scripts/verify-aiops-codex-runtime-contract-v3.mjs [--dry-run]

Environment:
  AIOPS_CODEX_RUNTIME_CONTRACT_V3_URL         App URL. Default: ${defaultBaseURL}
  AIOPS_CODEX_RUNTIME_CONTRACT_V3_OUT_DIR     Artifact directory. Default: ${outDir}
  AIOPS_CODEX_RUNTIME_CONTRACT_V3_TIMEOUT_MS  Marker wait timeout. Default: ${timeoutMs}
  AIOPS_CODEX_RUNTIME_CONTRACT_V3_PROMPT      Prompt submitted in live mode.
  AIOPS_CODEX_RUNTIME_CONTRACT_V3_USE_FIXTURE Default: 1. Set 0 to submit to a live backend.

Default mode injects a browser fixture with the seven runtime contract V3
timeline markers and verifies the real UI rendering. Set USE_FIXTURE=0 when
you want to submit the prompt to a live backend instead. --dry-run validates
the smoke manifest without launching a browser.
`);
  process.exit(0);
}

if (args.has("--dry-run")) {
  assertMarkerManifest(timelineMarkers);
  console.log(`dry-run ok: ${timelineMarkers.length} runtime contract V3 timeline markers configured`);
  for (const marker of timelineMarkers) {
    console.log(`MARKER ${marker.id}: ${marker.label}`);
  }
  process.exit(0);
}

assertMarkerManifest(timelineMarkers);
await fs.mkdir(outDir, { recursive: true });

const { chromium } = await import("../web/node_modules/playwright/index.mjs");
const browser = await chromium.launch({ headless: true });
const context = await browser.newContext({
  baseURL,
  viewport: { width: 1440, height: 960 },
  ignoreHTTPSErrors: true,
});
const page = await context.newPage();
const report = {
  baseURL,
  outDir,
  mode: useFixture ? "fixture" : "live-backend",
  startedAt: new Date().toISOString(),
  markers: timelineMarkers.map(({ id, label, requiredText }) => ({
    id,
    label,
    requiredText,
    visible: false,
  })),
  failures: [],
};

try {
  if (useFixture) {
    await context.addInitScript((fixture) => {
      window.__CODEX_UI_FIXTURE__ = fixture;
    }, runtimeContractV3Fixture());
  }
  await page.goto(baseURL, { waitUntil: "domcontentloaded", timeout: 30_000 });
  await savePageArtifact(page, "00-initial");

  if (!useFixture) {
    await waitForComposer(page);
    await submitMessage(
      page,
      process.env.AIOPS_CODEX_RUNTIME_CONTRACT_V3_PROMPT || defaultPrompt,
    );
    await savePageArtifact(page, "01-submitted");
  }

  const finalText = await waitForMarkers(page, timelineMarkers, timeoutMs);
  await savePageArtifact(page, "02-markers");
  for (const marker of report.markers) {
    marker.visible = marker.requiredText.every((needle) =>
      finalText.includes(needle),
    );
    console.log(`${marker.visible ? "PASS" : "FAIL"} ${marker.id}: ${marker.label}`);
  }

  report.failures = report.markers
    .filter((marker) => !marker.visible)
    .map((marker) => `missing ${marker.id}`);
  if (report.failures.length) {
    throw new Error(report.failures.join("; "));
  }
  console.log(`PASS runtime contract V3 smoke: ${timelineMarkers.length} timeline markers visible`);
} finally {
  report.finishedAt = new Date().toISOString();
  await fs.writeFile(
    path.join(outDir, "report.json"),
    `${JSON.stringify(report, null, 2)}\n`,
  );
  await browser.close();
}

function assertMarkerManifest(markers) {
  const ids = new Set();
  for (const marker of markers) {
    if (!marker.id || ids.has(marker.id)) {
      throw new Error(`invalid duplicate marker id ${JSON.stringify(marker.id)}`);
    }
    ids.add(marker.id);
    if (!marker.label || !marker.requiredText?.length) {
      throw new Error(`invalid marker manifest entry ${marker.id}`);
    }
  }
  if (markers.length !== 7) {
    throw new Error(`runtime contract V3 marker count=${markers.length}, want 7`);
  }
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

async function waitForMarkers(page, markers, maxWaitMs) {
  const started = Date.now();
  let lastText = "";
  while (Date.now() - started < maxWaitMs) {
    await expandProcessSections(page);
    lastText = await page.locator("body").innerText({ timeout: 10_000 }).catch(() => "");
    const missing = missingMarkers(lastText, markers);
    if (missing.length === 0) {
      return lastText;
    }
    await page.waitForTimeout(1000);
  }
  const missing = missingMarkers(lastText, markers);
  throw new Error(`missing runtime contract V3 markers: ${missing.join(", ")}`);
}

async function expandProcessSections(page) {
  await page
    .locator('[data-testid="aiops-process-header"][aria-expanded="false"]')
    .evaluateAll((buttons) => {
      for (const button of buttons) {
        button.dispatchEvent(new MouseEvent("click", { bubbles: true }));
      }
    })
    .catch(() => null);
  await page
    .locator(
      '[data-testid="aiops-process-transcript-body"] button[aria-expanded="false"]',
    )
    .evaluateAll((buttons) => {
      for (const button of buttons) {
        button.dispatchEvent(new MouseEvent("click", { bubbles: true }));
      }
    })
    .catch(() => null);
}

function missingMarkers(text, markers) {
  return markers
    .filter((marker) => !marker.requiredText.every((needle) => text.includes(needle)))
    .map((marker) => marker.id);
}

function runtimeContractV3Fixture() {
  const preset = resolveUiFixturePreset("runtime-contract-v3");
  if (!preset) {
    throw new Error("runtime-contract-v3 fixture preset not found");
  }
  return preset;
}
