import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Field } from "@/pages/settingsComponents";

export function ManagedResourceForm({
  busy,
  onSubmit,
}: {
  busy?: string;
  onSubmit: (payload: unknown) => void;
}) {
  const [id, setId] = useState("postgres-prod-primary");
  const [kind, setKind] = useState("postgresql");
  const [primaryHost, setPrimaryHost] = useState("120.77.239.90");
  const [secondaryHost, setSecondaryHost] = useState("120.77.239.90");
  const [monitorCredentialRef, setMonitorCredentialRef] = useState("pg-monitor-ref");
  const [repairCredentialRef, setRepairCredentialRef] = useState("pg-repair-ref");

  return (
    <form
      className="grid gap-3"
      onSubmit={(event) => {
        event.preventDefault();
        onSubmit({
          id,
          name: id,
          kind,
          provider: "self-managed",
          environment: "production",
          endpoints: [
            {
              id: `${id}-primary`,
              role: "primary",
              host: primaryHost,
              port: defaultPort(kind),
              serviceName: kind,
            },
            {
              id: `${id}-secondary-a`,
              role: "replica",
              host: secondaryHost,
              port: defaultPort(kind),
              serviceName: kind,
            },
          ],
          credentialRefs: {
            monitor: monitorCredentialRef,
            repair: repairCredentialRef,
          },
          monitorCredentialRef,
          repairCredentialRef,
          tags: ["production", kind],
        });
      }}
    >
      <div className="grid gap-3 md:grid-cols-4">
        <Field label="资源 ID">
          <Input value={id} onChange={(event) => setId(event.target.value)} />
        </Field>
        <Field label="资源类型">
          <Input value={kind} onChange={(event) => setKind(event.target.value)} placeholder="postgresql / redis / mysql" />
        </Field>
        <Field label="主节点 Host">
          <Input value={primaryHost} onChange={(event) => setPrimaryHost(event.target.value)} />
        </Field>
        <Field label="副本/节点 Host">
          <Input value={secondaryHost} onChange={(event) => setSecondaryHost(event.target.value)} />
        </Field>
      </div>
      <div className="grid gap-3 md:grid-cols-2">
        <Field label="监控凭据引用">
          <Input value={monitorCredentialRef} onChange={(event) => setMonitorCredentialRef(event.target.value)} />
        </Field>
        <Field label="修复凭据引用">
          <Input value={repairCredentialRef} onChange={(event) => setRepairCredentialRef(event.target.value)} />
        </Field>
      </div>
      <div>
        <Button type="submit" size="sm" disabled={Boolean(busy)} data-testid="operator-runtime-resource-save">
          保存受管资源
        </Button>
      </div>
    </form>
  );
}

function defaultPort(kind: string) {
  const normalized = kind.trim().toLowerCase();
  if (normalized === "redis") return 6379;
  if (normalized === "mysql") return 3306;
  if (normalized === "kafka") return 9092;
  if (normalized === "elasticsearch") return 9200;
  return 5432;
}
