import type { AiopsTranscriptBlock } from "./AiopsTranscript";
import { MessageMarkdown } from "./MessageMarkdown";

export function TranscriptTextBlock({ block }: { block: AiopsTranscriptBlock }) {
  const text = block.text?.text || "";
  if (!text.trim()) {
    return null;
  }

  return (
    <div className="max-w-none whitespace-pre-wrap break-words px-1 py-1 text-[15px] font-medium leading-7 tracking-normal text-slate-900">
      <MessageMarkdown text={text} />
    </div>
  );
}
