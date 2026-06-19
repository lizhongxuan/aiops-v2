type LabelMap = Record<string, string>;

function normalizeKey(value: unknown) {
  return String(value ?? "").trim().toLowerCase().replace(/[\s-]+/g, "_");
}

function fallbackLabel(value: unknown, fallbackPrefix: string) {
  const raw = String(value ?? "").trim();
  return raw ? `${fallbackPrefix}（${raw}）` : fallbackPrefix;
}

function labelFor(map: LabelMap, value: unknown, fallbackPrefix = "未知状态") {
  const key = normalizeKey(value);
  return map[key] || fallbackLabel(value, fallbackPrefix);
}

export const zhCaseStatusLabels: LabelMap = {
  created: "已创建",
  open: "待处理",
  active: "处理中",
  collecting_evidence: "采集证据",
  analyzing: "分析中",
  planning: "生成计划",
  waiting_confirmation: "等待确认",
  leasing_hosts: "锁定主机",
  running_workflow: "执行中",
  verifying: "验证中",
  recovered: "已恢复",
  failed: "失败",
  investigating: "排查中",
  mitigated: "已缓解",
  resolved: "已解决",
  closed: "已关闭",
  canceled: "已取消",
};

export const zhWorkflowStatusLabels: LabelMap = {
  draft: "草稿",
  validating: "校验中",
  validated: "已校验",
  dry_run: "试运行中",
  dry_run_passed: "试运行通过",
  running: "运行中",
  succeeded: "运行成功",
  success: "运行成功",
  failed: "运行失败",
  error: "运行异常",
  archived: "已归档",
  published: "已发布",
};

export const zhHostLeaseStatusLabels: LabelMap = {
  available: "可用",
  requested: "申请中",
  acquired: "已锁定",
  leased: "已锁定",
  occupied: "占用中",
  denied: "已拒绝",
  releasing: "释放中",
  released: "已释放",
  expired: "已过期",
  unhealthy: "不可用",
};

export const zhAgentUiTypeLabels: LabelMap = {
  coroot_chart: "Coroot 图表",
  trace_summary: "Trace 摘要",
  topology_slice: "拓扑片段",
  workflow_result: "Workflow 结果",
  verification_result: "验证结果",
  experience_match: "经验命中",
  ops_manual_match: "运维手册判定",
  ops_manual_search_result: "运维手册检索",
  ops_manual_preflight_result: "运维手册预检",
  ops_manual_fallback_guide: "运维手册降级步骤",
  runner_workflow_generation: "Workflow 生成进度",
  unsupported: "暂不支持的卡片类型",
  text: "文本消息",
  reasoning: "推理过程",
  plan: "执行计划",
  tool_call: "工具调用",
  tool_result: "工具结果",
  command: "命令执行",
  approval: "审批请求",
  evidence: "证据",
  final: "最终回复",
};

export const zhRedactionStatusLabels: LabelMap = {
  raw: "未脱敏",
  none: "未脱敏",
  redacted: "已脱敏",
  masked: "已遮罩",
  partial: "部分脱敏",
  failed: "脱敏失败",
};

export const zhPermissionStatusLabels: LabelMap = {
  allowed: "已授权",
  approved: "已批准",
  granted: "已授予",
  pending: "等待授权",
  denied: "已拒绝",
  rejected: "已拒绝",
  expired: "已过期",
  required: "需要授权",
};

export const zhRiskLevelLabels: LabelMap = {
  none: "无风险",
  info: "提示",
  low: "低风险",
  medium: "中风险",
  high: "高风险",
  critical: "严重风险",
  plan_only: "仅计划",
};

export const zhNavigationTitles: LabelMap = {
  "/": "AI 对话",
  "/incidents": "Case 工作台",
  "/opsgraph": "OpsGraph",
  "/opsgraph/graphs": "OpsGraph 列表",
  "/runner": "Runner Workflow",
  "/settings/llm": "LLM 配置",
  "/settings/hosts": "主机列表",
  "/settings/ops_manuals": "运维手册",
  "/settings/experience_packs": "运维手册",
  "/capabilities": "能力管理",
  "/mcp": "MCP 服务",
  "/coroot": "Coroot 观测",
  "/agent_ui": "Agent UI",
  "/debug/prompts": "Prompt Trace",
};

export function zhCaseStatusLabel(value: unknown) {
  return labelFor(zhCaseStatusLabels, value);
}

export function zhWorkflowStatusLabel(value: unknown) {
  return labelFor(zhWorkflowStatusLabels, value);
}

export function zhHostLeaseStatusLabel(value: unknown) {
  return labelFor(zhHostLeaseStatusLabels, value);
}

export function zhAgentUiTypeLabel(value: unknown) {
  return labelFor(zhAgentUiTypeLabels, value, "未知类型");
}

export function zhRedactionStatusLabel(value: unknown) {
  return labelFor(zhRedactionStatusLabels, value);
}

export function zhPermissionStatusLabel(value: unknown) {
  return labelFor(zhPermissionStatusLabels, value);
}

export function zhRiskLevelLabel(value: unknown) {
  return labelFor(zhRiskLevelLabels, value, "未知风险");
}

export function zhNavigationTitle(path: unknown) {
  return labelFor(zhNavigationTitles, path, "未知导航");
}
