import { type FormEvent, useEffect, useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";

import type { OpsGraphNode, OpsGraphRecord } from "./opsGraphTypes";
import { buildLLMContextPreview, relationshipFacts } from "./opsGraphViewModel";

export type OpsGraphHostOption = {
  id: string;
  label: string;
  value: string;
  description?: string;
};

const deploymentFields = [
  { key: "environment", label: "环境" },
  { key: "k8sCluster", label: "K8s 集群" },
  { key: "namespace", label: "命名空间" },
  { key: "workload", label: "工作负载" },
];

const opsFields = [
  { key: "ports", label: "端口" },
  { key: "owner", label: "负责人" },
  { key: "slo", label: "SLO" },
  { key: "runbook", label: "Runbook" },
  { key: "observabilityUrl", label: "观测链接" },
];

export function OpsGraphNodeDialog({
  graph,
  node,
  open,
  onOpenChange,
  onSave,
  hostOptions = [],
}: {
  graph: OpsGraphRecord;
  node: OpsGraphNode | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSave: (node: OpsGraphNode) => Promise<void> | void;
  hostOptions?: OpsGraphHostOption[];
}) {
  const [draft, setDraft] = useState<OpsGraphNode | null>(node ? cloneNode(node) : null);
  const [aliasesText, setAliasesText] = useState("");
  const [tagsText, setTagsText] = useState("");
  const [labelsText, setLabelsText] = useState("");
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    setDraft(node ? cloneNode(node) : null);
    setAliasesText((node?.aliases || []).join(", "));
    setTagsText((node?.tags || []).join(", "));
    setLabelsText(formatKeyValues(node?.labels || {}));
    setError("");
    setSaving(false);
  }, [node, open]);

  const facts = useMemo(() => node ? relationshipFacts(graph, node.id) : { upstreams: [], downstreams: [] }, [graph, node]);
  const preview = useMemo(() => node ? buildLLMContextPreview({ ...graph, nodes: graph.nodes.map((item) => item.id === node.id && draft ? draft : item) }, node.id) : "", [draft, graph, node]);

  function updateProperty(key: string, value: string) {
    if (!draft) return;
    setDraft({ ...draft, properties: { ...(draft.properties || {}), [key]: value } });
  }

  function selectHost(value: string) {
    if (!value) return;
    updateProperty("host", value);
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!draft) return;
    if (!draft.name.trim()) {
      setError("节点名称不能为空");
      return;
    }
    setError("");
    setSaving(true);
    try {
      const next: OpsGraphNode = {
        ...draft,
        name: draft.name.trim(),
        subtype: draft.type === "middleware" ? (draft.subtype || "generic").trim() || "generic" : undefined,
        aliases: splitList(aliasesText),
        tags: splitList(tagsText),
        labels: parseKeyValues(labelsText),
        properties: cleanProperties(draft.properties || {}),
      };
      await onSave(next);
    } catch (saveError) {
      setError(saveError instanceof Error ? saveError.message : "保存节点属性失败");
    } finally {
      setSaving(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex h-[min(88vh,820px)] min-h-0 flex-col overflow-hidden sm:max-w-4xl">
        <DialogHeader className="shrink-0 pr-8">
          <DialogTitle>{draft?.name || "节点属性"}</DialogTitle>
          <DialogDescription>编辑服务拓扑节点的身份、部署位置和运维上下文。</DialogDescription>
        </DialogHeader>
        {draft ? (
          <form className="flex min-h-0 min-w-0 max-w-full flex-1 flex-col gap-5 overflow-y-auto overflow-x-hidden overscroll-contain pr-1 pb-1" onSubmit={handleSubmit}>
            {error ? <div role="alert" className="rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{error}</div> : null}

            <section className="grid min-w-0 gap-3">
              <h3 className="text-sm font-semibold text-slate-950">基础信息</h3>
              <div className="grid min-w-0 gap-3 md:grid-cols-3">
                <label className="grid min-w-0 gap-1.5 text-xs text-slate-600">
                  名称
                  <Input className="min-w-0" name="name" value={draft.name} onChange={(event) => setDraft({ ...draft, name: event.target.value })} />
                </label>
                <label className="grid min-w-0 gap-1.5 text-xs text-slate-600">
                  类型
                  <select
                    name="type"
                    className="h-9 min-w-0 rounded-lg border bg-white px-2 text-sm"
                    value={draft.type}
                    onChange={(event) => setDraft({ ...draft, type: event.target.value as OpsGraphNode["type"] })}
                  >
                    <option value="service">服务</option>
                    <option value="middleware">中间件</option>
                    <option value="external">外部服务</option>
                  </select>
                </label>
                <label className="grid min-w-0 gap-1.5 text-xs text-slate-600">
                  子类型
                  <Input className="min-w-0" name="subtype" value={draft.subtype || ""} disabled={draft.type !== "middleware"} onChange={(event) => setDraft({ ...draft, subtype: event.target.value })} />
                </label>
              </div>
              <label className="grid min-w-0 gap-1.5 text-xs text-slate-600">
                描述
                <Textarea className="min-w-0" name="description" value={draft.description || ""} onChange={(event) => setDraft({ ...draft, description: event.target.value })} />
              </label>
              <div className="grid min-w-0 gap-3 md:grid-cols-3">
                <label className="grid min-w-0 gap-1.5 text-xs text-slate-600">
                  别名
                  <Input className="min-w-0" name="aliases" value={aliasesText} onChange={(event) => setAliasesText(event.target.value)} />
                </label>
                <label className="grid min-w-0 gap-1.5 text-xs text-slate-600">
                  标签
                  <Input className="min-w-0" name="tags" value={tagsText} onChange={(event) => setTagsText(event.target.value)} />
                </label>
                <label className="grid min-w-0 gap-1.5 text-xs text-slate-600">
                  Labels
                  <Input className="min-w-0" name="labels" value={labelsText} onChange={(event) => setLabelsText(event.target.value)} placeholder="domain=erp, tier=core" />
                </label>
              </div>
            </section>

            <section className="grid min-w-0 gap-3">
              <h3 className="text-sm font-semibold text-slate-950">部署属性</h3>
              <div className="grid min-w-0 gap-3 md:grid-cols-2">
                {deploymentFields.map((field) => (
                  <label key={field.key} className="grid min-w-0 gap-1.5 text-xs text-slate-600">
                    {field.label}
                    <Input className="min-w-0" name={field.key} value={draft.properties?.[field.key] || ""} onChange={(event) => updateProperty(field.key, event.target.value)} />
                  </label>
                ))}
                <div className="grid min-w-0 gap-1.5 text-xs text-slate-600 md:col-span-2">
                  <div className="flex min-w-0 items-center justify-between gap-2">
                    <span>主机</span>
                    <span className="truncate text-[11px] text-slate-400">可手动输入，也可从主机管理列表选择</span>
                  </div>
                  <div className="grid min-w-0 gap-2 sm:grid-cols-[minmax(0,1fr)_minmax(150px,0.45fr)]">
                    <Input
                      className="min-w-0 max-w-full"
                      name="host"
                      value={draft.properties?.host || ""}
                      placeholder="例如 10.0.0.11 或 prod-web-01"
                      onChange={(event) => updateProperty("host", event.target.value)}
                    />
                    <select
                      aria-label="从主机列表选择"
                      className="h-9 min-w-0 max-w-full rounded-lg border bg-white px-2 text-sm text-slate-700"
                      value={hostOptions.some((option) => option.value === draft.properties?.host) ? draft.properties?.host || "" : ""}
                      onChange={(event) => selectHost(event.target.value)}
                    >
                      <option value="">选择主机列表</option>
                      {hostOptions.map((option) => (
                        <option key={option.id} value={option.value}>
                          {option.description ? `${option.label} · ${option.description}` : option.label}
                        </option>
                      ))}
                    </select>
                  </div>
                </div>
              </div>
            </section>

            <section className="grid min-w-0 gap-3">
              <h3 className="text-sm font-semibold text-slate-950">运维属性</h3>
              <div className="grid min-w-0 gap-3 md:grid-cols-2">
                {opsFields.map((field) => (
                  <label key={field.key} className="grid min-w-0 gap-1.5 text-xs text-slate-600">
                    {field.label}
                    <Input className="min-w-0" name={field.key} value={draft.properties?.[field.key] || ""} onChange={(event) => updateProperty(field.key, event.target.value)} />
                  </label>
                ))}
              </div>
            </section>

            <section className="grid min-w-0 gap-3">
              <h3 className="text-sm font-semibold text-slate-950">关系与 LLM 上下文</h3>
              <div className="grid gap-2 text-xs text-slate-600 sm:grid-cols-2">
                <div className="rounded border bg-slate-50 p-2">上游：{facts.upstreams.length}</div>
                <div className="rounded border bg-slate-50 p-2">下游：{facts.downstreams.length}</div>
              </div>
              <pre className="max-h-40 overflow-auto whitespace-pre-wrap rounded border bg-slate-50 p-2 text-[11px] leading-5 text-slate-600">{preview}</pre>
            </section>

            <DialogFooter className="sticky bottom-0 z-10 mx-0 mb-0 mt-auto shrink-0 rounded-none border-t border-slate-100 bg-white/95 px-0 py-3 backdrop-blur">
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>取消</Button>
              <Button type="submit" disabled={saving}>{saving ? "保存中" : "保存属性"}</Button>
            </DialogFooter>
          </form>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}

function cloneNode(node: OpsGraphNode): OpsGraphNode {
  return {
    ...node,
    aliases: [...(node.aliases || [])],
    tags: [...(node.tags || [])],
    labels: { ...(node.labels || {}) },
    properties: { ...(node.properties || {}) },
  };
}

function splitList(value: string) {
  return value.split(",").map((item) => item.trim()).filter(Boolean);
}

function parseKeyValues(value: string) {
  const out: Record<string, string> = {};
  for (const item of value.split(",")) {
    const [rawKey, ...rest] = item.split("=");
    const key = rawKey?.trim();
    const nextValue = rest.join("=").trim();
    if (key && nextValue) out[key] = nextValue;
  }
  return out;
}

function formatKeyValues(value: Record<string, string>) {
  return Object.entries(value).map(([key, item]) => `${key}=${item}`).join(", ");
}

function cleanProperties(value: Record<string, string>) {
  const out: Record<string, string> = {};
  for (const [key, item] of Object.entries(value)) {
    if (item.trim()) out[key] = item.trim();
  }
  return out;
}
