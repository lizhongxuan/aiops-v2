import { Plus, RefreshCcw, Save, Search, Trash2, Zap } from "lucide-react";
import { useEffect, useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { ComplexPageFrame, EmptyPanel, MetricStrip, RiskBadge } from "@/pages/complexPageComponents";
import {
  compactText,
  deleteMcpServer,
  fetchMcpServers,
  refreshMcpServers,
  runMcpServerAction,
  saveMcpServer,
  type McpServerRecord,
} from "@/pages/complexPagesApi";
import { ConfirmButton, Field, LoadingState, SelectField, StatusAlert } from "@/pages/settingsComponents";

type McpDraft = {
  originalName: string;
  name: string;
  transport: string;
  command: string;
  argsText: string;
  url: string;
  envText: string;
  disabled: boolean;
};

const blankDraft: McpDraft = {
  originalName: "",
  name: "",
  transport: "http",
  command: "",
  argsText: "",
  url: "",
  envText: "{}",
  disabled: false,
};

function normalizeServer(item: Partial<McpServerRecord> = {}): McpServerRecord {
  return {
    name: compactText(item.name),
    transport: compactText(item.transport) || "http",
    command: compactText(item.command),
    args: Array.isArray(item.args) ? item.args.map(compactText).filter(Boolean) : [],
    url: compactText(item.url),
    env: item.env && typeof item.env === "object" ? item.env : {},
    disabled: Boolean(item.disabled),
    status: compactText(item.status) || "disconnected",
    error: compactText(item.error),
    toolCount: Number(item.toolCount || 0),
    resourceCount: Number(item.resourceCount || 0),
  };
}

function uniqueName(items: McpServerRecord[]) {
  const names = new Set(items.map((item) => compactText(item.name)).filter(Boolean));
  let index = 1;
  let candidate = `custom-mcp-${index}`;
  while (names.has(candidate)) {
    index += 1;
    candidate = `custom-mcp-${index}`;
  }
  return candidate;
}

function draftFromItem(item: McpServerRecord | null, items: McpServerRecord[]): McpDraft {
  const normalized = normalizeServer(item || {});
  return {
    originalName: normalized.name,
    name: normalized.name || uniqueName(items),
    transport: normalized.transport || "http",
    command: normalized.command || "",
    argsText: (normalized.args || []).join("\n"),
    url: normalized.url || "",
    envText: JSON.stringify(normalized.env || {}, null, 2),
    disabled: Boolean(normalized.disabled),
  };
}

function parseEnv(text: string) {
  const raw = compactText(text);
  if (!raw) return {};
  const parsed = JSON.parse(raw);
  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
    throw new Error("环境变量必须是 JSON 对象");
  }
  return Object.fromEntries(Object.entries(parsed).map(([key, value]) => [String(key), String(value ?? "")]));
}

function payloadFromDraft(draft: McpDraft): McpServerRecord {
  return {
    name: compactText(draft.name),
    transport: compactText(draft.transport),
    command: compactText(draft.command),
    args: draft.argsText.split("\n").map(compactText).filter(Boolean),
    url: compactText(draft.url),
    env: parseEnv(draft.envText),
    disabled: Boolean(draft.disabled),
  };
}

export function McpServersPage() {
  const [items, setItems] = useState<McpServerRecord[]>([]);
  const [configPath, setConfigPath] = useState("");
  const [selectedName, setSelectedName] = useState("");
  const [query, setQuery] = useState("");
  const [draft, setDraft] = useState<McpDraft>(blankDraft);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [message, setMessage] = useState<{ type: "success" | "error" | "info"; text: string } | null>(null);

  const filteredItems = useMemo(() => {
    const keyword = compactText(query).toLowerCase();
    if (!keyword) return items;
    return items.filter((item) => [item.name, item.transport, item.status, item.url, item.command].map((value) => compactText(value).toLowerCase()).some((value) => value.includes(keyword)));
  }, [items, query]);
  const selected = items.find((item) => item.name === selectedName) || null;

  function applyItems(nextItems: McpServerRecord[], preferred = selectedName) {
    const normalized = nextItems.map(normalizeServer);
    const next = normalized.find((item) => item.name === preferred) || normalized[0] || null;
    setItems(normalized);
    setSelectedName(next?.name || "");
    setDraft(draftFromItem(next, normalized));
  }

  async function load() {
    setLoading(true);
    try {
      const payload = await fetchMcpServers();
      setConfigPath(compactText(payload.configPath));
      applyItems(payload.items || []);
      setMessage(null);
    } catch (error) {
      setMessage({ type: "error", text: error instanceof Error ? error.message : "加载 MCP 列表失败" });
      applyItems([]);
    } finally {
      setLoading(false);
    }
  }

  function openCreate() {
    setSelectedName("");
    setDraft({ ...blankDraft, name: uniqueName(items) });
    setDialogOpen(true);
  }

  function openEdit(item: McpServerRecord) {
    setSelectedName(item.name);
    setDraft(draftFromItem(item, items));
    setDialogOpen(true);
  }

  async function save() {
    setSaving(true);
    try {
      const payload = payloadFromDraft(draft);
      if (!payload.name) throw new Error("请先填写 MCP 名称");
      const result = await saveMcpServer(draft.originalName && draft.originalName === payload.name ? draft.originalName : "", payload);
      if (draft.originalName && draft.originalName !== payload.name) {
        await deleteMcpServer(draft.originalName);
      }
      applyItems(result.items || [], payload.name);
      setDialogOpen(false);
      setMessage({ type: "success", text: "MCP server 已保存" });
    } catch (error) {
      setMessage({ type: "error", text: error instanceof Error ? error.message : "保存 MCP 失败" });
    } finally {
      setSaving(false);
    }
  }

  async function remove() {
    const target = selected?.name || draft.originalName || draft.name;
    if (!target) return;
    setSaving(true);
    try {
      const result = await deleteMcpServer(target);
      applyItems(result.items || []);
      setDialogOpen(false);
      setMessage({ type: "success", text: "MCP server 已删除" });
    } catch (error) {
      setMessage({ type: "error", text: error instanceof Error ? error.message : "删除 MCP 失败" });
    } finally {
      setSaving(false);
    }
  }

  async function action(name: string, nextAction: string) {
    setSaving(true);
    try {
      const result = nextAction === "refresh-all" ? await refreshMcpServers() : await runMcpServerAction(name, nextAction);
      applyItems(result.items || [], name);
      setMessage({ type: "success", text: "MCP action 已执行" });
    } catch (error) {
      setMessage({ type: "error", text: error instanceof Error ? error.message : "MCP action 失败" });
    } finally {
      setSaving(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  return (
    <ComplexPageFrame
      kicker="MCP Servers"
      title="MCP Servers"
      description="管理 MCP runtime server 配置；运行时 action 使用现有 MCP permission/action path。"
      actions={
        <>
          <Button variant="outline" onClick={() => void action(selectedName, "refresh-all")} disabled={saving}>
            <RefreshCcw />
            刷新全部
          </Button>
          <Button onClick={openCreate}>
            <Plus />
            添加 MCP
          </Button>
        </>
      }
    >
      {message ? <StatusAlert type={message.type} title={message.type === "error" ? "操作失败" : "操作完成"} message={message.text} /> : null}
      <MetricStrip
        items={[
          { label: "Servers", value: items.length },
          { label: "Connected", value: items.filter((item) => item.status === "connected").length, tone: "ok" },
          { label: "Disabled", value: items.filter((item) => item.disabled).length, tone: "warn" },
          { label: "Config", value: configPath || "mcp-servers.json" },
        ]}
      />
      {loading ? (
        <LoadingState label="加载 MCP servers" />
      ) : (
        <Card className="rounded-lg bg-white">
          <CardHeader>
            <CardTitle>Server 列表</CardTitle>
            <CardDescription>搜索、刷新、打开/关闭、编辑和删除 MCP server。</CardDescription>
          </CardHeader>
          <CardContent className="grid gap-3">
            <label className="relative">
              <Search className="pointer-events-none absolute left-2.5 top-2 h-4 w-4 text-slate-400" />
              <Input className="pl-8" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索名称、transport、status、url、command" />
            </label>
            {filteredItems.length ? (
              <div className="overflow-x-auto">
                <table className="w-full min-w-[860px] text-left text-sm">
                  <thead className="border-b text-xs uppercase tracking-normal text-slate-500">
                    <tr>
                      <th className="py-2 pr-3">Server</th>
                      <th className="py-2 pr-3">Transport</th>
                      <th className="py-2 pr-3">Status</th>
                      <th className="py-2 pr-3">Tools / Resources</th>
                      <th className="py-2 text-right">操作</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y">
                    {filteredItems.map((item) => (
                      <tr key={item.name}>
                        <td className="py-3 pr-3">
                          <div className="font-medium">{item.name}</div>
                          <div className="text-xs text-slate-500">{item.url || item.command || item.error || "-"}</div>
                        </td>
                        <td className="py-3 pr-3">{item.transport}</td>
                        <td className="py-3 pr-3"><RiskBadge value={item.disabled ? "disabled" : item.status} /></td>
                        <td className="py-3 pr-3">{item.toolCount || 0} / {item.resourceCount || 0}</td>
                        <td className="py-3">
                          <div className="flex justify-end gap-2">
                            <Button variant="outline" onClick={() => void action(item.name, item.status === "connected" ? "close" : "open")} disabled={saving}>
                              <Zap />
                              {item.status === "connected" ? "关闭" : "打开"}
                            </Button>
                            <Button variant="outline" onClick={() => void action(item.name, "refresh")} disabled={saving}>刷新</Button>
                            <Button variant="outline" onClick={() => openEdit(item)}>编辑</Button>
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            ) : (
              <EmptyPanel title="暂无 MCP server" description="添加一个 server 或调整搜索条件。" />
            )}
          </CardContent>
        </Card>
      )}

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>{draft.originalName ? "编辑 MCP server" : "添加 MCP server"}</DialogTitle>
          </DialogHeader>
          <div className="grid gap-3 md:grid-cols-2">
            <Field label="名称">
              <Input value={draft.name} onChange={(event) => setDraft((prev) => ({ ...prev, name: event.target.value }))} />
            </Field>
            <Field label="Transport">
              <SelectField value={draft.transport} onChange={(transport) => setDraft((prev) => ({ ...prev, transport }))} options={[{ label: "http", value: "http" }, { label: "command", value: "command" }, { label: "stdio", value: "stdio" }]} />
            </Field>
            <Field label="Command">
              <Input value={draft.command} onChange={(event) => setDraft((prev) => ({ ...prev, command: event.target.value }))} />
            </Field>
            <Field label="URL">
              <Input value={draft.url} onChange={(event) => setDraft((prev) => ({ ...prev, url: event.target.value }))} />
            </Field>
            <Field label="Disabled">
              <SelectField value={draft.disabled ? "disabled" : "enabled"} onChange={(value) => setDraft((prev) => ({ ...prev, disabled: value === "disabled" }))} options={[{ label: "enabled", value: "enabled" }, { label: "disabled", value: "disabled" }]} />
            </Field>
            <Field label="Args（每行一个）">
              <Textarea rows={4} value={draft.argsText} onChange={(event) => setDraft((prev) => ({ ...prev, argsText: event.target.value }))} />
            </Field>
            <Field label="Env JSON">
              <Textarea rows={4} value={draft.envText} onChange={(event) => setDraft((prev) => ({ ...prev, envText: event.target.value }))} />
            </Field>
          </div>
          <DialogFooter>
            <ConfirmButton variant="destructive" confirm={`确认删除 MCP ${draft.originalName || draft.name}？`} onConfirm={() => void remove()} disabled={!draft.originalName || saving}>
              <Trash2 />
              删除
            </ConfirmButton>
            <Button onClick={() => void save()} disabled={saving || !draft.name}>
              <Save />
              保存
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </ComplexPageFrame>
  );
}
