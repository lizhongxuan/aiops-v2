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

export type AgentUiCardDefinition = {
  type: AgentUiArtifactType;
  label: string;
  lifecycle: AgentUiCardLifecycle;
};

// Keep this built-in type set aligned with web/src/lib/agentUiCardDefinitions.ts and internal/appui/ui_card_service.go.
export const agentUiCardDefinitions: AgentUiCardDefinition[] = [
  { type: "coroot_chart", label: "Coroot 图表", lifecycle: "active" },
  { type: "trace_summary", label: "Trace 摘要", lifecycle: "active" },
  { type: "topology_slice", label: "拓扑片段", lifecycle: "active" },
  { type: "rca_report", label: "根因分析", lifecycle: "active" },
  { type: "workflow_result", label: "Workflow 结果", lifecycle: "active" },
  { type: "verification_result", label: "验证结果", lifecycle: "active" },
  { type: "experience_match", label: "经验命中", lifecycle: "active" },
  { type: "ops_manual_match", label: "运维手册判定", lifecycle: "active" },
  { type: "ops_manual_search_result", label: "运维手册检索", lifecycle: "active" },
  { type: "ops_manual_param_resolution", label: "运维手册参数解析", lifecycle: "active" },
  { type: "ops_manual_param_form", label: "运维手册参数表单", lifecycle: "active" },
  { type: "ops_manual_preflight_result", label: "运维手册预检", lifecycle: "active" },
  { type: "ops_manual_fallback_guide", label: "运维手册降级步骤", lifecycle: "active" },
  { type: "runner_workflow_generation", label: "Workflow 生成进度", lifecycle: "active" },
];
