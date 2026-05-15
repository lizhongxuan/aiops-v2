import { useMemo, useState } from "react";

import { resolveUiFixtureState } from "@/lib/uiFixtureRuntime";
import { ChatTransportProvider } from "@/transport/ChatTransportProvider";
import type { AiopsTransportState } from "@/transport/aiopsTransportTypes";

import { AiopsComposer } from "./components/AiopsComposer";
import { AiopsThread } from "./components/AiopsThread";
import { SessionContextBar } from "./components/SessionContextBar";

type ChatPageProps = {
  initialState?: AiopsTransportState;
  threadId?: string;
};

export function ChatPage({ initialState, threadId = "default" }: ChatPageProps) {
  const resolvedInitialState = useMemo(() => initialState ?? resolveUiFixtureState() ?? undefined, [initialState]);
  const [activeThreadId, setActiveThreadId] = useState(resolvedInitialState?.threadId || threadId);
  const [activeInitialState, setActiveInitialState] = useState(resolvedInitialState);
  const [activeAutoResume, setActiveAutoResume] = useState(false);

  return (
    <section className="flex h-full min-h-0 flex-col overflow-hidden text-zinc-950">
      <SessionContextBar
        kind="single_host"
        title="单机会话"
        newSessionLabel="新建会话"
        description="选择单台主机进行 AI Chat；消息发送仍走 AssistantTransport。"
        activeThreadId={activeThreadId}
        terminalHref="/terminal/server-local"
        onThreadChange={(nextThreadId, nextInitialState, autoResume) => {
          setActiveThreadId(nextThreadId);
          setActiveInitialState(nextInitialState);
          setActiveAutoResume(Boolean(autoResume));
        }}
      >
        <ChatTransportProvider
          key={activeThreadId}
          autoResume={activeAutoResume}
          initialState={activeInitialState}
          threadId={activeThreadId}
        >
          <div className="grid h-full min-h-0 flex-1 grid-rows-[minmax(0,1fr)_auto] overflow-hidden bg-white">
            <AiopsThread />
            <AiopsComposer variant="chat" />
          </div>
        </ChatTransportProvider>
      </SessionContextBar>
    </section>
  );
}
