import { type FormEvent, useEffect, useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";

import type { OpsGraphRecord, OpsGraphRelationship, OpsGraphRelationshipType } from "./opsGraphTypes";
import { nodeTypeLabel, relationshipLabel, topologyNodeMeta } from "./opsGraphViewModel";

const relationshipTypes: Array<{ value: OpsGraphRelationshipType; label: string }> = [
  { value: "depends_on", label: "依赖" },
  { value: "calls", label: "调用" },
  { value: "proxies_to", label: "代理" },
  { value: "publishes", label: "发布" },
  { value: "consumes", label: "消费" },
  { value: "runs_on", label: "部署于" },
  { value: "contains", label: "包含" },
  { value: "owns", label: "拥有" },
  { value: "affects", label: "影响" },
  { value: "owned_by", label: "归属" },
  { value: "handled_by", label: "处置" },
];

export function OpsGraphRelationshipDialog({
  graph,
  relationship,
  open,
  onOpenChange,
  onSave,
}: {
  graph: OpsGraphRecord;
  relationship: OpsGraphRelationship | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSave: (relationship: OpsGraphRelationship) => Promise<void> | void;
}) {
  const [draft, setDraft] = useState<OpsGraphRelationship | null>(relationship ? cloneRelationship(relationship) : null);
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    setDraft(relationship ? cloneRelationship(relationship) : null);
    setError("");
    setSaving(false);
  }, [relationship, open]);

  const nodeOptions = useMemo(() => graph.nodes || [], [graph.nodes]);
  const fromNode = nodeOptions.find((node) => node.id === draft?.from);
  const toNode = nodeOptions.find((node) => node.id === draft?.to);

  function updateProperty(key: string, value: string) {
    if (!draft) return;
    setDraft({ ...draft, properties: { ...(draft.properties || {}), [key]: value } });
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!draft) return;
    if (!draft.from || !draft.to || !draft.type) {
      setError("关系必须包含源节点、目标节点和类型");
      return;
    }
    if (draft.from === draft.to) {
      setError("源节点和目标节点不能相同");
      return;
    }
    setError("");
    setSaving(true);
    try {
      await onSave({
        ...draft,
        note: draft.note?.trim() || undefined,
        reason: draft.reason?.trim() || undefined,
        properties: cleanProperties(draft.properties || {}),
      });
    } catch (saveError) {
      setError(saveError instanceof Error ? saveError.message : "保存关系失败");
    } finally {
      setSaving(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[min(86vh,720px)] overflow-hidden sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{draft ? `编辑关系：${relationshipLabel(draft.type)}` : "关系"}</DialogTitle>
          <DialogDescription>编辑依赖方向、关系类型和协议端口等上下文。箭头方向为源节点指向目标节点。</DialogDescription>
        </DialogHeader>
        {draft ? (
          <form className="grid min-h-0 gap-4 overflow-y-auto pr-1" onSubmit={handleSubmit}>
            {error ? <div role="alert" className="rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{error}</div> : null}

            <section className="grid gap-3">
              <div className="rounded border bg-slate-50 p-3 text-xs leading-5 text-slate-600">
                <span className="font-medium text-slate-800">{nodeDisplayName(fromNode, draft.from)}</span>
                <span className="px-2 text-slate-400">→</span>
                <span className="font-medium text-slate-800">{nodeDisplayName(toNode, draft.to)}</span>
              </div>
              <div className="grid gap-3 sm:grid-cols-3">
                <label className="grid gap-1.5 text-xs text-slate-600">
                  源节点
                  <select
                    name="from"
                    className="h-8 rounded-lg border bg-white px-2 text-sm"
                    value={draft.from}
                    onChange={(event) => setDraft({ ...draft, from: event.target.value })}
                  >
                    {nodeOptions.map((node) => (
                      <option key={node.id} value={node.id}>{node.name || node.id}</option>
                    ))}
                  </select>
                </label>
                <label className="grid gap-1.5 text-xs text-slate-600">
                  关系类型
                  <select
                    name="type"
                    className="h-8 rounded-lg border bg-white px-2 text-sm"
                    value={draft.type}
                    onChange={(event) => setDraft({ ...draft, type: event.target.value as OpsGraphRelationshipType })}
                  >
                    {relationshipTypes.map((item) => (
                      <option key={item.value} value={item.value}>{item.label}</option>
                    ))}
                  </select>
                </label>
                <label className="grid gap-1.5 text-xs text-slate-600">
                  目标节点
                  <select
                    name="to"
                    className="h-8 rounded-lg border bg-white px-2 text-sm"
                    value={draft.to}
                    onChange={(event) => setDraft({ ...draft, to: event.target.value })}
                  >
                    {nodeOptions.map((node) => (
                      <option key={node.id} value={node.id}>{node.name || node.id}</option>
                    ))}
                  </select>
                </label>
              </div>
            </section>

            <section className="grid gap-3">
              <h3 className="text-sm font-semibold text-slate-950">连接属性</h3>
              <div className="grid gap-3 sm:grid-cols-3">
                <label className="grid gap-1.5 text-xs text-slate-600">
                  协议
                  <Input name="protocol" value={draft.properties?.protocol || ""} onChange={(event) => updateProperty("protocol", event.target.value)} />
                </label>
                <label className="grid gap-1.5 text-xs text-slate-600">
                  端口
                  <Input name="port" value={draft.properties?.port || ""} onChange={(event) => updateProperty("port", event.target.value)} />
                </label>
                <label className="grid gap-1.5 text-xs text-slate-600">
                  路径/Topic
                  <Input name="path" value={draft.properties?.path || ""} onChange={(event) => updateProperty("path", event.target.value)} />
                </label>
              </div>
              <div className="grid gap-3 sm:grid-cols-2">
                <label className="grid gap-1.5 text-xs text-slate-600">
                  说明
                  <Textarea name="note" value={draft.note || ""} onChange={(event) => setDraft({ ...draft, note: event.target.value })} />
                </label>
                <label className="grid gap-1.5 text-xs text-slate-600">
                  原因
                  <Textarea name="reason" value={draft.reason || ""} onChange={(event) => setDraft({ ...draft, reason: event.target.value })} />
                </label>
              </div>
            </section>

            <DialogFooter className="sticky bottom-0">
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>取消</Button>
              <Button type="submit" disabled={saving}>{saving ? "保存中" : "保存关系"}</Button>
            </DialogFooter>
          </form>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}

function cloneRelationship(relationship: OpsGraphRelationship): OpsGraphRelationship {
  return {
    ...relationship,
    properties: { ...(relationship.properties || {}) },
  };
}

function nodeDisplayName(node: OpsGraphRecord["nodes"][number] | undefined, fallback: string) {
  if (!node) return fallback || "-";
  const meta = topologyNodeMeta(node);
  return `${node.name || node.id} · ${meta.typeLabel || nodeTypeLabel(node.type)}`;
}

function cleanProperties(value: Record<string, string>) {
  const out: Record<string, string> = {};
  for (const [key, item] of Object.entries(value)) {
    if (item.trim()) out[key] = item.trim();
  }
  return out;
}
