import { Download, Plus, RotateCcw, Save, Upload, X } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Sheet, SheetContent, SheetDescription, SheetFooter, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { fetchAgentProfilePreview, type AgentProfilePreview, type CapabilitySnapshotItem } from "@/api/agentProfilePreview";
import { ConfirmButton, EmptyState, Field, LoadingState, SelectField, SettingsPageFrame, StatGrid, StatusAlert, ToneBadge } from "@/pages/settingsComponents";
import {
  exportAgentProfiles,
  fetchAgentProfile,
  fetchAgentProfiles,
  importAgentProfiles,
  resetAgentProfile,
  saveAgentProfile,
  type AgentProfileRecord,
  type McpCatalogItem,
  type SkillCatalogItem,
} from "@/pages/settingsApi";
import {
  agentProfileSignature,
  buildAgentProfilePayload,
  compactText,
  normalizeAgentProfile,
  normalizeActivationMode,
  normalizeMcpItem,
  normalizeMcpPermission,
  normalizeSkillItem,
  type AgentProfileDraft,
} from "@/pages/settingsCatalogViewModels";

function downloadJson(filename: string, content: string) {
  const blob = new Blob([content], { type: "application/json;charset=utf-8" });
  const url = window.URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = filename;
  link.click();
  window.URL.revokeObjectURL(url);
}

export function AgentProfilePage() {
  const [profiles, setProfiles] = useState<AgentProfileDraft[]>([]);
  const [skillCatalog, setSkillCatalog] = useState<SkillCatalogItem[]>([]);
  const [mcpCatalog, setMcpCatalog] = useState<McpCatalogItem[]>([]);
  const [selectedId, setSelectedId] = useState("main-agent");
  const [draft, setDraft] = useState<AgentProfileDraft>(() => normalizeAgentProfile());
  const [baseline, setBaseline] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [bindingSheet, setBindingSheet] = useState<"skills" | "mcps" | null>(null);
  const [preview, setPreview] = useState<AgentProfilePreview | null>(null);
  const [message, setMessage] = useState<{ type: "success" | "error" | "info"; text: string } | null>(null);
  const importInputRef = useRef<HTMLInputElement | null>(null);

  const isDirty = agentProfileSignature(draft) !== baseline;
  const selectedProfile = profiles.find((profile) => profile.id === selectedId) || profiles[0] || draft;
  const availableSkills = useMemo(() => {
    const boundIds = new Set(draft.skills.map((item) => item.id));
    return skillCatalog.map((item) => normalizeSkillItem(item)).filter((item) => item.id && !boundIds.has(item.id));
  }, [draft.skills, skillCatalog]);
  const availableMcps = useMemo(() => {
    const boundIds = new Set(draft.mcps.map((item) => item.id));
    return mcpCatalog.map((item) => normalizeMcpItem(item)).filter((item) => item.id && !boundIds.has(item.id));
  }, [draft.mcps, mcpCatalog]);

  function applyDraft(profile: AgentProfileRecord | AgentProfileDraft, catalogs = { skills: skillCatalog, mcps: mcpCatalog }) {
    const normalized = normalizeAgentProfile(profile, catalogs);
    setDraft(normalized);
    setBaseline(agentProfileSignature(normalized));
  }

  async function load() {
    setLoading(true);
    try {
      const listPayload = await fetchAgentProfiles().catch(() => null);
      const singlePayload = listPayload ? null : await fetchAgentProfile();
      const nextSkills = listPayload?.skillCatalog || [];
      const nextMcps = listPayload?.mcpCatalog || [];
      const rawProfiles: AgentProfileRecord[] = Array.isArray(listPayload?.items)
        ? listPayload.items
        : Array.isArray(listPayload?.profiles)
          ? listPayload.profiles
          : singlePayload
            ? [singlePayload]
            : [];
      const catalogs = { skills: nextSkills, mcps: nextMcps };
      const nextProfiles = (rawProfiles.length ? rawProfiles : [normalizeAgentProfile({}, catalogs)]).map((profile) => normalizeAgentProfile(profile, catalogs));
      const nextSelected = nextProfiles.find((profile) => profile.id === selectedId) || nextProfiles[0];
      setProfiles(nextProfiles);
      setSkillCatalog(nextSkills);
      setMcpCatalog(nextMcps);
      setSelectedId(nextSelected.id);
      applyDraft(nextSelected, catalogs);
      await loadProfilePreview(nextSelected.id);
      setMessage(null);
    } catch (error) {
      setMessage({ type: "error", text: error instanceof Error ? error.message : "加载 Agent Profiles 失败" });
    } finally {
      setLoading(false);
    }
  }

  async function loadProfilePreview(profileId: string) {
    setPreview(null);
    try {
      setPreview(await fetchAgentProfilePreview(profileId));
    } catch (error) {
      setMessage({ type: "info", text: error instanceof Error ? error.message : "Capability preview 暂不可用" });
    }
  }

  async function saveProfile() {
    setSaving(true);
    try {
      const saved = await saveAgentProfile(buildAgentProfilePayload(draft));
      const normalized = normalizeAgentProfile(saved, { skills: skillCatalog, mcps: mcpCatalog });
      setProfiles((prev) => {
        const index = prev.findIndex((item) => item.id === normalized.id);
        if (index < 0) return [...prev, normalized];
        return prev.map((item, itemIndex) => (itemIndex === index ? normalized : item));
      });
      setSelectedId(normalized.id);
      applyDraft(normalized);
      await loadProfilePreview(normalized.id);
      setMessage({ type: "success", text: "Agent Profile 已保存" });
    } catch (error) {
      setMessage({ type: "error", text: error instanceof Error ? error.message : "保存 Agent Profile 失败" });
    } finally {
      setSaving(false);
    }
  }

  async function resetProfile() {
    setSaving(true);
    try {
      const reset = await resetAgentProfile(selectedId);
      const normalized = normalizeAgentProfile(reset, { skills: skillCatalog, mcps: mcpCatalog });
      setProfiles((prev) => prev.map((item) => (item.id === normalized.id ? normalized : item)));
      applyDraft(normalized);
      await loadProfilePreview(normalized.id);
      setMessage({ type: "success", text: "Agent Profile 已恢复默认" });
    } catch (error) {
      setMessage({ type: "error", text: error instanceof Error ? error.message : "恢复默认失败" });
    } finally {
      setSaving(false);
    }
  }

  async function exportProfiles() {
    try {
      const payload = await exportAgentProfiles();
      const content = JSON.stringify(payload, null, 2);
      downloadJson(`agent-profiles-${new Date().toISOString().replace(/[:.]/g, "-")}.json`, content);
      setMessage({ type: "success", text: "Agent Profiles 已导出" });
    } catch (error) {
      setMessage({ type: "error", text: error instanceof Error ? error.message : "导出失败" });
    }
  }

  async function importFile(file: File) {
    setSaving(true);
    try {
      const text = await file.text();
      await importAgentProfiles(JSON.parse(text));
      await load();
      setMessage({ type: "success", text: "Agent Profiles 已导入" });
    } catch (error) {
      setMessage({ type: "error", text: error instanceof Error ? error.message : "导入失败" });
    } finally {
      setSaving(false);
    }
  }

  function switchProfile(profileId: string) {
    const next = profiles.find((profile) => profile.id === profileId);
    if (!next) return;
    setSelectedId(profileId);
    applyDraft(next);
    void loadProfilePreview(profileId);
  }

  function addSkill(item: SkillCatalogItem) {
    setDraft((prev) => ({ ...prev, skills: [...prev.skills, normalizeSkillItem(item)] }));
  }

  function addMcp(item: McpCatalogItem) {
    setDraft((prev) => ({ ...prev, mcps: [...prev.mcps, normalizeMcpItem(item)] }));
  }

  useEffect(() => {
    void load();
  }, []);

  return (
    <TooltipProvider>
      <SettingsPageFrame
        title="Agent Profile"
        description="管理 system prompt、运行参数、skills 与 MCP 绑定。"
        actions={
          <>
            <Button variant="outline" onClick={() => void exportProfiles()} disabled={saving}>
              <Download />
              导出
            </Button>
            <Button variant="outline" onClick={() => importInputRef.current?.click()} disabled={saving}>
              <Upload />
              导入
            </Button>
            <ConfirmButton variant="outline" confirm={`确认恢复 ${selectedId} 的默认配置吗？`} onConfirm={() => void resetProfile()} disabled={saving}>
              <RotateCcw />
              恢复默认
            </ConfirmButton>
            <Button data-testid="save-profile-btn" onClick={() => void saveProfile()} disabled={saving || !isDirty}>
              <Save />
              保存
            </Button>
          </>
        }
      >
        <input
          ref={importInputRef}
          type="file"
          accept="application/json,.json"
          className="hidden"
          onChange={(event) => {
            const file = event.target.files?.[0];
            if (file) void importFile(file);
            event.currentTarget.value = "";
          }}
        />
        {message ? <StatusAlert type={message.type} title={message.type === "error" ? "操作失败" : "操作完成"} message={message.text} /> : null}
        {loading ? (
          <LoadingState label="加载 Agent Profile" />
        ) : (
          <>
            <StatGrid
              items={[
                { label: "Profiles", value: profiles.length },
                { label: "Skills", value: draft.skills.length },
                { label: "MCPs", value: draft.mcps.length },
                { label: "状态", value: isDirty ? "未保存" : "已同步", tone: isDirty ? "warn" : "ok" },
              ]}
            />

            <EffectiveCapabilitiesPreview preview={preview} />

            <Tabs defaultValue="prompt" className="grid gap-4">
              <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
                <SelectField value={selectedProfile.id} onChange={switchProfile} options={profiles.map((profile) => ({ label: profile.name || profile.id, value: profile.id }))} />
                <TabsList>
                  <TabsTrigger value="prompt">Prompt</TabsTrigger>
                  <TabsTrigger value="bindings">Bindings</TabsTrigger>
                  <TabsTrigger value="runtime">Runtime</TabsTrigger>
                </TabsList>
              </div>

              <TabsContent value="prompt">
                <Card className="rounded-lg bg-white">
                  <CardHeader>
                    <CardTitle>基础信息与 System Prompt</CardTitle>
                    <CardDescription>保存时直接写入现有 Agent Profile API。</CardDescription>
                  </CardHeader>
                  <CardContent className="grid gap-4">
                    <div className="grid gap-4 md:grid-cols-2">
                      <Field label="Profile ID">
                        <Input value={draft.id} onChange={(event) => setDraft((prev) => ({ ...prev, id: event.target.value }))} />
                      </Field>
                      <Field label="名称">
                        <Input value={draft.name} onChange={(event) => setDraft((prev) => ({ ...prev, name: event.target.value }))} />
                      </Field>
                    </div>
                    <Field label="描述">
                      <Input value={draft.description} onChange={(event) => setDraft((prev) => ({ ...prev, description: event.target.value }))} />
                    </Field>
                    <Field label="System Prompt" hint={`${draft.systemPrompt.length} chars`}>
                      <Textarea rows={16} value={draft.systemPrompt} onChange={(event) => setDraft((prev) => ({ ...prev, systemPrompt: event.target.value }))} />
                    </Field>
                  </CardContent>
                </Card>
              </TabsContent>

              <TabsContent value="bindings">
                <div className="grid gap-4 xl:grid-cols-2">
                  <Card className="rounded-lg bg-white">
                    <CardHeader>
                      <div className="flex items-start justify-between gap-2">
                        <div>
                          <CardTitle>Skills</CardTitle>
                          <CardDescription>控制默认绑定的 Skill 与激活方式。</CardDescription>
                        </div>
                        <Button variant="outline" onClick={() => setBindingSheet("skills")}>
                          <Plus />
                          添加
                        </Button>
                      </div>
                    </CardHeader>
                    <CardContent className="grid gap-2">
                      {draft.skills.length ? (
                        draft.skills.map((skill) => (
                          <Card key={skill.id} size="sm" className="rounded-lg bg-white">
                            <CardContent className="flex items-center justify-between gap-2 pt-0">
                              <div>
                                <div className="font-medium">{skill.name || skill.id}</div>
                                <div className="text-xs text-slate-500">{skill.description || skill.source}</div>
                              </div>
                              <div className="flex items-center gap-2">
                                <ToneBadge>{skill.activationMode || skill.defaultActivationMode || "explicit_only"}</ToneBadge>
                                <Button variant="ghost" size="icon-sm" onClick={() => setDraft((prev) => ({ ...prev, skills: prev.skills.filter((item) => item.id !== skill.id) }))}>
                                  <X />
                                </Button>
                              </div>
                            </CardContent>
                          </Card>
                        ))
                      ) : (
                        <EmptyState title="暂无 Skill 绑定" description="从 catalog 添加默认绑定。" />
                      )}
                    </CardContent>
                  </Card>

                  <Card className="rounded-lg bg-white">
                    <CardHeader>
                      <div className="flex items-start justify-between gap-2">
                        <div>
                          <CardTitle>MCP</CardTitle>
                          <CardDescription>控制 Agent 可见 MCP 与权限边界。</CardDescription>
                        </div>
                        <Button variant="outline" onClick={() => setBindingSheet("mcps")}>
                          <Plus />
                          添加
                        </Button>
                      </div>
                    </CardHeader>
                    <CardContent className="grid gap-2">
                      {draft.mcps.length ? (
                        draft.mcps.map((mcp) => (
                          <Card key={mcp.id} size="sm" className="rounded-lg bg-white">
                            <CardContent className="flex items-center justify-between gap-2 pt-0">
                              <div>
                                <div className="font-medium">{mcp.name || mcp.id}</div>
                                <div className="text-xs text-slate-500">{mcp.type} · {mcp.source}</div>
                              </div>
                              <div className="flex items-center gap-2">
                                <ToneBadge tone={mcp.permission === "readwrite" ? "warning" : "default"}>{mcp.permission || "readonly"}</ToneBadge>
                                <Button variant="ghost" size="icon-sm" onClick={() => setDraft((prev) => ({ ...prev, mcps: prev.mcps.filter((item) => item.id !== mcp.id) }))}>
                                  <X />
                                </Button>
                              </div>
                            </CardContent>
                          </Card>
                        ))
                      ) : (
                        <EmptyState title="暂无 MCP 绑定" description="从 catalog 添加默认绑定。" />
                      )}
                    </CardContent>
                  </Card>
                </div>
              </TabsContent>

              <TabsContent value="runtime">
                <Card className="rounded-lg bg-white">
                  <CardHeader>
                    <CardTitle>运行参数</CardTitle>
                    <CardDescription>轻量编辑常用 runtime 字段，完整结构保留在 payload 中。</CardDescription>
                  </CardHeader>
                  <CardContent className="grid gap-4 md:grid-cols-3">
                    <Field label="Model">
                      <Input value={compactText(draft.runtime.model)} onChange={(event) => setDraft((prev) => ({ ...prev, runtime: { ...prev.runtime, model: event.target.value } }))} />
                    </Field>
                    <Field label="Approval Mode">
                      <SelectField
                        value={compactText(draft.runtime.approvalMode) || "on-request"}
                        onChange={(value) => setDraft((prev) => ({ ...prev, runtime: { ...prev.runtime, approvalMode: value } }))}
                        options={[
                          { label: "on-request", value: "on-request" },
                          { label: "never", value: "never" },
                          { label: "untrusted", value: "untrusted" },
                        ]}
                      />
                    </Field>
                    <Field label="Sandbox">
                      <SelectField
                        value={compactText(draft.runtime.sandboxMode) || "workspace-write"}
                        onChange={(value) => setDraft((prev) => ({ ...prev, runtime: { ...prev.runtime, sandboxMode: value } }))}
                        options={[
                          { label: "workspace-write", value: "workspace-write" },
                          { label: "read-only", value: "read-only" },
                          { label: "danger-full-access", value: "danger-full-access" },
                        ]}
                      />
                    </Field>
                  </CardContent>
                </Card>
              </TabsContent>
            </Tabs>
          </>
        )}

        <Sheet open={Boolean(bindingSheet)} onOpenChange={(open) => !open && setBindingSheet(null)}>
          <SheetContent className="sm:max-w-lg">
            <SheetHeader>
              <SheetTitle>{bindingSheet === "skills" ? "添加 Skill" : "添加 MCP"}</SheetTitle>
              <SheetDescription>从 catalog 中选择尚未绑定的条目。</SheetDescription>
            </SheetHeader>
            <div className="grid gap-2 overflow-auto px-4">
              {bindingSheet === "skills"
                ? availableSkills.map((skill) => (
                    <Button key={skill.id} variant="outline" className="h-auto justify-start whitespace-normal py-3" onClick={() => addSkill({ ...skill, activationMode: normalizeActivationMode(skill.defaultActivationMode, skill.defaultEnabled) })}>
                      <span className="text-left">
                        <span className="block font-medium">{skill.name || skill.id}</span>
                        <span className="block text-xs text-slate-500">{skill.description || skill.source}</span>
                      </span>
                    </Button>
                  ))
                : availableMcps.map((mcp) => (
                    <Tooltip key={mcp.id}>
                      <TooltipTrigger asChild>
                        <Button variant="outline" className="h-auto justify-start whitespace-normal py-3" onClick={() => addMcp({ ...mcp, permission: normalizeMcpPermission(mcp.permission) })}>
                          <span className="text-left">
                            <span className="block font-medium">{mcp.name || mcp.id}</span>
                            <span className="block text-xs text-slate-500">{mcp.type} · {mcp.permission}</span>
                          </span>
                        </Button>
                      </TooltipTrigger>
                      <TooltipContent>添加后保存 Profile 生效</TooltipContent>
                    </Tooltip>
                  ))}
              {bindingSheet === "skills" && !availableSkills.length ? <EmptyState title="没有可添加 Skill" description="所有 Skill 已绑定。" /> : null}
              {bindingSheet === "mcps" && !availableMcps.length ? <EmptyState title="没有可添加 MCP" description="所有 MCP 已绑定。" /> : null}
            </div>
            <SheetFooter>
              <Button variant="outline" onClick={() => setBindingSheet(null)}>关闭</Button>
            </SheetFooter>
          </SheetContent>
        </Sheet>
      </SettingsPageFrame>
    </TooltipProvider>
  );
}

function EffectiveCapabilitiesPreview({ preview }: { preview: AgentProfilePreview | null }) {
  const snapshot = preview?.capabilitySnapshot;
  const items = Array.isArray(snapshot?.items) ? snapshot.items : [];
  const enabled = items.filter((item) => item.enabled);
  const disabled = items.filter((item) => !item.enabled);

  return (
    <Card className="rounded-lg bg-white">
      <CardHeader>
        <div className="flex flex-col gap-2 md:flex-row md:items-start md:justify-between">
          <div>
            <CardTitle>Effective Capabilities</CardTitle>
            <CardDescription>当前 Profile 在下一 turn 生效的 Skill 与 MCP 可见性。</CardDescription>
          </div>
          <div className="font-mono text-xs text-slate-500">{snapshot?.fingerprint || "preview pending"}</div>
        </div>
      </CardHeader>
      <CardContent className="grid gap-4 xl:grid-cols-2">
        <CapabilityPreviewGroup title="Enabled" items={enabled} testId="effective-capabilities-enabled" />
        <CapabilityPreviewGroup title="Disabled / Pending" items={disabled} testId="effective-capabilities-disabled" />
      </CardContent>
    </Card>
  );
}

function CapabilityPreviewGroup({ title, items, testId }: { title: string; items: CapabilitySnapshotItem[]; testId: string }) {
  return (
    <div data-testid={testId} className="grid content-start gap-2">
      <div className="flex items-center justify-between gap-2">
        <div className="text-sm font-medium text-slate-900">{title}</div>
        <ToneBadge tone={title === "Enabled" ? "success" : "warning"}>{items.length}</ToneBadge>
      </div>
      {items.length ? (
        items.map((item) => <CapabilityPreviewRow key={`${item.kind}:${item.id}`} item={item} />)
      ) : (
        <div className="rounded-lg border border-dashed border-slate-200 p-3 text-sm text-slate-500">No capabilities</div>
      )}
    </div>
  );
}

function CapabilityPreviewRow({ item }: { item: CapabilitySnapshotItem }) {
  const status = item.runtimeStatus || item.approvalStatus || (item.enabled ? "enabled" : "disabled");
  return (
    <div className="rounded-lg border border-slate-200 p-3">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <div className="break-words text-sm font-medium text-slate-950">{item.id}</div>
          <div className="mt-1 flex flex-wrap gap-1.5">
            <ToneBadge>{item.kind}</ToneBadge>
            <ToneBadge>{item.sourceScope || item.source || "profile"}</ToneBadge>
            <ToneBadge tone={riskTone(item.risk)}>{item.risk || "low"}</ToneBadge>
            <ToneBadge tone={statusTone(status)}>{status}</ToneBadge>
          </div>
        </div>
        {item.policy ? <ToneBadge tone="warning">{item.policy}</ToneBadge> : null}
      </div>
      {item.reason ? <div className="mt-2 break-words text-xs text-slate-500">{item.reason}</div> : null}
    </div>
  );
}

function statusTone(status: string): "default" | "success" | "warning" | "danger" {
  const normalized = status.toLowerCase();
  if (normalized === "connected" || normalized === "available" || normalized === "enabled") return "success";
  if (normalized.includes("pending")) return "warning";
  if (normalized.includes("disabled") || normalized.includes("unavailable") || normalized.includes("denied")) return "danger";
  return "default";
}

function riskTone(risk?: string): "default" | "success" | "warning" | "danger" {
  if (risk === "high") return "danger";
  if (risk === "medium") return "warning";
  if (risk === "low") return "success";
  return "default";
}
