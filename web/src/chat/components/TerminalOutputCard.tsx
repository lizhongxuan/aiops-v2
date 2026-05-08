import type { AiopsTranscriptBlock } from "./AiopsTranscript";

export function TerminalOutputCard({ block }: { block: AiopsTranscriptBlock }) {
  const tool = block.tool;
  if (!tool) {
    return null;
  }

  const command = tool.command || tool.inputSummary || tool.summary || tool.title || "command";
  const output = terminalOutputText(tool.output);
  const statusLabel = terminalStatusLabel(tool.status, tool.exitCode);

  return (
    <div data-testid={`aiops-terminal-card-${block.id}`} className="mt-2 rounded-xl bg-slate-100 px-4 py-3 text-slate-500">
      <div className="text-[13px] leading-5 text-slate-500">{tool.title || "Shell"}</div>
      <div className="mt-2 whitespace-pre-wrap break-words font-mono text-[13px] leading-6 text-slate-950">
        $ {command}
      </div>
      <pre className="mt-3 max-h-60 overflow-auto rounded-md bg-slate-100 font-mono text-[13px] leading-6 whitespace-pre-wrap text-slate-500">
        {output || " "}
      </pre>
      <div className="mt-2 flex items-center justify-between gap-3 text-[13px] leading-5 text-slate-500">
        <span>{tool.output?.truncated ? "输出已截断" : ""}</span>
        <span>{statusLabel}</span>
      </div>
    </div>
  );
}

function terminalOutputText(output: NonNullable<NonNullable<AiopsTranscriptBlock["tool"]>["output"]> | undefined) {
  if (!output) {
    return "";
  }
  if (output.text) {
    return output.text;
  }
  return `${output.stdout || ""}${output.stderr || ""}`;
}

function terminalStatusLabel(status: NonNullable<AiopsTranscriptBlock["tool"]>["status"], exitCode?: number) {
  switch (status) {
    case "queued":
      return "排队中";
    case "running":
      return "运行中";
    case "completed":
      return exitCode === undefined ? "已完成" : `退出 ${exitCode}`;
    case "failed":
      return exitCode === undefined ? "失败" : `失败 ${exitCode}`;
    case "blocked":
      return "等待审批";
    case "rejected":
      return "已拒绝";
    default:
      return status;
  }
}
