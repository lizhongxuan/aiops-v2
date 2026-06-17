import { useEffect, useRef, useState, type ChangeEvent } from "react";
import { Link, useParams } from "react-router-dom";
import { ArrowLeft, Download, LayoutDashboard, Save, Upload } from "lucide-react";

import { listHostInventory, type HostInventoryItem } from "@/api/hostInventory";
import { createOpsGraphNode, createOpsGraphRelationship, exportOpsGraphYaml, getOpsGraph, importOpsGraphYaml, saveOpsGraphLayout, updateOpsGraphNode, updateOpsGraphRelationship } from "@/api/opsgraph";
import { Button } from "@/components/ui/button";
import { SettingsPageFrame, StatusAlert } from "@/pages/settingsComponents";

import { OpsGraphCanvas } from "./OpsGraphCanvas";
import { OpsGraphNodeDialog, type OpsGraphHostOption } from "./OpsGraphNodeDialog";
import { OpsGraphNodeList } from "./OpsGraphNodeList";
import { OpsGraphNodeSummary } from "./OpsGraphNodeSummary";
import { OpsGraphPalette, type OpsGraphPaletteItem } from "./OpsGraphPalette";
import { OpsGraphRelationshipDialog } from "./OpsGraphRelationshipDialog";
import type { OpsGraphNode, OpsGraphRecord, OpsGraphRelationship, OpsGraphRelationshipType } from "./opsGraphTypes";
import { autoLayoutOpsGraphLeftToRight, nextTopologyNodeName, visibleTopologyGraph } from "./opsGraphViewModel";

export function OpsGraphPage() {
  const { graphId = "graph.default" } = useParams();
  const [graph, setGraph] = useState<OpsGraphRecord | null>(null);
  const [error, setError] = useState("");
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
  const [selectedRelationshipId, setSelectedRelationshipId] = useState<string | null>(null);
  const [editingRelationshipId, setEditingRelationshipId] = useState<string | null>(null);
  const [editingNodeId, setEditingNodeId] = useState<string | null>(null);
  const [hostOptions, setHostOptions] = useState<OpsGraphHostOption[]>([]);
  const [notice, setNotice] = useState("");
  const [importingYaml, setImportingYaml] = useState(false);
  const [exportingYaml, setExportingYaml] = useState(false);
  const yamlImportInputRef = useRef<HTMLInputElement | null>(null);

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

  useEffect(() => {
    let active = true;
    void listHostInventory()
      .then((hosts) => {
        if (!active) return;
        setHostOptions(hosts.map(hostInventoryOption).filter((item): item is OpsGraphHostOption => Boolean(item)));
      })
      .catch(() => {
        if (active) setHostOptions([]);
      });
    return () => {
      active = false;
    };
  }, []);

  const topologyGraph = graph ? visibleTopologyGraph(graph) : null;
  const nodes = topologyGraph?.nodes || [];

  async function saveGraph() {
    if (!graph) return;
    await reloadGraph(graph.id);
  }

  async function exportGraphYAML() {
    if (!graph) return;
    setError("");
    setNotice("");
    setExportingYaml(true);
    try {
      const yamlText = await exportOpsGraphYaml(graph.id);
      const blob = new Blob([String(yamlText)], { type: "text/yaml;charset=utf-8" });
      const objectUrl = URL.createObjectURL(blob);
      const anchor = document.createElement("a");
      anchor.href = objectUrl;
      anchor.download = `${safeYamlFilename(graph.name || graph.id)}.yaml`;
      document.body.append(anchor);
      anchor.click();
      anchor.remove();
      URL.revokeObjectURL(objectUrl);
      setNotice("已导出 YAML");
    } catch (exportError) {
      setError(exportError instanceof Error ? exportError.message : "导出 YAML 失败");
    } finally {
      setExportingYaml(false);
    }
  }

  async function importGraphYAML(event: ChangeEvent<HTMLInputElement>) {
    if (!graph) return;
    const file = event.currentTarget.files?.[0];
    event.currentTarget.value = "";
    if (!file) return;
    setError("");
    setNotice("");
    setImportingYaml(true);
    try {
      const yamlText = await file.text();
      const payload = await importOpsGraphYaml(graph.id, yamlText);
      const nextGraph = payload.graph || payload;
      setGraph(nextGraph);
      setSelectedNodeId(null);
      setSelectedRelationshipId(null);
      setEditingNodeId(null);
      setEditingRelationshipId(null);
      setNotice("导入完成");
    } catch (importError) {
      setError(importError instanceof Error ? importError.message : "导入 YAML 失败");
    } finally {
      setImportingYaml(false);
    }
  }

  function arrangeGraphLayout() {
    if (!graph) return;
    const arranged = autoLayoutOpsGraphLeftToRight(topologyGraph || graph);
    saveLayoutFromCanvas(arranged.nodes.map((node) => ({
      id: node.id,
      position: node.position,
      collapsed: node.collapsed,
    })));
  }

  async function createNodeFromCanvas(input: { type: string; subtype?: string; name: string; position: { x: number; y: number } }) {
    if (!graph) return;
    const name = nextTopologyNodeName(nodes, input.name);
    const idPart = input.subtype && input.subtype !== "generic" ? `${input.subtype}.${name}` : name;
    const id = `${input.type}.${idPart}.${Date.now()}`.replace(/\s+/g, "-").toLowerCase();
    await createOpsGraphNode(graph.id, {
      id,
      type: input.type,
      subtype: input.subtype,
      name,
      position: input.position,
      properties: defaultPropertiesForNode(input.type, input.subtype),
    });
    await reloadGraph(graph.id);
  }

  async function createNodeFromPalette(item: OpsGraphPaletteItem) {
    await createNodeFromCanvas({
      type: item.type,
      subtype: item.subtype,
      name: defaultPaletteNodeName(item),
      position: paletteNodePosition(nodes.length),
    });
  }

  async function createRelationshipFromCanvas(input: { from: string; to: string; type: string }) {
    if (!graph) return;
    const type = relationshipTypeForManualConnection(nodes, input.to, input.type);
    await createOpsGraphRelationship(graph.id, {
      id: `edge.${input.from}.${type}.${input.to}`,
      from: input.from,
      type,
      to: input.to,
    });
    await reloadGraph(graph.id);
  }

  function selectNode(nodeId: string | null) {
    setSelectedNodeId(nodeId);
    if (nodeId) {
      setSelectedRelationshipId(null);
      setEditingRelationshipId(null);
    }
  }

  function selectRelationship(relationshipId: string | null) {
    setSelectedRelationshipId(relationshipId);
    setSelectedNodeId(null);
    if (relationshipId) setEditingRelationshipId(relationshipId);
  }

  async function saveRelationship(nextRelationship: OpsGraphRelationship, closeDialog = true) {
    if (!graph) return;
    await updateOpsGraphRelationship(graph.id, nextRelationship.id, nextRelationship);
    setSelectedRelationshipId(nextRelationship.id);
    if (closeDialog) setEditingRelationshipId(null);
    await reloadGraph(graph.id);
  }

  function reconnectRelationship(nextRelationship: OpsGraphRelationship) {
    void saveRelationship(nextRelationship, false).catch((saveError: unknown) => {
      setError(saveError instanceof Error ? saveError.message : "保存关系失败");
    });
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

  async function saveNodeFromDialog(nextNode: OpsGraphNode) {
    if (!graph) return;
    await updateOpsGraphNode(graph.id, nextNode.id, nextNode);
    setEditingNodeId(null);
    setSelectedNodeId(nextNode.id);
    await reloadGraph(graph.id);
  }

  const graphTitle = graph?.name || "OpsGraph";
  const graphDescription = graph
    ? `${graph.environment || "未设置环境"} · ${nodes.length} 节点 · ${topologyGraph?.edges?.length || 0} 关系`
    : "加载图谱中";
  const selectedNode = selectedNodeId ? nodes.find((node) => node.id === selectedNodeId) || null : null;
  const editingNode = editingNodeId ? nodes.find((node) => node.id === editingNodeId) || null : null;
  const editingRelationship = editingRelationshipId ? (topologyGraph?.edges || []).find((relationship) => relationship.id === editingRelationshipId) || null : null;

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
          <input
            ref={yamlImportInputRef}
            data-testid="opsgraph-yaml-import-input"
            type="file"
            accept=".yaml,.yml,text/yaml,application/yaml"
            className="sr-only"
            onChange={(event) => void importGraphYAML(event)}
          />
          <Button type="button" size="sm" variant="outline" onClick={() => yamlImportInputRef.current?.click()} disabled={!graph || importingYaml}>
            <Upload />
            {importingYaml ? "导入中" : "导入 YAML"}
          </Button>
          <Button type="button" size="sm" variant="outline" onClick={() => void exportGraphYAML()} disabled={!graph || exportingYaml}>
            <Download />
            {exportingYaml ? "导出中" : "导出 YAML"}
          </Button>
          <Button type="button" size="sm" variant="outline" onClick={arrangeGraphLayout} disabled={!graph || nodes.length === 0}>
            <LayoutDashboard />
            整理布局
          </Button>
        </>
      )}
      contentClassName="h-full min-h-0"
    >
      {error ? <StatusAlert type="error" title="加载失败" message={error} /> : null}
      {notice ? <StatusAlert type="success" title={notice} message={notice === "导入完成" ? "已用 YAML 内容更新当前图谱。" : "已生成当前图谱的 YAML 文件。"} /> : null}
      <section data-testid="opsgraph-editor-layout" className="grid min-h-0 flex-1 grid-cols-[clamp(160px,24vw,220px)_minmax(0,1fr)] gap-3">
        <aside className="grid min-h-0 min-w-0 grid-rows-[auto_minmax(0,1fr)] gap-3 overflow-hidden rounded-lg border bg-white p-3">
          <OpsGraphPalette onCreateNode={(type) => void createNodeFromPalette(type)} />
          <OpsGraphNodeList nodes={nodes} />
        </aside>
        <main data-testid="opsgraph-canvas-panel" className="relative min-h-0 min-w-0 rounded-lg border bg-slate-50 p-3">
          {!nodes.length ? (
            <div data-testid="opsgraph-empty-guide" className="grid h-full min-h-0 place-items-center text-center">
              <div>
                <h2 className="text-lg font-semibold text-slate-950">这个图谱现在是空的</h2>
                <p className="mt-2 max-w-md text-sm leading-6 text-slate-600">先添加一个业务服务，再连接它的中间件或外部依赖。部署位置、端口和负责人会作为节点属性保存。</p>
              </div>
            </div>
          ) : (
            topologyGraph ? (
              <>
                <OpsGraphCanvas
                  graph={topologyGraph}
                  onCreateNode={(input) => void createNodeFromCanvas(input)}
                  onCreateRelationship={(input) => void createRelationshipFromCanvas(input)}
                  onSelectNode={selectNode}
                  onSelectRelationship={selectRelationship}
                  onReconnectRelationship={reconnectRelationship}
                  selectedRelationshipId={selectedRelationshipId}
                  onSaveLayout={saveLayoutFromCanvas}
                />
                {selectedNode ? (
                  <OpsGraphNodeSummary
                    graph={topologyGraph}
                    node={selectedNode}
                    onEdit={() => setEditingNodeId(selectedNode.id)}
                  />
                ) : null}
              </>
            ) : null
          )}
        </main>
      </section>
      {graph ? (
        <OpsGraphNodeDialog
          graph={topologyGraph || graph}
          node={editingNode}
          hostOptions={hostOptions}
          open={Boolean(editingNode)}
          onOpenChange={(open) => {
            if (!open) setEditingNodeId(null);
          }}
          onSave={(node) => saveNodeFromDialog(node)}
        />
      ) : null}
      {graph ? (
        <OpsGraphRelationshipDialog
          graph={topologyGraph || graph}
          relationship={editingRelationship}
          open={Boolean(editingRelationship)}
          onOpenChange={(open) => {
            if (!open) setEditingRelationshipId(null);
          }}
          onSave={(relationship) => saveRelationship(relationship)}
        />
      ) : null}
    </SettingsPageFrame>
  );
}

function hostInventoryOption(host: HostInventoryItem): OpsGraphHostOption | null {
  const id = firstNonEmpty(host.id, host.hostId, host.name, host.hostname, host.address, host.ip);
  const label = firstNonEmpty(host.name, host.hostname, host.hostId, host.id, host.address, host.ip);
  const value = firstNonEmpty(host.address, host.ip, host.hostname, host.name, host.id, host.hostId);
  if (!value) return null;
  const description = [host.address || host.ip, host.sshUser ? `ssh ${host.sshUser}${host.sshPort ? `:${host.sshPort}` : ""}` : ""].filter(Boolean).join(" · ");
  return { id: id || value, label: label || value, value, description };
}

function firstNonEmpty(...values: Array<string | undefined>) {
  return values.find((value) => Boolean(value && value.trim()))?.trim() || "";
}

function defaultPaletteNodeName(item: OpsGraphPaletteItem) {
  if (item.type === "service") return "新服务";
  if (item.type === "middleware" && (!item.subtype || item.subtype === "generic")) return "新中间件";
  if (item.type === "external") return "新外部服务";
  return `新${item.label}`;
}

function paletteNodePosition(index: number) {
  return {
    x: 96 + (index % 2) * 300,
    y: 96 + Math.floor(index / 2) * 220,
  };
}

function safeYamlFilename(name: string) {
  const normalized = name.trim().replace(/[\\/:*?"<>|]+/g, "-").replace(/\s+/g, "-");
  return normalized || "opsgraph";
}

export function relationshipTypeForManualConnection(
  nodes: OpsGraphNode[],
  targetId: string,
  fallback: OpsGraphRelationshipType | string = "depends_on",
): OpsGraphRelationshipType {
  void nodes;
  void targetId;
  if (fallback === "calls" || fallback === "depends_on" || fallback === "publishes" || fallback === "consumes" || fallback === "proxies_to") {
    return fallback;
  }
  return "depends_on";
}

function defaultPropertiesForNode(type: string, subtype?: string) {
  if (type === "service") return { environment: "prod", ports: "8080/http" };
  if (type === "external") return { ports: "443/https" };
  if (type !== "middleware") return {};
  switch (subtype) {
    case "redis":
      return { ports: "6379/redis" };
    case "postgres":
      return { ports: "5432/postgres", role: "primary" };
    case "mysql":
      return { ports: "3306/mysql", role: "primary" };
    case "zk":
      return { ports: "2181/zk" };
    case "rabbitmq":
      return { ports: "5672/amqp" };
    case "nginx":
      return { ports: "80/http, 443/https" };
    default:
      return {};
  }
}
