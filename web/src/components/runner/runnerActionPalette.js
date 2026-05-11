import { getNodeTypeDefinition } from "./nodeTypeRegistry";

const USEFUL_NODE_KEYS = new Set(["shell", "condition", "approval"]);
const ACTION_ORDER = new Map([
  ["cmd.run", 10],
  ["shell.run", 20],
  ["condition.evaluate", 30],
  ["condition.branch", 30],
  ["manual.approval", 40],
  ["approval.wait", 40],
  ["notify.send", 50],
  ["notify.handler", 50],
  ["variable.aggregate", 60],
]);

const CATEGORY_LABELS = {
  command: "命令",
  script: "脚本",
  control: "逻辑",
  基础: "命令",
  逻辑: "逻辑",
  治理: "治理",
};

function actionIdentity(action = {}) {
  return String(action.action || action.name || action.label || action.title || "").trim();
}

export function isRunnerPaletteActionUseful(action = {}) {
  return USEFUL_NODE_KEYS.has(getNodeTypeDefinition(action).key);
}

export function getRunnerActionDescription(action = {}) {
  const definition = getNodeTypeDefinition(action);
  if (definition.key !== "action" && definition.description) return definition.description;
  const description = String(action.description || "").trim();
  if (description) return description;
  return definition.description || "添加到当前工作流。";
}

export function getRunnerActionCategoryLabel(action = {}) {
  const definition = getNodeTypeDefinition(action);
  if (definition.key === "condition" || definition.key === "variable-aggregator") return "逻辑";
  if (definition.key === "approval" || definition.key === "notify") return "治理";
  if (definition.key === "shell") return "脚本";
  if (definition.key === "command") return "命令";
  const raw = String(action.category || definition.category || "其他");
  return CATEGORY_LABELS[raw] || raw;
}

export function getRunnerPaletteActions(actions = []) {
  const seen = new Set();
  return [...actions]
    .filter(isRunnerPaletteActionUseful)
    .filter((action) => {
      const key = actionIdentity(action) || getNodeTypeDefinition(action).key;
      if (seen.has(key)) return false;
      seen.add(key);
      return true;
    })
    .sort((left, right) => {
      const leftRank = ACTION_ORDER.get(actionIdentity(left)) ?? Number.MAX_SAFE_INTEGER;
      const rightRank = ACTION_ORDER.get(actionIdentity(right)) ?? Number.MAX_SAFE_INTEGER;
      if (leftRank !== rightRank) return leftRank - rightRank;
      return getRunnerActionCategoryLabel(left).localeCompare(getRunnerActionCategoryLabel(right), "zh-CN");
    });
}
