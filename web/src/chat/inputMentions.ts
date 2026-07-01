export const INPUT_MENTIONS_METADATA_KEY = "aiops.input.mentions.v1";

export type AiopsMentionKind =
  | "host"
  | "capability"
  | "ops_manual"
  | "ops_graph"
  | "file"
  | "unknown";

export type AiopsMentionSource =
  | "selection"
  | "typed_fallback"
  | "history_restore";

export type AiopsMentionRange = {
  start: number;
  end: number;
};

export type AiopsMentionBinding = {
  version: 1;
  tokenId: string;
  sigil: "@";
  display: string;
  rawText: string;
  kind: AiopsMentionKind;
  path: string;
  source: AiopsMentionSource;
  range: AiopsMentionRange;
  selectedAtMs?: number;
  payload?: Record<string, unknown>;
};

export type HostMentionBindingInput = {
  tokenId: string;
  rawText: string;
  range: AiopsMentionRange;
  hostId: string;
  address?: string;
  displayName?: string;
  status?: string;
  source?: AiopsMentionSource;
};

export type CapabilityMention = "coroot" | "ops_graph" | "ops_manuals";

export type CapabilityMentionBindingInput = {
  tokenId: string;
  rawText: string;
  range: AiopsMentionRange;
  capability: CapabilityMention;
  source?: AiopsMentionSource;
};

export type OpsManualMentionBindingInput = {
  tokenId: string;
  rawText: string;
  range: AiopsMentionRange;
  manualId: string;
  title: string;
  workflowId?: string;
  status?: string;
  source?: AiopsMentionSource;
};

export type OpsGraphMentionBindingInput = {
  tokenId: string;
  rawText: string;
  range: AiopsMentionRange;
  graphId: string;
  name: string;
  environment?: string;
  source?: AiopsMentionSource;
};

type InputMentionEnvelope = {
  version: 1;
  mentions: AiopsMentionBinding[];
};

export function buildHostMentionBinding(input: HostMentionBindingInput): AiopsMentionBinding {
  const hostId = cleanText(input.hostId);
  const rawText = cleanText(input.rawText);
  return {
    version: 1,
    tokenId: cleanText(input.tokenId),
    sigil: "@",
    display: rawText,
    rawText,
    kind: "host",
    path: `host://${encodeURIComponent(hostId)}`,
    source: input.source || "selection",
    range: normalizeRange(input.range),
    selectedAtMs: Date.now(),
    payload: {
      hostId,
      address: cleanText(input.address) || hostId,
      displayName: cleanText(input.displayName) || hostId,
      ...(cleanText(input.status) ? { status: cleanText(input.status) } : {}),
    },
  };
}

export function buildCapabilityMentionBinding(input: CapabilityMentionBindingInput): AiopsMentionBinding {
  const capability = normalizeCapability(input.capability);
  const rawText = cleanText(input.rawText);
  return {
    version: 1,
    tokenId: cleanText(input.tokenId),
    sigil: "@",
    display: rawText,
    rawText,
    kind: "capability",
    path: `capability://${capability}`,
    source: input.source || "selection",
    range: normalizeRange(input.range),
    selectedAtMs: Date.now(),
  };
}

export function buildOpsManualMentionBinding(input: OpsManualMentionBindingInput): AiopsMentionBinding {
  const manualId = cleanText(input.manualId);
  const rawText = cleanText(input.rawText);
  const title = cleanText(input.title) || manualId;
  const workflowId = cleanText(input.workflowId);
  const status = cleanText(input.status);
  return {
    version: 1,
    tokenId: cleanText(input.tokenId),
    sigil: "@",
    display: title || rawText,
    rawText,
    kind: "ops_manual",
    path: `ops-manual://${encodeURIComponent(manualId)}`,
    source: input.source || "selection",
    range: normalizeRange(input.range),
    selectedAtMs: Date.now(),
    payload: {
      manualId,
      title,
      ...(workflowId ? { workflowId } : {}),
      ...(status ? { status } : {}),
    },
  };
}

export function buildOpsGraphMentionBinding(input: OpsGraphMentionBindingInput): AiopsMentionBinding {
  const graphId = cleanText(input.graphId);
  const rawText = cleanText(input.rawText);
  const name = cleanText(input.name) || graphId;
  const environment = cleanText(input.environment);
  return {
    version: 1,
    tokenId: cleanText(input.tokenId),
    sigil: "@",
    display: name || rawText,
    rawText,
    kind: "ops_graph",
    path: `ops-graph://${encodeURIComponent(graphId)}`,
    source: input.source || "selection",
    range: normalizeRange(input.range),
    selectedAtMs: Date.now(),
    payload: {
      graphId,
      name,
      ...(environment ? { environment } : {}),
    },
  };
}

export function reconcileMentionBindings(
  text: string,
  bindings: AiopsMentionBinding[],
): AiopsMentionBinding[] {
  const currentText = String(text || "");
  return bindings.filter((binding) => {
    if (!binding || binding.version !== 1) return false;
    const range = normalizeRange(binding.range);
    if (range.start < 0 || range.end > currentText.length || range.end <= range.start) {
      return false;
    }
    return currentText.slice(range.start, range.end) === binding.rawText;
  });
}

export function buildInputMentionMetadata(bindings: AiopsMentionBinding[]): Record<string, string> {
  const normalized = bindings.filter((binding) => binding && binding.version === 1);
  if (normalized.length === 0) return {};
  const envelope: InputMentionEnvelope = { version: 1, mentions: normalized };
  return { [INPUT_MENTIONS_METADATA_KEY]: JSON.stringify(envelope) };
}

export function deriveHostMentionMetadata(bindings: AiopsMentionBinding[]): Record<string, string> {
  const hostMentions = bindings
    .filter((binding) => binding.kind === "host" && isStrongMentionSource(binding.source))
    .map((binding) => {
      const payload = objectPayload(binding.payload);
      const hostId = cleanText(payload.hostId) || decodeHostPath(binding.path);
      const address = cleanText(payload.address) || hostId;
      const displayName = cleanText(payload.displayName) || hostId;
      return {
        tokenId: binding.tokenId,
        raw: binding.rawText,
        value: hostId,
        start: binding.range.start,
        end: binding.range.end,
        hostId,
        address,
        displayName,
        source: "inventory",
        resolved: true,
        confidence: 1,
      };
    })
    .filter((mention) => mention.hostId);

  if (hostMentions.length === 0) return {};
  return {
    "aiops.hostops.mentions": JSON.stringify(hostMentions),
    "aiops.hostops.clientDetectedMultiHost": String(uniqueHostKeys(hostMentions).length >= 2),
  };
}

export function deriveCapabilityMentionMetadata(bindings: AiopsMentionBinding[]): Record<string, string> {
  const packs: string[] = [];
  const tools: string[] = [];
  const metadata: Record<string, string> = {};
  for (const binding of bindings) {
    if (!isStrongMentionSource(binding.source)) continue;
    if (binding.kind === "ops_graph") {
      const payload = objectPayload(binding.payload);
      const graphId = cleanText(payload.graphId) || decodeResourcePath(binding.path, "ops-graph://");
      const name = cleanText(payload.name) || graphId;
      packs.push("opsgraph");
      metadata["aiops.opsGraph.explicitMention"] = "true";
      if (graphId) metadata["aiops.opsGraph.graphId"] = graphId;
      if (name) metadata["aiops.opsGraph.graphName"] = name;
      continue;
    }
    if (binding.kind === "ops_manual") {
      const payload = objectPayload(binding.payload);
      const manualId = cleanText(payload.manualId) || decodeResourcePath(binding.path, "ops-manual://");
      const workflowId = cleanText(payload.workflowId);
      const title = cleanText(payload.title) || manualId;
      packs.push("ops_manual_flow");
      tools.push("search_ops_manuals");
      metadata["aiops.opsManuals.explicitMention"] = "true";
      if (manualId) {
        metadata.opsManualManualId = manualId;
        metadata.manualId = manualId;
      }
      if (workflowId) {
        metadata.opsManualWorkflowId = workflowId;
        metadata.workflowId = workflowId;
      }
      if (title) metadata.opsManualManualTitle = title;
      continue;
    }
    if (binding.kind !== "capability") continue;
    const capability = decodeCapabilityPath(binding.path);
    if (capability === "coroot") {
      metadata["aiops.coroot.explicitRCA"] = "true";
      metadata["aiops.coroot.rcaDisplayAllowed"] = "true";
    }
    if (capability === "ops_graph") {
      packs.push("opsgraph");
      metadata["aiops.opsGraph.explicitMention"] = "true";
    }
    if (capability === "ops_manuals") {
      packs.push("ops_manual_flow");
      tools.push("search_ops_manuals");
      metadata["aiops.opsManuals.explicitMention"] = "true";
    }
  }
  if (packs.length > 0) metadata.enableToolPack = uniqueValues(packs).join(",");
  if (tools.length > 0) metadata.enableTool = uniqueValues(tools).join(",");
  return metadata;
}

function normalizeRange(range: AiopsMentionRange): AiopsMentionRange {
  const start = Number.isFinite(range?.start) ? Math.max(0, Math.trunc(range.start)) : 0;
  const end = Number.isFinite(range?.end) ? Math.max(start, Math.trunc(range.end)) : start;
  return { start, end };
}

function normalizeCapability(value: CapabilityMention): CapabilityMention {
  if (value === "coroot" || value === "ops_graph" || value === "ops_manuals") {
    return value;
  }
  return "coroot";
}

function decodeHostPath(path: string) {
  const raw = cleanText(path).replace(/^host:\/\//, "");
  try {
    return decodeURIComponent(raw);
  } catch {
    return raw;
  }
}

function decodeCapabilityPath(path: string): CapabilityMention | "" {
  const raw = cleanText(path).replace(/^capability:\/\//, "");
  if (raw === "coroot" || raw === "ops_graph" || raw === "ops_manuals") return raw;
  return "";
}

function decodeResourcePath(path: string, prefix: string) {
  const raw = cleanText(path).replace(prefix, "");
  try {
    return decodeURIComponent(raw);
  } catch {
    return raw;
  }
}

function objectPayload(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};
}

function uniqueHostKeys(items: Array<{ hostId: string; address: string }>) {
  return uniqueValues(items.map((item) => cleanText(item.hostId) || cleanText(item.address)).filter(Boolean));
}

function uniqueValues(values: string[]) {
  return Array.from(new Set(values.map(cleanText).filter(Boolean)));
}

function isStrongMentionSource(source: AiopsMentionSource) {
  return source === "selection" || source === "history_restore";
}

function cleanText(value: unknown) {
  return typeof value === "string" ? value.trim() : "";
}
