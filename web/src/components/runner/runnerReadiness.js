const IMPLICIT_EXIT_PORTS = new Set(["failure", "rejected", "timeout"]);

function actionKey(action) {
  return String(action?.action || action?.name || action?.id || "").trim();
}

function nodeName(node) {
  return String(node?.step?.name || node?.label || node?.id || "").trim();
}

function nodeAction(node) {
  return String(node?.step?.action || node?.action || "").trim();
}

function nodeArgs(node) {
  return node?.step?.args && typeof node.step.args === "object" ? node.step.args : {};
}

function normalizeSchema(schema) {
  if (!schema) return {};
  if (typeof schema === "string") {
    try {
      return JSON.parse(schema);
    } catch (_err) {
      return {};
    }
  }
  return typeof schema === "object" ? schema : {};
}

function requiredFieldsForAction(action) {
  const schema = normalizeSchema(action?.input_schema || action?.inputs_schema || action?.args_schema);
  return Array.isArray(schema.required) ? schema.required.map((field) => String(field)).filter(Boolean) : [];
}

function hasValue(value) {
  if (value === null || value === undefined) return false;
  if (typeof value === "string") return value.trim() !== "";
  return true;
}

function issue(code, message, patch = {}) {
  return { code, message, severity: "blocker", ...patch };
}

function info(code, message, patch = {}) {
  return { code, message, severity: "info", ...patch };
}

export function checkRunnerWorkflowReadiness({ workflow = {}, graph = {}, actions = [] } = {}) {
  const blockers = [];
  const warnings = [];
  const infos = [];
  const workflowName = String(workflow?.name || graph?.workflow?.name || "").trim();
  const nodes = Array.isArray(graph?.nodes) ? graph.nodes : [];
  const edges = Array.isArray(graph?.edges) ? graph.edges : [];
  const actionByName = new Map(actions.map((action) => [actionKey(action), action]).filter(([key]) => key));
  const nodeIds = new Set(nodes.map((node) => String(node?.id || "")).filter(Boolean));
  const names = new Map();

  if (!workflowName) {
    blockers.push(issue("workflow_name_missing", "工作流缺少名称。"));
  }

  if (nodes.length === 0) {
    blockers.push(issue("graph_nodes_missing", "工作流至少需要一个节点。"));
  }

  for (const node of nodes) {
    const id = String(node?.id || "").trim();
    const name = nodeName(node);
    const actionName = nodeAction(node);

    if (!id) {
      blockers.push(issue("node_id_missing", "节点缺少 id。"));
      continue;
    }

    if (name) {
      if (names.has(name)) {
        blockers.push(issue("duplicate_node_name", `节点名称重复：${name}`, { nodeId: id, field: "name" }));
      } else {
        names.set(name, id);
      }
    }

    if (node?.type === "action" || actionName) {
      const action = actionByName.get(actionName);
      if (!action) {
        blockers.push(issue("unknown_action", `动作不存在：${actionName || "(empty)"}`, { nodeId: id, field: "action" }));
        continue;
      }
      const args = nodeArgs(node);
      for (const field of requiredFieldsForAction(action)) {
        if (!hasValue(args[field])) {
          blockers.push(issue("required_arg_missing", `节点 ${name || id} 缺少必填参数：${field}`, { nodeId: id, field }));
        }
      }
    }

    const outputPorts = Array.isArray(node?.ports)
      ? node.ports.filter((port) => port?.type === "output").map((port) => String(port?.id || "").trim())
      : [];
    for (const portId of outputPorts) {
      if (!IMPLICIT_EXIT_PORTS.has(portId)) continue;
      const hasExplicitEdge = edges.some((edge) => String(edge?.source || "") === id && String(edge?.source_port || edge?.sourcePort || "") === portId);
      if (!hasExplicitEdge) {
        infos.push(info("failure_defaults_to_exit_path", `节点 ${name || id} 的 ${portId} 未设置时默认退出。`, { nodeId: id, portId }));
      }
    }
  }

  for (const edge of edges) {
    const edgeId = String(edge?.id || `${edge?.source || ""}-${edge?.target || ""}`);
    const source = String(edge?.source || "").trim();
    const target = String(edge?.target || "").trim();
    if (!nodeIds.has(source) || !nodeIds.has(target)) {
      blockers.push(issue("dangling_edge", "连线指向不存在的节点。", { edgeId, source, target }));
    }
  }

  return {
    ready: blockers.length === 0,
    canRun: blockers.length === 0,
    blockers,
    warnings,
    infos,
  };
}

