import { compactText } from "./workspaceViewModel";
import {
  buildMcpBundleSectionCards,
  buildMcpBundleSectionConfig,
  getMcpBundlePreset,
  getMcpBundlePresetRegistry,
  MCP_BUNDLE_PRESET_KEYS,
} from "./mcpBundlePresetRegistry";
import { normalizeMcpScope, normalizeMcpUiCards } from "./mcpUiCardModel";

function asArray(value) {
  return Array.isArray(value) ? value : [];
}

function asObject(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

function normalizeScopeString(value = "") {
  const text = compactText(value);
  if (!text) return {};

  const scope = {};
  const parts = text.split(/[,\s]+/).filter(Boolean);
  let slashFallback = null;

  for (const part of parts) {
    const match = part.match(/^([^:=]+)[:=](.+)$/);
    if (match) {
      const key = compactText(match[1]).toLowerCase().replace(/[\s-]+/g, "_");
      const rawValue = compactText(match[2]);
      if (["host", "host_id"].includes(key)) scope.hostId = rawValue;
      else if (["service"].includes(key)) scope.service = rawValue;
      else if (["cluster"].includes(key)) scope.cluster = rawValue;
      else if (["env", "environment"].includes(key)) scope.env = rawValue;
      else if (["time_range", "timerange"].includes(key)) scope.timeRange = rawValue;
      else if (["resource_type", "type"].includes(key)) scope.resourceType = rawValue;
      else if (["resource_id", "id"].includes(key)) scope.resourceId = rawValue;
      else scope.extras = { ...(scope.extras || {}), [key]: rawValue };
      continue;
    }
    if (!slashFallback && part.includes("/")) {
      slashFallback = part;
    }
  }

  if (slashFallback) {
    const slashParts = slashFallback.split("/").map((segment) => compactText(segment)).filter(Boolean);
    if (slashParts.length === 1) {
      scope.resourceType = scope.resourceType || slashParts[0];
    } else if (slashParts.length >= 2) {
      scope.resourceType = scope.resourceType || slashParts[0];
      scope.resourceId = scope.resourceId || slashParts.slice(1).join("/");
    }
  }

  if (Object.keys(scope).length) {
    return scope;
  }

  return {
    resourceType: text,
  };
}

export function normalizeMcpBundleScope(value = {}, defaults = {}) {
  if (typeof value === "string") {
    return normalizeMcpScope({
      ...asObject(defaults),
      ...normalizeScopeString(value),
    });
  }

  const source = asObject(value);
  const parsed = typeof source.rawScope === "string" ? normalizeScopeString(source.rawScope) : {};
  const merged = {
    ...asObject(defaults),
    ...parsed,
    ...source,
  };

  if (typeof merged.scope === "string") {
    Object.assign(merged, normalizeScopeString(merged.scope));
    delete merged.scope;
  }

  return normalizeMcpScope(merged);
}

function hasRemediationSignal(source = {}) {
  const sections = asArray(source.sections);
  return Boolean(
    compactText(source.rootCauseType || source.root_cause_type || source.rootCause?.type || source.root_cause?.type) ||
      compactText(source.rootCause || source.root_cause) ||
      sections.some((section) => compactText(section?.kind).toLowerCase() === "root_cause"),
  );
}

function resolveSubjectType(source = {}, scope = {}) {
  const subject = asObject(source.subject);
  return compactText(
    source.subjectType ||
      source.subject_type ||
      subject.type ||
      scope.resourceType ||
      scope.type ||
      "",
  ).toLowerCase();
}

export function resolveMcpBundlePresetKey(source = {}, defaults = {}) {
  const normalizedSource = asObject(source);
  const normalizedDefaults = asObject(defaults);
  const explicitBundleKind = compactText(normalizedSource.bundleKind || normalizedSource.bundle_kind || normalizedDefaults.bundleKind).toLowerCase();

  // Coroot-specific resolution FIRST: detect coroot source, mcpServer, or
  // toolName prefix so that generic bundleKind values like "monitor_bundle"
  // still route to the Coroot-specific presets when the data originates from
  // Coroot.
  const mcpServer = compactText(normalizedSource.mcpServer || normalizedSource.mcp_server || normalizedDefaults.mcpServer).toLowerCase();
  const payloadSource = compactText(normalizedSource.source || normalizedDefaults.source).toLowerCase();
  const toolName = compactText(normalizedSource.toolName || normalizedSource.tool_name || normalizedDefaults.toolName).toLowerCase();
  // Recognised Coroot toolName prefixes — used both for the isCoroot gate and
  // for finer-grained preset routing inside the Coroot branch.
  const COROOT_TOOL_PREFIXES = [
    "coroot.",           // generic catch-all  (e.g. coroot.list_services)
    "coroot.topology",   // service dependency topology
    "coroot.host_overview", // host overview / host metrics
    "coroot.service_overview", // service overview
    "coroot.alerts",     // alert queries
    "coroot.rca",        // root-cause analysis
    "coroot.metrics",    // raw metric queries
  ];

  const isCorootToolName = toolName.startsWith("coroot.") ||
    COROOT_TOOL_PREFIXES.some(
      (prefix) => toolName === prefix || toolName.startsWith(prefix),
    );

  const isCoroot =
    mcpServer.includes("coroot") ||
    payloadSource.includes("coroot") ||
    isCorootToolName ||
    explicitBundleKind === "coroot_service_monitor" ||
    explicitBundleKind === "coroot_incident_rca" ||
    explicitBundleKind === "coroot_host_overview" ||
    explicitBundleKind === "coroot_topology";

  if (isCoroot) {
    // RCA-specific routing
    if (
      hasRemediationSignal(normalizedSource) ||
      explicitBundleKind === "coroot_incident_rca" ||
      toolName.startsWith("coroot.rca")
    ) {
      return MCP_BUNDLE_PRESET_KEYS.COROOT_INCIDENT_RCA;
    }
    // Topology, host overview, service overview, and all other Coroot data
    // route to the Coroot service monitor preset (which includes topology section).
    return MCP_BUNDLE_PRESET_KEYS.COROOT_SERVICE_MONITOR;
  }

  // Generic explicit bundleKind fallback (non-Coroot payloads).
  if (explicitBundleKind === "remediation_bundle") {
    return MCP_BUNDLE_PRESET_KEYS.ROOT_CAUSE_REMEDIATION;
  }
  if (explicitBundleKind === "monitor_bundle") {
    return MCP_BUNDLE_PRESET_KEYS.MIDDLEWARE_SERVICE_MONITOR;
  }

  const scope = normalizeMcpBundleScope(normalizedSource.scope || normalizedDefaults.scope || {});
  const subjectType = resolveSubjectType(normalizedSource, scope);
  if (hasRemediationSignal(normalizedSource)) {
    return MCP_BUNDLE_PRESET_KEYS.ROOT_CAUSE_REMEDIATION;
  }
  if (["middleware", "service"].includes(subjectType) || compactText(scope.service)) {
    return MCP_BUNDLE_PRESET_KEYS.MIDDLEWARE_SERVICE_MONITOR;
  }
  return MCP_BUNDLE_PRESET_KEYS.MIDDLEWARE_SERVICE_MONITOR;
}

function findExplicitSectionCards(sectionKind, source = {}) {
  const section = asArray(source.sections).find((item) => compactText(item?.kind).toLowerCase() === compactText(sectionKind).toLowerCase());
  return normalizeMcpUiCards(section?.cards || []);
}

function buildSectionCardsFromPreset(preset, source, scope, sectionKind) {
  const explicitCards = findExplicitSectionCards(sectionKind, source);
  if (explicitCards.length) {
    return explicitCards;
  }

  const cardGroups = buildMcpBundleSectionCards(preset, source, scope);
  return normalizeMcpUiCards(cardGroups[sectionKind] || []);
}

export function buildMcpBundleCardCombos(preset = {}, source = {}, scope = {}) {
  const normalizedPreset = getMcpBundlePreset(preset?.key || preset?.bundleKind || preset?.presetKey || preset);
  return normalizedPreset.sectionKinds.map((kind, index) => {
    const cards = buildSectionCardsFromPreset(normalizedPreset, source, scope, kind);
    return {
      id: `${normalizedPreset.key}-${kind}-${index + 1}`,
      sectionKind: kind,
      cards,
      cardCount: cards.length,
    };
  });
}

export function buildMcpBundleSections(preset = {}, source = {}, scope = {}) {
  const normalizedPreset = getMcpBundlePreset(preset?.key || preset?.bundleKind || preset?.presetKey || preset);
  const sectionConfig = buildMcpBundleSectionConfig(normalizedPreset, source);
  const cardCombos = buildMcpBundleCardCombos(normalizedPreset, source, scope);
  const cardMap = new Map(cardCombos.map((combo) => [combo.sectionKind, combo.cards]));

  return sectionConfig.map((section) => ({
    ...section,
    cards: cardMap.get(section.kind) || [],
  }));
}

export function resolveMcpBundlePreset(source = {}, defaults = {}) {
  const normalizedSource = asObject(source);
  const normalizedDefaults = asObject(defaults);
  const scope = normalizeMcpBundleScope(normalizedSource.scope || normalizedSource.scope_hint || normalizedDefaults.scope || {});
  const subjectType = resolveSubjectType(normalizedSource, scope);
  const rootCauseType = compactText(
    normalizedSource.rootCauseType ||
      normalizedSource.root_cause_type ||
      normalizedSource.rootCause?.type ||
      normalizedSource.root_cause?.type ||
      "",
  ).toLowerCase();
  const presetKey = resolveMcpBundlePresetKey(
    {
      ...normalizedSource,
      scope,
      subjectType,
      rootCauseType,
    },
    normalizedDefaults,
  );
  const preset = getMcpBundlePreset(presetKey);
  const sectionConfig = buildMcpBundleSectionConfig(preset, normalizedSource);
  const cardCombos = buildMcpBundleCardCombos(preset, normalizedSource, scope);
  const sections = buildMcpBundleSections(preset, normalizedSource, scope);

  return {
    presetKey,
    bundleKind: preset.bundleKind,
    preset,
    scope,
    subjectType,
    rootCauseType,
    sectionConfig,
    cardCombos,
    sections,
  };
}

export {
  getMcpBundlePreset,
  getMcpBundlePresetRegistry,
  MCP_BUNDLE_PRESET_KEYS,
};

export default {
  normalizeMcpBundleScope,
  resolveMcpBundlePresetKey,
  resolveMcpBundlePreset,
  buildMcpBundleCardCombos,
  buildMcpBundleSections,
  getMcpBundlePreset,
  getMcpBundlePresetRegistry,
  MCP_BUNDLE_PRESET_KEYS,
};
