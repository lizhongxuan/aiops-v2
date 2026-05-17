export function RCAHypothesisRank({ hypotheses }: { hypotheses: Array<Record<string, unknown>> }) {
  if (!hypotheses.length) {
    return null;
  }

  return (
    <ol className="space-y-2">
      {hypotheses.map((hypothesis, index) => {
        const support = stringArray(hypothesis.supportingEvidenceRefs);
        const contradictions = stringArray(hypothesis.contradictingEvidenceRefs);
        const missing = stringArray(hypothesis.missingEvidence);

        return (
          <li key={display(hypothesis.id) || index} className="border-l-2 border-slate-200 pl-3">
            <div className="flex items-center justify-between gap-3">
              <span className="font-medium text-slate-900">{display(hypothesis.titleZh) || display(hypothesis.title) || "候选假设"}</span>
              <span className="shrink-0 text-xs text-slate-500">{Math.round(confidence(hypothesis.confidence) * 100)}%</span>
            </div>
            {support.length ? <p className="mt-1 text-xs text-slate-500">支持证据：{support.join(", ")}</p> : null}
            {contradictions.length ? <p className="mt-1 text-xs text-amber-700">矛盾证据：{contradictions.join(", ")}</p> : null}
            {missing.length ? <p className="mt-1 text-xs text-slate-500">缺失证据：{missing.join("；")}</p> : null}
          </li>
        );
      })}
    </ol>
  );
}

function confidence(value: unknown): number {
  return typeof value === "number" && Number.isFinite(value) ? Math.max(0, Math.min(1, value)) : 0;
}

function stringArray(value: unknown): string[] {
  return Array.isArray(value) ? value.map(display).filter(Boolean) : [];
}

function display(value: unknown): string {
  return typeof value === "string" || typeof value === "number" ? String(value) : "";
}
