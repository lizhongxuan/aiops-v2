const commandOutputsSchema = {
  type: "object",
  properties: {
    stdout: { type: "string", title: "stdout" },
    stderr: { type: "string", title: "stderr" },
    exit_code: { type: "integer", title: "exit_code" },
  },
};

const standardActionPorts = {
  inputs: [{ id: "in", label: "输入" }],
  outputs: [
    { id: "next", label: "下一步" },
    { id: "failure", label: "失败" },
  ],
};

const approvalPorts = {
  inputs: [{ id: "in", label: "输入" }],
  outputs: [
    { id: "approved", label: "通过" },
    { id: "rejected", label: "拒绝" },
  ],
};

const conditionPorts = {
  inputs: [{ id: "in", label: "输入" }],
  outputs: [
    { id: "if", label: "IF" },
    { id: "else", label: "ELSE" },
  ],
};

const waitPorts = {
  inputs: [{ id: "in", label: "输入" }],
  outputs: [
    { id: "next", label: "下一步" },
    { id: "timeout", label: "超时" },
  ],
};

const notifyPorts = {
  inputs: [{ id: "in", label: "输入" }],
  outputs: [{ id: "next", label: "下一步" }],
};

const commonCapabilities = ["structured_io", "variables", "targets", "timeout", "retries"];

const approvalOutputsSchema = {
  type: "object",
  properties: {
    decision: { type: "string", enum: ["approved", "rejected"], title: "decision" },
    actor: { type: "string", title: "actor" },
    comment: { type: "string", title: "comment" },
  },
};

const withSchemaAliases = (action) => ({
  ...action,
  input_schema: action.input_schema || action.inputs_schema,
  output_schema: action.output_schema || action.outputs_schema,
});

const fallbackRunnerActions = [
  {
    action: "cmd.run",
    label: "Command",
    category: "基础",
    risk: "medium",
    capabilities: [...commonCapabilities, "failure_path"],
    default_ports: standardActionPorts,
    description: "执行单条命令，适合检查、查询和轻量操作。",
    inputs_schema: {
      type: "object",
      required: ["cmd"],
      properties: {
        cmd: { type: "string", title: "Command", description: "通过 /bin/sh -c 执行的命令。" },
        dir: { type: "string", title: "Working directory" },
        env: { type: "object", title: "Environment" },
      },
    },
    outputs_schema: commandOutputsSchema,
  },
  {
    action: "shell.run",
    label: "Shell Script",
    category: "基础",
    risk: "medium",
    capabilities: [...commonCapabilities, "failure_path"],
    default_ports: standardActionPorts,
    description: "执行 shell 脚本片段，可配置输入、输出、重试和超时。",
    inputs_schema: {
      type: "object",
      required: ["script"],
      properties: {
        script: { type: "string", title: "Script", description: "Shell 脚本内容。" },
        dir: { type: "string", title: "Working directory" },
        env: { type: "object", title: "Environment" },
        export_vars: { type: "boolean", title: "Export RUNNER_EXPORT_* variables" },
      },
    },
    outputs_schema: commandOutputsSchema,
  },
  {
    action: "script.shell",
    label: "Stored Script",
    category: "基础",
    risk: "medium",
    capabilities: [...commonCapabilities, "failure_path"],
    default_ports: standardActionPorts,
    description: "调用已登记脚本，适合生产 Runbook 的标准动作。",
    inputs_schema: {
      type: "object",
      properties: {
        script_ref: { type: "string", title: "Script ref" },
        script: { type: "string", title: "Inline script" },
        args: { type: "array", title: "Arguments", items: { type: "string" } },
        dir: { type: "string", title: "Working directory" },
        env: { type: "object", title: "Environment" },
      },
    },
    outputs_schema: commandOutputsSchema,
  },
  {
    action: "approval.wait",
    label: "人工审批",
    category: "治理",
    risk: "high",
    capabilities: ["structured_io", "variables", "approval", "branching"],
    default_ports: approvalPorts,
    description: "在高风险步骤前暂停，等待人工确认后继续。",
    inputs_schema: {
      type: "object",
      properties: {
        subjects: { type: "array", title: "Approvers", items: { type: "string" } },
        timeout: { type: "string", title: "Timeout" },
        risk_reason: { type: "string", title: "Risk reason" },
      },
    },
    outputs_schema: approvalOutputsSchema,
  },
  {
    action: "condition.branch",
    label: "条件分支",
    category: "逻辑",
    risk: "low",
    capabilities: ["structured_io", "variables", "branching"],
    default_ports: conditionPorts,
    description: "根据变量或表达式选择 IF / ELSE 后续路径。",
    inputs_schema: {
      type: "object",
      required: ["expression"],
      properties: {
        expression: { type: "string", title: "Expression" },
      },
    },
    outputs_schema: {
      type: "object",
      properties: {
        matched: { type: "boolean", title: "matched" },
        branch: { type: "string", title: "branch" },
      },
    },
  },
  {
    action: "wait.event",
    label: "等待事件",
    category: "逻辑",
    risk: "low",
    capabilities: ["structured_io", "variables", "timeout"],
    default_ports: waitPorts,
    description: "等待外部事件、状态变化或固定时间窗口。",
    inputs_schema: {
      type: "object",
      properties: {
        event: { type: "string", title: "Event" },
        timeout: { type: "string", title: "Timeout" },
      },
    },
    outputs_schema: {
      type: "object",
      properties: {
        event: { type: "string", title: "event" },
        timed_out: { type: "boolean", title: "timed_out" },
      },
    },
  },
  {
    action: "notify.send",
    label: "通知",
    category: "治理",
    risk: "low",
    capabilities: ["structured_io", "variables", "notification"],
    default_ports: notifyPorts,
    description: "发送通知或触发通知处理器。",
    inputs_schema: {
      type: "object",
      required: ["template"],
      properties: {
        channel: { type: "string", title: "Channel", enum: ["slack", "email", "webhook", "pagerduty"] },
        recipients: { type: "array", title: "Recipients", items: { type: "string" } },
        template: { type: "string", title: "Template" },
      },
    },
    outputs_schema: {
      type: "object",
      properties: {
        delivered: { type: "boolean", title: "delivered" },
        message_id: { type: "string", title: "message_id" },
      },
    },
  },
];

export const FALLBACK_RUNNER_ACTIONS = fallbackRunnerActions.map(withSchemaAliases);
