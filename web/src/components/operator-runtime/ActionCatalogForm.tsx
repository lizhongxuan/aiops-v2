import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Field, SelectField } from "@/pages/settingsComponents";

export function ActionCatalogForm({
  busy,
  onSubmit,
}: {
  busy?: string;
  onSubmit: (payload: unknown) => void;
}) {
  const [id, setId] = useState("postgres.replication.reconnect_replica.v1");
  const [displayName, setDisplayName] = useState("重连 PG 从库复制");
  const [riskLevel, setRiskLevel] = useState("medium");

  return (
    <form
      className="grid gap-3"
      onSubmit={(event) => {
        event.preventDefault();
        onSubmit({
          id,
          displayName,
          riskLevel,
          targetKind: "postgres_replica",
          inputSchema: { resourceId: "string", primaryHost: "string", replicaHost: "string" },
          steps: [
            { id: "check_service", kind: "check_service" },
            { id: "reload_config", kind: "reload_config" },
            { id: "restart_service", kind: "restart_service", requiresApproval: true },
          ],
          confirmationRequiredSteps: ["restart_service"],
        });
      }}
    >
      <div className="grid gap-3 md:grid-cols-[1fr_1fr_160px]">
        <Field label="动作 ID">
          <Input value={id} onChange={(event) => setId(event.target.value)} />
        </Field>
        <Field label="名称">
          <Input value={displayName} onChange={(event) => setDisplayName(event.target.value)} />
        </Field>
        <Field label="风险等级">
          <SelectField
            aria-label="风险等级"
            value={riskLevel}
            onChange={setRiskLevel}
            options={[
              { label: "low", value: "low" },
              { label: "medium", value: "medium" },
              { label: "high", value: "high" },
            ]}
          />
        </Field>
      </div>
      <div>
        <Button type="submit" size="sm" disabled={Boolean(busy)} data-testid="operator-runtime-action-save">
          保存动作
        </Button>
      </div>
    </form>
  );
}
