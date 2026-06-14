import httpClient from "./httpClient";
import { resolveUiFixtureRuntime } from "@/lib/uiFixtureRuntime";
import type {
  AiopsHostAgentEvidenceTrace,
  AiopsHostAgentPromptSection,
  AiopsHostAgentRuntimeProfile,
  AiopsHostAgentToolTrace,
  AiopsHostAgentTraceEntry,
} from "@/transport/aiopsTransportTypes";

export type HostTranscriptItemType =
  | "manager_message"
  | "user_followup"
  | "assistant_message"
  | "tool_call"
  | "tool_result"
  | "approval"
  | "error"
  | string;

export type HostOpsTranscriptItem = {
  id: string;
  type: HostTranscriptItemType;
  content?: string;
  toolName?: string;
  approvalId?: string;
  status?: string;
  payload?: Record<string, unknown>;
  createdAt?: string;
};

export type HostChildAgentTranscript = {
  childAgentId: string;
  items: HostOpsTranscriptItem[];
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
};

type HostOpsHttpClient = {
  get(path: string): Promise<unknown>;
  post?(path: string, body?: unknown): Promise<unknown>;
};

export function createHostOpsApi(client: HostOpsHttpClient = httpClient) {
  return {
    async getChildAgentTranscript(childAgentId: string): Promise<HostChildAgentTranscript> {
      const fixtureTranscript = getFixtureChildAgentTranscript(childAgentId);
      if (fixtureTranscript) {
        return normalizeChildAgentTranscript(fixtureTranscript);
      }
      const payload = await client.get(
        `/api/v1/host-ops/child-agents/${encodeURIComponent(childAgentId)}/transcript`,
      );
      return normalizeChildAgentTranscript(payload);
    },
    async submitApprovalDecision(approvalId: string, decision: string): Promise<unknown> {
      if (!client.post) {
        throw new Error("approval decision endpoint is unavailable");
      }
      return client.post(`/api/v1/approvals/${encodeURIComponent(approvalId)}/decision`, { decision });
    },
  };
}

export function normalizeChildAgentTranscript(payload: unknown): HostChildAgentTranscript {
  if (!isRecord(payload)) {
    return { childAgentId: "", items: [] };
  }

  return {
    childAgentId: stringValue(payload.childAgentId ?? payload.child_agent_id),
    items: Array.isArray(payload.items) ? payload.items.map(normalizeTranscriptItem) : [],
    runtimeProfile: isRecord(payload.runtimeProfile ?? payload.runtime_profile)
      ? (payload.runtimeProfile ?? payload.runtime_profile) as AiopsHostAgentRuntimeProfile
      : undefined,
    contextDecisions: traceArray(payload.contextDecisions ?? payload.context_decisions),
    promptSections: traceArray(payload.promptSections ?? payload.prompt_sections) as AiopsHostAgentPromptSection[] | undefined,
    toolSurfaceSnapshot: traceArray(payload.toolSurfaceSnapshot ?? payload.tool_surface_snapshot) as AiopsHostAgentToolTrace[] | undefined,
    mcpInstructionDeltas: traceArray(payload.mcpInstructionDeltas ?? payload.mcp_instruction_deltas),
    skillActivationTrace: traceArray(payload.skillActivationTrace ?? payload.skill_activation_trace),
    approvalTrace: traceArray(payload.approvalTrace ?? payload.approval_trace),
    evidenceTrace: traceArray(payload.evidenceTrace ?? payload.evidence_trace) as AiopsHostAgentEvidenceTrace[] | undefined,
    reportTimeline: traceArray(payload.reportTimeline ?? payload.report_timeline),
    agentMessages: traceArray(payload.agentMessages ?? payload.agent_messages),
  };
}

function normalizeTranscriptItem(item: unknown, index: number): HostOpsTranscriptItem {
  if (!isRecord(item)) {
    return {
      id: `item-${index + 1}`,
      type: "assistant_message",
      content: stringValue(item),
    };
  }

  const payload = isRecord(item.payload) ? item.payload : undefined;
  return {
    id: stringValue(item.id) || `item-${index + 1}`,
    type: stringValue(item.type) || "assistant_message",
    content: optionalString(item.content),
    toolName: optionalString(item.toolName ?? item.tool_name),
    approvalId: optionalString(item.approvalId ?? item.approval_id),
    status: optionalString(item.status),
    payload,
    createdAt: optionalString(item.createdAt ?? item.created_at),
  };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}

function stringValue(value: unknown): string {
  if (value === undefined || value === null) {
    return "";
  }
  return String(value);
}

function optionalString(value: unknown): string | undefined {
  const normalized = stringValue(value);
  return normalized ? normalized : undefined;
}

function traceArray(value: unknown): AiopsHostAgentTraceEntry[] | undefined {
  return Array.isArray(value) ? value.map((item) => isRecord(item) ? item : { content: stringValue(item) }) : undefined;
}

function getFixtureChildAgentTranscript(childAgentId: string): unknown | undefined {
  if (typeof window === "undefined") {
    return undefined;
  }
  const fixture =
    (window as unknown as { __CODEX_UI_FIXTURE__?: unknown }).__CODEX_UI_FIXTURE__ ?? resolveUiFixtureRuntime();
  if (!isRecord(fixture) || !isRecord(fixture.state) || !isRecord(fixture.state.hostOpsTranscripts)) {
    return undefined;
  }
  return fixture.state.hostOpsTranscripts[childAgentId];
}

const hostOpsApi = createHostOpsApi();

export const getChildAgentTranscript = hostOpsApi.getChildAgentTranscript;
export const submitHostOpsApprovalDecision = hostOpsApi.submitApprovalDecision;
