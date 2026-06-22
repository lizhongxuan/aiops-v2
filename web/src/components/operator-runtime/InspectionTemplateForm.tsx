import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Field } from "@/pages/settingsComponents";

const defaultPrimarySQL =
  "select application_name, client_addr, state, sync_state, sent_lsn, write_lsn, flush_lsn, replay_lsn from pg_stat_replication";
const defaultReplicaSQL =
  "select pg_is_in_recovery() as in_recovery, true as receiver_running, 0 as replay_lag_seconds, 0 as replay_lag_bytes, pg_last_wal_receive_lsn() as receive_lsn, pg_last_wal_replay_lsn() as replay_lsn";
const defaultFields = [
  "replica.reachable:bool",
  "replica.inRecovery:bool",
  "replica.receiverRunning:bool",
  "replica.replayLagSeconds:number",
  "replica.replayLagBytes:number",
  "replica.receiveLsn:string",
  "replica.replayLsn:string",
].join("\n");

export function InspectionTemplateForm({
  busy,
  onSubmit,
}: {
  busy?: string;
  onSubmit: (payload: unknown) => void;
}) {
  const [id, setId] = useState("postgres.replication.basic.v1");
  const [name, setName] = useState("PG 主从复制基础巡检");
  const [intervalSeconds, setIntervalSeconds] = useState("60");
  const [primarySql, setPrimarySql] = useState(defaultPrimarySQL);
  const [replicaSql, setReplicaSql] = useState(defaultReplicaSQL);
  const [outputFields, setOutputFields] = useState(defaultFields);

  return (
    <form
      className="grid gap-3"
      onSubmit={(event) => {
        event.preventDefault();
        onSubmit({
          id,
          name,
          objectKind: "postgres_replication",
          intervalSeconds: Number(intervalSeconds) || 60,
          primarySql,
          replicaSql,
          outputFields: parseOutputFields(outputFields),
        });
      }}
    >
      <div className="grid gap-3 md:grid-cols-[1fr_1fr_120px]">
        <Field label="巡检模板 ID">
          <Input value={id} onChange={(event) => setId(event.target.value)} />
        </Field>
        <Field label="名称">
          <Input value={name} onChange={(event) => setName(event.target.value)} />
        </Field>
        <Field label="间隔秒数">
          <Input value={intervalSeconds} onChange={(event) => setIntervalSeconds(event.target.value)} />
        </Field>
      </div>
      <Field label="Primary SQL">
        <Textarea rows={3} value={primarySql} onChange={(event) => setPrimarySql(event.target.value)} />
      </Field>
      <Field label="Replica SQL">
        <Textarea rows={3} value={replicaSql} onChange={(event) => setReplicaSql(event.target.value)} />
      </Field>
      <Field label="输出字段">
        <Textarea rows={5} value={outputFields} onChange={(event) => setOutputFields(event.target.value)} />
      </Field>
      <div>
        <Button type="submit" size="sm" disabled={Boolean(busy)} data-testid="operator-runtime-template-save">
          保存巡检模板
        </Button>
      </div>
    </form>
  );
}

function parseOutputFields(text: string) {
  return text
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean)
    .map((line) => {
      const [name, type = "string"] = line.split(":").map((part) => part.trim());
      return { name, type };
    });
}
