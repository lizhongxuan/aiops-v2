import { useAssistantApi, useAssistantTransportState } from "@assistant-ui/react";
import { useEffect, useMemo, useState, type PropsWithChildren } from "react";

import { fetchAssistantTransportResumeState } from "@/transport/assistantTransportControl";
import { ChatTransportProvider } from "@/transport/ChatTransportProvider";
import { createInitialAiopsTransportState } from "@/transport/aiopsTransportRuntime";
import { getCachedAiopsTransportState } from "@/transport/aiopsTransportStateCache";
import type { AiopsTransportState } from "@/transport/aiopsTransportTypes";
import { WorkflowTransportActivityProvider } from "./WorkflowTransportActivityContext";

export const WORKFLOW_TRANSPORT_RESUME_EVENT = "aiops:workflow-transport:resume";
export const WORKFLOW_TRANSPORT_RESUME_SCHEMA_VERSION = "aiops.workflow.transport.resume.v1";

export type WorkflowTransportResumeEventDetail = {
  schemaVersion: typeof WORKFLOW_TRANSPORT_RESUME_SCHEMA_VERSION;
  workflowId: string;
  threadId: string;
  sessionId: string;
  turnId: string;
  approvalId: string;
};

export function WorkflowTransportBoundary({ workflowId, children }: PropsWithChildren<{ workflowId: string }>) {
  const identity = useMemo(() => workflowTransportIdentity(workflowId), [workflowId]);
  const cached = useMemo(() => getCachedAiopsTransportState(identity.cacheScope), [identity.cacheScope]);
  const [initialState, setInitialState] = useState<AiopsTransportState>(() => cached || createInitialAiopsTransportState(identity.threadId));
  const [ready, setReady] = useState(() => Boolean(cached) || !readWorkflowTransportSession(identity.storageKey));

  useEffect(() => {
    let active = true;
    const sessionId = readWorkflowTransportSession(identity.storageKey);
    if (cached || !sessionId) {
      setInitialState(cached || createInitialAiopsTransportState(identity.threadId));
      setReady(true);
      return () => { active = false; };
    }
    setReady(false);
    void fetchAssistantTransportResumeState(sessionId, identity.threadId)
      .then((state) => {
        if (!active) return;
        if (state) {
          setInitialState(state);
        } else {
          clearWorkflowTransportSession(identity.storageKey);
          setInitialState(createInitialAiopsTransportState(identity.threadId));
        }
      })
      .catch(() => {
        if (!active) return;
        clearWorkflowTransportSession(identity.storageKey);
        setInitialState(createInitialAiopsTransportState(identity.threadId));
      })
      .finally(() => {
        if (active) setReady(true);
      });
    return () => { active = false; };
  }, [cached, identity.storageKey, identity.threadId]);

  if (!ready) {
    return <div className="flex min-h-[240px] items-center justify-center text-sm text-zinc-500" data-testid="workflow-transport-resume">正在恢复工作流会话...</div>;
  }

  return (
    <ChatTransportProvider
      autoResume={shouldResumeWorkflowTransport(initialState)}
      cacheScope={identity.cacheScope}
      initialState={initialState}
      key={`${identity.threadId}:${initialState.sessionId || "new"}`}
      threadId={identity.threadId}
    >
      <WorkflowTransportActivityProvider>
        <WorkflowTransportSessionPointer storageKey={identity.storageKey} />
        <WorkflowTransportResumeListener workflowId={workflowId} threadId={identity.threadId} />
        {children}
      </WorkflowTransportActivityProvider>
    </ChatTransportProvider>
  );
}

function WorkflowTransportResumeListener({ workflowId, threadId }: { workflowId: string; threadId: string }) {
  const api = useAssistantApi();
  const state = useAssistantTransportState() as AiopsTransportState | undefined;
  const sessionId = state?.sessionId?.trim() || "";

  useEffect(() => {
    const resume = (event: Event) => {
      const detail = readWorkflowTransportResumeEvent(event);
      if (!detail
        || detail.workflowId !== workflowId
        || detail.threadId !== threadId
        || detail.sessionId !== sessionId
        || state?.threadId !== threadId) return;
      // This only refreshes the exact blocked Runtime turn. Approval and Apply
      // remain explicit server-owned actions in the AI panel.
      api.thread().unstable_resumeRun({});
    };
    window.addEventListener(WORKFLOW_TRANSPORT_RESUME_EVENT, resume);
    return () => window.removeEventListener(WORKFLOW_TRANSPORT_RESUME_EVENT, resume);
  }, [api, sessionId, state?.threadId, threadId, workflowId]);
  return null;
}

function WorkflowTransportSessionPointer({ storageKey }: { storageKey: string }) {
  const state = useAssistantTransportState() as AiopsTransportState | undefined;
  useEffect(() => {
    const sessionId = state?.sessionId?.trim();
    if (sessionId) writeWorkflowTransportSession(storageKey, sessionId);
  }, [state?.sessionId, storageKey]);
  return null;
}

export function workflowTransportIdentity(workflowId: string) {
  const normalized = workflowId.trim();
  const encoded = encodeURIComponent(normalized || "unselected");
  return {
    threadId: `workflow:${normalized || "unselected"}`,
    cacheScope: `workflow:${normalized || "unselected"}`,
    storageKey: `aiops.workflow.transport.session.${encoded}`,
  };
}

export function readWorkflowTransportSession(storageKey: string) {
  try {
    return window.sessionStorage.getItem(storageKey)?.trim() || "";
  } catch {
    return "";
  }
}

export function writeWorkflowTransportSession(storageKey: string, sessionId: string) {
  try {
    window.sessionStorage.setItem(storageKey, sessionId.trim());
  } catch {
    // Resume is a convenience; storage failure must not block the workspace.
  }
}

export function emitWorkflowTransportResume(detail: WorkflowTransportResumeEventDetail) {
  const canonical = canonicalWorkflowTransportResumeDetail(detail);
  if (!canonical) throw new TypeError("workflow transport resume detail is invalid");
  window.dispatchEvent(new CustomEvent<WorkflowTransportResumeEventDetail>(WORKFLOW_TRANSPORT_RESUME_EVENT, { detail: canonical }));
}

export function readWorkflowTransportResumeEvent(event: Event): WorkflowTransportResumeEventDetail | undefined {
  if (event.type !== WORKFLOW_TRANSPORT_RESUME_EVENT || !(event instanceof CustomEvent)) return undefined;
  return canonicalWorkflowTransportResumeDetail(event.detail);
}

function canonicalWorkflowTransportResumeDetail(value: unknown): WorkflowTransportResumeEventDetail | undefined {
  if (!value || typeof value !== "object" || Array.isArray(value)) return undefined;
  const source = value as Record<string, unknown>;
  const keys = Object.keys(source).sort().join("\0");
  if (keys !== ["schemaVersion", "workflowId", "threadId", "sessionId", "turnId", "approvalId"].sort().join("\0")
    || source.schemaVersion !== WORKFLOW_TRANSPORT_RESUME_SCHEMA_VERSION) return undefined;
  const fields = [source.workflowId, source.threadId, source.sessionId, source.turnId, source.approvalId];
  if (!fields.every(canonicalTransportIdentifier)) return undefined;
  return {
    schemaVersion: WORKFLOW_TRANSPORT_RESUME_SCHEMA_VERSION,
    workflowId: source.workflowId as string,
    threadId: source.threadId as string,
    sessionId: source.sessionId as string,
    turnId: source.turnId as string,
    approvalId: source.approvalId as string,
  };
}

function canonicalTransportIdentifier(value: unknown): value is string {
  return typeof value === "string" && value.length > 0 && value === value.trim() && !/[\0\r\n]/.test(value);
}

function clearWorkflowTransportSession(storageKey: string) {
  try {
    window.sessionStorage.removeItem(storageKey);
  } catch {
    // Ignore unavailable browser storage.
  }
}

function shouldResumeWorkflowTransport(state: AiopsTransportState) {
  return state.status === "working" || state.status === "blocked" || Object.keys(state.runtimeLiveness?.activeTurns || {}).length > 0;
}
