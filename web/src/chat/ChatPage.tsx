import { useState } from "react";

import { ChatTransportProvider } from "@/transport/ChatTransportProvider";
import type { AiopsTransportExperiencePackSuggestion, AiopsTransportState } from "@/transport/aiopsTransportTypes";

import { AiopsComposer } from "./components/AiopsComposer";
import { AiopsThread } from "./components/AiopsThread";
import { ExperiencePackSuggestionConfirmation } from "./components/ExperiencePackChatArtifacts";
import { SessionContextBar } from "./components/SessionContextBar";

type ChatPageProps = {
  initialState?: AiopsTransportState;
  threadId?: string;
};

export function ChatPage({ initialState, threadId = "default" }: ChatPageProps) {
  const [activeThreadId, setActiveThreadId] = useState(threadId);
  const [activeInitialState, setActiveInitialState] = useState(initialState);
  const [activeAutoResume, setActiveAutoResume] = useState(false);
  const [draftText, setDraftText] = useState("");
  const [selectedExperienceSuggestion, setSelectedExperienceSuggestion] = useState<AiopsTransportExperiencePackSuggestion | null>(null);

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
          setDraftText("");
          setSelectedExperienceSuggestion(null);
        }}
      >
        <ChatTransportProvider
          key={activeThreadId}
          autoResume={activeAutoResume}
          initialState={activeInitialState}
          threadId={activeThreadId}
        >
          <div className="grid h-full min-h-0 flex-1 grid-rows-[minmax(0,1fr)_auto] overflow-hidden bg-white">
            <AiopsThread
              draftText={draftText}
              onSelectExperienceSuggestion={setSelectedExperienceSuggestion}
            />
            {selectedExperienceSuggestion ? (
              <ExperiencePackSuggestionConfirmation
                suggestion={selectedExperienceSuggestion}
                onCancel={() => setSelectedExperienceSuggestion(null)}
              />
            ) : (
              <AiopsComposer
                variant="chat"
                onDraftTextChange={setDraftText}
                onMessageSubmitted={() => setDraftText("")}
              />
            )}
          </div>
        </ChatTransportProvider>
      </SessionContextBar>
    </section>
  );
}
