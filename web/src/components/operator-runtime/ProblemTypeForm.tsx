import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Field, SelectField } from "@/pages/settingsComponents";

import { createProblemTypePresetPayload } from "./operatorRuntimeModels";
import type { ProblemPreset } from "./operatorRuntimeModels";

export function ProblemTypeForm({
  busy,
  onSubmit,
}: {
  busy?: string;
  onSubmit: (payload: unknown) => void;
}) {
  const [preset, setPreset] = useState<ProblemPreset>("lag_high");
  const [draft, setDraft] = useState(() => createProblemTypePresetPayload("lag_high"));

  function applyPreset(nextPreset: string) {
    const typedPreset = nextPreset === "receiver_stopped" ? "receiver_stopped" : "lag_high";
    setPreset(typedPreset);
    setDraft(createProblemTypePresetPayload(typedPreset));
  }

  return (
    <form
      className="grid gap-3"
      onSubmit={(event) => {
        event.preventDefault();
        onSubmit(draft);
      }}
    >
      <div className="grid gap-3 md:grid-cols-[240px_1fr_160px_140px]">
        <Field label="问题类型">
          <SelectField
            aria-label="问题类型预设"
            data-testid="operator-runtime-problem-preset"
            value={preset}
            onChange={applyPreset}
            options={[
              { label: "复制延迟过高", value: "lag_high" },
              { label: "WAL receiver 停止", value: "receiver_stopped" },
            ]}
          />
        </Field>
        <Field label="名称">
          <Input value={String(draft.displayName)} onChange={(event) => setDraft({ ...draft, displayName: event.target.value })} />
        </Field>
        <Field label="严重级别">
          <SelectField
            aria-label="严重级别"
            value={String(draft.severity)}
            onChange={(value) => setDraft({ ...draft, severity: value })}
            options={[
              { label: "warning", value: "warning" },
              { label: "critical", value: "critical" },
            ]}
          />
        </Field>
        <Field label="持续秒数">
          <Input value={String(draft.forSeconds)} onChange={(event) => setDraft({ ...draft, forSeconds: Number(event.target.value) || 60 })} />
        </Field>
      </div>
      <div className="grid gap-3 md:grid-cols-[1fr_220px]">
        <Field label="问题 ID">
          <Input value={String(draft.id)} onChange={(event) => setDraft({ ...draft, id: event.target.value })} />
        </Field>
        <Field label="推荐动作">
          <Input
            value={String(draft.recommendedActionRefs[0] || "")}
            onChange={(event) => setDraft({ ...draft, recommendedActionRefs: [event.target.value] })}
          />
        </Field>
      </div>
      <div className="rounded-lg border bg-slate-50 p-3 text-xs text-slate-700">
        {draft.conditions.map((condition, index) => (
          <div key={`${condition.field}-${index}`}>
            {condition.field} {condition.operator} {condition.value.type === "bool" ? String(condition.value.bool) : condition.value.number}
          </div>
        ))}
      </div>
      <div>
        <Button type="submit" size="sm" disabled={Boolean(busy)} data-testid="operator-runtime-problem-save">
          保存问题类型
        </Button>
      </div>
    </form>
  );
}
