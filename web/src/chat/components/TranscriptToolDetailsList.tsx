export function TranscriptToolDetailsList({
  text,
  childSummaries = [],
}: {
  text?: string;
  childSummaries?: string[];
}) {
  const lines = (text || "")
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean);
  const visibleLines = lines.length > 0 ? lines : childSummaries.map((line) => line.trim()).filter(Boolean);

  if (visibleLines.length === 0) {
    return null;
  }

  return (
    <div className="mt-2 space-y-2 pl-8 text-[14px] leading-6 text-slate-400">
      {visibleLines.map((line, index) => (
        <div key={`${index}-${line}`} data-testid="aiops-tool-detail-row" className="truncate text-slate-400">
          {line}
        </div>
      ))}
    </div>
  );
}
