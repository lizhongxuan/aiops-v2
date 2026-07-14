import "@/assistant.config";

import {
  AssistantRuntimeProvider,
  useAssistantApi,
  useAssistantState,
  useAssistantTransportState,
  useAssistantTransportRuntime,
} from "@assistant-ui/react";
import type { PropsWithChildren } from "react";
import { useEffect, useMemo, useRef } from "react";

import { canonicalizeTransportTranscript, createAiopsTransportConverter } from "./aiopsTransportConverter";
import {
  markAiopsTransportCanceled,
  markAiopsTransportFailed,
  normalizeAiopsTransportState,
} from "./aiopsTransportRuntime";
import { setCachedAiopsTransportState, type AiopsTransportCacheScope } from "./aiopsTransportStateCache";
import type { AiopsTransportState } from "./aiopsTransportTypes";
import { toUserFacingTransportErrorMessage } from "./transportErrorMessage";

type ChatTransportProviderProps = PropsWithChildren<{
  autoResume?: boolean;
  cacheScope?: AiopsTransportCacheScope;
  initialState?: AiopsTransportState;
  threadId?: string;
}>;

export function ChatTransportProvider({
  autoResume = false,
  cacheScope,
  children,
  initialState,
  threadId = "default",
}: ChatTransportProviderProps) {
  const initialTransportState = useMemo(
    () => canonicalizeTransportTranscript(normalizeAiopsTransportState(initialState, threadId)),
    [initialState, threadId],
  );
  const converter = useMemo(() => createAiopsTransportConverter(), []);
  const runtime = useAssistantTransportRuntime<AiopsTransportState>({
    initialState: initialTransportState,
    api: "/api/v1/assistant/transport",
    resumeApi: "/api/v1/assistant/resume",
    protocol: "data-stream",
    converter,
    headers: {
      "Content-Type": "application/json",
      Accept: "text/plain",
    },
    onError(error, { updateState }) {
      updateState((state) => markAiopsTransportFailed(state, toUserFacingTransportErrorMessage(error)));
    },
    onCancel({ updateState, error }) {
      if (error) {
        return;
      }
      updateState((state) => markAiopsTransportCanceled(state, "client canceled"));
    },
  });

  return (
    <AssistantRuntimeProvider runtime={runtime}>
      <AssistantTransportReady threadId={threadId} shouldAutoResume={autoResume}>
        <AiopsTransportCacheWriter cacheScope={cacheScope} />
        {children}
      </AssistantTransportReady>
    </AssistantRuntimeProvider>
  );
}

function AiopsTransportCacheWriter({ cacheScope }: { cacheScope?: AiopsTransportCacheScope }) {
  const state = useAssistantTransportState() as AiopsTransportState | undefined;

  useEffect(() => {
    if (!cacheScope || !state?.sessionId) {
      return;
    }
    setCachedAiopsTransportState(cacheScope, state);
  }, [cacheScope, state]);

  return null;
}

function AssistantTransportReady({
  children,
  threadId,
  shouldAutoResume,
}: PropsWithChildren<{ threadId: string; shouldAutoResume: boolean }>) {
  const api = useAssistantApi();
  const resumedThreadRef = useRef("");
  const ready = useAssistantState(({ thread }) => {
    const extras = thread.extras as { state?: unknown } | undefined;
    return !!extras && typeof extras === "object" && "state" in extras;
  });

  useEffect(() => {
    if (!ready || !shouldAutoResume || resumedThreadRef.current === threadId) {
      return;
    }
    resumedThreadRef.current = threadId;
    api.thread().unstable_resumeRun({});
  }, [api, ready, shouldAutoResume, threadId]);

  if (!ready) {
    return <div className="flex min-h-[240px] items-center justify-center text-sm text-zinc-500">Loading chat...</div>;
  }
  return <>{children}</>;
}
