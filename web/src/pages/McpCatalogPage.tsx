import { Plus, RefreshCw, Save, Search, Trash2 } from "lucide-react";
import { useEffect, useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { ConfirmButton, EmptyState, Field, LoadingState, SelectField, SettingsPageFrame, StatGrid, StatusAlert, ToneBadge } from "@/pages/settingsComponents";
import { deleteMcpCatalogItem, fetchMcpCatalog, saveMcpCatalogItem } from "@/pages/settingsApi";
import type { McpCatalogItem } from "@/pages/settingsApi";
import {
  buildMcpPayload,
  createBlankMcpDraft,
  generateUniqueId,
  matchesMcpSearch,
  mcpSignature,
  normalizeMcpItem,
  normalizeMcpPermission,
  type McpCatalogDraft,
} from "@/pages/settingsCatalogViewModels";

export function McpCatalogPage() {
  const [catalog, setCatalog] = useState<McpCatalogItem[]>([]);
  const [selectedId, setSelectedId] = useState("");
  const [query, setQuery] = useState("");
  const [draft, setDraft] = useState<McpCatalogDraft>(() => createBlankMcpDraft());
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState<{ type: "success" | "error" | "info"; text: string } | null>(null);

  const normalizedCatalog = useMemo(() => catalog.map((item) => normalizeMcpItem(item)), [catalog]);
  const filteredCatalog = useMemo(() => normalizedCatalog.filter((item) => matchesMcpSearch(item, query)), [normalizedCatalog, query]);
  const selectedItem = normalizedCatalog.find((item) => item.id === selectedId) || null;
  const isDirty = mcpSignature(draft) !== mcpSignature(selectedItem);

  function syncDraftFromItem(item: Partial<McpCatalogItem> | null) {
    const normalized = item ? normalizeMcpItem(item) : createBlankMcpDraft(normalizedCatalog.length + 1);
    setDraft({
      ...normalized,
      id: normalized.id || `custom-mcp-${normalizedCatalog.length + 1}`,
      name: normalized.name || "Custom MCP",
    });
  }

  async function load() {
    setLoading(true);
    try {
      const payload = await fetchMcpCatalog();
      const items = payload.items || [];
      setCatalog(items);
      const next = items.find((item) => item.id === selectedId) || items[0] || null;
      setSelectedId(next?.id || "");
      syncDraftFromItem(next);
      setMessage(null);
    } catch (error) {
      setMessage({ type: "error", text: error instanceof Error ? error.message : "加载 MCP Catalog 失败" });
    } finally {
      setLoading(false);
    }
  }

  function createNewMcp() {
    const nextId = generateUniqueId("custom-mcp", normalizedCatalog);
    setSelectedId(nextId);
    syncDraftFromItem({ id: nextId, name: "Custom MCP", type: "http", source: "local", defaultEnabled: false, permission: "readonly" });
  }

  async function saveMcp() {
    const payload = buildMcpPayload(draft);
    if (!payload.id || !payload.name) {
      setMessage({ type: "error", text: "请先填写 MCP ID 和名称。" });
      return;
    }
    setSaving(true);
    try {
      const result = await saveMcpCatalogItem(payload);
      const items = result.items || (result.item ? normalizedCatalog.map((item) => (item.id === draft.originalId || item.id === payload.id ? result.item! : item)) : catalog);
      setCatalog(items);
      setSelectedId((result.item?.id || payload.id) as string);
      syncDraftFromItem(result.item || payload);
      setMessage({ type: "success", text: "MCP 已保存" });
    } catch (error) {
      setMessage({ type: "error", text: error instanceof Error ? error.message : "保存 MCP 失败" });
    } finally {
      setSaving(false);
    }
  }

  async function removeMcp() {
    const targetId = selectedItem?.id || draft.originalId || draft.id;
    if (!targetId) return;
    setSaving(true);
    try {
      const result = await deleteMcpCatalogItem(targetId);
      const items = result.items || catalog.filter((item) => item.id !== targetId);
      setCatalog(items);
      const next = items[0] || null;
      setSelectedId(next?.id || "");
      syncDraftFromItem(next);
      setMessage({ type: "success", text: "MCP 已删除" });
    } catch (error) {
      setMessage({ type: "error", text: error instanceof Error ? error.message : "删除 MCP 失败" });
    } finally {
      setSaving(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  return (
    <SettingsPageFrame
      title="MCP 管理"
      description="维护可供 Agent 绑定和调用的 MCP catalog，支持新增、删除、搜索、权限和显式确认设置。"
      actions={
        <>
          <Button variant="outline" onClick={() => void load()} disabled={loading || saving}>
            <RefreshCw />
            刷新
          </Button>
          <Button onClick={createNewMcp}>
            <Plus />
            新增
          </Button>
        </>
      }
    >
      <StatGrid
        items={[
          { label: "总数", value: normalizedCatalog.length },
          { label: "筛选结果", value: filteredCatalog.length },
          { label: "显式确认", value: normalizedCatalog.filter((item) => item.requiresExplicitUserApproval).length },
        ]}
      />
      {message ? <StatusAlert type={message.type} title={message.type === "error" ? "操作失败" : "操作完成"} message={message.text} /> : null}
      {isDirty ? <StatusAlert type="info" title="未保存修改" message="点击保存后才会写回 catalog。" /> : null}

      {loading ? (
        <LoadingState label="加载 MCP" />
      ) : (
        <div className="grid gap-4 xl:grid-cols-[360px_minmax(0,1fr)]">
          <Card className="rounded-lg bg-white">
            <CardHeader>
              <CardTitle>MCP Catalog</CardTitle>
              <CardDescription>点击条目查看并编辑 catalog item 详情。</CardDescription>
            </CardHeader>
            <CardContent className="grid gap-3">
              <label className="relative">
                <Search className="pointer-events-none absolute left-2.5 top-2 h-4 w-4 text-slate-400" />
                <Input className="pl-8" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索 ID / 名称 / 类型 / 来源" />
              </label>
              <div className="grid max-h-[520px] gap-2 overflow-auto">
                {filteredCatalog.length ? (
                  filteredCatalog.map((item) => (
                    <button key={item.id} type="button" className="text-left" onClick={() => { setSelectedId(item.id); syncDraftFromItem(item); }}>
                      <Card size="sm" className={item.id === selectedId ? "rounded-lg border-slate-400 bg-slate-50" : "rounded-lg bg-white"}>
                        <CardContent className="pt-0">
                          <div className="flex items-center justify-between gap-2">
                            <div className="font-medium text-slate-900">{item.name || item.id}</div>
                            <ToneBadge tone={item.permission === "readwrite" ? "warning" : "default"}>{item.permission}</ToneBadge>
                          </div>
                          <div className="mt-1 text-xs leading-5 text-slate-500">{item.type} · {item.source}</div>
                        </CardContent>
                      </Card>
                    </button>
                  ))
                ) : (
                  <EmptyState title="没有匹配 MCP" description="调整关键词或新增 MCP。" />
                )}
              </div>
            </CardContent>
          </Card>

          <Card className="rounded-lg bg-white">
            <CardHeader>
              <CardTitle>MCP 详情</CardTitle>
              <CardDescription>字段会直接保存到 `/api/v1/agent-mcps/:id`。</CardDescription>
            </CardHeader>
            <CardContent className="grid gap-4">
              <div className="grid gap-4 md:grid-cols-2">
                <Field label="MCP ID">
                  <Input value={draft.id} onChange={(event) => setDraft((prev) => ({ ...prev, id: event.target.value }))} />
                </Field>
                <Field label="名称">
                  <Input value={draft.name} onChange={(event) => setDraft((prev) => ({ ...prev, name: event.target.value }))} />
                </Field>
                <Field label="类型">
                  <Input value={draft.type || ""} onChange={(event) => setDraft((prev) => ({ ...prev, type: event.target.value }))} />
                </Field>
                <Field label="来源">
                  <Input value={draft.source || ""} onChange={(event) => setDraft((prev) => ({ ...prev, source: event.target.value }))} />
                </Field>
                <Field label="默认启用">
                  <SelectField
                    value={draft.defaultEnabled ? "enabled" : "disabled"}
                    onChange={(value) => setDraft((prev) => ({ ...prev, defaultEnabled: value === "enabled" }))}
                    options={[
                      { label: "enabled", value: "enabled" },
                      { label: "disabled", value: "disabled" },
                    ]}
                  />
                </Field>
                <Field label="权限">
                  <SelectField
                    value={draft.permission || "readonly"}
                    onChange={(value) => setDraft((prev) => ({ ...prev, permission: normalizeMcpPermission(value) }))}
                    options={[
                      { label: "readonly", value: "readonly" },
                      { label: "readwrite", value: "readwrite" },
                    ]}
                  />
                </Field>
                <Field label="显式确认">
                  <SelectField
                    value={draft.requiresExplicitUserApproval ? "required" : "not_required"}
                    onChange={(value) => setDraft((prev) => ({ ...prev, requiresExplicitUserApproval: value === "required" }))}
                    options={[
                      { label: "required", value: "required" },
                      { label: "not_required", value: "not_required" },
                    ]}
                  />
                </Field>
              </div>
              <div className="flex flex-wrap justify-end gap-2">
                <ConfirmButton variant="destructive" confirm={`确认删除 MCP ${selectedItem?.id || draft.id}？`} onConfirm={() => void removeMcp()} disabled={saving || !selectedItem}>
                  <Trash2 />
                  删除
                </ConfirmButton>
                <Button onClick={() => void saveMcp()} disabled={saving || !draft.id || !draft.name}>
                  <Save />
                  保存
                </Button>
              </div>
            </CardContent>
          </Card>
        </div>
      )}
    </SettingsPageFrame>
  );
}
