export type OpsGraphNodeType =
  | "business"
  | "service"
  | "endpoint"
  | "middleware"
  | "middleware_cluster"
  | "middleware_instance"
  | "host"
  | "k8s"
  | "case"
  | "workflow";

export type OpsGraphRelationshipType =
  | "owns"
  | "contains"
  | "calls"
  | "depends_on"
  | "runs_on"
  | "affects"
  | "owned_by"
  | "handled_by";

export type OpsGraphPosition = {
  x: number;
  y: number;
};

export type OpsGraphViewport = {
  x: number;
  y: number;
  zoom: number;
};

export type OpsGraphNode = {
  id: string;
  type: OpsGraphNodeType;
  name: string;
  description?: string;
  parentId?: string;
  aliases?: string[];
  tags?: string[];
  labels?: Record<string, string>;
  properties?: Record<string, string>;
  position?: OpsGraphPosition;
  container?: boolean;
  collapsed?: boolean;
};

export type OpsGraphRelationship = {
  id: string;
  from: string;
  type: OpsGraphRelationshipType;
  to: string;
  note?: string;
  reason?: string;
};

export type OpsGraphRecord = {
  id: string;
  name: string;
  description?: string;
  environment?: string;
  isDefault?: boolean;
  nodes: OpsGraphNode[];
  edges: OpsGraphRelationship[];
  viewport?: OpsGraphViewport;
};

export type OpsGraphSummary = {
  id: string;
  name: string;
  description?: string;
  environment?: string;
  isDefault: boolean;
  nodeCount: number;
  relationshipCount: number;
  issueCount: number;
  updatedAt?: string;
};

export type OpsGraphValidationIssue = {
  code: string;
  level: "error" | "warning" | string;
  message: string;
  graphId?: string;
  nodeId?: string;
  edgeId?: string;
  relation?: string;
};
