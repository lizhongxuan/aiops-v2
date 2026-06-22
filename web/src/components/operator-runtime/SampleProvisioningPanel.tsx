import { Database, GitBranch, ListChecks, Play, ShieldCheck, Wrench } from "lucide-react";
import type { ComponentType } from "react";
import { useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Field, ToneBadge } from "@/pages/settingsComponents";

import { createSamplePayloads } from "./operatorRuntimeModels";

export type ProvisionStepKey = "resource" | "inspectionTemplate" | "problemType" | "action" | "workflowBinding" | "rule";

const steps: Array<{ key: ProvisionStepKey; label: string; icon: ComponentType<{ className?: string }> }> = [
  { key: "resource", label: "创建受管资源", icon: Database },
  { key: "inspectionTemplate", label: "创建巡检模板", icon: ListChecks },
  { key: "problemType", label: "创建问题类型", icon: ShieldCheck },
  { key: "action", label: "创建动作", icon: Wrench },
  { key: "workflowBinding", label: "创建 Workflow 绑定", icon: GitBranch },
  { key: "rule", label: "创建并启用守护规则", icon: Play },
];

export function SampleProvisioningPanel({
  busy,
  completed,
  onCreate,
}: {
  busy?: string;
  completed: Record<string, boolean>;
  onCreate: (key: ProvisionStepKey, payload: unknown) => void;
}) {
  const [host, setHost] = useState("120.77.239.90");
  const [resourceName, setResourceName] = useState("postgres-prod-primary");
  const payloads = useMemo(() => createSamplePayloads(host, resourceName), [resourceName, host]);

  function payloadFor(key: ProvisionStepKey) {
    if (key === "rule") return undefined;
    return payloads[key];
  }

  return (
    <Card className="rounded-lg bg-white" size="sm">
      <CardHeader className="pb-0">
        <CardTitle>从内置示例快速开始</CardTitle>
        <CardDescription>复制 PostgreSQL 主从复制守护示例，最后创建并启用守护规则；这只是通用 Operator 的一个模板。</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-3">
        <div className="grid gap-3 md:grid-cols-[1fr_220px]">
          <Field label="资源名称">
            <Input
              aria-label="managed resource name"
              value={resourceName}
              onChange={(event) => setResourceName(event.target.value)}
            />
          </Field>
          <Field label="Host">
            <Input
              aria-label="resource host"
              data-testid="operator-runtime-host"
              value={host}
              onChange={(event) => setHost(event.target.value)}
            />
          </Field>
        </div>
        <div className="grid gap-2 md:grid-cols-2 xl:grid-cols-3">
          {steps.map((step) => {
            const Icon = step.icon;
            return (
              <Button
                key={step.key}
                type="button"
                variant={completed[step.key] ? "secondary" : "outline"}
                className="h-10 justify-start"
                data-testid={`operator-runtime-${step.key}`}
                disabled={Boolean(busy)}
                onClick={() => onCreate(step.key, payloadFor(step.key))}
              >
                <Icon className="h-4 w-4" />
                {step.label}
                {completed[step.key] ? <ToneBadge tone="success">done</ToneBadge> : null}
              </Button>
            );
          })}
        </div>
      </CardContent>
    </Card>
  );
}
