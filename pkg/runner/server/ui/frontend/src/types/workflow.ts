export type NodeType =
  | "start"
  | "action"
  | "condition"
  | "parallel"
  | "join"
  | "handler"
  | "group"
  | "subflow"
  | "manual_approval"
  | "end";

export type EdgeKind =
  | "next"
  | "success"
  | "failure"
  | "condition"
  | "always"
  | "approval_approved"
  | "approval_rejected";

export type RunStatus =
  | "idle"
  | "queued"
  | "running"
  | "success"
  | "failed"
  | "canceled"
  | "cancelled"
  | "interrupted"
  | "waiting";

export interface WorkflowStep {
  id?: string;
  name?: string;
  action?: string;
  target?: string | string[];
  targets?: string[];
  args?: Record<string, unknown>;
  when?: string;
  retries?: number;
  timeout?: string;
  continue_on_error?: boolean;
  expect_vars?: string[];
  must_vars?: string[];
}

export interface WorkflowHandler {
  name?: string;
  action?: string;
  args?: Record<string, unknown>;
}

export interface WorkflowDefinition {
  version: string;
  name: string;
  description?: string;
  env_packages?: string[];
  validation_env?: Record<string, unknown>;
  inventory?: Record<string, unknown>;
  vars?: Record<string, unknown>;
  steps?: WorkflowStep[];
}

export interface WorkflowSummary {
  name: string;
  description?: string;
  version?: string;
  labels?: Record<string, string>;
  save_note?: string;
  status?: "draft" | "published" | string;
  published_at?: string;
  created_at?: string;
  updated_at?: string;
}

export interface WorkflowVersion {
  id: string;
  name: string;
  description?: string;
  version?: string;
  status?: "draft" | "published" | string;
  save_note?: string;
  reason?: string;
  checksum?: string;
  yaml: string;
  published_at?: string;
  created_at?: string;
}

export interface WorkflowBundleVersion extends WorkflowVersion {}

export interface WorkflowBundle {
  bundle_version?: string;
  exported_at?: string;
  name: string;
  description?: string;
  version?: string;
  yaml: string;
  labels?: Record<string, string>;
  save_note?: string;
  status?: "draft" | "published" | string;
  published_at?: string;
  versions?: WorkflowBundleVersion[];
}

export interface GraphPosition {
  x: number;
  y: number;
}

export interface NodeRunState {
  run_id?: string;
  status?: RunStatus | string;
  message?: string;
  started_at?: string;
  finished_at?: string;
  hosts?: Record<string, unknown>;
}

export interface WorkflowNode {
  id: string;
  type: NodeType;
  position: GraphPosition;
  step_id?: string;
  step_name?: string;
  step?: WorkflowStep;
  handler_name?: string;
  handler?: WorkflowHandler;
  label?: string;
  parent_id?: string;
  collapsed?: boolean;
  state?: NodeRunState;
  approval?: {
    subjects?: string[];
    timeout?: string;
    on_timeout?: string;
  };
  subflow?: {
    workflow_name?: string;
    vars?: Record<string, unknown>;
  };
  join?: {
    strategy?: string;
    failure_threshold?: number;
  };
  ui?: Record<string, unknown>;
}

export interface WorkflowEdge {
  id: string;
  source: string;
  source_port?: string;
  target: string;
  target_port?: string;
  kind?: EdgeKind;
  condition?: string;
  state?: {
    status?: string;
    selected_at?: string;
  };
  ui?: Record<string, unknown>;
}

export interface CompiledWorkflowResult {
  workflow: WorkflowDefinition;
  yaml: string;
  warnings?: WorkflowIssue[];
}

export interface CreateGraphWorkflowRequest {
  graph: WorkflowGraph;
  labels?: Record<string, string>;
  save_note?: string;
}

export interface CreatedGraphWorkflowResult extends CompiledWorkflowResult {
  name: string;
  status: "draft" | "published" | string;
  graph: WorkflowGraph;
}

export interface WorkflowGraph {
  version: string;
  workflow: WorkflowDefinition;
  layout?: {
    direction?: string;
    viewport?: { x: number; y: number; zoom: number };
    ui?: Record<string, unknown>;
  };
  nodes: WorkflowNode[];
  edges: WorkflowEdge[];
  ui?: Record<string, unknown>;
}

export interface ActionSpec {
  action: string;
  title: string;
  category: string;
  description?: string;
  risk?: "read_only" | "low" | "medium" | "high" | string;
  node_type?: NodeType;
  args_schema?: JsonSchema;
  defaults?: Record<string, unknown>;
  required_args?: string[];
  outputs?: OutputSpec[];
  examples?: ActionExample[];
  experimental?: boolean;
  deprecated?: boolean;
}

export interface ActionExample {
  title: string;
  description?: string;
  args?: Record<string, unknown>;
}

export interface OutputSpec {
  name: string;
  type?: string;
  description?: string;
}

export interface JsonSchema {
  type?: string | string[];
  title?: string;
  description?: string;
  properties?: Record<string, JsonSchema>;
  items?: JsonSchema;
  required?: string[];
  enum?: unknown[];
  default?: unknown;
  minLength?: number;
  additionalProperties?: boolean | JsonSchema;
  [key: string]: unknown;
}

export interface WorkflowIssue {
  severity?: string;
  type: string;
  node_id?: string;
  edge_id?: string;
  field?: string;
  message: string;
  suggestion?: string;
}

export interface ValidationResult {
  valid: boolean;
  errors: WorkflowIssue[];
  warnings: WorkflowIssue[];
  summary?: string;
}

export interface DryRunResult extends ValidationResult {
  workflow_name?: string;
  steps_count: number;
  target_hosts: string[];
  actions_used: string[];
  agents_status: Record<string, unknown>;
  path_simulation?: PathSimulation;
  yaml?: string;
}

export interface NodeDebugRequest {
  graph: WorkflowGraph;
  vars?: Record<string, unknown>;
  target?: string;
  mode?: "dry_run" | "mock" | "local" | "run" | string;
}

export interface NodeDebugResult {
  node_id: string;
  action?: string;
  status: "success" | "failed" | "skipped" | string;
  output?: Record<string, unknown>;
  stdout?: string;
  stderr?: string;
  error?: string;
}

export interface PathSimulation {
  reachable_node_ids: string[];
  selected_edge_ids: string[];
  skipped_edge_ids?: string[];
  unresolved_edge_ids?: string[];
  paths: SimulatedPath[];
  conditions?: SimulatedCondition[];
  summary: string;
}

export interface SimulatedPath {
  node_ids: string[];
  edge_ids: string[];
  terminal_node_id?: string;
  status: string;
}

export interface SimulatedCondition {
  edge_id: string;
  expression?: string;
  result?: boolean;
  error?: string;
}

export interface RunResponse {
  run_id: string;
  status: RunStatus | string;
  workflow_name?: string;
  created_at: string;
}

export interface RunEvent {
  id?: string;
  type: string;
  run_id?: string;
  node_id?: string;
  edge_id?: string;
  step?: string;
  host?: string;
  status?: RunStatus | string;
  message?: string;
  ts?: string;
  timestamp?: string;
  payload?: Record<string, unknown>;
  output?: Record<string, unknown>;
}
