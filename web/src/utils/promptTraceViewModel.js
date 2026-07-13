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
const ARTIFACT_ID_KEYS = ["artifactId", "artifact_id", "agentUiArtifactId", "agent_ui_artifact_id", "id"];
const LLM_REQUEST_ID_KEYS = ["llmRequestId", "llm_request_id", "modelCallId", "model_call_id", "requestId", "request_id"];
const TOOL_CALL_ID_KEYS = ["toolCallId", "tool_call_id", "callId", "call_id"];
const EVIDENCE_REF_KEYS = ["evidenceRef", "evidence_ref", "evidenceId", "evidence_id", "rawRef", "raw_ref"];
const CASE_ID_KEYS = ["caseId", "case_id", "incidentId", "incident_id"];
const CONTEXT_GOVERNANCE_EMPTY_TEXT = "暂无上下文治理事件";
const REDACTION_STATUS_LABELS = {
  redacted: "已脱敏",
  partial: "部分脱敏",
  failed: "脱敏失败",
  restricted: "权限受限",
  none: "未脱敏",
  raw: "未脱敏",
};
const DETAIL_EMPTY_TEXT = {
  systemPrompt: "暂无 system prompt",
  developerPrompt: "暂无 developer prompt",
  userPrompt: "暂无 user prompt",
  toolMessages: "暂无 tool messages",
  retrievalContext: "暂无 retrieval context",
  output: "暂无输出",
  error: "暂无错误",
  tokens: "暂无 token 信息",
  duration: "暂无耗时",
};
const SENSITIVE_VALUE = "[已脱敏]";

export function parsePromptTrace(input) {
  const warnings = [];
  const payload = parseInput(input, warnings);
  const isTraceV2 = payload.schemaVersion === "aiops.trace/v2";
  if (!isTraceV2) {
    warnings.push(warning("danger", "当前 PromptTrace UI 只支持 aiops.trace/v2 trace document。"));
  }
  const stepContext = isPlainObject(payload.stepContext) ? payload.stepContext : {};
  const modelInput = Array.isArray(stepContext.modelInput)
    ? stepContext.modelInput
    : Array.isArray(payload.modelInput)
      ? payload.modelInput
    : [];
  if (!Array.isArray(stepContext.modelInput) && !Array.isArray(payload.modelInput)) {
    warnings.push(warning("warning", "trace 中没有 modelInput[]，只能展示空输入。"));
  }

  const v2ToolSurface = isPlainObject(payload.toolSurface) ? payload.toolSurface : {};
  const visibleTools = Array.isArray(v2ToolSurface.modelVisibleTools)
    ? v2ToolSurface.modelVisibleTools.map(compactText).filter(Boolean)
    : [];
  if (!Array.isArray(v2ToolSurface.modelVisibleTools)) {
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
  const caseId = compactText(payload.caseId || payload.metadata?.["eval.caseId"] || payload.metadata?.caseId);
  const sessionId = compactText(payload.sessionId);
  const turnId = compactText(payload.turnId);

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

  const toolRegistryText = redactSensitiveText(compactText(prompt.tools) || layers.find((item) => item.promptLayer === "tool_index")?.content || "");
  const riskyTools = visibleTools.filter((tool) => RISKY_TOOLS.has(tool));
  const agentUiSources = buildAgentUiSources(payload, layers, { caseId, sessionId, turnId });
  const contextGovernance = buildContextGovernanceViewModel(payload);
  const toolSurface = buildToolSurfaceViewModel(payload, visibleTools);
  const specialInput = buildSpecialInputViewModel(payload);
  const providerRequest = isPlainObject(payload.providerRequest) ? payload.providerRequest : {};
  const rawPayloadRefs = Array.isArray(payload.rawPayloadRefs)
    ? payload.rawPayloadRefs.map((ref) => ({
      id: compactText(ref?.id),
      kind: compactText(ref?.kind),
      path: compactText(ref?.path),
      sha256: compactText(ref?.sha256),
      bytes: numberOrZero(ref?.bytes),
    })).filter((ref) => ref.id || ref.path)
    : [];

  return {
    raw: redactSensitiveValue(payload),
    summary: {
      schemaVersion: payload.schemaVersion ?? null,
      kind: compactText(payload.kind),
      caseId,
      sessionId,
      turnId,
      iteration: Number.isFinite(Number(payload.iteration)) ? Number(payload.iteration) : null,
      createdAt: compactText(payload.createdAt),
      messageCount: layers.length,
      visibleToolCount: visibleTools.length,
      promptCharCount,
      toolRegistryCharCount: toolRegistryText.length,
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
      surface: toolSurface,
    },
    toolSurface,
    providerRequest,
    rawPayloadRefs,
    agentUiSources,
    agentUiArtifacts: agentUiSources.flatArtifacts,
    contextGovernance,
    specialInput,
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

export function redactSensitiveText(value = "") {
  let text = compactText(value);
  if (!text) return "";
  text = text.replace(/([a-z][a-z0-9+.-]*:\/\/)([^@\s,;]*?:)([^@\s,;]+)(@)/gi, `$1$2${SENSITIVE_VALUE}$4`);
  text = text.replace(/(request\s*body\s*[:=]\s*)(\{[\s\S]*?\}|\[[\s\S]*?\]|"[^"]*"|'[^']*'|\S+)/gi, `$1${SENSITIVE_VALUE}`);
  text = text.replace(/((?:api[\s_-]*key|token|password|secret|cookie|authorization)\s*[:=]\s*)(["']?)[^\s,;}\]"']+/gi, `$1${SENSITIVE_VALUE}`);
  text = text.replace(/(["'](?:api[\s_-]*key|token|password|secret|cookie|authorization)["']\s*:\s*)(["'])(?:\\.|(?!\2).)*\2/gi, `$1$2${SENSITIVE_VALUE}$2`);
  text = text.replace(/(\\["'](?:api[\s_-]*key|token|password|secret|cookie|authorization)\\["']\s*:\s*\\["'])(?:\\.|[^\\])*?(\\["'])/gi, `$1${SENSITIVE_VALUE}$2`);
  return text;
}

function redactSensitiveValue(value) {
  if (typeof value === "string") return redactSensitiveText(value);
  if (Array.isArray(value)) return value.map(redactSensitiveValue);
  if (!isPlainObject(value)) return value;
  return Object.fromEntries(
    Object.entries(value).map(([key, entry]) => {
      if (isSensitiveKey(key)) return [key, SENSITIVE_VALUE];
      return [key, redactSensitiveValue(entry)];
    }),
  );
}

function isSensitiveKey(key = "") {
  return /^(?:api[_\s-]*key|token|password|secret|cookie|authorization|request[_\s-]*body)$/i.test(key);
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
  const providerRole = compactText(message.providerRole || message.role);
  const semanticRole = compactText(message.semanticRole || message.semantic_role || message.role);
  const promptLayer = compactText(message.promptLayer);
  const content = redactSensitiveText(compactText(message.content));
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

function buildAgentUiSources(payload = {}, layers = [], context = {}) {
  const toolCalls = collectToolCalls(payload);
  const toolCallsById = new Map(toolCalls.map((item) => [item.id, item]).filter(([id]) => id));
  const explicitLlmRequests = collectLlmRequests(payload, layers);
  const defaultLlmRequestId = firstText(
    pickFromSource(payload, LLM_REQUEST_ID_KEYS),
    pickFromSource(payload.metadata, LLM_REQUEST_ID_KEYS),
    Number.isFinite(Number(payload.iteration)) ? `iteration-${Number(payload.iteration)}` : "",
    "llm-request",
  );
  const flatArtifacts = collectAgentUiArtifacts(payload)
    .map((source, index) => normalizeAgentUiSourceArtifact(source, index, context, toolCallsById, defaultLlmRequestId))
    .filter((item) => item.artifactId);
  const userLayer = findLast(layers, (item) => item.providerRole === "user" || item.semanticRole === "user");
  const llmRequests = buildLlmRequests(flatArtifacts, toolCalls, explicitLlmRequests, payload, layers);
  const userRequests = llmRequests.length || userLayer
    ? [
        {
          id: context.turnId || userLayer?.id || "user-request",
          turnId: context.turnId || "",
          title: "用户请求",
          content: userLayer?.content || "",
          preview: userLayer?.preview || "",
          llmRequests,
        },
      ]
    : [];

  return {
    session: {
      id: context.sessionId || "",
      caseId: context.caseId || "",
    },
    summary: {
      artifactCount: flatArtifacts.length,
      userRequestCount: userRequests.length,
      llmRequestCount: llmRequests.length,
    },
    userRequests,
    flatArtifacts,
  };
}

function buildContextGovernanceViewModel(payload = {}) {
  const events = [
    ...collectionToRecords(payload.contextGovernance, "id"),
    ...collectionToRecords(payload.context_governance, "id"),
    ...collectionToRecords(payload.metadata?.contextGovernance, "id"),
    ...collectionToRecords(payload.metadata?.context_governance, "id"),
  ].map(normalizeContextGovernanceEvent);

  const budgetEvents = events.filter((event) => event.budgetItems.length > 0);
  const compactionEvents = events.filter((event) => {
    const kind = event.kind.toLowerCase();
    return kind.includes("compact") || event.compactedIds.length > 0 || event.droppedGroupIds.length > 0;
  });
  const materializationEvents = events.filter((event) => {
    const kind = event.kind.toLowerCase();
    return kind.includes("material") || kind.includes("spill") || kind.includes("externalize") || kind.includes("tool_result") || kind.includes("tool.result");
  });
  const externalReferences = events.flatMap((event) => event.referenceIds.map((referenceId) => ({
    id: `${event.id}-ref-${referenceId}`,
    referenceId,
    eventId: event.id,
    layer: event.layer,
    kind: event.kind,
    label: `${event.layer || "context"} / ${event.kind || "event"}`,
  })));
  const externalKnowledgeEvidence = normalizeWebLearnEvidence(firstCollection(payload.webLearnEvidence, payload.web_learn_evidence, payload.metadata?.webLearnEvidence, payload.metadata?.web_learn_evidence));
  const environmentContext = normalizeEnvironmentContext(payload);

  return {
    emptyText: CONTEXT_GOVERNANCE_EMPTY_TEXT,
    events,
    summary: {
      eventCount: events.length,
      budgetEventCount: budgetEvents.length,
      compactionEventCount: compactionEvents.length,
      materializationEventCount: materializationEvents.length,
      externalReferenceCount: externalReferences.length,
      externalKnowledgeCount: externalKnowledgeEvidence.length,
      hasEnvironmentContext: Boolean(environmentContext),
      hasCompaction: compactionEvents.length > 0,
      hasMaterialization: materializationEvents.length > 0,
      hasExternalReferences: externalReferences.length > 0,
      hasExternalKnowledge: externalKnowledgeEvidence.length > 0,
    },
    budgetEvents,
    compactionEvents,
    materializationEvents,
    externalReferences,
    externalKnowledgeEvidence,
    environmentContext,
  };
}

function normalizeWebLearnEvidence(value) {
  return collectionToRecords(value, "id").map((entry, index) => {
    const id = redactSensitiveText(firstText(entry.id, entry.ID, `weblearn-${index + 1}`));
    const sourceUrl = redactSensitiveText(firstText(entry.sourceURL, entry.sourceUrl, entry.source_url, entry.url));
    return {
      id,
      kind: redactSensitiveText(firstText(entry.kind, entry.Kind, "external_knowledge")),
      query: redactSensitiveText(firstText(entry.query, entry.Query)),
      sourceTitle: redactSensitiveText(firstText(entry.sourceTitle, entry.source_title, entry.title)),
      sourceURL: sourceUrl,
      sourceKind: redactSensitiveText(firstText(entry.sourceKind, entry.source_kind)),
      product: redactSensitiveText(firstText(entry.product)),
      version: redactSensitiveText(firstText(entry.version)),
      applicability: redactSensitiveText(firstText(entry.applicability)),
      confidence: redactSensitiveText(firstText(entry.confidence)),
      relevantExcerpt: redactSensitiveText(firstText(entry.relevantExcerpt, entry.relevant_excerpt, entry.excerpt)),
      retrievedAt: redactSensitiveText(firstText(entry.retrievedAt, entry.retrieved_at)),
    };
  }).filter((entry) => entry.id || entry.query || entry.sourceTitle || entry.sourceURL || entry.relevantExcerpt);
}

function normalizeEnvironmentContext(payload = {}) {
  const metadata = isPlainObject(payload.metadata) ? payload.metadata : {};
  const source = firstObject(
    payload.environmentContext,
    payload.environment_context,
    metadata.environmentContext,
    metadata.environment_context,
  );
  const targetRefs = collectMetadataStringList(
    source.targetRefs,
    source.target_refs,
    metadata["aiops.target.refs"],
    metadata["aiops.tool.targetRefs"],
  );
  const compactContext = redactSensitiveText(firstText(
    source.compactContext,
    source.compact_context,
    metadata["aiops.env.compactContext"],
    metadata["aiops.env.context"],
  ));
  const readOnlyReason = redactSensitiveText(firstText(
    source.readOnlyReason,
    source.read_only_reason,
    metadata["aiops.env.readOnlyReason"],
  ));
  const hasConflict = Boolean(source.hasConflict || source.has_conflict)
    || Boolean(readOnlyReason)
    || /ConflictFacts|target_conflict|conflict/i.test(compactContext);
  if (!targetRefs.length && !compactContext && !readOnlyReason && !hasConflict) return null;
  return {
    targetRefs,
    compactContext,
    readOnlyReason,
    hasConflict,
  };
}

function collectMetadataStringList(...values) {
  const out = [];
  for (const value of values) {
    const entries = Array.isArray(value)
      ? value
      : compactText(value).split(/[,\n\r\t]+/);
    for (const entry of entries) {
      const text = redactSensitiveText(compactText(entry));
      if (text && !out.includes(text)) out.push(text);
    }
  }
  return out;
}

function buildToolSurfaceViewModel(payload = {}, visibleTools = []) {
  const v2Surface = isPlainObject(payload.toolSurface) ? payload.toolSurface : {};
  const top = isPlainObject(payload.toolSurfaceTrace) ? payload.toolSurfaceTrace : {};
  const promptTrace = isPlainObject(payload.promptInputTrace) ? payload.promptInputTrace : {};
  const modelVisibleTools = collectStringList(v2Surface.modelVisibleTools);
  const dispatchableTools = collectStringList(v2Surface.dispatchableTools);
  const hiddenReasons = isPlainObject(v2Surface.hiddenReasons) ? v2Surface.hiddenReasons : {};
  const loadedTools = collectStringList(top.loadedTools, payload.loadedToolsDelta, promptTrace.loadedToolsDelta);
  const loadedPacks = collectStringList(top.loadedPacks, payload.loadedPacksDelta, promptTrace.loadedPacksDelta);
  const selectionEvents = [
    ...collectionToRecords(payload.toolSelectionEvents, "id"),
    ...collectionToRecords(promptTrace.toolSelectionEvents, "id"),
  ];
  const selectedTools = collectStringList(
    top.selectedTools,
    loadedTools,
    ...selectionEvents.map((event) => event.loadedTools),
  );
  const initialTools = collectStringList(top.initialTools);
  const effectiveInitialTools = initialTools.length ? initialTools : deriveInitialTools(modelVisibleTools.length ? modelVisibleTools : visibleTools, selectedTools);
  const deferredFamilies = normalizeDeferredFamilies(firstCollection(top.deferredFamilies, promptTrace.deferredToolDirectory));
  const filteredTools = normalizeFilteredTools(firstCollection(top.filteredTools), selectionEvents);
  const mcpHealth = normalizeMcpHealth(top.mcpHealth, deferredFamilies);
  const toolSearchEvents = normalizeToolSearchEvents(firstCollection(top.toolSearchEvents, payload.toolSearchEvents, promptTrace.toolSearchEvents));
  const rejectedToolReasons = [
    ...normalizeRejectedToolReasons(firstCollection(top.rejectedToolReasons, payload.rejectedToolCalls, promptTrace.rejectedToolCalls)),
    ...toolSearchEvents.flatMap((event) => event.rejectedReasons.map((reason) => ({
      ...reason,
      errorType: reason.reason || reason.filteredReason,
      requiredAction: reason.healthStatus,
      source: "tool_search",
    }))),
  ];

  return {
    summary: {
      initialToolCount: effectiveInitialTools.length,
      baseRegistryCount: numberOrZero(top.baseRegistryCount) || effectiveInitialTools.length,
      deferredFamilyCount: deferredFamilies.length,
      loadedToolCount: loadedTools.length,
      loadedPackCount: loadedPacks.length,
      filteredToolCount: filteredTools.length,
      mcpHealthCount: mcpHealth.length,
      toolSearchEventCount: toolSearchEvents.length,
      selectedToolCount: selectedTools.length,
      rejectedToolReasonCount: rejectedToolReasons.length,
      visibleCount: modelVisibleTools.length || visibleTools.length,
      dispatchableCount: dispatchableTools.length,
      hiddenCount: Object.keys(hiddenReasons).length,
    },
    visible: modelVisibleTools.length ? modelVisibleTools : collectStringList(visibleTools),
    dispatchable: dispatchableTools,
    hiddenReasons,
    initialTools: effectiveInitialTools,
    deferredFamilies,
    loadedTools,
    loadedPacks,
    filteredTools,
    mcpHealth,
    toolSearchEvents,
    selectedTools,
    rejectedToolReasons,
  };
}

function buildSpecialInputViewModel(payload = {}) {
  const promptTrace = isPlainObject(payload.promptInputTrace) ? payload.promptInputTrace : {};
  const state = firstObject(promptTrace.specialInputWorldState, payload.specialInputWorldState);
  if (!isPlainObject(state) || Object.keys(state).length === 0) return null;
  const activeGrant = firstObject(state.activeExecutionScope, state.activeGrant);
  const roleBindings = collectionToRecords(firstCollection(state.activeRoleBindings, state.roleBindings), "id")
    .map((binding) => ({
      id: redactSensitiveText(firstText(binding.id, binding.bindingHash)),
      roleKey: redactSensitiveText(firstText(binding.roleKey, binding.role, binding.targetRole)),
      runtimeName: redactSensitiveText(firstText(binding.runtimeName, binding.runtime_name)),
      resourceKind: redactSensitiveText(firstText(binding.resourceKind, binding.resource_kind)),
      resourceId: redactSensitiveText(firstText(binding.resourceId, binding.resourceID, binding.resource_id)),
      environmentKey: redactSensitiveText(firstText(binding.environmentKey, binding.environment_key)),
      clusterKey: redactSensitiveText(firstText(binding.clusterKey, binding.cluster_key)),
      bindingHash: redactSensitiveText(firstText(binding.bindingHash, binding.binding_hash)),
    }))
    .filter((binding) => binding.roleKey || binding.resourceId || binding.bindingHash);
  const pendingConfirmations = collectionToRecords(state.pendingConfirmations, "id")
    .map((pending) => ({
      id: redactSensitiveText(firstText(pending.id)),
      kind: redactSensitiveText(firstText(pending.kind)),
      reason: redactSensitiveText(firstText(pending.reason)),
      roleKey: redactSensitiveText(firstText(pending.roleKey, pending.role_key)),
      candidateIds: collectStringList(pending.candidateIds, pending.candidate_ids),
    }))
    .filter((pending) => pending.id || pending.reason);
  const conflicts = collectionToRecords(state.conflicts, "id")
    .map((conflict) => ({
      id: redactSensitiveText(firstText(conflict.id, conflict.traceHash, conflict.trace_hash)),
      kind: redactSensitiveText(firstText(conflict.kind)),
      roleKey: redactSensitiveText(firstText(conflict.roleKey, conflict.role_key)),
      environmentKey: redactSensitiveText(firstText(conflict.environmentKey, conflict.environment_key)),
      clusterKey: redactSensitiveText(firstText(conflict.clusterKey, conflict.cluster_key)),
      resourceIds: collectStringList(conflict.resourceIds, conflict.resource_ids),
      reasons: collectStringList(conflict.reasons),
    }))
    .filter((conflict) => conflict.id || conflict.roleKey || conflict.reasons.length);
  const readPlan = firstObject(state.readPlan);

  return {
    schemaVersion: redactSensitiveText(firstText(state.schemaVersion)),
    turnId: redactSensitiveText(firstText(state.turnId, state.turn_id)),
    modelSummary: redactSensitiveText(firstText(state.modelSummary, state.summary)),
    activeGrant: isPlainObject(activeGrant) && Object.keys(activeGrant).length ? {
      id: redactSensitiveText(firstText(activeGrant.id)),
      resourceKind: redactSensitiveText(firstText(activeGrant.resourceKind, activeGrant.resource_kind)),
      resourceId: redactSensitiveText(firstText(activeGrant.resourceId, activeGrant.resourceID, activeGrant.resource_id)),
      display: redactSensitiveText(firstText(activeGrant.display)),
      allowedActions: collectStringList(activeGrant.allowedActions, activeGrant.allowed_actions),
      validationHash: redactSensitiveText(firstText(activeGrant.validationHash, activeGrant.validation_hash)),
      status: redactSensitiveText(firstText(activeGrant.status)),
      trustLevel: redactSensitiveText(firstText(activeGrant.trustLevel, activeGrant.trust_level)),
    } : null,
    roleBindings,
    pendingConfirmations,
    conflicts,
    readPlan: isPlainObject(readPlan) && Object.keys(readPlan).length ? {
      activeGrantId: redactSensitiveText(firstText(readPlan.activeGrantId, readPlan.active_grant_id)),
      activeResourceKind: redactSensitiveText(firstText(readPlan.activeResourceKind, readPlan.active_resource_kind)),
      activeResourceId: redactSensitiveText(firstText(readPlan.activeResourceId, readPlan.active_resource_id)),
      visibleFactIds: collectStringList(readPlan.visibleFactIds, readPlan.visible_fact_ids),
      candidateFactIds: collectStringList(readPlan.candidateFactIds, readPlan.candidate_fact_ids),
      roleBindingHashes: collectStringList(readPlan.roleBindingHashes, readPlan.role_binding_hashes),
      pendingConfirmationIds: collectStringList(readPlan.pendingConfirmationIds, readPlan.pending_confirmation_ids),
    } : null,
    summary: {
      hasActiveGrant: isPlainObject(activeGrant) && Object.keys(activeGrant).length > 0,
      roleBindingCount: roleBindings.length,
      pendingConfirmationCount: pendingConfirmations.length,
      conflictCount: conflicts.length,
    },
  };
}

function deriveInitialTools(visibleTools = [], selectedTools = []) {
  const selected = new Set(selectedTools.map(compactText));
  return collectStringList(visibleTools).filter((tool) => !selected.has(tool));
}

function normalizeDeferredFamilies(value) {
  return collectionToRecords(value, "pack").map((entry) => ({
    pack: redactSensitiveText(firstText(entry.pack, entry.name)),
    capability: redactSensitiveText(firstText(entry.capability, entry.capabilityKind, entry.capability_kind)),
    source: redactSensitiveText(firstText(entry.source)),
    mcpServerId: redactSensitiveText(firstText(entry.mcpServerId, entry.mcp_server_id, entry.mcpServerID)),
    healthStatus: redactSensitiveText(firstText(entry.healthStatus, entry.health_status)),
    unavailableReason: redactSensitiveText(firstText(entry.unavailableReason, entry.unavailable_reason)),
    toolCount: numberOrZero(entry.toolCount || entry.tool_count),
    requiresHealth: Boolean(entry.requiresHealth || entry.requires_health),
    requiresApproval: Boolean(entry.requiresApproval || entry.requires_approval),
    requiresSelect: Boolean(entry.requiresSelect || entry.requires_select),
    resourceTypes: collectStringList(entry.resourceTypes, entry.resource_types),
    operationKinds: collectStringList(entry.operationKinds, entry.operation_kinds),
  })).filter((entry) => entry.pack || entry.capability || entry.mcpServerId);
}

function normalizeFilteredTools(directValue, selectionEvents = []) {
  const byTool = new Map();
  for (const entry of collectionToRecords(directValue, "toolName")) {
    const toolName = redactSensitiveText(firstText(entry.toolName, entry.tool_name, entry.name));
    if (!toolName) continue;
    byTool.set(toolName, {
      toolName,
      reason: redactSensitiveText(firstText(entry.reason, entry.errorType, entry.error_type)),
    });
  }
  for (const event of selectionEvents) {
    const reasons = isPlainObject(event.notLoadedReasons) ? event.notLoadedReasons : {};
    for (const name of collectStringList(event.notLoaded, event.not_loaded)) {
      if (!name || byTool.has(name)) continue;
      byTool.set(name, {
        toolName: name,
        reason: redactSensitiveText(firstText(reasons[name], event.reason)),
      });
    }
  }
  return Array.from(byTool.values());
}

function normalizeMcpHealth(value, deferredFamilies = []) {
  const byServer = new Map();
  if (isPlainObject(value)) {
    for (const [serverId, status] of Object.entries(value)) {
      const key = redactSensitiveText(serverId);
      if (!key) continue;
      byServer.set(key, {
        serverId: key,
        status: redactSensitiveText(compactText(status)),
      });
    }
  }
  for (const family of deferredFamilies) {
    if (!family.mcpServerId || byServer.has(family.mcpServerId)) continue;
    const status = family.healthStatus || (family.requiresHealth ? "unknown" : "");
    if (!status) continue;
    byServer.set(family.mcpServerId, {
      serverId: family.mcpServerId,
      status: redactSensitiveText(status),
    });
  }
  return Array.from(byServer.values()).sort((left, right) => left.serverId.localeCompare(right.serverId));
}

function normalizeToolSearchEvents(value) {
  return collectionToRecords(value, "id").map((event, index) => {
    const request = firstObject(event.request, event.searchRequest, event.search_request);
    const rejectedReasons = normalizeToolSearchRejectedReasons(firstCollection(event.rejectedReasons, event.rejected_reasons, event.rejected));
    const rejectedCount = numberOrZero(event.rejectedCount || event.rejected_count) || rejectedReasons.length;
    return {
      id: redactSensitiveText(firstText(event.id, `tool-search-${index + 1}`)),
      mode: redactSensitiveText(firstText(event.mode)),
      query: redactSensitiveText(firstText(event.query, request.query)),
      ranker: redactSensitiveText(firstText(event.ranker, request.ranker)),
      intent: redactSensitiveText(firstText(event.intent, request.intent)),
      targetRefs: collectStringList(event.targetRefs, event.target_refs, request.targetRefs, request.target_refs),
      requiredCaps: collectStringList(event.requiredCaps, event.required_caps, request.requiredCaps, request.required_caps),
      forbiddenCaps: collectStringList(event.forbiddenCaps, event.forbidden_caps, request.forbiddenCaps, request.forbidden_caps),
      riskLevel: redactSensitiveText(firstText(event.riskLevel, event.risk_level, request.riskLevel, request.risk_level)),
      environmentFacts: collectStringList(event.environmentFacts, event.environment_facts, request.environmentFacts, request.environment_facts),
      targetCompatibility: redactSensitiveText(firstText(event.targetCompatibility, event.target_compatibility)),
      riskDecision: redactSensitiveText(firstText(event.riskDecision, event.risk_decision)),
      matchReasons: collectStringList(event.matchReasons, event.match_reasons),
      matchCount: numberOrZero(event.matchCount || event.match_count),
      rejectedCount,
      matches: collectStringList(event.matches),
      mcpHealth: normalizeMcpHealth(firstObject(event.mcpHealth, event.mcp_health, request.mcpHealth, request.mcp_health)),
      rejectedReasons,
      reason: redactSensitiveText(firstText(event.reason)),
    };
  });
}

function normalizeToolSearchRejectedReasons(value) {
  return collectionToRecords(value, "toolName").map((entry) => ({
    toolName: redactSensitiveText(firstText(entry.toolName, entry.tool_name, entry.name)),
    reason: redactSensitiveText(firstText(entry.reason)),
    status: redactSensitiveText(firstText(entry.status)),
    source: redactSensitiveText(firstText(entry.source)),
    mcpServerId: redactSensitiveText(firstText(entry.mcpServerId, entry.mcp_server_id, entry.mcpServerID)),
    healthStatus: redactSensitiveText(firstText(entry.healthStatus, entry.health_status)),
    filteredReason: redactSensitiveText(firstText(entry.filteredReason, entry.filtered_reason)),
  })).filter((entry) => entry.toolName || entry.reason || entry.filteredReason);
}

function normalizeRejectedToolReasons(value) {
  return collectionToRecords(value, "toolName").map((entry) => ({
    toolName: redactSensitiveText(firstText(entry.toolName, entry.tool_name, entry.name)),
    errorType: redactSensitiveText(firstText(entry.errorType, entry.error_type)),
    reason: redactSensitiveText(firstText(entry.reason)),
    requiredAction: redactSensitiveText(firstText(entry.requiredAction, entry.required_action)),
  })).filter((entry) => entry.toolName || entry.reason || entry.errorType);
}

function firstCollection(...values) {
  for (const value of values) {
    if (Array.isArray(value) && value.length > 0) return value;
    if (isPlainObject(value) && Object.keys(value).length > 0) return value;
  }
  return [];
}

function normalizeContextGovernanceEvent(source, index) {
  const metadata = metadataFor(source);
  const id = redactSensitiveText(firstText(source.id, metadata.id, `context-governance-${index + 1}`));
  const layer = redactSensitiveText(firstText(source.layer, metadata.layer));
  const kind = redactSensitiveText(firstText(source.kind, source.type, metadata.kind, metadata.type));
  const message = redactSensitiveText(firstText(source.message, source.summary, metadata.message, metadata.summary));
  const toolCallId = redactSensitiveText(firstText(source.toolCallId, source.tool_call_id, metadata.toolCallId, metadata.tool_call_id));
  const toolName = redactSensitiveText(firstText(source.toolName, source.tool_name, metadata.toolName, metadata.tool_name));
  const materializationTier = redactSensitiveText(firstText(
    source.materializationTier,
    source.materialization_tier,
    source.tier,
    metadata.materializationTier,
    metadata.materialization_tier,
    metadata.tier,
  ));
  const originalBytes = numberOrZero(firstText(source.originalBytes, source.original_bytes, metadata.originalBytes, metadata.original_bytes));
  const inlineBytes = numberOrZero(firstText(source.inlineBytes, source.inline_bytes, metadata.inlineBytes, metadata.inline_bytes));
  const retryAttempt = numberOrZero(firstText(source.retryAttempt, source.retry_attempt, metadata.retryAttempt, metadata.retry_attempt));
  const retryMax = numberOrZero(firstText(source.retryMax, source.retry_max, metadata.retryMax, metadata.retry_max));
  const referenceIds = collectStringList(source.referenceIds, source.reference_ids, source.references, source.refs, metadata.referenceIds, metadata.reference_ids);
  const compactedIds = collectStringList(source.compactedIds, source.compacted_ids, metadata.compactedIds, metadata.compacted_ids);
  const droppedGroupIds = collectStringList(source.droppedGroupIds, source.dropped_group_ids, metadata.droppedGroupIds, metadata.dropped_group_ids);
  const budgetItems = normalizeBudgetItems(firstObject(source.budget, metadata.budget));

  return {
    id,
    layer,
    kind,
    message,
    toolCallId,
    toolName,
    materializationTier,
    originalBytes,
    inlineBytes,
    createdAt: redactSensitiveText(firstText(source.createdAt, source.created_at, metadata.createdAt, metadata.created_at)),
    timeout: Boolean(source.timeout || metadata.timeout),
    retryAttempt,
    retryMax,
    retryLabel: retryAttempt || retryMax ? `${retryAttempt || 0}/${retryMax || 0}` : "",
    budgetItems,
    referenceIds,
    compactedIds,
    droppedGroupIds,
    hasCompaction: kind.toLowerCase().includes("compact") || compactedIds.length > 0 || droppedGroupIds.length > 0,
    hasMaterialization: /material|spill|externalize|tool[._-]?result/.test(kind.toLowerCase()),
    raw: redactSensitiveValue(source),
  };
}

function normalizeBudgetItems(budget = {}) {
  if (!isPlainObject(budget)) return [];
  return Object.entries(budget)
    .map(([key, value]) => ({
      key: redactSensitiveText(key),
      label: budgetLabel(key),
      value: Number.isFinite(Number(value)) ? Number(value) : compactText(value),
    }))
    .filter((item) => item.key)
    .sort((left, right) => left.key.localeCompare(right.key));
}

function budgetLabel(key = "") {
  const labels = {
    maxContextTokens: "Max Context",
    reservedOutputTokens: "Reserved Output",
    effectiveContextWindow: "Effective Window",
    warningThreshold: "Warning",
    autoCompactThreshold: "Auto Compact",
    blockingLimit: "Blocking Limit",
    smallContextMode: "Small Context",
  };
  return labels[key] || key;
}

function collectStringList(...values) {
  const out = [];
  for (const value of values) {
    const entries = Array.isArray(value) ? value : compactText(value) ? [value] : [];
    for (const entry of entries) {
      const text = redactSensitiveText(compactText(entry));
      if (text && !out.includes(text)) out.push(text);
    }
  }
  return out;
}

function numberOrZero(value) {
  const number = Number(value);
  return Number.isFinite(number) ? number : 0;
}

function firstFiniteNumber(...values) {
  for (const value of values) {
    const text = compactText(value);
    if (!text && value !== 0) continue;
    const number = Number(value);
    if (Number.isFinite(number)) return number;
  }
  return 0;
}

function findLast(items, predicate) {
  for (let index = items.length - 1; index >= 0; index -= 1) {
    if (predicate(items[index])) return items[index];
  }
  return null;
}

function collectAgentUiArtifacts(payload = {}) {
  const byId = new Map();
  for (const entry of [
    ...collectionToRecords(payload.artifacts, "artifact_id"),
    ...collectionToRecords(payload.metadata?.artifacts, "artifact_id"),
    ...collectionToRecords(payload.agentUiArtifacts, "artifact_id"),
    ...collectionToRecords(payload.metadata?.agentUiArtifacts, "artifact_id"),
  ]) {
    const artifactId = pickFromSource(entry, ARTIFACT_ID_KEYS);
    if (!artifactId) continue;
    const current = byId.get(artifactId) || {};
    byId.set(artifactId, mergeArtifactRecord(current, entry));
  }
  return Array.from(byId.values());
}

function mergeArtifactRecord(current, next) {
  const currentMetadata = metadataFor(current);
  const nextMetadata = metadataFor(next);
  return {
    ...current,
    ...next,
    metadata: {
      ...currentMetadata,
      ...nextMetadata,
    },
  };
}

function normalizeAgentUiSourceArtifact(source, index, context, toolCallsById, defaultLlmRequestId) {
  const artifactId = pickFromSource(source, ARTIFACT_ID_KEYS) || `agent-ui-artifact-${index + 1}`;
  const toolCallId = pickFromSource(source, TOOL_CALL_ID_KEYS);
  const toolCall = toolCallId ? toolCallsById.get(toolCallId) : null;
  const llmRequestId = firstText(pickFromSource(source, LLM_REQUEST_ID_KEYS), toolCall?.llmRequestId, defaultLlmRequestId);
  const type = redactSensitiveText(firstText(pickFromSource(source, ["type", "kind", "artifactType", "artifact_type"]), "agent_ui_artifact"));
  const title = redactSensitiveText(firstText(pickFromSource(source, ["titleZh", "title", "name"]), artifactId));
  const evidenceRef = redactSensitiveText(firstText(pickFromSource(source, EVIDENCE_REF_KEYS), firstArrayText(source.evidenceRefs), firstArrayText(source.evidence_refs)));
  const caseId = firstText(pickFromSource(source, CASE_ID_KEYS), context.caseId);
  const redactionStatus = firstText(pickFromSource(source, ["redactionStatus", "redaction_status", "redacted"]));

  return {
    id: `agent-ui-source-${artifactId}`,
    artifactId,
    type,
    title,
    llmRequestId,
    toolCallId,
    evidenceRef,
    caseId,
    redactionStatus,
    redactionStatusLabel: redactionStatusLabel(redactionStatus),
    generatedBy: toolCall
      ? {
          kind: "tool_call",
          id: toolCall.id,
          name: toolCall.name,
          label: `工具调用 ${toolCall.name || toolCall.id}`,
          llmRequestId,
        }
      : {
          kind: "llm_request",
          id: llmRequestId,
          name: "",
          label: `LLMRequest ${llmRequestId}`,
          llmRequestId,
        },
    raw: redactSensitiveValue(source),
  };
}

function collectLlmRequests(payload = {}, layers = []) {
  const records = [
    ...collectionToRecords(payload.llmRequests, "id"),
    ...collectionToRecords(payload.llm_requests, "id"),
    ...collectionToRecords(payload.modelRequests, "id"),
    ...collectionToRecords(payload.model_requests, "id"),
    ...collectionToRecords(payload.metadata?.llmRequests, "id"),
    ...collectionToRecords(payload.metadata?.llm_requests, "id"),
  ];

  return records.map((record, index) => normalizeLlmRequest(record, index, payload, layers));
}

function normalizeLlmRequest(source, index, payload, layers) {
  const metadata = metadataFor(source);
  const requestBody = firstObject(source.requestBody, source.request_body, source.body, source.input, metadata.requestBody, metadata.request_body);
  const messages = collectRequestMessages(requestBody, source, payload);
  const id = firstText(pickFromSource(source, LLM_REQUEST_ID_KEYS), source.id, metadata.id, `llm-request-${index + 1}`);
  return {
    id,
    label: `LLMRequest ${id}`,
    detail: buildLlmRequestDetail(source, metadata, requestBody, messages, layers, payload),
  };
}

function collectRequestMessages(requestBody, source, payload) {
  const directMessages = [
    ...(Array.isArray(requestBody?.messages) ? requestBody.messages : []),
    ...(Array.isArray(source.messages) ? source.messages : []),
    ...(Array.isArray(source.modelInput) ? source.modelInput : []),
    ...(Array.isArray(payload.messages) ? payload.messages : []),
  ];
  return directMessages.filter(isPlainObject);
}

function buildLlmRequestDetail(source, metadata, requestBody, messages, layers, payload) {
  const systemPrompt = firstText(
    messagesForRole(messages, "system"),
    pickFromSource(source, ["systemPrompt", "system_prompt"]),
    pickFromSource(metadata, ["systemPrompt", "system_prompt"]),
    payload.prompt?.system,
    layerContent(layers, "system"),
  );
  const developerPrompt = firstText(
    messagesForRole(messages, "developer"),
    pickFromSource(source, ["developerPrompt", "developer_prompt"]),
    pickFromSource(metadata, ["developerPrompt", "developer_prompt"]),
    payload.prompt?.developer,
    layerContent(layers, "developer"),
  );
  const userPrompt = firstText(
    messagesForRole(messages, "user"),
    pickFromSource(source, ["userPrompt", "user_prompt", "prompt"]),
    pickFromSource(metadata, ["userPrompt", "user_prompt"]),
    layerContent(layers, "user"),
  );
  const toolMessages = firstText(
    messagesForRole(messages, "tool"),
    source.toolMessages,
    source.tool_messages,
    metadata.toolMessages,
    metadata.tool_messages,
    layers.filter((item) => item.providerRole === "tool" || item.semanticRole.includes("tool")).map((item) => item.content),
  );
  const retrievalContext = firstText(
    source.retrievalContext,
    source.retrieval_context,
    source.context,
    source.contexts,
    source.documents,
    metadata.retrievalContext,
    metadata.retrieval_context,
    payload.retrievalContext,
    payload.retrieval_context,
    layers.filter((item) => /retrieval|context|memory/.test(`${item.promptLayer} ${item.semanticRole}`.toLowerCase())).map((item) => item.content),
  );
  const output = firstText(
    source.output,
    source.response,
    source.completion,
    source.assistantOutput,
    source.assistant_output,
    metadata.output,
    payload.output,
    payload.response,
    layerContent(layers, "assistant"),
  );
  const hasOutput = Boolean(compactText(output));
  const error = firstText(source.error, source.errorMessage, source.error_message, metadata.error, payload.error, payload.errorMessage);
  const metrics = {
    durationMs: firstFiniteNumber(source.durationMs, source.duration_ms, source.latencyMs, source.latency_ms, source.elapsedMs, source.elapsed_ms, metadata.durationMs, metadata.duration_ms),
    firstDeltaMs: firstFiniteNumber(source.firstDeltaMs, source.first_delta_ms, source.firstTokenMs, source.first_token_ms, metadata.firstDeltaMs, metadata.first_delta_ms, payload.firstDeltaMs, payload.first_delta_ms),
    streamMs: firstFiniteNumber(source.streamMs, source.stream_ms, metadata.streamMs, metadata.stream_ms, payload.streamMs, payload.stream_ms),
    deltaCount: firstFiniteNumber(source.deltaCount, source.delta_count, metadata.deltaCount, metadata.delta_count, payload.deltaCount, payload.delta_count),
    outputChars: firstFiniteNumber(source.outputChars, source.output_chars, metadata.outputChars, metadata.output_chars, payload.outputChars, payload.output_chars),
  };
  const finishReason = redactSensitiveText(firstText(source.finishReason, source.finish_reason, metadata.finishReason, metadata.finish_reason, payload.finishReason, payload.finish_reason));
  const promptCharCount = layers.reduce((sum, item) => sum + item.charCount, 0) || promptObjectCharCount(isPlainObject(payload.prompt) ? payload.prompt : {});

  return {
    systemPrompt: redactOrEmpty(systemPrompt, DETAIL_EMPTY_TEXT.systemPrompt),
    developerPrompt: redactOrEmpty(developerPrompt, DETAIL_EMPTY_TEXT.developerPrompt),
    userPrompt: redactOrEmpty(userPrompt, DETAIL_EMPTY_TEXT.userPrompt),
    toolMessages: redactOrEmpty(toolMessages, DETAIL_EMPTY_TEXT.toolMessages),
    retrievalContext: redactOrEmpty(retrievalContext, DETAIL_EMPTY_TEXT.retrievalContext),
    output: redactOrEmpty(output, DETAIL_EMPTY_TEXT.output),
    hasOutput,
    error: redactOrEmpty(error, DETAIL_EMPTY_TEXT.error),
    tokens: formatUsage(source.usage || metadata.usage || payload.usage || requestBody?.usage),
    duration: formatDuration(firstText(source.durationMs, source.duration_ms, source.latencyMs, source.latency_ms, source.elapsedMs, source.elapsed_ms, metadata.durationMs, metadata.duration_ms)),
    finishReason: finishReason || "",
    metrics,
    slowCauses: buildLlmSlowCauses(metrics, promptCharCount),
  };
}

function buildLlmSlowCauses(metrics, promptCharCount) {
  const causes = [];
  if ((metrics.firstDeltaMs || 0) > 30000) causes.push({ id: "first_delta_slow", label: "首 token 慢" });
  if ((metrics.outputChars || 0) > 6000) causes.push({ id: "output_too_long", label: "输出过长" });
  if ((metrics.deltaCount || 0) > 1000) causes.push({ id: "stream_fragmented", label: "流式碎片过多" });
  if ((promptCharCount || 0) > 30000) causes.push({ id: "prompt_large", label: "提示内容偏大" });
  return causes;
}

function buildLlmRequests(artifacts, toolCalls, explicitLlmRequests, payload, layers) {
  const byRequest = new Map();
  for (const request of explicitLlmRequests) {
    byRequest.set(request.id, {
      id: request.id,
      label: request.label,
      detail: request.detail,
      toolCalls: [],
      generatedArtifacts: [],
    });
  }

  for (const artifact of artifacts) {
    const requestId = artifact.llmRequestId || "llm-request";
    if (!byRequest.has(requestId)) {
      byRequest.set(requestId, {
        id: requestId,
        label: `LLMRequest ${requestId}`,
        detail: buildLlmRequestDetail({}, {}, {}, [], layers, payload),
        toolCalls: [],
        generatedArtifacts: [],
      });
    }
    byRequest.get(requestId).generatedArtifacts.push(artifact);
  }

  for (const request of byRequest.values()) {
    const artifactToolCallIds = new Set(request.generatedArtifacts.map((item) => item.toolCallId).filter(Boolean));
    request.toolCalls = toolCalls.filter((toolCall) => {
      return toolCall.llmRequestId === request.id || artifactToolCallIds.has(toolCall.id);
    });
  }

  return Array.from(byRequest.values());
}

function collectToolCalls(payload = {}) {
  const records = [
    ...collectionToRecords(payload.toolCalls, "id"),
    ...collectionToRecords(payload.metadata?.toolCalls, "id"),
  ];

  const llmRequests = [
    ...collectionToRecords(payload.llmRequests, "id"),
    ...collectionToRecords(payload.llm_requests, "id"),
    ...collectionToRecords(payload.modelRequests, "id"),
    ...collectionToRecords(payload.model_requests, "id"),
    ...collectionToRecords(payload.metadata?.llmRequests, "id"),
    ...collectionToRecords(payload.metadata?.llm_requests, "id"),
  ];
  for (const llmRequest of llmRequests) {
    const llmRequestId = firstText(pickFromSource(llmRequest, LLM_REQUEST_ID_KEYS), llmRequest.id);
    for (const toolCall of [
      ...collectionToRecords(llmRequest.toolCalls, "id"),
      ...collectionToRecords(llmRequest.tool_calls, "id"),
    ]) {
      records.push({
        ...toolCall,
        llmRequestId: pickFromSource(toolCall, LLM_REQUEST_ID_KEYS) || llmRequestId,
      });
    }
  }

  for (const message of Array.isArray(payload.modelInput) ? payload.modelInput : []) {
    const messageLlmRequestId = pickFromSource(message, LLM_REQUEST_ID_KEYS);
    for (const toolCall of collectionToRecords(message.toolCalls, "id")) {
      records.push({
        ...toolCall,
        llmRequestId: pickFromSource(toolCall, LLM_REQUEST_ID_KEYS) || messageLlmRequestId,
      });
    }
  }

  const byId = new Map();
  records.forEach((record, index) => {
    const normalized = normalizeToolCall(record, index);
    if (!normalized.id) return;
    byId.set(normalized.id, { ...(byId.get(normalized.id) || {}), ...normalized });
  });
  return Array.from(byId.values());
}

function normalizeToolCall(source, index) {
  const fn = isPlainObject(source.function) ? source.function : {};
  const metadata = metadataFor(source);
  const id = firstText(pickFromSource(source, ["id", "toolCallId", "tool_call_id", "callId", "call_id"]), `tool-call-${index + 1}`);
  const name = firstText(source.name, source.toolName, source.tool_name, fn.name, metadata.name, metadata.toolName, metadata.tool_name);
  const llmRequestId = pickFromSource(source, LLM_REQUEST_ID_KEYS);
  return {
    id,
    name,
    type: firstText(source.type, "function"),
    llmRequestId,
    arguments: redactSensitiveText(firstText(fn.arguments, source.arguments, source.args)),
  };
}

function messagesForRole(messages, role) {
  return messages
    .filter((message) => compactText(message.role || message.providerRole || message.semanticRole).toLowerCase() === role)
    .map((message) => message.content)
    .filter(Boolean);
}

function layerContent(layers, key) {
  const needle = key.toLowerCase();
  return layers
    .filter((item) => `${item.providerRole} ${item.semanticRole} ${item.promptLayer}`.toLowerCase().includes(needle))
    .map((item) => item.content);
}

function firstObject(...values) {
  return values.find(isPlainObject) || {};
}

function redactOrEmpty(value, emptyText) {
  const text = redactSensitiveText(compactText(value));
  return text || emptyText;
}

function formatUsage(usage) {
  if (!isPlainObject(usage)) return DETAIL_EMPTY_TEXT.tokens;
  const prompt = firstText(usage.prompt_tokens, usage.promptTokens, usage.input_tokens, usage.inputTokens);
  const completion = firstText(usage.completion_tokens, usage.completionTokens, usage.output_tokens, usage.outputTokens);
  const total = firstText(usage.total_tokens, usage.totalTokens, usage.total);
  if (!prompt && !completion && !total) return DETAIL_EMPTY_TEXT.tokens;
  return `prompt ${prompt || "-"} / completion ${completion || "-"} / total ${total || "-"}`;
}

function formatDuration(value) {
  if (!compactText(value)) return DETAIL_EMPTY_TEXT.duration;
  const number = Number(value);
  if (!Number.isFinite(number)) return DETAIL_EMPTY_TEXT.duration;
  return `${number} ms`;
}

function collectionToRecords(value, idKey) {
  if (Array.isArray(value)) {
    return value.filter(Boolean).map((item) => (isPlainObject(item) ? item : { value: item }));
  }
  if (isPlainObject(value)) {
    return Object.entries(value).map(([key, item]) => {
      if (isPlainObject(item)) {
        return { [idKey]: key, ...item };
      }
      return { [idKey]: key, value: item };
    });
  }
  return [];
}

function pickFromSource(source, keys) {
  if (!isPlainObject(source)) return "";
  const metadata = metadataFor(source);
  const generatedBy = isPlainObject(source.generatedBy)
    ? source.generatedBy
    : isPlainObject(metadata.generatedBy)
      ? metadata.generatedBy
      : {};

  for (const key of keys) {
    const value = firstText(source[key], metadata[key], generatedBy[key]);
    if (value) return value;
  }
  return "";
}

function metadataFor(source) {
  if (!isPlainObject(source)) return {};
  return {
    ...(isPlainObject(source.meta) ? source.meta : {}),
    ...(isPlainObject(source.metadata) ? source.metadata : {}),
  };
}

function firstArrayText(value) {
  return Array.isArray(value) ? firstText(...value) : "";
}

function firstText(...values) {
  for (const value of values) {
    if (Array.isArray(value) && value.length === 0) continue;
    const text = compactText(value);
    if (text) return text;
  }
  return "";
}

function redactionStatusLabel(status) {
  const key = compactText(status).toLowerCase();
  return REDACTION_STATUS_LABELS[key] || compactText(status);
}

function warning(severity, message, targetId) {
  return { severity, message, targetId: targetId || "" };
}

function isPlainObject(value) {
  return !!value && typeof value === "object" && !Array.isArray(value);
}
