export function RCAEvidenceList({
  evidenceRefs,
  rawRefs,
  restricted,
}: {
  evidenceRefs: string[];
  rawRefs: Array<Record<string, unknown>>;
  restricted?: boolean;
}) {
  if (restricted) {
    return <p className="text-xs text-amber-800">权限受限，仅展示 RCA 摘要和可见结论。</p>;
  }

  return (
    <div className="grid gap-2 text-xs text-slate-600">
      <div>Evidence：{evidenceRefs.length ? evidenceRefs.join(", ") : "未提供"}</div>
      {rawRefs.map((ref, index) => {
        const uri = display(ref.uri);
        return uri ? (
          <div key={`${uri}-${index}`} className="break-all font-mono text-[11px] text-slate-500">
            {uri}
          </div>
        ) : null;
      })}
    </div>
  );
}

function display(value: unknown): string {
  return typeof value === "string" || typeof value === "number" ? String(value) : "";
}
