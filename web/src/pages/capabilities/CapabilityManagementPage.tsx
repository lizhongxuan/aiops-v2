import { RefreshCcw, Search } from "lucide-react";
import { useEffect, useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { EmptyState, LoadingState, SettingsPageFrame, StatusAlert, ToneBadge } from "@/pages/settingsComponents";

import { fetchCapabilities } from "./capabilityManagementApi";
import type { CapabilityViewItem } from "./capabilityManagementTypes";
import { buildCapabilityManagementViewModel } from "./capabilityManagementViewModel";

function DetailSection({ title, value }: { title: string; value: string }) {
  return (
    <section className="grid gap-1 rounded-lg border border-slate-200 bg-white p-3">
      <h3 className="text-sm font-semibold text-slate-900">{title}</h3>
      <p className="break-words text-sm leading-6 text-slate-600">{value || "未声明"}</p>
    </section>
  );
}

export function CapabilityManagementPage() {
  const [items, setItems] = useState<CapabilityViewItem[]>([]);
  const [query, setQuery] = useState("");
  const [selected, setSelected] = useState<CapabilityViewItem | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  async function load() {
    setLoading(true);
    setError("");
    try {
      const payload = await fetchCapabilities();
      setItems(buildCapabilityManagementViewModel(payload).items);
    } catch (loadError) {
      setItems([]);
      setError(loadError instanceof Error ? loadError.message : "加载能力列表失败");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  const filteredItems = useMemo(() => {
    const keyword = query.trim().toLowerCase();
    if (!keyword) return items;
    return items.filter((item) => [item.name, item.description, item.sourceLabel, item.connectionSummary].some((value) => value.toLowerCase().includes(keyword)));
  }, [items, query]);

  return (
    <SettingsPageFrame title="能力管理" description="统一查看前端可用能力、来源、连接、权限风险、运行时和审计信息。">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-normal text-slate-950">能力管理</h1>
          <p className="mt-1 text-sm text-slate-500">统一能力入口只展示可用能力清单与详情。</p>
        </div>
        <Button variant="outline" onClick={() => void load()} disabled={loading}>
          <RefreshCcw />
          刷新
        </Button>
      </div>

      {error ? <StatusAlert type="error" title="加载失败" message={error} /> : null}

      <Card className="rounded-lg bg-white">
        <CardHeader>
          <CardTitle>能力列表</CardTitle>
          <CardDescription>点击能力查看来源、连接、权限风险、运行时和审计详情。</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-3">
          <label className="relative">
            <Search className="pointer-events-none absolute left-2.5 top-2 h-4 w-4 text-slate-400" />
            <Input className="pl-8" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索能力、来源或连接" aria-label="搜索能力" />
          </label>

          {loading ? (
            <LoadingState label="加载能力" />
          ) : filteredItems.length ? (
            <div className="overflow-x-auto">
              <table className="w-full min-w-[760px] text-left text-sm">
                <thead className="border-b text-xs uppercase tracking-normal text-slate-500">
                  <tr>
                    <th className="py-2 pr-3">能力</th>
                    <th className="py-2 pr-3">来源</th>
                    <th className="py-2 pr-3">连接</th>
                    <th className="py-2 pr-3">权限与风险</th>
                  </tr>
                </thead>
                <tbody>
                  {filteredItems.map((item) => (
                    <tr key={item.id} className="border-b last:border-0">
                      <td className="py-3 pr-3 align-top">
                        <button type="button" className="text-left font-medium text-slate-950 hover:text-slate-700" onClick={() => setSelected(item)}>
                          {item.name}
                        </button>
                        {item.description ? <div className="mt-1 max-w-xl text-xs leading-5 text-slate-500">{item.description}</div> : null}
                      </td>
                      <td className="py-3 pr-3 align-top"><ToneBadge>{item.sourceLabel}</ToneBadge></td>
                      <td className="py-3 pr-3 align-top text-slate-600">{item.connectionSummary}</td>
                      <td className="py-3 pr-3 align-top text-slate-600">{item.permissionRiskSummary}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            <EmptyState title="暂无能力" description="当前没有可展示的统一能力记录。" />
          )}
        </CardContent>
      </Card>

      <Dialog open={Boolean(selected)} onOpenChange={(open) => !open && setSelected(null)}>
        <DialogContent className="max-h-[86vh] overflow-y-auto sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>{selected?.name || "能力详情"}</DialogTitle>
            <DialogDescription>{selected?.description || "能力详情"}</DialogDescription>
          </DialogHeader>
          {selected ? (
            <div className="grid gap-3">
              <DetailSection title="来源" value={selected.sourceLabel} />
              <DetailSection title="连接" value={selected.connectionSummary} />
              <DetailSection title="权限与风险" value={selected.permissionRiskSummary} />
              <DetailSection title="运行时" value={selected.runtimeSummary} />
              <DetailSection title="审计" value={selected.auditSummary} />
            </div>
          ) : null}
        </DialogContent>
      </Dialog>
    </SettingsPageFrame>
  );
}
