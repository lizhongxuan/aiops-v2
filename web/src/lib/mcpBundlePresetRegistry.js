import { compactText } from "./workspaceViewModel";

export const MCP_BUNDLE_PRESET_KEYS = Object.freeze({
  MIDDLEWARE_SERVICE_MONITOR: "middleware_service_monitor",
  ROOT_CAUSE_REMEDIATION: "root_cause_remediation",
  COROOT_SERVICE_MONITOR: "coroot_service_monitor",
  COROOT_INCIDENT_RCA: "coroot_incident_rca",
});

function buildSectionTitle(kind) {
  return compactText(kind)
    .replace(/_/g, " ")
    .replace(/\b\w/g, (char) => char.toUpperCase());
}

function makeBlueprint(uiKind, title, summary = "") {
  return {
    uiKind,
    title,
    summary,
  };
}

const MONITOR_SECTION_BLUEPRINTS = Object.freeze({
  overview: () => [
    makeBlueprint("readonly_summary", "当前状态", "汇总当前中间件或服务的健康态势"),
  ],
  trends: () => [
    makeBlueprint("readonly_chart", "趋势", "展示最近时间窗口内的变化趋势"),
  ],
  alerts: () => [
    makeBlueprint("readonly_summary", "告警", "呈现当前告警与异常摘要"),
  ],
  changes: () => [
    makeBlueprint("readonly_summary", "变更", "汇总最近的配置或发布变更"),
  ],
  dependencies: () => [
    makeBlueprint("readonly_summary", "依赖", "呈现关键依赖和关联面"),
  ],
});

const REMEDIATION_SECTION_BLUEPRINTS = Object.freeze({
  root_cause: (source = {}) => [
    makeBlueprint(
      "readonly_summary",
      "根因",
      compactText(source.rootCause || source.root_cause || source.rootCauseSummary || source.root_cause_summary || "定位根因并给出结论"),
    ),
  ],
  impact: (source = {}) => [
    makeBlueprint(
      "readonly_summary",
      "影响",
      compactText(source.impact || source.impactSummary || source.impact_summary || "说明问题对业务的影响"),
    ),
  ],
  recommended_actions: (source = {}) => {
    const actions = Array.isArray(source.recommendedActions || source.recommended_actions)
      ? source.recommendedActions || source.recommended_actions
      : [];
    if (actions.length) return actions;
    return [
      makeBlueprint(
        "action_panel",
        "推荐操作",
        compactText(source.remediationHint || source.remediation_hint || "提供一组可执行的处理动作"),
      ),
    ];
  },
  control_panels: (source = {}) => {
    const panels = Array.isArray(source.controlPanels || source.control_panels)
      ? source.controlPanels || source.control_panels
      : [];
    if (panels.length) return panels;
    return [
      makeBlueprint(
        "action_panel",
        "控制面板",
        compactText(source.controlHint || source.control_hint || "提供控制或回滚类操作"),
      ),
    ];
  },
  validation_panels: (source = {}) => {
    const panels = Array.isArray(source.validationPanels || source.validation_panels)
      ? source.validationPanels || source.validation_panels
      : [];
    if (panels.length) return panels;
    return [
      makeBlueprint(
        "readonly_chart",
        "验证面板",
        compactText(source.validationHint || source.validation_hint || "提供验证结果和回归观察"),
      ),
    ];
  },
});

const MONITOR_PRESET = Object.freeze({
  key: MCP_BUNDLE_PRESET_KEYS.MIDDLEWARE_SERVICE_MONITOR,
  label: "middleware/service monitor",
  bundleKind: "monitor_bundle",
  sectionKinds: ["overview", "trends", "alerts", "changes", "dependencies"],
  sectionTitles: {
    overview: "概览",
    trends: "趋势",
    alerts: "异常",
    changes: "变更",
    dependencies: "依赖",
  },
  cardBlueprints: MONITOR_SECTION_BLUEPRINTS,
});

const REMEDIATION_PRESET = Object.freeze({
  key: MCP_BUNDLE_PRESET_KEYS.ROOT_CAUSE_REMEDIATION,
  label: "root cause remediation",
  bundleKind: "remediation_bundle",
  sectionKinds: ["root_cause", "impact", "recommended_actions", "control_panels", "validation_panels"],
  sectionTitles: {
    root_cause: "根因",
    impact: "影响",
    recommended_actions: "推荐操作",
    control_panels: "控制面板",
    validation_panels: "验证面板",
  },
  cardBlueprints: REMEDIATION_SECTION_BLUEPRINTS,
});

const COROOT_MONITOR_SECTION_BLUEPRINTS = Object.freeze({
  overview: () => [
    makeBlueprint("readonly_summary", "服务概览", "Coroot 服务健康状态与摘要指标"),
    makeBlueprint("readonly_summary", "KPI 指标", "关键性能指标条"),
  ],
  trends: () => [
    makeBlueprint("readonly_chart", "指标趋势", "Coroot 服务关键指标时间序列"),
  ],
  alerts: () => [
    makeBlueprint("readonly_chart", "告警列表", "Coroot 服务当前告警状态表"),
  ],
  topology: () => [
    makeBlueprint("topology_card", "拓扑概览", "Coroot 服务依赖拓扑摘要"),
  ],
});

const COROOT_RCA_SECTION_BLUEPRINTS = Object.freeze({
  incident_timeline: (source = {}) => [
    makeBlueprint(
      "readonly_summary",
      "事件时间线",
      compactText(source.incidentSummary || source.incident_summary || "Coroot 事件时间线"),
    ),
  ],
  root_cause: (source = {}) => [
    makeBlueprint(
      "readonly_summary",
      "根因分析",
      compactText(source.rootCause || source.root_cause || "Coroot RCA 根因定位"),
    ),
  ],
  evidence: (source = {}) => {
    const items = Array.isArray(source.evidence) ? source.evidence : [];
    if (items.length) return items;
    return [
      makeBlueprint("readonly_summary", "证据链", compactText(source.evidenceHint || "Coroot RCA 关联证据")),
    ];
  },
  suggestions: (source = {}) => {
    const items = Array.isArray(source.suggestions) ? source.suggestions : [];
    if (items.length) return items;
    return [
      makeBlueprint("action_panel", "修复建议", compactText(source.suggestionHint || "Coroot RCA 修复建议")),
    ];
  },
});

const COROOT_MONITOR_PRESET = Object.freeze({
  key: MCP_BUNDLE_PRESET_KEYS.COROOT_SERVICE_MONITOR,
  label: "coroot service monitor",
  bundleKind: "monitor_bundle",
  sectionKinds: ["overview", "trends", "alerts", "topology"],
  sectionTitles: {
    overview: "服务概览",
    trends: "指标趋势",
    alerts: "告警列表",
    topology: "拓扑概览",
  },
  cardBlueprints: COROOT_MONITOR_SECTION_BLUEPRINTS,
});

const COROOT_RCA_PRESET = Object.freeze({
  key: MCP_BUNDLE_PRESET_KEYS.COROOT_INCIDENT_RCA,
  label: "coroot incident rca",
  bundleKind: "remediation_bundle",
  sectionKinds: ["incident_timeline", "root_cause", "evidence", "suggestions"],
  sectionTitles: {
    incident_timeline: "事件时间线",
    root_cause: "根因分析",
    evidence: "证据链",
    suggestions: "修复建议",
  },
  cardBlueprints: COROOT_RCA_SECTION_BLUEPRINTS,
});

export const MCP_BUNDLE_PRESET_REGISTRY = Object.freeze({
  [MONITOR_PRESET.key]: MONITOR_PRESET,
  [REMEDIATION_PRESET.key]: REMEDIATION_PRESET,
  [COROOT_MONITOR_PRESET.key]: COROOT_MONITOR_PRESET,
  [COROOT_RCA_PRESET.key]: COROOT_RCA_PRESET,
});

export function getMcpBundlePresetRegistry() {
  return MCP_BUNDLE_PRESET_REGISTRY;
}

export function getMcpBundlePreset(key = "") {
  return MCP_BUNDLE_PRESET_REGISTRY[compactText(key)] || MONITOR_PRESET;
}

export function listMcpBundlePresetKeys() {
  return Object.keys(MCP_BUNDLE_PRESET_REGISTRY);
}

export function buildMcpBundleSectionConfig(preset = MONITOR_PRESET, source = {}) {
  const normalizedPreset = getMcpBundlePreset(preset?.key || preset?.bundleKind || preset?.presetKey || preset);
  return normalizedPreset.sectionKinds.map((kind, index) => ({
    id: `${normalizedPreset.key}-${kind}-${index + 1}`,
    kind,
    title: normalizedPreset.sectionTitles[kind] || buildSectionTitle(kind),
    summary: "",
  }));
}

export function buildMcpBundleSectionCards(preset = MONITOR_PRESET, source = {}, scope = {}) {
  const normalizedPreset = getMcpBundlePreset(preset?.key || preset?.bundleKind || preset?.presetKey || preset);
  const sectionCards = {};
  for (const kind of normalizedPreset.sectionKinds) {
    const blueprintFactory = normalizedPreset.cardBlueprints[kind];
    const blueprints = blueprintFactory ? blueprintFactory(source, scope) : [];
    sectionCards[kind] = Array.isArray(blueprints) ? blueprints : [];
  }
  return sectionCards;
}

export default MCP_BUNDLE_PRESET_REGISTRY;
