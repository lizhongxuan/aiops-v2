import { Plus, Save, Search, Trash2 } from "lucide-react";
import { useEffect, useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { ConfirmButton, EmptyState, Field, LoadingState, SelectField, SettingsPageFrame, StatGrid, StatusAlert, ToneBadge } from "@/pages/settingsComponents";
import { deleteSkillCatalogItem, fetchSkillCatalog, saveSkillCatalogItem } from "@/pages/settingsApi";
import type { SkillCatalogItem } from "@/pages/settingsApi";
import {
  buildSkillPayload,
  createBlankSkillDraft,
  generateUniqueId,
  matchesSkillSearch,
  normalizeActivationMode,
  normalizeSkillItem,
  skillSignature,
  type SkillCatalogDraft,
} from "@/pages/settingsCatalogViewModels";

export function SkillCatalogPage() {
  const [catalog, setCatalog] = useState<SkillCatalogItem[]>([]);
  const [selectedId, setSelectedId] = useState("");
  const [query, setQuery] = useState("");
  const [draft, setDraft] = useState<SkillCatalogDraft>(() => createBlankSkillDraft());
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState<{ type: "success" | "error" | "info"; text: string } | null>(null);

  const normalizedCatalog = useMemo(() => catalog.map((item) => normalizeSkillItem(item)), [catalog]);
  const filteredCatalog = useMemo(() => normalizedCatalog.filter((item) => matchesSkillSearch(item, query)), [normalizedCatalog, query]);
  const selectedItem = normalizedCatalog.find((item) => item.id === selectedId) || null;
  const isDirty = skillSignature(draft) !== skillSignature(selectedItem);

  function syncDraftFromItem(item: Partial<SkillCatalogItem> | null) {
    const normalized = item ? normalizeSkillItem(item) : createBlankSkillDraft(normalizedCatalog.length + 1);
    setDraft({
      ...normalized,
      id: normalized.id || `custom-skill-${normalizedCatalog.length + 1}`,
      name: normalized.name || "Custom Skill",
    });
  }

  async function load() {
    setLoading(true);
    try {
      const payload = await fetchSkillCatalog();
      const items = payload.items || [];
      setCatalog(items);
      const next = items.find((item) => item.id === selectedId) || items[0] || null;
      setSelectedId(next?.id || "");
      syncDraftFromItem(next);
      setMessage(null);
    } catch (error) {
      setMessage({ type: "error", text: error instanceof Error ? error.message : "加载 Skill Catalog 失败" });
    } finally {
      setLoading(false);
    }
  }

  function createNewSkill() {
    const nextId = generateUniqueId("custom-skill", normalizedCatalog);
    setSelectedId(nextId);
    syncDraftFromItem({ id: nextId, name: "Custom Skill", source: "local", defaultEnabled: false, defaultActivationMode: "explicit_only" });
  }

  async function saveSkill() {
    const payload = buildSkillPayload(draft);
    if (!payload.id || !payload.name) {
      setMessage({ type: "error", text: "请先填写 Skill ID 和名称。" });
      return;
    }
    setSaving(true);
    try {
      const result = await saveSkillCatalogItem(payload);
      const items = result.items || (result.item ? normalizedCatalog.map((item) => (item.id === draft.originalId || item.id === payload.id ? result.item! : item)) : catalog);
      setCatalog(items);
      setSelectedId((result.item?.id || payload.id) as string);
      syncDraftFromItem(result.item || payload);
      setMessage({ type: "success", text: "Skill 已保存" });
    } catch (error) {
      setMessage({ type: "error", text: error instanceof Error ? error.message : "保存 Skill 失败" });
    } finally {
      setSaving(false);
    }
  }

  async function removeSkill() {
    const targetId = selectedItem?.id || draft.originalId || draft.id;
    if (!targetId) return;
    setSaving(true);
    try {
      const result = await deleteSkillCatalogItem(targetId);
      const items = result.items || catalog.filter((item) => item.id !== targetId);
      setCatalog(items);
      const next = items[0] || null;
      setSelectedId(next?.id || "");
      syncDraftFromItem(next);
      setMessage({ type: "success", text: "Skill 已删除" });
    } catch (error) {
      setMessage({ type: "error", text: error instanceof Error ? error.message : "删除 Skill 失败" });
    } finally {
      setSaving(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  return (
    <SettingsPageFrame
      title="Skills 管理"
      description="维护可供 Agent 绑定和调用的 skills catalog，支持添加、删除、搜索与基础字段编辑。"
      actions={
        <Button onClick={createNewSkill}>
          <Plus />
          新增
        </Button>
      }
    >
      <StatGrid
        items={[
          { label: "总数", value: normalizedCatalog.length },
          { label: "筛选结果", value: filteredCatalog.length },
          { label: "当前选择", value: selectedItem?.name || "新建项" },
        ]}
      />
      {message ? <StatusAlert type={message.type} title={message.type === "error" ? "操作失败" : "操作完成"} message={message.text} /> : null}
      {isDirty ? <StatusAlert type="info" title="未保存修改" message="点击保存后才会写回 catalog。" /> : null}

      {loading ? (
        <LoadingState label="加载 Skills" />
      ) : (
        <div className="grid gap-4 xl:grid-cols-[360px_minmax(0,1fr)]">
          <Card className="rounded-lg bg-white">
            <CardHeader>
              <CardTitle>Skill Catalog</CardTitle>
              <CardDescription>点击条目查看并编辑 catalog item 详情。</CardDescription>
            </CardHeader>
            <CardContent className="grid gap-3">
              <label className="relative">
                <Search className="pointer-events-none absolute left-2.5 top-2 h-4 w-4 text-slate-400" />
                <Input className="pl-8" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索 ID / 名称 / 来源" />
              </label>
              <div className="grid max-h-[520px] gap-2 overflow-auto">
                {filteredCatalog.length ? (
                  filteredCatalog.map((item) => (
                    <button key={item.id} type="button" className="text-left" onClick={() => { setSelectedId(item.id); syncDraftFromItem(item); }}>
                      <Card size="sm" className={item.id === selectedId ? "rounded-lg border-slate-400 bg-slate-50" : "rounded-lg bg-white"}>
                        <CardContent className="pt-0">
                          <div className="flex items-center justify-between gap-2">
                            <div className="font-medium text-slate-900">{item.name || item.id}</div>
                            <ToneBadge tone={item.defaultEnabled ? "success" : "default"}>{item.defaultActivationMode}</ToneBadge>
                          </div>
                          <div className="mt-1 text-xs leading-5 text-slate-500">{item.description || item.source}</div>
                        </CardContent>
                      </Card>
                    </button>
                  ))
                ) : (
                  <EmptyState title="没有匹配 Skill" description="调整关键词或新增 Skill。" />
                )}
              </div>
            </CardContent>
          </Card>

          <Card className="rounded-lg bg-white">
            <CardHeader>
              <CardTitle>Skill 详情</CardTitle>
              <CardDescription>字段会直接保存到 `/api/v1/agent-skills/:id`。</CardDescription>
            </CardHeader>
            <CardContent className="grid gap-4">
              <div className="grid gap-4 md:grid-cols-2">
                <Field label="Skill ID">
                  <Input value={draft.id} onChange={(event) => setDraft((prev) => ({ ...prev, id: event.target.value }))} />
                </Field>
                <Field label="名称">
                  <Input value={draft.name} onChange={(event) => setDraft((prev) => ({ ...prev, name: event.target.value }))} />
                </Field>
                <Field label="来源">
                  <Input value={draft.source || ""} onChange={(event) => setDraft((prev) => ({ ...prev, source: event.target.value }))} />
                </Field>
                <Field label="默认激活">
                  <SelectField
                    value={draft.defaultActivationMode || "explicit_only"}
                    onChange={(value) => setDraft((prev) => ({ ...prev, defaultActivationMode: normalizeActivationMode(value, prev.defaultEnabled), defaultEnabled: value === "default_enabled" }))}
                    options={[
                      { label: "default_enabled", value: "default_enabled" },
                      { label: "explicit_only", value: "explicit_only" },
                      { label: "disabled", value: "disabled" },
                    ]}
                  />
                </Field>
              </div>
              <Field label="描述">
                <Textarea rows={6} value={draft.description || ""} onChange={(event) => setDraft((prev) => ({ ...prev, description: event.target.value }))} />
              </Field>
              <div className="flex flex-wrap justify-end gap-2">
                <ConfirmButton variant="destructive" confirm={`确认删除 Skill ${selectedItem?.id || draft.id}？`} onConfirm={() => void removeSkill()} disabled={saving || !selectedItem}>
                  <Trash2 />
                  删除
                </ConfirmButton>
                <Button onClick={() => void saveSkill()} disabled={saving || !draft.id || !draft.name}>
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
