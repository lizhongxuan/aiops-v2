import { useMemo, useState } from "react";

import type { OperatorRuntimeItem } from "@/api/operatorRuntime";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Field, SelectField } from "@/pages/settingsComponents";

import { itemId } from "./operatorRuntimeModels";

export function WorkflowBindingForm({
  actions,
  busy,
  onSubmit,
}: {
  actions: OperatorRuntimeItem[];
  busy?: string;
  onSubmit: (payload: unknown) => void;
}) {
  const actionOptions = useMemo(
    () =>
      actions.length
        ? actions.map((item) => ({ label: itemId(item), value: itemId(item) }))
        : [{ label: "postgres.replication.reconnect_replica.v1", value: "postgres.replication.reconnect_replica.v1" }],
    [actions],
  );
  const [id, setId] = useState("builtin.postgres.replication_reconnect_replica.v1");
  const [actionRef, setActionRef] = useState(actionOptions[0]?.value || "postgres.replication.reconnect_replica.v1");
  const [workflowRef, setWorkflowRef] = useState("builtin.postgres.replication_reconnect_replica.v1");
  const [maxReplayLagSeconds, setMaxReplayLagSeconds] = useState("10");

  return (
    <form
      className="grid gap-3"
      onSubmit={(event) => {
        event.preventDefault();
        onSubmit({
          id,
          actionRef,
          workflowRef,
          workflowVersion: "v1",
          capabilities: ["preflight", "act", "verify"],
          inputMapping: {
            resourceId: "guard.resourceId",
            primaryHost: "resource.endpoint.primary.host",
            replicaHost: "event.target.host",
          },
          verifyPolicy: {
            receiverRunningRequired: true,
            maxReplayLagSeconds: Number(maxReplayLagSeconds) || 10,
            timeoutSeconds: 300,
            intervalSeconds: 30,
          },
        });
      }}
    >
      <div className="grid gap-3 md:grid-cols-[1fr_1fr_1fr_160px]">
        <Field label="绑定 ID">
          <Input value={id} onChange={(event) => setId(event.target.value)} />
        </Field>
        <Field label="动作">
          <SelectField aria-label="动作" value={actionRef} onChange={setActionRef} options={actionOptions} />
        </Field>
        <Field label="Workflow">
          <Input value={workflowRef} onChange={(event) => setWorkflowRef(event.target.value)} />
        </Field>
        <Field label="恢复延迟阈值">
          <Input value={maxReplayLagSeconds} onChange={(event) => setMaxReplayLagSeconds(event.target.value)} />
        </Field>
      </div>
      <div>
        <Button type="submit" size="sm" disabled={Boolean(busy)} data-testid="operator-runtime-binding-save">
          保存 Workflow 绑定
        </Button>
      </div>
    </form>
  );
}
