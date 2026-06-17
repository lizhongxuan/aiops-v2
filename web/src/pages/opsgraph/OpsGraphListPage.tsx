import { Plus, Trash2, Upload } from "lucide-react";
import { useEffect, useState } from "react";
import { Link, useNavigate } from "react-router-dom";

import { createOpsGraph, deleteOpsGraph, listOpsGraphs } from "@/api/opsgraph";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { ConfirmButton, EmptyState, SettingsPageFrame, StatusAlert, ToneBadge } from "@/pages/settingsComponents";

import type { OpsGraphSummary } from "./opsGraphTypes";

const DEFAULT_GRAPH_NAME = "新建图谱";

export function OpsGraphListPage() {
  const navigate = useNavigate();
  const [graphs, setGraphs] = useState<OpsGraphSummary[]>([]);
  const [error, setError] = useState("");
  const [creating, setCreating] = useState(false);
  const [deletingGraphId, setDeletingGraphId] = useState("");

  useEffect(() => {
    let active = true;
    void listOpsGraphs()
      .then((payload) => {
        if (!active) return;
        setGraphs(payload.graphs || payload.items || []);
      })
      .catch((loadError) => {
        if (!active) return;
        setError(loadError instanceof Error ? loadError.message : "加载 OpsGraph 失败");
      });
    return () => {
      active = false;
    };
  }, []);

  async function createBlankGraph() {
    setCreating(true);
    setError("");
    try {
      const payload = await createOpsGraph({
        id: `graph.manual-${Date.now()}`,
        name: nextDefaultGraphName(graphs),
        nodes: [],
        edges: [],
      });
      const graph = payload.graph || payload;
      navigate(`/opsgraph/${encodeURIComponent(graph.id)}`);
    } catch (createError) {
      setError(createError instanceof Error ? createError.message : "新建 OpsGraph 失败");
    } finally {
      setCreating(false);
    }
  }

  async function createExampleGraph() {
    setCreating(true);
    setError("");
    try {
      const payload = await createOpsGraph(exampleGraphPayload());
      const graph = payload.graph || payload;
      navigate(`/opsgraph/${encodeURIComponent(graph.id)}`);
    } catch (createError) {
      setError(createError instanceof Error ? createError.message : "创建示例 OpsGraph 失败");
    } finally {
      setCreating(false);
    }
  }

  async function removeGraph(graph: OpsGraphSummary) {
    setDeletingGraphId(graph.id);
    setError("");
    try {
      await deleteOpsGraph(graph.id);
      setGraphs((current) => current.filter((item) => item.id !== graph.id));
    } catch (deleteError) {
      setError(deleteError instanceof Error ? deleteError.message : "删除 OpsGraph 失败");
    } finally {
      setDeletingGraphId("");
    }
  }

  return (
    <SettingsPageFrame
      title="OpsGraph"
      description="手工维护服务、中间件、主机和 K8s 的最小运维关系图。"
      actions={(
        <div className="flex flex-wrap gap-2">
          <Button type="button" onClick={() => void createBlankGraph()} disabled={creating}><Plus />新建图谱</Button>
          <Button type="button" variant="outline" onClick={() => void createExampleGraph()} disabled={creating}><Upload />从示例开始</Button>
        </div>
      )}
    >
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-normal text-slate-950">OpsGraph</h1>
          <p className="mt-1 text-sm text-slate-500">每张图谱独立保存，可用于不同环境、系统或演练场景。</p>
        </div>
        <div className="flex flex-wrap gap-2 sm:hidden">
          <Button type="button" onClick={() => void createBlankGraph()} disabled={creating}><Plus />新建图谱</Button>
          <Button type="button" variant="outline" onClick={() => void createExampleGraph()} disabled={creating}><Upload />从示例开始</Button>
        </div>
      </div>

      {error ? <StatusAlert type="error" title="加载失败" message={error} /> : null}

      {graphs.length ? (
        <div className="grid gap-3">
          {graphs.map((graph) => (
            <Card key={graph.id} className="rounded-lg bg-white">
              <CardHeader>
                <CardTitle className="flex flex-wrap items-center gap-2 text-base">
                  {graph.name}
                  {graph.isDefault ? <ToneBadge tone="success">默认</ToneBadge> : null}
                </CardTitle>
              </CardHeader>
              <CardContent className="flex flex-wrap items-center justify-between gap-3 text-sm text-slate-600">
                <span>{graph.environment || "未设置环境"} · {graph.nodeCount || 0} 节点 · {graph.relationshipCount || 0} 关系</span>
                <div className="flex flex-wrap items-center gap-2">
                  <Button asChild type="button" variant="outline">
                    <Link to={`/opsgraph/${encodeURIComponent(graph.id)}`}>打开</Link>
                  </Button>
                  <ConfirmButton
                    type="button"
                    variant="destructive"
                    aria-label={`删除图谱 ${graph.name || graph.id}`}
                    confirm={`确认删除图谱 ${graph.name || graph.id}？`}
                    onConfirm={() => void removeGraph(graph)}
                    disabled={deletingGraphId === graph.id}
                  >
                    <Trash2 />
                    删除
                  </ConfirmButton>
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      ) : (
        <EmptyState title="还没有图谱" description="新建一张空图，或从示例模板开始手工调整。" />
      )}
    </SettingsPageFrame>
  );
}

export function nextDefaultGraphName(graphs: Array<Pick<OpsGraphSummary, "name">>, base = DEFAULT_GRAPH_NAME) {
  let maxSuffix = 0;
  const prefix = `${base}-`;
  for (const graph of graphs) {
    const name = graph.name?.trim();
    if (name === base) {
      maxSuffix = Math.max(maxSuffix, 1);
      continue;
    }
    if (!name?.startsWith(prefix)) continue;
    const suffix = Number.parseInt(name.slice(prefix.length), 10);
    if (Number.isInteger(suffix) && String(suffix) === name.slice(prefix.length) && suffix > maxSuffix) {
      maxSuffix = suffix;
    }
  }
  return maxSuffix === 0 ? base : `${base}-${maxSuffix + 1}`;
}

function exampleGraphPayload() {
  const suffix = Date.now();
  return {
    id: `graph.example-${suffix}`,
    name: "示例图谱",
    environment: "demo",
    nodes: [
      { id: `service.checkout-${suffix}`, type: "service", name: "checkout-api", position: { x: 96, y: 120 }, properties: { environment: "prod", k8sCluster: "prod-k8s", namespace: "shop", workload: "deployment/checkout-api", ports: "8080/http", owner: "platform-sre" } },
      { id: `middleware.postgres-${suffix}`, type: "middleware", subtype: "postgres", name: "checkout-postgres", position: { x: 380, y: 120 }, properties: { host: "db-01", ports: "5432/postgres", role: "primary" } },
      { id: `middleware.redis-${suffix}`, type: "middleware", subtype: "redis", name: "checkout-redis", position: { x: 380, y: 280 }, properties: { ports: "6379/redis" } },
      { id: `external.payment-${suffix}`, type: "external", name: "payment-provider", position: { x: 660, y: 120 }, properties: { domain: "pay.example.com", ports: "443/https" } },
    ],
    edges: [
      { id: `edge.checkout-postgres-${suffix}`, from: `service.checkout-${suffix}`, type: "depends_on", to: `middleware.postgres-${suffix}`, properties: { protocol: "postgres", port: "5432" } },
      { id: `edge.checkout-redis-${suffix}`, from: `service.checkout-${suffix}`, type: "depends_on", to: `middleware.redis-${suffix}`, properties: { protocol: "redis", port: "6379" } },
      { id: `edge.checkout-payment-${suffix}`, from: `service.checkout-${suffix}`, type: "calls", to: `external.payment-${suffix}`, properties: { protocol: "https", port: "443" } },
    ],
  };
}
