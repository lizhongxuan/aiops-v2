export type AiopsTransportStatus =
  | "idle"
  | "working"
  | "blocked"
  | "failed"
  | "canceled";

export type AiopsTransportTurnStatus =
  | "submitted"
  | "working"
  | "blocked"
  | "completed"
  | "failed"
  | "canceled";

export type AiopsTransportProcessKind =
  | "plan"
  | "assistant"
  | "reasoning"
  | "search"
  | "command"
  | "file"
  | "tool"
  | "evidence"
  | "approval"
  | "mcp"
  | "system"
  | "subagent";

export type AiopsTransportProcessStatus =
  | "queued"
  | "running"
  | "completed"
  | "failed"
  | "blocked"
  | "rejected";

export type AiopsTransportFinalStatus = "running" | "completed" | "failed";

export type AiopsTransportState = {
  schemaVersion: string;
  sessionId: string;
  threadId: string;
  status: AiopsTransportStatus;
  currentTurnId?: string;
  opsRun?: AiopsTransportOpsRun;
  turns: Record<string, AiopsTransportTurn>;
  turnOrder: string[];
  pendingApprovals: Record<string, AiopsTransportApproval>;
  mcpSurfaces: Record<string, AiopsTransportMcpSurface>;
  artifacts: Record<string, AiopsTransportArtifact>;
  runtimeLiveness: AiopsRuntimeLiveness;
  hostMissions: Record<string, AiopsTransportHostMission>;
  childAgents: Record<string, AiopsTransportChildAgent>;
  activeHostMissionId?: string;
  lastError?: string;
  seq: number;
  updatedAt: string;
};

export type AiopsTransportOpsRun = {
  id: string;
  sessionId?: string;
  turnId?: string;
  clientTurnId?: string;
  source: string;
  status: string;
  title?: string;
  targetSummary?: string;
  evidenceCount?: number;
  currentStep?: string;
};

export type AiopsTransportTurn = {
  id: string;
  user?: AiopsTransportMessage;
  intent?: AiopsTransportIntent;
  process?: AiopsProcessBlock[];
  contextGovernance?: AiopsContextGovernanceEvent[];
  agentUiArtifacts?: AiopsTransportAgentUiArtifact[];
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
  targetSummary?: string;
  risk?: string;
  riskSummary?: string;
  expectedEffect?: string;
  rollback?: string;
  confidence?: string;
  window?: string;
  rawRef?: string;
  evidenceRefs?: string[];
  mock?: boolean;
  exitCode?: number;
  durationMs?: number;
  materializationTier?: string;
  originalBytes?: number;
  inlineBytes?: number;
  externalReferences?: AiopsExternalReference[];
  updatedAt?: string;
};

export type AiopsContextGovernanceEvent = {
  id?: string;
  layer: string;
  kind: string;
  message?: string;
  budget?: Record<string, unknown>;
  referenceIds?: string[];
  compactedIds?: string[];
  droppedGroupIds?: string[];
  retryAttempt?: number;
  retryMax?: number;
  timeout?: boolean;
  createdAt?: string;
};

export type AiopsExternalReference = {
  id: string;
  kind?: string;
  uri?: string;
  cardRef?: string;
  filePath?: string;
  title?: string;
  summary?: string;
  contentType?: string;
  digest?: string;
  bytes?: number;
};

export type AiopsTransportPlanStep = {
  id: string;
  index?: number;
  text: string;
  status?: string;
  summary?: string;
  title?: string;
  risk?: string;
  hostIds?: string[];
  childAgentIds?: string[];
  approvalRequired?: boolean;
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

export type AiopsTransportAgentUiArtifact = {
  id: string;
  type: string;
  title?: string;
  titleZh?: string;
  summary?: string;
  summaryZh?: string;
  status?: string;
  severity?: string;
  dataRef?: string;
  renderer?: string;
  schemaVersion?: string;
  inlineData?: unknown;
  payload?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
  actions?: Array<Record<string, unknown>>;
  mcpCard?: Record<string, unknown>;
  source?: string;
  caseId?: string;
  evidenceRef?: string;
  promptTraceId?: string;
  permissionScope?: string;
  redactionStatus?: string;
  originalType?: string;
  createdAt?: string;
  updatedAt?: string;
};

export type AiopsRuntimeLiveness = {
  activeTurns: Record<string, boolean>;
  activeAgents: Record<string, boolean>;
  pendingApprovals: Record<string, boolean>;
  pendingUserInputs: Record<string, boolean>;
  activeCommandStreams: Record<string, boolean>;
};

export type HostMissionStatus =
  | "planning"
  | "waiting_plan_acceptance"
  | "spawning_children"
  | "running"
  | "waiting_approval"
  | "completed"
  | "failed"
  | "cancelled";

export type HostChildAgentStatus =
  | "planned"
  | "spawning"
  | "running"
  | "waiting"
  | "approval_required"
  | "completed"
  | "failed"
  | "cancelled";

export type AiopsTransportHostMission = {
  id: string;
  turnId: string;
  status: HostMissionStatus | string;
  planRequired: boolean;
  planAccepted: boolean;
  mentionedHosts: AiopsTransportHostMention[];
  childAgentIds: string[];
  planSteps?: AiopsTransportPlanStep[];
  managerAgentId?: string;
  activeChildAgentId?: string;
  createdAt?: string;
  updatedAt?: string;
};

export type AiopsTransportHostMention = {
  tokenId: string;
  raw: string;
  hostId?: string;
  address?: string;
  displayName?: string;
  source: "inventory" | "ip_literal" | "hostname_literal" | string;
  resolved: boolean;
};

export type AiopsTransportChildAgent = {
  id: string;
  missionId: string;
  parentAgentId?: string;
  sessionId: string;
  hostId: string;
  hostAddress?: string;
  hostDisplayName: string;
  role?: string;
  task?: string;
  currentStepTitle?: string;
  status: HostChildAgentStatus | string;
  planStepIds?: string[];
  lastInputPreview?: string;
  lastOutputPreview?: string;
  error?: string;
  runtimeProfile?: AiopsHostAgentRuntimeProfile;
  contextDecisions?: AiopsHostAgentTraceEntry[];
  promptSections?: AiopsHostAgentPromptSection[];
  toolSurfaceSnapshot?: AiopsHostAgentToolTrace[];
  mcpInstructionDeltas?: AiopsHostAgentTraceEntry[];
  skillActivationTrace?: AiopsHostAgentTraceEntry[];
  approvalTrace?: AiopsHostAgentTraceEntry[];
  evidenceTrace?: AiopsHostAgentEvidenceTrace[];
  reportTimeline?: AiopsHostAgentTraceEntry[];
  agentMessages?: AiopsHostAgentTraceEntry[];
  subtaskStatus?: string;
  queueReason?: string;
  source?: string;
  startedAt?: string;
  updatedAt?: string;
  completedAt?: string;
};

export type AiopsHostAgentRuntimeProfile = {
  id?: string;
  name?: string;
  capabilities?: string[];
  [key: string]: unknown;
};

export type AiopsHostAgentTraceEntry = {
  id?: string;
  title?: string;
  name?: string;
  event?: string;
  role?: string;
  content?: string;
  summary?: string;
  status?: string;
  source?: string;
  sourceRef?: string;
  redaction?: string;
  hash?: string;
  ref?: string;
  [key: string]: unknown;
};

export type AiopsHostAgentPromptSection = AiopsHostAgentTraceEntry & {
  category?: string;
  sectionId?: string;
  retentionRank?: string;
  retentionClass?: string;
  compactAction?: string;
  compactSchema?: string;
};

export type AiopsHostAgentToolTrace = AiopsHostAgentTraceEntry & {
  name?: string;
  source?: "host_agent_tool" | "human_terminal" | string;
  toolName?: string;
};

export type AiopsHostAgentEvidenceTrace = AiopsHostAgentTraceEntry & {
  artifactRef?: string;
  evidenceRef?: string;
};
