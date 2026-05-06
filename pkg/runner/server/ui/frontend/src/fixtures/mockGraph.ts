import type { ActionSpec, RunEvent, WorkflowGraph, WorkflowSummary } from "../types/workflow";

export const mockGraph: WorkflowGraph = {
  version: "v1",
  workflow: {
    version: "v0.1",
    name: "service-restart-candidate",
    description: "Validate agent health, restart a service, and wait for approval before rollout.",
    vars: {
      service: "billing-api",
      environment: "staging",
    },
    inventory: {
      groups: {
        app: {
          hosts: ["staging-a", "staging-b"],
          vars: { capabilities: ["cmd.run", "shell.run"] },
        },
      },
      hosts: {
        "staging-a": {
          address: "agent://staging-a",
          vars: { capabilities: ["cmd.run", "shell.run"] },
        },
        "staging-b": {
          address: "agent://staging-b",
          vars: { capabilities: ["cmd.run"] },
        },
        "metrics-agent": {
          address: "agent://metrics-agent",
          vars: { capabilities: ["script.python"] },
        },
      },
    },
  },
  layout: {
    direction: "LR",
    viewport: { x: 0, y: 0, zoom: 1 },
  },
  nodes: [
    {
      id: "start",
      type: "start",
      label: "Trigger",
      position: { x: 80, y: 160 },
    },
    {
      id: "probe",
      type: "action",
      label: "Probe hosts",
      position: { x: 330, y: 120 },
      step_name: "probe-hosts",
      step: {
        name: "probe-hosts",
        action: "cmd.run",
        target: ["staging-a", "staging-b"],
        args: { cmd: "systemctl is-active ${service}" },
      },
    },
    {
      id: "restart",
      type: "action",
      label: "Restart service",
      position: { x: 610, y: 120 },
      step_name: "restart-service",
      step: {
        name: "restart-service",
        action: "shell.run",
        target: ["staging-a", "staging-b"],
        args: { script: "sudo systemctl restart ${service}\nsleep 3\nsystemctl status ${service}" },
      },
    },
    {
      id: "approval",
      type: "manual_approval",
      label: "Operator approval",
      position: { x: 910, y: 120 },
      approval: {
        subjects: ["oncall", "service-owner"],
        timeout: "30m",
        on_timeout: "reject",
      },
    },
    {
      id: "verify",
      type: "action",
      label: "Verify metrics",
      position: { x: 1210, y: 120 },
      step_name: "verify-metrics",
      step: {
        name: "verify-metrics",
        action: "script.python",
        target: "metrics-agent",
        args: { script_ref: "check-error-rate", args: ["${service}", "${environment}"] },
      },
    },
    {
      id: "end",
      type: "end",
      label: "Done",
      position: { x: 1490, y: 160 },
    },
  ],
  edges: [
    { id: "start-probe", source: "start", target: "probe", kind: "next" },
    { id: "probe-restart", source: "probe", target: "restart", kind: "success" },
    { id: "restart-approval", source: "restart", target: "approval", kind: "success" },
    { id: "approval-verify", source: "approval", target: "verify", kind: "approval_approved" },
    { id: "verify-end", source: "verify", target: "end", kind: "success" },
  ],
};

export const mockActions: ActionSpec[] = [
  {
    action: "cmd.run",
    title: "Command",
    category: "command",
    description: "Run a shell command through /bin/sh -c on each target.",
    risk: "medium",
    node_type: "action",
    required_args: ["cmd"],
    defaults: { cmd: "echo hello" },
    args_schema: {
      type: "object",
      required: ["cmd"],
      properties: {
        cmd: { type: "string", title: "Command", minLength: 1 },
        dir: { type: "string", title: "Working directory" },
      },
    },
  },
  {
    action: "shell.run",
    title: "Shell Script",
    category: "script",
    description: "Run inline shell script content through /bin/sh.",
    risk: "high",
    node_type: "action",
    required_args: ["script"],
    defaults: { script: "set -e\necho ok" },
  },
  {
    action: "script.shell",
    title: "Stored Shell Script",
    category: "script",
    description: "Run shell script content resolved by the script service or supplied inline.",
    risk: "high",
    node_type: "action",
    defaults: { script_ref: "restore.sh" },
  },
  {
    action: "script.python",
    title: "Stored Python Script",
    category: "script",
    description: "Run Python script content resolved by the script service or supplied inline.",
    risk: "high",
    node_type: "action",
    defaults: { script_ref: "verify.py" },
  },
  {
    action: "manual.approval",
    title: "Manual Approval",
    category: "control",
    description: "Pause a graph run until an operator approves or rejects the node.",
    risk: "medium",
    node_type: "manual_approval",
    defaults: { subjects: ["oncall"], timeout: "30m", on_timeout: "reject" },
    experimental: true,
  },
  {
    action: "workflow.run",
    title: "Subflow",
    category: "control",
    description: "Invoke another saved workflow as a graph node.",
    risk: "medium",
    node_type: "subflow",
    required_args: ["workflow"],
    defaults: { workflow: "restore-verify", vars: { service: "${vars.service}" } },
    args_schema: {
      type: "object",
      required: ["workflow"],
      properties: {
        workflow: { type: "string", title: "Workflow", minLength: 1 },
        vars: { type: "object", title: "Input variables", additionalProperties: true },
      },
    },
    outputs: [{ name: "run_id", type: "string" }],
    experimental: true,
  },
];

export const mockWorkflows: WorkflowSummary[] = [
  {
    name: "restore-verify",
    version: "v3",
    description: "Restore verification child workflow.",
  },
  {
    name: "service-restart-candidate",
    version: "v0.1",
    description: "Validate agent health, restart a service, and wait for approval before rollout.",
  },
];

export const mockRunEvents: RunEvent[] = [
  { id: "evt-1", type: "run_start", run_id: "run-mock-042", status: "running", message: "Run accepted", ts: "2026-05-03T13:40:00Z" },
  { id: "evt-2", type: "node_finished", run_id: "run-mock-042", status: "success", message: "2 hosts healthy", output: { node_id: "probe" }, ts: "2026-05-03T13:40:05Z" },
  { id: "evt-3", type: "edge_selected", run_id: "run-mock-042", status: "selected", message: "Selected restart branch", output: { edge_id: "probe-restart" }, ts: "2026-05-03T13:40:06Z" },
  { id: "evt-4", type: "node_started", run_id: "run-mock-042", status: "running", message: "Restarting billing-api", output: { node_id: "restart" }, ts: "2026-05-03T13:40:08Z" },
  {
    id: "evt-5",
    type: "output_delta",
    run_id: "run-mock-042",
    step: "restart",
    host: "billing-01",
    status: "running",
    message: "stdout output received",
    output: { stream: "stdout", chunk: "systemctl restart billing-api\n" },
    ts: "2026-05-03T13:40:10Z",
  },
  {
    id: "evt-6",
    type: "host_result",
    run_id: "run-mock-042",
    step: "restart",
    host: "billing-01",
    status: "success",
    message: "host billing-01 finished with status=success",
    output: {
      stdout: "systemctl restart billing-api\nactive\n",
      stderr: "",
      exit_code: 0,
      vars: { restarted: true },
      runner_debug: { mode: "remote", resolved_address: "10.0.12.21" },
    },
    ts: "2026-05-03T13:40:12Z",
  },
  { id: "evt-7", type: "node_started", run_id: "run-mock-042", status: "waiting", message: "Waiting for oncall approval", output: { node_id: "approval" }, ts: "2026-05-03T13:40:18Z" },
];
