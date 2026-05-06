const DEFAULT_INPUT_PORT = { id: "in", label: "输入" };

const NODE_DEFINITIONS = [
  {
    key: "start",
    types: ["start"],
    actions: ["start"],
    label: "开始",
    category: "基础",
    iconText: "IN",
    tone: "blue",
    risk: "low",
    inputs: [],
    outputs: [{ id: "next", label: "下一步" }],
    description: "工作流输入和初始化上下文。",
  },
  {
    key: "end",
    types: ["end"],
    actions: ["end"],
    label: "结束",
    category: "基础",
    iconText: "OUT",
    tone: "green",
    risk: "low",
    inputs: [DEFAULT_INPUT_PORT],
    outputs: [],
    description: "工作流终止节点，汇总最终状态。",
  },
  {
    key: "command",
    actions: ["cmd.run"],
    label: "Command",
    category: "基础",
    iconText: "CMD",
    tone: "slate",
    risk: "medium",
    inputs: [DEFAULT_INPUT_PORT],
    outputs: [
      { id: "next", label: "下一步" },
      { id: "failure", label: "失败" },
    ],
    description: "执行单条命令，适合检查、查询和轻量操作。",
  },
  {
    key: "shell",
    actions: ["shell.run"],
    label: "Shell Script",
    category: "基础",
    iconText: "SH",
    tone: "green",
    risk: "medium",
    inputs: [DEFAULT_INPUT_PORT],
    outputs: [
      { id: "next", label: "下一步" },
      { id: "failure", label: "失败" },
    ],
    description: "执行 shell 脚本片段，可配置输入、输出、重试和超时。",
  },
  {
    key: "stored-script",
    actions: ["script.shell", "script.python"],
    label: "Stored Script",
    category: "基础",
    iconText: "SCR",
    tone: "green",
    risk: "medium",
    inputs: [DEFAULT_INPUT_PORT],
    outputs: [
      { id: "next", label: "下一步" },
      { id: "failure", label: "失败" },
    ],
    description: "调用已登记脚本，适合生产 Runbook 的标准动作。",
  },
  {
    key: "condition",
    types: ["condition"],
    actions: ["condition.branch", "condition.evaluate"],
    label: "条件分支",
    category: "逻辑",
    iconText: "IF",
    tone: "cyan",
    risk: "low",
    inputs: [DEFAULT_INPUT_PORT],
    outputs: [
      { id: "if", label: "IF" },
      { id: "else", label: "ELSE" },
    ],
    description: "根据变量或表达式选择 IF / ELSE 后续路径。",
  },
  {
    key: "approval",
    types: ["approval"],
    actions: ["approval.wait", "manual.approval"],
    label: "人工审批",
    category: "治理",
    iconText: "APP",
    tone: "amber",
    risk: "high",
    inputs: [DEFAULT_INPUT_PORT],
    outputs: [
      { id: "approved", label: "通过" },
      { id: "rejected", label: "拒绝" },
    ],
    description: "在高风险步骤前暂停，等待人工确认后继续。",
  },
  {
    key: "wait",
    actions: ["wait.event"],
    label: "等待事件",
    category: "逻辑",
    iconText: "WAIT",
    tone: "amber",
    risk: "low",
    inputs: [DEFAULT_INPUT_PORT],
    outputs: [
      { id: "next", label: "下一步" },
      { id: "timeout", label: "超时" },
    ],
    description: "等待外部事件、状态变化或固定时间窗口。",
  },
  {
    key: "notify",
    actions: ["notify.send", "notify.handler"],
    label: "通知",
    category: "治理",
    iconText: "NTF",
    tone: "blue",
    risk: "low",
    inputs: [DEFAULT_INPUT_PORT],
    outputs: [{ id: "next", label: "下一步" }],
    description: "发送通知或触发通知处理器。",
  },
];

const FALLBACK_DEFINITION = {
  key: "action",
  label: "Action",
  category: "动作",
  iconText: "RUN",
  tone: "slate",
  risk: "medium",
  inputs: [DEFAULT_INPUT_PORT],
  outputs: [{ id: "next", label: "下一步" }],
  description: "添加到当前工作流。",
};

const RECOVERY_PORTS = new Set(["failure", "rejected", "timeout"]);
const RECOVERY_RECOMMENDATION_ORDER = ["notify", "approval", "wait"];

export function getNodeTypeDefinition(item = {}) {
  const action = getItemAction(item);
  const type = String(item.type || "").toLowerCase();
  const match = NODE_DEFINITIONS.find((definition) => {
    const actions = definition.actions || [];
    const types = definition.types || [];
    return actions.includes(action) || (type && types.includes(type));
  });
  if (match) return cloneDefinition(match);
  return {
    ...cloneDefinition(FALLBACK_DEFINITION),
    label: item.label || item.title || item.name || action || FALLBACK_DEFINITION.label,
    description: item.description || FALLBACK_DEFINITION.description,
  };
}

export function getNodePorts(node = {}) {
  const explicit = normalizeExplicitPorts(node.ports);
  if (explicit) return explicit;
  const definition = getNodeTypeDefinition(node);
  return {
    inputs: normalizePorts(definition.inputs),
    outputs: normalizePorts(definition.outputs),
  };
}

export function getActionDefaultPorts(action = {}) {
  const explicit = normalizeExplicitPorts(action.default_ports || action.ports);
  if (explicit) return explicit;
  const definition = getNodeTypeDefinition(action);
  return {
    inputs: normalizePorts(definition.inputs),
    outputs: normalizePorts(definition.outputs),
  };
}

export function getNodeCanvasMeta(node = {}) {
  const definition = getNodeTypeDefinition(node);
  const action = getItemAction(node);
  const ports = getNodePorts(node);
  return {
    key: definition.key,
    label: node.label || node.step?.name || definition.label,
    displayLabel: node.label || definition.label,
    action: action || definition.key,
    category: definition.category,
    description: node.description || node.step?.description || definition.description,
    iconText: definition.iconText,
    tone: definition.tone,
    risk: node.risk?.level || node.risk || definition.risk,
    inputCount: ports.inputs.length,
    outputCount: ports.outputs.length,
  };
}

export function isActionAllowedAfterPort(action = {}, sourcePort = "next") {
  const normalizedPort = String(sourcePort || "next");
  if (!RECOVERY_PORTS.has(normalizedPort)) {
    return true;
  }
  const definition = getNodeTypeDefinition(action);
  return ["notify", "approval", "wait"].includes(definition.key);
}

export function filterActionsForSourcePort(actions = [], sourcePort = "next") {
  return sortActionsForSourcePort(actions.filter((action) => isActionAllowedAfterPort(action, sourcePort)), sourcePort);
}

export function isActionRecommendedAfterPort(action = {}, sourcePort = "next") {
  const normalizedPort = String(sourcePort || "next");
  if (!RECOVERY_PORTS.has(normalizedPort)) return false;
  return RECOVERY_RECOMMENDATION_ORDER.includes(getNodeTypeDefinition(action).key);
}

export function sortActionsForSourcePort(actions = [], sourcePort = "next") {
  return [...actions].sort((left, right) => {
    const leftRank = getRecommendationRank(left, sourcePort);
    const rightRank = getRecommendationRank(right, sourcePort);
    if (leftRank !== rightRank) return leftRank - rightRank;
    return 0;
  });
}

export function getActionIdentity(action = {}) {
  return getItemAction(action) || String(action.name || action.label || action.title || "").trim();
}

export function getConnectionValidationMessage(code) {
  const messages = {
    duplicate_connection: "这条连线已经存在。",
    invalid_source: "源节点不存在。",
    invalid_source_port: "当前节点没有这个输出端口。",
    invalid_target: "目标节点不存在。",
    invalid_target_port: "目标节点没有这个输入端口。",
    self_connection: "不能连接到自身。",
  };
  return messages[code] || "这条连线不符合当前节点端口规则。";
}

function getItemAction(item = {}) {
  return String(item.action || item.step?.action || item.handler?.action || item.name || "").trim();
}

function getRecommendationRank(action, sourcePort) {
  if (!isActionRecommendedAfterPort(action, sourcePort)) return Number.MAX_SAFE_INTEGER;
  const definition = getNodeTypeDefinition(action);
  const index = RECOVERY_RECOMMENDATION_ORDER.indexOf(definition.key);
  return index === -1 ? Number.MAX_SAFE_INTEGER : index;
}

function normalizeExplicitPorts(ports) {
  if (Array.isArray(ports)) {
    const inputs = ports.filter((port) => isInputPort(port));
    const outputs = ports.filter((port) => isOutputPort(port));
    if (inputs.length > 0 || outputs.length > 0) {
      return {
        inputs: normalizePorts(inputs),
        outputs: normalizePorts(outputs),
      };
    }
    return null;
  }
  if (!ports || (!Array.isArray(ports.inputs) && !Array.isArray(ports.outputs))) return null;
  return {
    inputs: normalizePorts(ports.inputs),
    outputs: normalizePorts(ports.outputs),
  };
}

function isInputPort(port = {}) {
  const type = String(port.type || port.kind || "").toLowerCase();
  return type === "input" || type === "target";
}

function isOutputPort(port = {}) {
  const type = String(port.type || port.kind || "").toLowerCase();
  return type === "output" || type === "source";
}

function normalizePorts(ports = []) {
  return ports.map((port) => {
    if (typeof port === "string") return { id: port, label: port };
    return {
      id: String(port.id || port.name || "").trim(),
      label: port.label || port.title || port.id || port.name,
    };
  }).filter((port) => port.id);
}

function cloneDefinition(definition) {
  return {
    ...definition,
    actions: [...(definition.actions || [])],
    types: [...(definition.types || [])],
    inputs: normalizePorts(definition.inputs),
    outputs: normalizePorts(definition.outputs),
  };
}
