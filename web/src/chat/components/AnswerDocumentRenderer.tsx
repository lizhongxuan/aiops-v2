import { LineChart } from "lucide-react";

import { AgentUiArtifactPart } from "@/components/chat/AgentUiArtifactPart";
import { buildAnswerDocument } from "@/chat/answerDocument/answerDocumentBuilder";
import type { ArtifactSlot } from "@/chat/answerDocument/types";
import type { AiopsTransportAgentUiArtifact } from "@/transport/aiopsTransportTypes";

import { MessageMarkdown } from "./MessageMarkdown";

type AnswerDocumentRendererProps = {
  finalText: string;
  artifacts: AiopsTransportAgentUiArtifact[];
  deferredArtifacts: AiopsTransportAgentUiArtifact[];
};

export function AnswerDocumentRenderer({ finalText, artifacts, deferredArtifacts }: AnswerDocumentRendererProps) {
  const nodes = buildAnswerDocument({ finalText, artifacts, deferredArtifacts });
  if (!nodes.length) {
    return null;
  }

  return (
    <div
      className="max-w-none space-y-3 py-1 text-[15px] leading-7 text-slate-950"
      data-testid="aiops-answer-document"
    >
      <div data-testid="aiops-final-text">
      {nodes.map((node) => {
        if (node.type === "section") {
          return (
            <div
              key={node.section.id}
              data-testid={`aiops-answer-section-${node.section.kind}`}
            >
              {node.section.title ? <div className="font-semibold text-slate-950">{node.section.title}：</div> : null}
              <MessageMarkdown text={node.section.markdown} />
            </div>
          );
        }
        return <ArtifactSlotView key={node.slot.id} slot={node.slot} />;
      })}
      </div>
    </div>
  );
}

function ArtifactSlotView({ slot }: { slot: ArtifactSlot }) {
  if (slot.state === "deferred") {
    return <DeferredCorootChartNotice />;
  }
  return slot.artifact ? <AgentUiArtifactPart artifact={slot.artifact} /> : null;
}

function DeferredCorootChartNotice() {
  return (
    <div
      data-testid="coroot-chart-deferred-notice"
      className="flex items-center gap-2 rounded-lg border border-slate-200 bg-slate-50 px-3 py-2 text-sm leading-6 text-slate-600"
    >
      <LineChart className="h-4 w-4 shrink-0 text-slate-400" />
      <span>已生成 Coroot 图表，分析完成后展开</span>
    </div>
  );
}
