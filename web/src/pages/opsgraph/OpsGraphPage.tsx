import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { ArrowLeft, Save } from "lucide-react";

import { createOpsGraphNode, createOpsGraphRelationship, getOpsGraph, saveOpsGraphLayout } from "@/api/opsgraph";
import { Button } from "@/components/ui/button";
import { SettingsPageFrame, StatusAlert } from "@/pages/settingsComponents";

import { OpsGraphCanvas } from "./OpsGraphCanvas";
import { OpsGraphNodeList } from "./OpsGraphNodeList";
import { OpsGraphPalette } from "./OpsGraphPalette";
import type { OpsGraphNode, OpsGraphRecord, OpsGraphRelationshipType } from "./opsGraphTypes";
import { nodeTypeLabel } from "./opsGraphViewModel";

export function OpsGraphPage() {
  const { graphId = "graph.default" } = useParams();
  const [graph, setGraph] = useState<OpsGraphRecord | null>(null);
  const [error, setError] = useState("");

  async function reloadGraph(targetGraphId = graphId) {
    const payload = await getOpsGraph(targetGraphId);
    setGraph(payload.graph || payload);
  }

  useEffect(() => {
    let active = true;
    setError("");
    void getOpsGraph(graphId)
      .then((payload) => {
        if (!active) return;
        setGraph(payload.graph || payload);
      })
      .catch((loadError) => {
        if (!active) return;
        setGraph({ id: graphId, name: "默认图谱", nodes: [], edges: [] });
        setError(loadError instanceof Error ? loadError.message : "加载 OpsGraph 失败");
      });
    return () => {
      active = false;
    };
  }, [graphId]);

  const nodes = graph?.nodes || [];

  async function saveGraph() {
    if (!graph) return;
    await reloadGraph(graph.id);
  }

  async function createNodeFromCanvas(input: { type: string; name: string; position: { x: number; y: number }; parentId?: string }) {
    if (!graph) return;
    const id = `${input.type}.${input.name}.${Date.now()}`.replace(/\s+/g, "-").toLowerCase();
    await createOpsGraphNode(graph.id, {
      id,
      type: input.type,
      name: input.name,
      parentId: input.parentId,
      position: input.position,
      container: input.type === "host" || input.type === "k8s" || input.type === "middleware_cluster",
    });
    await reloadGraph(graph.id);
  }

  async function createNodeFromPalette(type: string) {
    await createNodeFromCanvas({
      type,
      name: defaultPaletteNodeName(type),
      position: paletteNodePosition(nodes.length),
    });
  }

  async function createRelationshipFromCanvas(input: { from: string; to: string; type: string }) {
    if (!graph) return;
    const type = relationshipTypeForManualConnection(graph.nodes, input.to, input.type);
    await createOpsGraphRelationship(graph.id, {
      id: `edge.${input.from}.${type}.${input.to}`,
      from: input.from,
      type,
      to: input.to,
    });
    await reloadGraph(graph.id);
  }

  function saveLayoutFromCanvas(
    layoutNodes: Array<{ id: string; position?: { x: number; y: number }; collapsed?: boolean }>,
    viewport?: { x: number; y: number; zoom: number },
  ) {
    if (!graph) return;
    const layoutById = new Map(layoutNodes.map((node) => [node.id, node]));
    setGraph((current) => {
      if (!current || current.id !== graph.id) return current;
      return {
        ...current,
        viewport: viewport || current.viewport,
        nodes: current.nodes.map((node) => {
          const layout = layoutById.get(node.id);
          if (!layout) return node;
          return {
            ...node,
            position: layout.position || node.position,
            collapsed: layout.collapsed ?? node.collapsed,
          };
        }),
      };
    });
    void saveOpsGraphLayout(graph.id, { nodes: layoutNodes, viewport }).catch((saveError: unknown) => {
      setError(saveError instanceof Error ? saveError.message : "保存布局失败");
    });
  }

  const graphTitle = graph?.name || "OpsGraph";
  const graphDescription = graph
    ? `${graph.environment || "未设置环境"} · ${nodes.length} 节点 · ${graph.edges?.length || 0} 关系`
    : "加载图谱中";

  return (
    <SettingsPageFrame
      title={graphTitle}
      description={graphDescription}
      actions={(
        <>
          <Button asChild type="button" size="sm" variant="outline">
            <Link to="/opsgraph/graphs">
              <ArrowLeft />
              返回列表
            </Link>
          </Button>
          <Button type="button" size="sm" variant="outline" onClick={() => void saveGraph()} disabled={!graph}>
            <Save />
            保存
          </Button>
        </>
      )}
      contentClassName="h-full min-h-0"
    >
      {error ? <StatusAlert type="error" title="加载失败" message={error} /> : null}
      <section data-testid="opsgraph-editor-layout" className="grid min-h-0 flex-1 grid-cols-[clamp(160px,24vw,220px)_minmax(0,1fr)] gap-3">
        <aside className="grid min-h-0 min-w-0 grid-rows-[auto_minmax(0,1fr)] gap-3 overflow-hidden rounded-lg border bg-white p-3">
          <OpsGraphPalette onCreateNode={(type) => void createNodeFromPalette(type)} />
          <OpsGraphNodeList nodes={nodes} />
        </aside>
        <main data-testid="opsgraph-canvas-panel" className="min-h-0 min-w-0 rounded-lg border bg-slate-50 p-3">
          {!nodes.length ? (
            <div data-testid="opsgraph-empty-guide" className="grid h-full min-h-0 place-items-center text-center">
              <div>
                <h2 className="text-lg font-semibold text-slate-950">这个图谱现在是空的</h2>
                <p className="mt-2 max-w-md text-sm leading-6 text-slate-600">先添加一个服务，再把它连接到依赖、中间件、主机或 K8s。你录入的关系会被 Case 和 AI 对话用于定位上下文。</p>
              </div>
            </div>
          ) : (
            graph ? (
              <OpsGraphCanvas
                graph={graph}
                onCreateNode={(input) => void createNodeFromCanvas(input)}
                onCreateRelationship={(input) => void createRelationshipFromCanvas(input)}
                onSaveLayout={saveLayoutFromCanvas}
              />
            ) : null
          )}
        </main>
      </section>
    </SettingsPageFrame>
  );
}

function defaultPaletteNodeName(type: string) {
  switch (type) {
    case "service":
      return "新服务";
    case "middleware":
      return "新中间件";
    case "host":
      return "新主机";
    case "k8s":
      return "新K8s";
    default:
      return nodeTypeLabel(type);
  }
}

function paletteNodePosition(index: number) {
  return {
    x: 96 + (index % 2) * 300,
    y: 96 + Math.floor(index / 2) * 220,
  };
}

export function relationshipTypeForManualConnection(
  nodes: OpsGraphNode[],
  targetId: string,
  fallback: OpsGraphRelationshipType | string = "depends_on",
): OpsGraphRelationshipType {
  const target = nodes.find((node) => node.id === targetId);
  if (target?.type === "host" || target?.type === "k8s") return "runs_on";
  if (fallback === "runs_on" || fallback === "contains" || fallback === "calls" || fallback === "owns" || fallback === "affects" || fallback === "owned_by" || fallback === "handled_by") {
    return fallback;
  }
  return "depends_on";
}
