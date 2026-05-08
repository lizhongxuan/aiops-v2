export type AiopsTransportStatus = "idle" | "working" | "blocked" | "failed" | "canceled";

export type AiopsTransportTurnStatus = "submitted" | "working" | "blocked" | "completed" | "failed" | "canceled";

export type AiopsTransportProcessKind =
  | "plan"
  | "reasoning"
  | "search"
  | "command"
  | "file"
  | "tool"
  | "evidence"
  | "approval"
  | "mcp"
  | "system";

export type AiopsTransportProcessStatus = "queued" | "running" | "completed" | "failed" | "blocked" | "rejected";

export type AiopsTransportFinalStatus = "running" | "completed" | "failed";

export type AiopsTransportState = {
  schemaVersion: string;
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
  intent?: AiopsTransportIntent;
  process?: AiopsProcessBlock[];
  final?: AiopsTransportFinal;
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

export type AiopsTransportIntent = {
  text: string;
  status: string;
};

export type AiopsTransportFinal = {
  id: string;
  text: string;
  status: AiopsTransportFinalStatus;
};

export type AiopsProcessBlock = {
  id: string;
  kind: AiopsTransportProcessKind;
  displayKind?: string;
  status: AiopsTransportProcessStatus;
  text: string;
  command?: string;
  inputSummary?: string;
  outputPreview?: string;
  steps?: AiopsTransportPlanStep[];
  queries?: string[];
  results?: AiopsSearchResult[];
  approvalId?: string;
  source?: string;
  confidence?: string;
  window?: string;
  rawRef?: string;
  exitCode?: number;
  durationMs?: number;
  updatedAt?: string;
};

export type AiopsTransportPlanStep = {
  id: string;
  text: string;
  status?: string;
  summary?: string;
};

export type AiopsSearchResult = {
  title?: string;
  url?: string;
  snippet?: string;
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
  pinned?: boolean;
  updatedAt?: string;
};

export type AiopsTransportArtifact = {
  id: string;
  turnId?: string;
  kind?: string;
  title?: string;
  preview?: string;
  rawRef?: string;
  createdAt?: string;
  modifiedAt?: string;
};

export type AiopsRuntimeLiveness = {
  activeTurns: Record<string, boolean>;
  activeAgents: Record<string, boolean>;
  pendingApprovals: Record<string, boolean>;
  pendingUserInputs: Record<string, boolean>;
  activeCommandStreams: Record<string, boolean>;
};
