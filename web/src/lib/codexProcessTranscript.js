const INTERNAL_DISPLAY_KINDS = new Set([
  "runtime.prepare_context",
  "runtime.compile_prompt",
  "runtime.prepare_tools",
  "runtime.call_model",
]);

const RAW_TOOL_NAMES = new Set([
  "exec_command",
  "shell_command",
  "code_mode",
]);

function normalizeText(value) {
  return String(value || "").replace(/\s+/g, " ").trim();
}

function isPureThinkingPlaceholder(value) {
  const text = normalizeText(value).replace(/\s+/g, "");
  return /^(?:正在思考|正在思考中)[。.!！…]*$/u.test(text);
}

function normalizeNarrationText(value) {
  return normalizeText(value)
    .replace(/\.\.\.$/u, "")
    .replace(/\s+/g, "")
    .replace(/[，。；：、,. ;:]/gu, "")
    .replace(/[“”"'`]/g, "");
}

function areNearDuplicateNarrations(left = "", right = "") {
  const leftText = normalizeNarrationText(left);
  const rightText = normalizeNarrationText(right);
  if (!leftText || !rightText) return false;
  if (leftText === rightText) return true;
  const minComparableLength = 48;
  if (leftText.length < minComparableLength || rightText.length < minComparableLength) return false;
  const prefixLength = Math.min(80, leftText.length, rightText.length);
  const leftPrefix = leftText.slice(0, prefixLength);
  const rightPrefix = rightText.slice(0, prefixLength);
  return leftPrefix === rightPrefix || leftText.includes(rightPrefix) || rightText.includes(leftPrefix);
}

function shouldReplaceNarration(existing = {}, candidate = {}) {
  const existingText = normalizeText(existing.text);
  const candidateText = normalizeText(candidate.text);
  if (!candidateText) return false;
  if (candidateText.endsWith("...") && !existingText.endsWith("...")) return false;
  if (existingText.endsWith("...") && !candidateText.endsWith("...")) return true;
  return candidateText.length > existingText.length;
}

function stripMatchingQuotes(value = "") {
  const text = String(value || "").trim();
  if (text.length >= 2 && ((text.startsWith("'") && text.endsWith("'")) || (text.startsWith('"') && text.endsWith('"')))) {
    return text.slice(1, -1);
  }
  return text;
}

function displayCommand(value = "") {
  const raw = normalizeText(value);
  if (!raw) return "";
  const shellMatch = raw.match(/^(?:\/[\w./-]+\/)?(?:zsh|bash|sh)\s+-lc\s+([\s\S]+)$/);
  if (shellMatch) return stripMatchingQuotes(shellMatch[1]);
  return raw;
}

function isRawToolName(value) {
  const text = normalizeText(value);
  return RAW_TOOL_NAMES.has(text) || /^(?:已运行|正在运行|运行失败)命令$/u.test(text);
}

function isHiddenItem(item) {
  return item?.visibility === "hidden"
    || INTERNAL_DISPLAY_KINDS.has(item?.displayKind);
}

function stableId(turnId, kind, seed) {
  const cleanSeed = String(seed || kind)
    .replace(/\s+/g, "-")
    .replace(/[^a-zA-Z0-9_.:-]/g, "")
    .slice(0, 80);
  return `${turnId}:${kind}:${cleanSeed || kind}`;
}

function blockBase(turnId, kind, source = {}) {
  return {
    id: stableId(turnId, kind, source.id || source.eventId || source.toolCallId || source.command || source.inputSummary || source.text),
    turnId,
    kind,
    displayKind: source.displayKind || "",
    status: source.status || "completed",
    text: normalizeText(source.text),
    visibility: source.visibility || "primary",
    startedAt: source.startedAt || source.createdAt || "",
    updatedAt: source.updatedAt || source.createdAt || "",
  };
}

function commandFromItem(item) {
  const candidates = [
    item?.command,
    item?.inputSummary,
    item?.summary,
    item?.text,
  ].map(displayCommand).filter(Boolean);
  return candidates.find((candidate) => !isRawToolName(candidate)) || "";
}

function commandText(status, command) {
  if (status === "running" || status === "queued") return `正在运行 ${command}`;
  if (status === "failed") return `运行失败 ${command}`;
  return `已运行 ${command}`;
}

function searchText(status, item) {
  if (status === "running" || status === "queued") return "正在搜索网页";
  const count = Array.isArray(item?.queries) && item.queries.length > 1
    ? item.queries.length
    : Array.isArray(item?.results) && item.results.length > 1
      ? item.results.length
      : 0;
  return count > 1 ? `已搜索网页 ${count} 次` : "已搜索网页";
}

function toCommandBlock(turnId, item) {
  const command = commandFromItem(item);
  if (!command) return null;
  const block = blockBase(turnId, "command-step", item);
  return {
    ...block,
    displayKind: item.displayKind || "host.command",
    text: commandText(block.status, command),
    command,
    inputSummary: item.inputSummary || command,
    outputPreview: item.outputPreview || item.output || item.summary || "",
  };
}

function toSearchBlock(turnId, item) {
  const block = blockBase(turnId, "search-step", item);
  const inputSummary = normalizeText(item.inputSummary || item.query || item.summary || item.text);
  const queries = Array.isArray(item.queries) && item.queries.length > 0
    ? item.queries
    : inputSummary
      ? [inputSummary]
      : [];
  return {
    ...block,
    displayKind: item.displayKind || "browser.search",
    text: searchText(block.status, { ...item, queries }),
    inputSummary,
    queries,
    results: Array.isArray(item.results) ? item.results : [],
  };
}

function toReasoningBlock(turnId, item) {
  const text = normalizeText(item.text || item.summary || item.inputSummary);
  if (!text || isPureThinkingPlaceholder(text)) return null;
  return {
    ...blockBase(turnId, "reasoning-summary", { ...item, text }),
    displayKind: item.displayKind || "reasoning.summary",
    text,
  };
}

function normalizePlanSteps(steps = []) {
  if (!Array.isArray(steps)) return [];
  return steps
    .map((step, index) => {
      const text = normalizeText(step?.text || step?.summary || step?.title);
      if (!text) return null;
      return {
        id: step?.id || `step-${index + 1}`,
        text,
        status: normalizeText(step?.status || "pending"),
        summary: normalizeText(step?.summary),
      };
    })
    .filter(Boolean);
}

function toPlanBlock(turnId, item) {
  const steps = normalizePlanSteps(item.steps);
  const activeStep = steps.find((step) => step.status === "running") || steps.find((step) => step.status === "in_progress");
  const fallbackStep = steps[steps.length - 1] || null;
  const text = normalizeText(item.text || item.summary || activeStep?.text || fallbackStep?.text || "计划");
  if (!text && steps.length === 0) return null;
  return {
    ...blockBase(turnId, "plan-step", { ...item, text }),
    displayKind: item.displayKind || "plan",
    text,
    summary: normalizeText(item.summary),
    steps,
  };
}

function toEvidenceBlock(turnId, item) {
  const title = normalizeText(item.title);
  const summary = normalizeText(item.summary);
  const text = normalizeText(item.text || (title && summary ? `${title}（${summary}）` : title || summary));
  if (!text) return null;
  return {
    ...blockBase(turnId, "evidence-step", { ...item, text }),
    displayKind: item.displayKind || "evidence",
    text,
    source: normalizeText(item.source),
    confidence: normalizeText(item.confidence),
    window: normalizeText(item.window),
    rawRef: normalizeText(item.rawRef),
  };
}

function typedProcessText(item = {}, fallback = "步骤") {
  return normalizeText(item.text || item.title || item.summary || item.reason || fallback);
}

function typedProcessMeta(item = {}) {
  return {
    summary: normalizeText(item.summary),
    command: normalizeText(item.command || item.inputSummary),
    reason: normalizeText(item.reason),
    risk: normalizeText(item.risk),
    source: normalizeText(item.source),
    runbookId: normalizeText(item.runbookId),
    runbookStep: normalizeText(item.runbookStep),
    expectedEffect: normalizeText(item.expectedEffect),
    rollback: normalizeText(item.rollback),
    confidence: normalizeText(item.confidence),
    window: normalizeText(item.window),
    rawRef: normalizeText(item.rawRef),
  };
}

function toRunbookBlock(turnId, item) {
  const text = typedProcessText(item, "Runbook");
  if (!text) return null;
  return {
    ...blockBase(turnId, "runbook-step", { ...item, text }),
    displayKind: item.displayKind || "runbook.step",
    text,
    ...typedProcessMeta(item),
  };
}

function toProposalBlock(turnId, item) {
  const text = typedProcessText(item, "Action proposal");
  if (!text) return null;
  return {
    ...blockBase(turnId, "proposal-step", { ...item, text }),
    displayKind: item.displayKind || "action.proposal",
    text,
    ...typedProcessMeta(item),
  };
}

function toVerificationBlock(turnId, item) {
  const text = typedProcessText(item, "Verification");
  if (!text) return null;
  return {
    ...blockBase(turnId, "verification-step", { ...item, text }),
    displayKind: item.displayKind || "verification.metric",
    text,
    ...typedProcessMeta(item),
  };
}

function toIncidentBlock(turnId, item) {
  const text = typedProcessText(item, "Incident");
  if (!text) return null;
  return {
    ...blockBase(turnId, "incident-step", { ...item, text }),
    displayKind: item.displayKind || "incident.evidence",
    text,
    ...typedProcessMeta(item),
  };
}

function toFileBlock(turnId, item) {
  const text = normalizeText(item.text || item.summary || item.inputSummary || item.path);
  if (!text) return null;
  return {
    ...blockBase(turnId, "file-step", { ...item, text }),
    text,
    inputSummary: item.inputSummary || item.path || "",
    outputPreview: item.outputPreview || item.output || "",
  };
}

function toMcpBlock(turnId, item) {
  const text = normalizeText(item.text || item.summary || item.inputSummary || item.toolName);
  if (!text) return null;
  return {
    ...blockBase(turnId, "mcp-step", { ...item, text }),
    text,
    inputSummary: item.inputSummary || item.toolName || "",
    outputPreview: item.outputPreview || item.output || "",
  };
}

function itemToBlock(turnId, item) {
  if (!item || isHiddenItem(item)) return null;
  const kind = item.kind || "";
  const displayKind = item.displayKind || "";
  const normalizedDisplayKind = displayKind.toLowerCase();
  if (kind === "runbook" || normalizedDisplayKind.startsWith("runbook.")) return toRunbookBlock(turnId, item);
  if (kind === "proposal" || normalizedDisplayKind === "action.proposal" || normalizedDisplayKind === "fallback.plan" || normalizedDisplayKind.startsWith("proposal.")) {
    return toProposalBlock(turnId, item);
  }
  if (kind === "verification" || normalizedDisplayKind.startsWith("verification.")) return toVerificationBlock(turnId, item);
  if (kind === "incident" || normalizedDisplayKind.startsWith("incident.")) return toIncidentBlock(turnId, item);
  if (kind === "plan" || displayKind === "plan") return toPlanBlock(turnId, item);
  if (kind === "evidence" || displayKind.startsWith("evidence.")) return toEvidenceBlock(turnId, item);
  if (kind === "approval" || displayKind.startsWith("approval.")) return approvalBlock(turnId, item);
  if (kind === "command" || displayKind === "host.command" || displayKind === "shell_command" || displayKind === "exec_command") {
    return toCommandBlock(turnId, item);
  }
  if (kind === "search" || displayKind === "browser.search") return toSearchBlock(turnId, item);
  if (kind === "reasoning" || displayKind === "reasoning.summary") return toReasoningBlock(turnId, item);
  if (displayKind.startsWith("browser.")) return toFileBlock(turnId, item);
  if (kind === "file" || displayKind.startsWith("file.")) return toFileBlock(turnId, item);
  if (kind === "mcp" || displayKind.startsWith("mcp.")) return toMcpBlock(turnId, item);
  if (kind === "assistant" || kind === "assistant_message") return toReasoningBlock(turnId, item);
  return null;
}

function dedupeBlocks(blocks) {
  const seen = new Set();
  const result = [];
  const narrationKinds = new Set(["assistant-intent", "assistant-result", "reasoning-summary"]);
  for (const block of blocks) {
    if (!block || block.visibility === "hidden") continue;
    const normalizedBlockText = normalizeText(block.text);
    if (narrationKinds.has(block.kind) && normalizedBlockText) {
      const duplicateIndex = result.findIndex((existing) => (
        narrationKinds.has(existing.kind) &&
        areNearDuplicateNarrations(existing.text, normalizedBlockText)
      ));
      if (duplicateIndex >= 0) {
        if (shouldReplaceNarration(result[duplicateIndex], block)) result[duplicateIndex] = block;
        continue;
      }
    }
    const key = [
      block.kind,
      block.command || "",
      block.inputSummary || "",
      normalizedBlockText,
    ].join("\u0000");
    if (seen.has(key)) continue;
    seen.add(key);
    result.push(block);
  }
  return result;
}

function isFinalDuplicate(text, finalText) {
  const normalizedText = normalizeText(text);
  const normalizedFinal = normalizeText(finalText);
  return normalizedText && normalizedFinal && normalizedText === normalizedFinal;
}

function assistantBlocks(turnId, assistantMessages = [], finalText = "") {
  const visibleMessages = assistantMessages
    .map((message) => ({
      ...message,
      text: normalizeText(message?.text),
    }))
    .filter((message) => message.text && !isPureThinkingPlaceholder(message.text) && !isFinalDuplicate(message.text, finalText));
  if (visibleMessages.length === 0) return [];

  const [first, ...rest] = visibleMessages;
  const blocks = [{
    ...blockBase(turnId, "assistant-intent", first),
    text: first.text,
  }];
  for (const message of rest) {
    blocks.push({
      ...blockBase(turnId, "assistant-result", message),
      text: message.text,
    });
  }
  return blocks;
}

function headerText(status, elapsedLabel) {
  const suffix = elapsedLabel ? ` ${elapsedLabel}` : "";
  if (status === "aborted") return `已停止${suffix}`;
  if (status === "failed") return `失败${suffix}`;
  if (status === "completed") return `已处理${suffix}`;
  if (status === "blocked") return `等待确认${suffix}`;
  return `正在处理${suffix}`;
}

function normalizeStatus(input) {
  if (input.status) return input.status;
  if (input.failed) return "failed";
  if (input.active) return "running";
  return "completed";
}

function approvalBlock(turnId, approval) {
  if (!approval) return null;
  const command = commandFromItem(approval);
  const block = blockBase(turnId, "inline-approval", {
    ...approval,
    id: approval.id || approval.approvalId,
    displayKind: approval.displayKind || "approval.command",
    status: approval.status || "blocked",
    text: "等待确认",
  });
  return {
    ...block,
    text: "等待确认",
    command,
    reason: approval.reason || approval.summary || "",
    risk: approval.risk || "",
    targets: Array.isArray(approval.targets) ? approval.targets : [],
    approvalId: approval.id || approval.approvalId,
    approvalType: approval.type || approval.approvalType || "command",
  };
}

export function buildCodexProcessTranscript(input = {}) {
  const turnId = input.turnId || input.id || "turn";
  const status = normalizeStatus(input);
  const elapsedLabel = normalizeText(input.elapsedLabel || input.durationLabel);
  const header = {
    id: stableId(turnId, "header", status),
    turnId,
    kind: "header",
    displayKind: "turn.header",
    status,
    text: headerText(status, elapsedLabel),
    visibility: "primary",
    startedAt: input.startedAt || "",
    updatedAt: input.updatedAt || input.completedAt || "",
  };

  const processBlocks = (input.processItems || [])
    .map((item) => itemToBlock(turnId, item))
    .filter(Boolean);
  const assistant = assistantBlocks(turnId, input.assistantMessages || [], input.finalText || "");
  const approval = approvalBlock(turnId, input.approval);
  const liveHint = normalizeText(input.liveHint);
  const liveHintBlock = liveHint && !isPureThinkingPlaceholder(liveHint) && processBlocks.length
    ? {
        ...blockBase(turnId, "reasoning-summary", {
          id: "live-hint",
          displayKind: "runtime.live_hint",
          status,
          text: liveHint,
        }),
        displayKind: "runtime.live_hint",
        text: liveHint,
      }
    : null;
  const visibleProcessBlocks = dedupeBlocks([...assistant, liveHintBlock, ...processBlocks]);
  const showThinking = Boolean(input.modelRunning && status !== "completed" && status !== "failed");
  const finalText = normalizeText(input.finalText);
  const finalBlock = finalText
    ? {
        id: stableId(turnId, "final-answer", "final"),
        turnId,
        kind: "final-answer",
        displayKind: "assistant.final",
        status: status === "completed" ? "completed" : "running",
        text: input.finalText,
        visibility: "primary",
        startedAt: "",
        updatedAt: "",
      }
    : null;

  const blocks = dedupeBlocks([
    header,
    ...visibleProcessBlocks,
    approval,
    finalBlock,
  ]);

  return {
    turnId,
    status,
    active: Boolean(input.active),
    header,
    blocks,
    showThinking,
    collapsedByDefault: Boolean(input.collapsedByDefault),
  };
}
