import { Plus, Upload } from "lucide-react";
import { useEffect, useState } from "react";
import { Link, useNavigate } from "react-router-dom";

import { createOpsGraph, listOpsGraphs } from "@/api/opsgraph";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { EmptyState, SettingsPageFrame, StatusAlert, ToneBadge } from "@/pages/settingsComponents";

import type { OpsGraphSummary } from "./opsGraphTypes";

export function OpsGraphListPage() {
  const navigate = useNavigate();
  const [graphs, setGraphs] = useState<OpsGraphSummary[]>([]);
  const [error, setError] = useState("");
  const [creating, setCreating] = useState(false);

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
        name: "新建图谱",
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
                <Button asChild type="button" variant="outline">
                  <Link to={`/opsgraph/${encodeURIComponent(graph.id)}`}>打开</Link>
                </Button>
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

function exampleGraphPayload() {
  const suffix = Date.now();
  return {
    id: `graph.example-${suffix}`,
    name: "示例图谱",
    environment: "demo",
    nodes: [
      { id: `service.checkout-${suffix}`, type: "service", name: "checkout-api", position: { x: 96, y: 96 } },
      { id: `middleware.redis-${suffix}`, type: "middleware", name: "redis-cache", position: { x: 256, y: 96 } },
      { id: `host.worker-${suffix}`, type: "host", name: "worker-01", container: true, position: { x: 96, y: 256 } },
      { id: `k8s.prod-${suffix}`, type: "k8s", name: "prod-cluster", container: true, position: { x: 256, y: 256 } },
    ],
    edges: [
      { id: `edge.checkout-redis-${suffix}`, from: `service.checkout-${suffix}`, type: "depends_on", to: `middleware.redis-${suffix}` },
      { id: `edge.checkout-k8s-${suffix}`, from: `service.checkout-${suffix}`, type: "runs_on", to: `k8s.prod-${suffix}` },
      { id: `edge.redis-host-${suffix}`, from: `middleware.redis-${suffix}`, type: "runs_on", to: `host.worker-${suffix}` },
    ],
  };
}
