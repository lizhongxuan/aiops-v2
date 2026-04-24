import { compactText } from "./workspaceViewModel";

export const MCP_UI_KINDS = Object.freeze([
  "readonly_summary",
  "readonly_chart",
  "action_panel",
  "form_panel",
  "topology_card",
]);

export const MCP_UI_PLACEMENTS = Object.freeze([
  "inline_final",
  "inline_action",
  "side_panel",
  "drawer",
  "modal",
]);

export const MCP_BUNDLE_KINDS = Object.freeze([
  "monitor_bundle",
  "remediation_bundle",
]);

export const MCP_PAYLOAD_SOURCES = Object.freeze([
  "mcp",
  "local",
  "remote",
  "host",
  "workspace",
]);

export const MCP_MONITOR_SECTION_KINDS = Object.freeze([
  "overview",
  "trends",
  "alerts",
  "changes",
  "dependencies",
]);

export const MCP_REMEDIATION_SECTION_KINDS = Object.freeze([
  "root_cause",
  "impact",
  "recommended_actions",
  "control_panels",
  "validation_panels",
]);

const UI_KIND_SET = new Set(MCP_UI_KINDS);
const PLACEMENT_SET = new Set(MCP_UI_PLACEMENTS);
const BUNDLE_KIND_SET = new Set(MCP_BUNDLE_KINDS);
const PAYLOAD_SOURCE_SET = new Set(MCP_PAYLOAD_SOURCES);

function asArray(value) {
  return Array.isArray(value) ? value : [];
}

function asObject(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

function normalizePositiveNumber(value, fallback = 0) {
  const numeric = Number(value);
  return Number.isFinite(numeric) && numeric >= 0 ? numeric : fallback;
}

function normalizeUiKind(value) {
  const normalized = compactText(value).toLowerCase();
  return UI_KIND_SET.has(normalized) ? normalized : "readonly_summary";
}

function normalizeSectionKind(bundleKind, value) {
  const normalized = compactText(value).toLowerCase();
  if (!normalized) return "";
  if (bundleKind === "monitor_bundle") {
    if (["topology", "dependency", "dependencies"].includes(normalized)) return "dependencies";
  }
  return normalized;
}

function defaultPlacementForUiKind(uiKind) {
  switch (uiKind) {
    case "action_panel":
    case "form_panel":
      return "inline_action";
    case "readonly_chart":
    case "readonly_summary":
    default:
      return "inline_final";
  }
}

function normalizePlacement(value, uiKind) {
  const normalized = compactText(value).toLowerCase();
  if (PLACEMENT_SET.has(normalized)) {
    return normalized;
  }
  return defaultPlacementForUiKind(uiKind);
}

function normalizeApprovalMode(value, mutation) {
  const normalized = compactText(value).toLowerCase();
  if (["auto", "none", "optional", "required"].includes(normalized)) {
    return normalized;
  }
  return mutation ? "required" : "none";
}

export function normalizeMcpPayloadSource(value, fallback = "mcp") {
  const normalized = compactText(value).toLowerCase().replace(/[\s-]+/g, "_");
  if (!normalized) return compactText(fallback || "mcp");
  if (PAYLOAD_SOURCE_SET.has(normalized)) return normalized;
  if (normalized.includes("workspace") || normalized.includes("protocol")) return "workspace";
  if (normalized.includes("host")) return "host";
  if (normalized.includes("remote")) return "remote";
  if (normalized.includes("local") || normalized.includes("desktop") || normalized.includes("client")) return "local";
  if (normalized.includes("mcp")) return "mcp";
  return compactText(value || fallback || "mcp");
}

export function normalizeMcpUiAction(action = {}, index = 0, uiKind = "readonly_summary") {
  const actionObject = asObject(action);
  const label = compactText(actionObject.label || actionObject.title || actionObject.name || actionObject.intent);
  const intent = compactText(actionObject.intent || actionObject.kind || actionObject.type || "open");
  const inferredMutation =
    typeof actionObject.mutation === "boolean"
      ? actionObject.mutation
      : ["action_panel", "form_panel"].includes(uiKind);

  return {
    id: compactText(actionObject.id || `${intent || "action"}-${index + 1}`),
    label: label || "未命名操作",
    intent: intent || "open",
    mutation: inferredMutation,
    approvalMode: normalizeApprovalMode(actionObject.approvalMode || actionObject.approval_mode, inferredMutation),
    confirmText: compactText(actionObject.confirmText || actionObject.confirm_text || ""),
    permissionPath: compactText(
      actionObject.permissionPath
        || actionObject.permission_path
        || actionObject.payloadSchema?.permissionPath
        || actionObject.payloadSchema?.permission_path
        || actionObject.payload_schema?.permissionPath
        || actionObject.payload_schema?.permission_path,
    ),
    payloadSchema: asObject(actionObject.payloadSchema || actionObject.payload_schema),
    target: asObject(actionObject.target),
    params: asObject(actionObject.params || actionObject.arguments),
    disabled: Boolean(actionObject.disabled),
    destructive: Boolean(actionObject.destructive),
  };
}

export function normalizeMcpFreshness(value = {}) {
  if (typeof value === "string") {
    return {
      capturedAt: "",
      ttlSec: 0,
      staleAt: "",
      label: compactText(value),
    };
  }
  const freshness = asObject(value);
  return {
    capturedAt: compactText(freshness.capturedAt || freshness.captured_at || freshness.updatedAt || freshness.updated_at),
    ttlSec: normalizePositiveNumber(freshness.ttlSec || freshness.ttl_sec),
    staleAt: compactText(freshness.staleAt || freshness.stale_at || ""),
    label: compactText(freshness.label || ""),
  };
}

export function normalizeMcpScope(value = {}) {
  if (typeof value === "string") {
    return {
      hostId: "",
      service: "",
      cluster: "",
      env: "",
      timeRange: compactText(value),
      resourceType: "",
      resourceId: "",
      extras: {},
    };
  }
  const scope = asObject(value);
  return {
    hostId: compactText(scope.hostId || scope.host_id),
    service: compactText(scope.service),
    cluster: compactText(scope.cluster),
    env: compactText(scope.env || scope.environment),
    timeRange: compactText(scope.timeRange || scope.time_range),
    resourceType: compactText(scope.resourceType || scope.resource_type || scope.type),
    resourceId: compactText(scope.resourceId || scope.resource_id || scope.id),
    extras: Object.fromEntries(
      Object.entries(scope).filter(([key]) => {
        return ![
          "hostId",
          "host_id",
          "service",
          "cluster",
          "env",
          "environment",
          "timeRange",
          "time_range",
          "resourceType",
          "resource_type",
          "type",
          "resourceId",
          "resource_id",
          "id",
        ].includes(key);
      }),
    ),
  };
}

export function normalizeMcpPayloadErrors(value = []) {
  const rawErrors = Array.isArray(value) ? value : [value];
  return rawErrors
    .map((item, index) => {
      if (typeof item === "string") {
        const message = compactText(item);
        if (!message) return null;
        return {
          id: `mcp-error-${index + 1}`,
          code: "",
          message,
          detail: "",
          retryable: false,
          source: "",
        };
      }

      const source = asObject(item);
      const message = compactText(source.message || source.text || source.summary || source.error || source.detail);
      if (!message) return null;
      const detail = compactText(source.detail || "");
      return {
        id: compactText(source.id || source.code || `mcp-error-${index + 1}`),
        code: compactText(source.code || source.name),
        message,
        detail: detail && detail !== message ? detail : "",
        retryable: Boolean(source.retryable),
        source: compactText(source.source),
      };
    })
    .filter(Boolean);
}

export function isMcpUiCardPayload(value) {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  return Boolean(value.uiKind || value.ui_kind || value.mcpServer || value.mcp_server || value.visual || value.actions);
}

export function normalizeMcpUiCard(input = {}, defaults = {}) {
  const source = asObject(input);
  const normalizedDefaults = asObject(defaults);
  const uiKind = normalizeUiKind(source.uiKind || source.ui_kind || normalizedDefaults.uiKind);
  const placement = normalizePlacement(source.placement || normalizedDefaults.placement, uiKind);

  return {
    id: compactText(source.id || normalizedDefaults.id || "mcp-ui-card"),
    source: compactText(source.source || normalizedDefaults.source || "mcp"),
    mcpServer: compactText(source.mcpServer || source.mcp_server || normalizedDefaults.mcpServer),
    uiKind,
    placement,
    title: compactText(source.title || source.name || normalizedDefaults.title || "MCP 卡片"),
    summary: compactText(source.summary || source.description || normalizedDefaults.summary),
    freshness: normalizeMcpFreshness(source.freshness || normalizedDefaults.freshness),
    scope: normalizeMcpScope(source.scope || normalizedDefaults.scope),
    visual: asObject(source.visual || normalizedDefaults.visual),
    actions: asArray(source.actions || normalizedDefaults.actions).map((action, index) =>
      normalizeMcpUiAction(action, index, uiKind),
    ),
    error: compactText(source.error || normalizedDefaults.error || ""),
    errors: normalizeMcpPayloadErrors(source.errors || normalizedDefaults.errors || []),
    empty: Boolean(source.empty),
  };
}

export function normalizeMcpUiActions(inputs = [], defaults = {}) {
  const normalizedDefaults = asObject(defaults);
  const uiKind = normalizeUiKind(normalizedDefaults.uiKind);
  return asArray(inputs).map((action, index) =>
    normalizeMcpUiAction(action, index, uiKind),
  );
}

export function normalizeMcpUiCards(inputs = [], defaults = {}) {
  return asArray(inputs).map((item, index) =>
    normalizeMcpUiCard(item, {
      ...defaults,
      id: compactText(item?.id || `${compactText(defaults?.id || "mcp-ui-card")}-${index + 1}`),
    }),
  );
}

function normalizeBundleKind(value) {
  const normalized = compactText(value).toLowerCase();
  return BUNDLE_KIND_SET.has(normalized) ? normalized : "monitor_bundle";
}

function standardSectionKinds(bundleKind) {
  return bundleKind === "remediation_bundle" ? MCP_REMEDIATION_SECTION_KINDS : MCP_MONITOR_SECTION_KINDS;
}

function labelizeSectionKind(kind) {
  return compactText(kind)
    .replace(/_/g, " ")
    .replace(/\b\w/g, (char) => char.toUpperCase());
}

function normalizeBundleSubject(value = {}) {
  if (typeof value === "string") {
    return {
      type: "service",
      name: compactText(value),
      env: "",
      hostId: "",
      service: "",
      cluster: "",
      resourceId: "",
    };
  }
  const subject = asObject(value);
  return {
    type: compactText(subject.type || "service"),
    name: compactText(subject.name || subject.service || subject.resourceId || subject.resource_id),
    env: compactText(subject.env || subject.environment),
    hostId: compactText(subject.hostId || subject.host_id),
    service: compactText(subject.service),
    cluster: compactText(subject.cluster),
    resourceId: compactText(subject.resourceId || subject.resource_id || subject.id),
  };
}

function normalizeSection(section = {}, index = 0) {
  const source = asObject(section);
  return {
    id: compactText(source.id || `${compactText(source.kind || "section")}-${index + 1}`),
    kind: compactText(source.kind || ""),
    title: compactText(source.title || source.label || ""),
    summary: compactText(source.summary || ""),
    cards: normalizeMcpUiCards(source.cards || []),
  };
}

function normalizeActivityEntry(entry = {}, index = 0) {
  if (typeof entry === "string") {
    const label = compactText(entry);
    return {
      id: `mcp-activity-${index + 1}`,
      label,
      detail: "",
      tone: "",
      at: "",
    };
  }
  const source = asObject(entry);
  return {
    id: compactText(source.id || `mcp-activity-${index + 1}`),
    label: compactText(source.label || source.title || source.name || source.summary),
    detail: compactText(source.detail || source.description || ""),
    tone: compactText(source.tone || source.status || ""),
    at: compactText(source.at || source.ts || source.timestamp || ""),
  };
}

export function isMcpBundlePayload(value) {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  return Boolean(value.bundleKind || value.bundle_kind || value.sections || value.subject);
}

export function normalizeMcpBundle(input = {}, defaults = {}) {
  const source = asObject(input);
  const normalizedDefaults = asObject(defaults);
  const bundleKind = normalizeBundleKind(source.bundleKind || source.bundle_kind || normalizedDefaults.bundleKind);
  const allowedKinds = new Set(standardSectionKinds(bundleKind));
  const providedSections = asArray(source.sections || normalizedDefaults.sections).map((section, index) =>
    normalizeSection(section, index),
  );
  const providedByKind = new Map(
    providedSections
      .map((section) => ({
        ...section,
        kind: normalizeSectionKind(bundleKind, section.kind),
      }))
      .filter((section) => allowedKinds.has(section.kind))
      .map((section) => [section.kind, section]),
  );

  const sections = standardSectionKinds(bundleKind).map((kind, index) => {
    const existing = providedByKind.get(kind);
    if (existing) {
      return {
        ...existing,
        kind,
        title: existing.title || labelizeSectionKind(kind),
      };
    }
    return {
      id: `${kind}-${index + 1}`,
      kind,
      title: labelizeSectionKind(kind),
      summary: "",
      cards: [],
    };
  });

  return {
    bundleId: compactText(source.bundleId || source.bundle_id || normalizedDefaults.bundleId || "mcp-bundle"),
    bundleKind,
    source: compactText(source.source || normalizedDefaults.source || "mcp"),
    mcpServer: compactText(source.mcpServer || source.mcp_server || normalizedDefaults.mcpServer),
    subject: normalizeBundleSubject(source.subject || normalizedDefaults.subject),
    summary: compactText(source.summary || normalizedDefaults.summary),
    freshness: normalizeMcpFreshness(source.freshness || normalizedDefaults.freshness),
    scope: normalizeMcpScope(source.scope || normalizedDefaults.scope),
    sections,
    rootCause: compactText(source.rootCause || source.root_cause || ""),
    confidence: normalizePositiveNumber(source.confidence, 0),
    recommendedActions: normalizeMcpUiCards(source.recommendedActions || source.recommended_actions || []),
    validationPanels: normalizeMcpUiCards(source.validationPanels || source.validation_panels || []),
    lastActivity: normalizeActivityEntry(source.lastActivity || source.last_activity || {}),
    recentActivities: asArray(source.recentActivities || source.recent_activities || source.activities).map((entry, index) =>
      normalizeActivityEntry(entry, index),
    ).filter((entry) => entry.label),
    error: compactText(source.error || normalizedDefaults.error || ""),
    errors: normalizeMcpPayloadErrors(source.errors || normalizedDefaults.errors || []),
    empty: Boolean(source.empty),
  };
}
