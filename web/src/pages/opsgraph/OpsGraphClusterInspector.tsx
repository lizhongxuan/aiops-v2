import type { OpsGraphNode, OpsGraphRelationship } from "./opsGraphTypes";
import { summarizeClusterDeployment } from "./opsGraphViewModel";

export function OpsGraphClusterInspector({
  cluster,
  nodes,
  relationships,
}: {
  cluster: OpsGraphNode;
  nodes: OpsGraphNode[];
  relationships: OpsGraphRelationship[];
}) {
  const summary = summarizeClusterDeployment(cluster.id, nodes, relationships);
  return (
    <section className="grid gap-2 rounded-lg border bg-white p-3 text-sm">
      <div className="font-medium text-slate-950">{cluster.name}</div>
      <div className="text-slate-600">{summary.deploymentLabel}</div>
      {summary.badges.length ? <div className="text-xs text-slate-500">{summary.badges.join(" / ")}</div> : null}
    </section>
  );
}
