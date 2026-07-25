#!/usr/bin/env bash
set -euo pipefail

evidence_root="${1:-}"
if [[ -z "${evidence_root}" || ! -d "${evidence_root}" ]]; then
  echo "usage: $0 <workflow-evidence-directory>" >&2
  exit 2
fi

node --input-type=module - "${evidence_root}" <<'NODE'
import fs from "node:fs";
import path from "node:path";

const root = path.resolve(process.argv[2]);
const allowedEnvironmentKeys = new Set([
  "schemaVersion", "profile", "runnerOS", "arch", "browser", "browserVersion",
  "commitSHA", "buildSHA", "activePhase", "capabilityTags", "viewport", "url",
  "title", "manifestDigest", "startedAt", "completedAt", "counts",
]);
const allowedRedactions = new Set(["[redacted]", "<redacted>", "***", "redacted"]);
const findings = [];
const protectedValues = [
  process.env.AIOPS_LIVE_MODEL_API_KEY,
  process.env.ZAI_API_KEY,
  process.env.OPENAI_API_KEY,
].filter((value) => typeof value === "string" && value.length >= 8);
let scanned = 0;

function walk(directory) {
  for (const entry of fs.readdirSync(directory, { withFileTypes: true })) {
    const fullPath = path.join(directory, entry.name);
    if (entry.isSymbolicLink()) {
      findings.push(`${fullPath}: symbolic links are not allowed in evidence`);
    } else if (entry.isDirectory()) {
      walk(fullPath);
    } else if (entry.isFile()) {
      inspect(fullPath);
    }
  }
}

function inspect(file) {
  const raw = fs.readFileSync(file);
  if (raw.includes(0)) return;
  const text = raw.toString("utf8");
  scanned += 1;
  const patterns = [[/(?:authorization|proxy-authorization)\s*[:=]\s*(?:bearer|basic)\s+[A-Za-z0-9._~+\/-]{12,}/ig, "authorization credential"]];
  if (/\.(?:jsonl?|har|log|txt|xml|junit)$/i.test(file)) {
    patterns.push(
      [/(?:api[_-]?key|access[_-]?token|refresh[_-]?token|client[_-]?secret|password)\s*[=:]\s*["']?([^\s,"'}]{8,})/ig, "secret-like field"],
      [/[?&](?:api[_-]?key|access[_-]?token|password)=([^&#\s]{8,})/ig, "secret-like URL parameter"],
    );
  }
  for (const [pattern, label] of patterns) {
    for (const match of text.matchAll(pattern)) {
      const candidate = String(match[1] || match[0]).replace(/^['"]|['"]$/g, "");
      if (allowedRedactions.has(candidate.toLowerCase()) || candidate.startsWith("${")) continue;
      const line = text.slice(0, match.index).split("\n").length;
      findings.push(`${file}:${line}: ${label}`);
    }
  }
  for (const protectedValue of protectedValues) {
    if (text.includes(protectedValue)) findings.push(`${file}: configured live-provider secret is present`);
  }
  if (/\.(?:json|har)$/i.test(file)) inspectStructuredSecrets(file, text);
  if (path.basename(file) === "environment.json") inspectEnvironment(file, text);
}

function inspectStructuredSecrets(file, text) {
  let value;
  try {
    value = JSON.parse(text);
  } catch {
    return;
  }
  const secretKey = /^(?:authorization|proxy-authorization|api[_-]?key|access[_-]?token|refresh[_-]?token|client[_-]?secret|password)$/i;
  const visit = (item, pointer) => {
    if (Array.isArray(item)) return item.forEach((child, index) => visit(child, `${pointer}/${index}`));
    if (!item || typeof item !== "object") return;
    for (const [key, child] of Object.entries(item)) {
      const next = `${pointer}/${key}`;
      if (secretKey.test(key) && typeof child === "string") {
        const normalized = child.trim().toLowerCase();
        if (child.length >= 8 && !allowedRedactions.has(normalized) && !child.startsWith("${")) {
          findings.push(`${file}:${next}: secret field is not redacted`);
        }
      }
      visit(child, next);
    }
  };
  visit(value, "");
}

function inspectEnvironment(file, text) {
  let value;
  try {
    value = JSON.parse(text);
  } catch {
    findings.push(`${file}: environment.json is not valid JSON`);
    return;
  }
  if (!value || Array.isArray(value) || typeof value !== "object") {
    findings.push(`${file}: environment.json must be an allowlisted object`);
    return;
  }
  for (const key of Object.keys(value)) {
    if (!allowedEnvironmentKeys.has(key)) findings.push(`${file}: environment key ${key} is not allowlisted`);
  }
}

walk(root);
if (findings.length) {
  console.error("workflow evidence redaction failed:");
  for (const finding of findings) console.error(`- ${finding}`);
  process.exit(1);
}
console.log(`workflow evidence redaction passed: ${scanned} text files scanned`);
NODE
