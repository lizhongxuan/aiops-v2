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

  const evidenceCount = evidenceRefs.length + rawRefs.length;
  if (!evidenceCount) {
    return null;
  }
  return (
    <p className="text-xs leading-5 text-slate-500">
      已纳入 {evidenceCount} 条 RCA 证据，关键内容已在结论、传播路径和指标段落中展示。
    </p>
  );
}
