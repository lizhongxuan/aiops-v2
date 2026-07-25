import { WORKFLOW_CHANGE_ARTIFACT_CONTRACTS } from "@/transport/aiopsTransportTypes";

export const AGENT_UI_ARTIFACT_TYPES = [
  "coroot_chart",
  "trace_summary",
  "topology_slice",
  "rca_report",
  "workflow_result",
  "workflow_context",
  "workflow_edit_plan",
  "workflow_patch_preview",
  "workflow_patch_apply",
  "workflow_patch_result",
  "workflow_patch_validation",
  "workflow_conflict",
  "workflow_manual_candidate",
  "workflow_tool_timeline",
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
  strictContract?: boolean;
  display?: AgentUiCardDisplay;
};

const WORKFLOW_CHANGE_LABELS = {
  plan: "修改计划",
  preview: "待确认修改",
  receipt: "修改已保存",
  validation: "检查结果",
  conflict: "画布已发生变化",
} as const;

const WORKFLOW_CHANGE_CARD_DEFINITIONS: AgentUiCardDefinition[] = WORKFLOW_CHANGE_ARTIFACT_CONTRACTS.map((contract) => ({
  type: contract.type,
  label: WORKFLOW_CHANGE_LABELS[contract.kind],
  lifecycle: "active",
  renderer: contract.renderer,
  artifactTypes: [contract.type],
  schemaVersion: contract.schemaVersion,
  component: "WorkflowChangeArtifact",
  fallback: "safe_summary",
  strictContract: true,
  display: { icon: contract.kind === "conflict" ? "activity" : "list-checks", selfRendered: true, suppressInlineData: true, hideFooter: true },
}));

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
  { type: "workflow_context", label: "Workflow 上下文", lifecycle: "active", fallback: "json_summary", display: { icon: "list-checks" } },
  ...WORKFLOW_CHANGE_CARD_DEFINITIONS.slice(0, 2),
  { type: "workflow_patch_apply", label: "Workflow Patch 审批", lifecycle: "active", fallback: "json_summary", display: { icon: "shield-check" } },
  ...WORKFLOW_CHANGE_CARD_DEFINITIONS.slice(2),
  { type: "workflow_manual_candidate", label: "运维手册候选", lifecycle: "active", fallback: "json_summary", display: { icon: "shield-check" } },
  { type: "workflow_tool_timeline", label: "Workflow 工具时间线", lifecycle: "active", fallback: "json_summary", display: { icon: "activity" } },
  { type: "verification_result", label: "验证结果", lifecycle: "active", display: { icon: "check-circle" } },
  { type: "experience_match", label: "经验命中", lifecycle: "active", display: { icon: "shield-check" } },
  { type: "ops_manual_match", label: "运维手册判定", lifecycle: "active", display: { icon: "shield-check", selfRendered: true } },
  { type: "ops_manual_search_result", label: "运维手册检索", lifecycle: "active", display: { icon: "shield-check", selfRendered: true } },
  { type: "ops_manual_param_resolution", label: "运维手册参数解析", lifecycle: "active", display: { icon: "shield-check", selfRendered: true } },
  { type: "ops_manual_param_form", label: "运维手册参数表单", lifecycle: "active", display: { icon: "shield-check", selfRendered: true } },
  { type: "ops_manual_preflight_result", label: "运维手册预检", lifecycle: "active", display: { icon: "shield-check", selfRendered: true } },
  { type: "ops_manual_fallback_guide", label: "运维手册降级步骤", lifecycle: "active", display: { icon: "shield-check" } },
  { type: "runner_workflow_generation", label: "Workflow 生成进度", lifecycle: "active", display: { icon: "git-branch", selfRendered: true } },
];
