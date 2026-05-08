import type { AiopsTranscriptBlock } from "./AiopsTranscript";

export function TranscriptThinkingBlock({ block }: { block: AiopsTranscriptBlock }) {
  if (!block.thinking) {
    return null;
  }

  return <div className="px-1 text-[15px] leading-7 text-slate-400">正在思考</div>;
}
