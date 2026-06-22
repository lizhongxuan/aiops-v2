import { useEffect, useMemo, useState } from "react";

import type { OperatorRuntimeItem } from "@/api/operatorRuntime";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Field, SelectField } from "@/pages/settingsComponents";

import { itemId } from "./operatorRuntimeModels";

export function GuardRuleForm({
  resources,
  inspectionTemplates,
  problemTypes,
  actions,
  workflowBindings,
  busy,
  onSubmit,
}: {
  resources: OperatorRuntimeItem[];
  inspectionTemplates: OperatorRuntimeItem[];
  problemTypes: OperatorRuntimeItem[];
  actions: OperatorRuntimeItem[];
  workflowBindings: OperatorRuntimeItem[];
  busy?: string;
  onSubmit: (payload: unknown) => void;
}) {
  const resourceOptions = useOptions(resources);
  const templateOptions = useOptions(inspectionTemplates);
  const problemOptions = useOptions(problemTypes);
  const actionOptions = useOptions(actions);
  const bindingOptions = useOptions(workflowBindings);
  const [id, setId] = useState("pg-runtime-autoheal-rule");
  const [scheduleSeconds, setScheduleSeconds] = useState("60");
  const [cooldownSeconds, setCooldownSeconds] = useState("1800");
  const [resourceRef, setResourceRef] = useState(resourceOptions[0]?.value || "");
  const [templateRef, setTemplateRef] = useState(templateOptions[0]?.value || "");
  const [problemTypeRef, setProblemTypeRef] = useState(problemOptions[0]?.value || "");
  const [actionRef, setActionRef] = useState(actionOptions[0]?.value || "");
  const [workflowBindingRef, setWorkflowBindingRef] = useState(bindingOptions[0]?.value || "");

  useFirstOption(resourceOptions, resourceRef, setResourceRef);
  useFirstOption(templateOptions, templateRef, setTemplateRef);
  useFirstOption(problemOptions, problemTypeRef, setProblemTypeRef);
  useFirstOption(actionOptions, actionRef, setActionRef);
  useFirstOption(bindingOptions, workflowBindingRef, setWorkflowBindingRef);

  return (
    <form
      className="grid gap-3"
      onSubmit={(event) => {
        event.preventDefault();
        onSubmit({
          id,
          name: id,
          enabled: false,
          resourceRef,
          clusterRef: resourceRef,
          templateRef,
          problemTypeRefs: [problemTypeRef].filter(Boolean),
          actionRefs: [actionRef].filter(Boolean),
          workflowBindingRefs: [workflowBindingRef].filter(Boolean),
          scheduleSeconds: Number(scheduleSeconds) || 60,
          cooldownSeconds: Number(cooldownSeconds) || 1800,
          maxConcurrency: 1,
          disableAfterConsecutiveFailures: 3,
          policy: {
            maxAutoRisk: "medium",
            requireApprovalStepKinds: ["restart_service"],
          },
        });
      }}
    >
      <div className="grid gap-3 md:grid-cols-[1fr_130px_130px]">
        <Field label="守护规则 ID">
          <Input value={id} onChange={(event) => setId(event.target.value)} />
        </Field>
        <Field label="巡检间隔">
          <Input value={scheduleSeconds} onChange={(event) => setScheduleSeconds(event.target.value)} />
        </Field>
        <Field label="冷却时间">
          <Input value={cooldownSeconds} onChange={(event) => setCooldownSeconds(event.target.value)} />
        </Field>
      </div>
      <div className="grid gap-3 md:grid-cols-3">
        <Field label="受管资源">
          <SelectField aria-label="受管资源" value={resourceRef} onChange={setResourceRef} options={resourceOptions} />
        </Field>
        <Field label="巡检模板">
          <SelectField aria-label="巡检模板" value={templateRef} onChange={setTemplateRef} options={templateOptions} />
        </Field>
        <Field label="问题类型">
          <SelectField aria-label="问题类型" value={problemTypeRef} onChange={setProblemTypeRef} options={problemOptions} />
        </Field>
        <Field label="推荐动作">
          <SelectField aria-label="推荐动作" value={actionRef} onChange={setActionRef} options={actionOptions} />
        </Field>
        <Field label="Workflow 绑定">
          <SelectField aria-label="Workflow 绑定" value={workflowBindingRef} onChange={setWorkflowBindingRef} options={bindingOptions} />
        </Field>
      </div>
      <div>
        <Button type="submit" size="sm" disabled={Boolean(busy)} data-testid="operator-runtime-rule-save">
          保存并启用守护规则
        </Button>
      </div>
    </form>
  );
}

function useOptions(items: OperatorRuntimeItem[]) {
  return useMemo(() => items.map((item) => ({ label: itemId(item), value: itemId(item) })), [items]);
}

function useFirstOption(options: Array<{ value: string }>, current: string, setValue: (value: string) => void) {
  useEffect(() => {
    if (!current && options[0]?.value) {
      setValue(options[0].value);
    }
  }, [current, options, setValue]);
}
