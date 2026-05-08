export type AiopsTransportStatus = "idle" | "working" | "blocked" | "failed" | "canceled";

export type AiopsTransportTurnStatus = "submitted" | "working" | "blocked" | "completed" | "failed" | "canceled";

export type AiopsTransportProcessStatus = "queued" | "running" | "completed" | "failed" | "blocked" | "rejected";

export type AiopsTransportState = {
  schemaVersion: "aiops.transport.v2";
  sessionId: string;
  threadId: string;
  status: AiopsTransportStatus;
  currentTurnId?: string;
  turns: Record<string, AiopsTransportTurn>;
  turnOrder: string[];
  pendingApprovals: Record<string, AiopsTransportApproval>;
  mcpSurfaces: Record<string, AiopsTransportMcpSurface>;
  artifacts: Record<string, AiopsTransportArtifact>;
  runtimeLiveness: AiopsRuntimeLiveness;
  lastError?: string;
  seq: number;
  updatedAt: string;
};

export type AiopsTransportTurn = {
  id: string;
  user?: AiopsTransportMessage;
  blockOrder: string[];
  blocksById: Record<string, AiopsTranscriptBlock>;
  status: AiopsTransportTurnStatus;
  startedAt?: string;
  completedAt?: string;
  updatedAt?: string;
};

export type AiopsTransportMessage = {
  id: string;
  text: string;
  createdAt?: string;
};

export type AiopsTranscriptBlockType = "text" | "tool" | "aggregate" | "approval" | "thinking" | "artifact";

type AiopsTranscriptBlockBase = {
  id: string;
  createdAt?: string;
  updatedAt?: string;
};

export type AiopsTranscriptBlock =
  | (AiopsTranscriptBlockBase & {
      type: "text";
      text: AiopsTextBlock;
      tool?: never;
      aggregate?: never;
      approval?: never;
      thinking?: never;
      artifact?: never;
    })
  | (AiopsTranscriptBlockBase & {
      type: "tool";
      text?: never;
      tool: AiopsToolBlock;
      aggregate?: never;
      approval?: never;
      thinking?: never;
      artifact?: never;
    })
  | (AiopsTranscriptBlockBase & {
      type: "aggregate";
      text?: never;
      tool?: never;
      aggregate: AiopsAggregateBlock;
      approval?: never;
      thinking?: never;
      artifact?: never;
    })
  | (AiopsTranscriptBlockBase & {
      type: "approval";
      text?: never;
      tool?: never;
      aggregate?: never;
      approval: AiopsApprovalBlock;
      thinking?: never;
      artifact?: never;
    })
  | (AiopsTranscriptBlockBase & {
      type: "thinking";
      text?: never;
      tool?: never;
      aggregate?: never;
      approval?: never;
      thinking: AiopsThinkingBlock;
      artifact?: never;
    })
  | (AiopsTranscriptBlockBase & {
      type: "artifact";
      text?: never;
      tool?: never;
      aggregate?: never;
      approval?: never;
      thinking?: never;
      artifact: AiopsArtifactBlock;
    });

export type AiopsTranscriptTextStatus = "streaming" | "completed";

export type AiopsTextBlock = {
  role: string;
  text: string;
  status: AiopsTranscriptTextStatus;
};

export type AiopsTranscriptToolKind = "command" | "search" | "file" | "mcp" | "browser" | "list" | "other";

export type AiopsToolOutput = {
  stdout: string;
  stderr: string;
  text: string;
  truncated: boolean;
  rawRef?: string;
};

export type AiopsToolBlock = {
  toolKind: AiopsTranscriptToolKind;
  toolName?: string;
  title: string;
  summary: string;
  status: AiopsTransportProcessStatus;
  command?: string;
  inputSummary?: string;
  output: AiopsToolOutput;
  exitCode?: number;
  durationMs?: number;
  startedAt?: string;
  completedAt?: string;
  approvalId?: string;
};

export type AiopsAggregateCounts = {
  command?: number;
  search?: number;
  fileRead?: number;
  fileEdit?: number;
  list?: number;
  mcp?: number;
  browser?: number;
  other?: number;
};

export type AiopsAggregateBlock = {
  summary: string;
  status: string;
  childBlockIds: string[];
  counts: AiopsAggregateCounts;
};

export type AiopsApprovalBlock = {
  approvalId: string;
  approvalKind: string;
  title: string;
  summary: string;
  command?: string;
  status: string;
  requestedAt: string;
  resolvedAt?: string;
};

export type AiopsThinkingBlock = {
  text: string;
  status: string;
};

export type AiopsArtifactBlock = {
  artifactId: string;
  kind: string;
  title: string;
  summary: string;
};

export type AiopsTransportApproval = {
  id: string;
  turnId?: string;
  type?: string;
  status?: string;
  command?: string;
  reason?: string;
  requestedAt?: string;
  resolvedAt?: string;
};

export type AiopsTransportMcpSurface = {
  id: string;
  kind?: string;
  title?: string;
  status?: string;
  lifecycle?: AiopsTransportLifecycleState;
  pinned?: boolean;
  cards?: AiopsAgentUICard[];
  app?: AiopsIframeAppSurface;
  actions?: AiopsTransportActionBinding[];
  artifactIds?: string[];
  updatedAt?: string;
};

export type AiopsTransportArtifact = {
  id: string;
  turnId?: string;
  kind?: string;
  title?: string;
  preview?: string;
  previewData?: AiopsArtifactPreview;
  rawRef?: string;
  lifecycle?: AiopsTransportLifecycleState;
  actions?: AiopsTransportActionBinding[];
  createdAt?: string;
  modifiedAt?: string;
};

export type AiopsTransportLifecycleState = "created" | "loading" | "ready" | "failed" | "disposed";

export type AiopsAgentUICard = {
  id: string;
  kind?: string;
  title?: string;
  summary?: string;
  status?: string;
  artifactId?: string;
  surfaceId?: string;
  actions?: AiopsTransportActionBinding[];
};

export type AiopsArtifactPreview = {
  contentType?: string;
  text?: string;
  url?: string;
  rawRef?: string;
  truncated?: boolean;
  metadata?: Record<string, string>;
};

export type AiopsIframeAppSurface = {
  url?: string;
  sandbox?: string;
  height?: number;
  width?: number;
  permissions?: string[];
};

export type AiopsTransportActionBinding = {
  id: string;
  label?: string;
  command?: string;
  target?: string;
  params?: Record<string, unknown>;
  requiresApproval?: boolean;
};

export type AiopsRuntimeLiveness = {
  activeTurns: Record<string, boolean>;
  activeAgents: Record<string, boolean>;
  pendingApprovals: Record<string, boolean>;
  pendingUserInputs: Record<string, boolean>;
  activeCommandStreams: Record<string, boolean>;
};
