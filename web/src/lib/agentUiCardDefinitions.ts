export const AGENT_UI_ARTIFACT_TYPES = [
  "coroot_chart",
  "trace_summary",
  "topology_slice",
  "rca_report",
  "workflow_result",
  "verification_result",
  "experience_match",
  "ops_manual_match",
  "ops_manual_search_result",
  "ops_manual_param_resolution",
  "ops_manual_param_form",
  "ops_manual_preflight_result",
  "ops_manual_fallback_guide",
  "runner_workflow_generation",
] as const;

export type AgentUiArtifactType = typeof AGENT_UI_ARTIFACT_TYPES[number];
export type AgentUiCardLifecycle = "active" | "deprecated" | "disabled";
export type AgentUiCardDensity = "normal" | "compact";

export type AgentUiCardDisplay = {
  label?: string;
  icon?: string;
  density?: AgentUiCardDensity;
  hideSummary?: boolean;
  hideFooter?: boolean;
  hideTypeBadge?: boolean;
  suppressInlineData?: boolean;
  selfRendered?: boolean;
  titleTransform?: "strip_chart_suffix_to_subject";
  providerLabel?: string;
  subjectLabel?: string;
};

export type AgentUiCardDefinition = {
  type: AgentUiArtifactType;
  label: string;
  lifecycle: AgentUiCardLifecycle;
  renderer?: string;
  artifactTypes?: string[];
  schemaVersion?: string;
  component?: string;
  fallback?: string;
  display?: AgentUiCardDisplay;
};

// Keep this built-in type set aligned with web/src/lib/agentUiCardDefinitions.ts and internal/appui/ui_card_service.go.
export const agentUiCardDefinitions: AgentUiCardDefinition[] = [
  {
    type: "coroot_chart",
    label: "Coroot 图表",
    lifecycle: "active",
    renderer: "coroot.chart.v1",
    artifactTypes: ["coroot_chart", "observability.chart"],
    schemaVersion: "coroot.chart.v1",
    component: "CorootChartArtifact",
    fallback: "json_summary",
    display: {
      label: "服务",
      icon: "line-chart",
      density: "compact",
      hideSummary: true,
      hideFooter: true,
      hideTypeBadge: true,
      suppressInlineData: true,
      titleTransform: "strip_chart_suffix_to_subject",
      providerLabel: "Coroot",
      subjectLabel: "服务",
    },
  },
  { type: "trace_summary", label: "Trace 摘要", lifecycle: "active", display: { icon: "activity" } },
  { type: "topology_slice", label: "拓扑片段", lifecycle: "active", display: { icon: "git-branch" } },
  { type: "rca_report", label: "根因分析", lifecycle: "active", display: { icon: "git-branch", suppressInlineData: true } },
  { type: "workflow_result", label: "Workflow 结果", lifecycle: "active", display: { icon: "list-checks" } },
  { type: "verification_result", label: "验证结果", lifecycle: "active", display: { icon: "check-circle" } },
  { type: "experience_match", label: "经验命中", lifecycle: "active", display: { icon: "shield-check" } },
  { type: "ops_manual_match", label: "运维手册判定", lifecycle: "active", display: { icon: "shield-check", selfRendered: true } },
  { type: "ops_manual_search_result", label: "运维手册检索", lifecycle: "active", display: { icon: "shield-check", selfRendered: true } },
  { type: "ops_manual_param_resolution", label: "运维手册参数解析", lifecycle: "active", display: { icon: "shield-check", selfRendered: true } },
  { type: "ops_manual_param_form", label: "运维手册参数表单", lifecycle: "active", display: { icon: "shield-check", selfRendered: true } },
  { type: "ops_manual_preflight_result", label: "运维手册预检", lifecycle: "active", display: { icon: "shield-check", selfRendered: true } },
  { type: "ops_manual_fallback_guide", label: "运维手册降级步骤", lifecycle: "active", display: { icon: "shield-check" } },
  { type: "runner_workflow_generation", label: "Workflow 生成进度", lifecycle: "active", display: { icon: "git-branch" } },
];
