const HASH_LABELS = [
  ["stableHash", "Stable"],
  ["systemHash", "System"],
  ["developerHash", "Developer"],
  ["toolRegistryHash", "Tools"],
  ["runtimePolicyHash", "Runtime Policy"],
  ["protocolStateHash", "Protocol State"],
];

const RISKY_TOOLS = new Set(["exec_command", "run_command", "shell", "terminal"]);
const PROMPT_SIZE_WARNING = 20_000;
const MESSAGE_SIZE_WARNING = 20_000;

export function parsePromptTrace(input) {
  const warnings = [];
  const payload = parseInput(input, warnings);
  const modelInput = Array.isArray(payload.modelInput) ? payload.modelInput : [];
  if (!Array.isArray(payload.modelInput)) {
    warnings.push(warning("warning", "trace 中没有 modelInput[]，只能展示空输入。"));
  }

  const visibleTools = Array.isArray(payload.visibleTools) ? payload.visibleTools.map(compactText).filter(Boolean) : [];
  if (!Array.isArray(payload.visibleTools)) {
    warnings.push(warning("info", "trace 中没有 visibleTools[]。"));
  }

  const prompt = isPlainObject(payload.prompt) ? payload.prompt : {};
  const promptFingerprint = isPlainObject(payload.promptFingerprint) ? payload.promptFingerprint : {};
  if (!isPlainObject(payload.promptFingerprint)) {
    warnings.push(warning("info", "trace 中没有 promptFingerprint。"));
  }

  const fingerprints = HASH_LABELS.map(([key, label]) => {
    const value = compactText(promptFingerprint[key]);
    return {
      key,
      label,
      value,
      shortValue: shortHash(value),
      missing: !value,
    };
  });

  const layers = modelInput
    .map((message, fallbackIndex) => buildLayer(message, fallbackIndex, promptFingerprint))
    .sort((left, right) => left.index - right.index);

  const roleCounts = countBy(layers, (item) => item.providerRole || "unknown");
  const layerCounts = countBy(layers, (item) => item.promptLayer || item.semanticRole || "unknown");
  const promptCharCount = layers.reduce((sum, item) => sum + item.charCount, 0) || promptObjectCharCount(prompt);
  const largestLayer = layers.reduce((largest, item) => (item.charCount > (largest?.charCount || 0) ? item : largest), null);
  const hasUserMessage = layers.some((item) => item.providerRole === "user" || item.semanticRole === "user");

  if (!hasUserMessage) {
    warnings.push(warning("warning", "本次模型输入中没有 user message。", layers[0]?.id));
  }
  if (visibleTools.length === 0) {
    warnings.push(warning("info", "本次模型调用没有 visible tools。"));
  }
  if (promptCharCount > PROMPT_SIZE_WARNING) {
    warnings.push(warning("warning", `本次 prompt 较大：${formatCount(promptCharCount)} chars。`, largestLayer?.id));
  }

  for (const layer of layers) {
    if (!layer.content) {
      layer.warnings.push("content 为空");
      warnings.push(warning("warning", `message ${formatIndex(layer.index)} content 为空。`, layer.id));
    }
    if (layer.charCount > MESSAGE_SIZE_WARNING) {
      layer.warnings.push(`content 较大：${formatCount(layer.charCount)} chars`);
    }
  }

  const toolRegistryText = compactText(prompt.tools) || layers.find((item) => item.promptLayer === "tool_index")?.content || "";
  const riskyTools = visibleTools.filter((tool) => RISKY_TOOLS.has(tool));

  return {
    raw: payload,
    summary: {
      schemaVersion: payload.schemaVersion ?? null,
      kind: compactText(payload.kind),
      caseId: compactText(payload.caseId || payload.metadata?.["eval.caseId"] || payload.metadata?.caseId),
      sessionId: compactText(payload.sessionId),
      turnId: compactText(payload.turnId),
      iteration: Number.isFinite(Number(payload.iteration)) ? Number(payload.iteration) : null,
      createdAt: compactText(payload.createdAt),
      messageCount: layers.length,
      visibleToolCount: visibleTools.length,
      promptCharCount,
      roleCounts,
      layerCounts,
      largestLayer,
      hasUserMessage,
    },
    fingerprints,
    layers,
    messages: layers,
    tools: {
      visible: visibleTools,
      risky: riskyTools,
      registryText: toolRegistryText,
      registryCharCount: toolRegistryText.length,
    },
    warnings,
  };
}

export function shortHash(value = "") {
  const text = compactText(value);
  if (!text) return "";
  if (text.length <= 16) return text;
  return `${text.slice(0, 8)}...${text.slice(-6)}`;
}

export function formatCount(value = 0) {
  const number = Number(value) || 0;
  return number.toLocaleString();
}

export function formatIndex(index) {
  const number = Number(index);
  if (!Number.isFinite(number)) return "--";
  return String(number).padStart(2, "0");
}

export function compactText(value) {
  if (value == null) return "";
  if (typeof value === "string") return value.trim();
  if (typeof value === "number" || typeof value === "boolean") return String(value);
  try {
    return JSON.stringify(value, null, 2).trim();
  } catch {
    return String(value).trim();
  }
}

function parseInput(input, warnings) {
  if (typeof input === "string") {
    const text = input.trim();
    if (!text) return {};
    try {
      return JSON.parse(text);
    } catch {
      warnings.push(warning("danger", "JSON trace 解析失败。"));
      return {};
    }
  }
  if (isPlainObject(input)) return input;
  if (input == null) return {};
  warnings.push(warning("warning", "trace 输入不是 JSON object。"));
  return {};
}

function buildLayer(message = {}, fallbackIndex, promptFingerprint = {}) {
  const index = Number.isFinite(Number(message.index)) ? Number(message.index) : fallbackIndex;
  const providerRole = compactText(message.providerRole);
  const semanticRole = compactText(message.semanticRole);
  const promptLayer = compactText(message.promptLayer);
  const content = compactText(message.content);
  const toolCalls = Array.isArray(message.toolCalls) ? message.toolCalls : [];
  const toolCallId = compactText(message.toolCallId);
  const hash = hashForLayer(promptLayer, semanticRole, promptFingerprint);
  return {
    id: `message-${formatIndex(index)}`,
    index,
    providerRole,
    semanticRole,
    promptLayer,
    title: layerTitle({ providerRole, semanticRole, promptLayer }),
    content,
    preview: previewText(content),
    charCount: content.length,
    lineCount: content ? content.split(/\r?\n/).length : 0,
    hash,
    shortHash: shortHash(hash),
    toolCalls,
    toolCallCount: toolCalls.length,
    toolCallId,
    warnings: [],
  };
}

function hashForLayer(promptLayer, semanticRole, fingerprint = {}) {
  const key = `${promptLayer || semanticRole}`.toLowerCase();
  if (key.includes("system")) return compactText(fingerprint.systemHash);
  if (key.includes("developer")) return compactText(fingerprint.developerHash);
  if (key.includes("tool")) return compactText(fingerprint.toolRegistryHash);
  if (key.includes("runtime_policy") || key.includes("policy")) return compactText(fingerprint.runtimePolicyHash);
  if (key.includes("protocol")) return compactText(fingerprint.protocolStateHash);
  return "";
}

function layerTitle({ providerRole, semanticRole, promptLayer }) {
  const key = `${promptLayer || semanticRole || providerRole}`.toLowerCase();
  if (key.includes("tool_index")) return "Tool Registry";
  if (key.includes("runtime_policy")) return "Runtime Policy";
  if (key.includes("protocol")) return "Protocol State";
  if (key.includes("developer")) return "Developer";
  if (key.includes("system")) return "System";
  if (key.includes("tool")) return "Tool Result";
  if (key.includes("memory")) return "Memory";
  if (key.includes("context")) return "Context";
  if (key.includes("user") || providerRole === "user") return "Conversation / User";
  if (providerRole === "assistant") return "Assistant";
  return "Message";
}

function previewText(content, maxLines = 12) {
  const lines = String(content || "").split(/\r?\n/);
  return lines.slice(0, maxLines).join("\n");
}

function countBy(items, keyFn) {
  return items.reduce((acc, item) => {
    const key = compactText(keyFn(item)) || "unknown";
    acc[key] = (acc[key] || 0) + 1;
    return acc;
  }, {});
}

function promptObjectCharCount(prompt = {}) {
  return ["system", "developer", "tools", "policy", "stable", "dynamic"].reduce((sum, key) => {
    return sum + compactText(prompt[key]).length;
  }, 0);
}

function warning(severity, message, targetId) {
  return { severity, message, targetId: targetId || "" };
}

function isPlainObject(value) {
  return !!value && typeof value === "object" && !Array.isArray(value);
}
