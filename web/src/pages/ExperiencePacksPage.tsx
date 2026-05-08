import { Plus, Search } from "lucide-react";
import { useEffect, useMemo, useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { EmptyState, SettingsPageFrame, ToneBadge } from "@/pages/settingsComponents";
import { compactText } from "@/pages/settingsCatalogViewModels";

import { experiencePacks } from "@/data/opsWorkspace";

type ExperiencePack = {
  id: string;
  name: string;
  summary: string;
  version: string;
  confidence: string;
  confidenceTone?: string;
  status: string;
  statusTone?: string;
  risk: string;
  platform: string;
  bindings: string;
  purpose: string;
  versionTrail?: Array<{ label: string; state?: string }>;
  versionNote?: string;
  resources?: string[];
};

function toTone(tone?: string): "default" | "success" | "warning" | "danger" {
  if (tone === "success") return "success";
  if (tone === "warning" || tone === "purple") return "warning";
  if (tone === "danger") return "danger";
  return "default";
}

export function ExperiencePacksPage() {
  const packs = experiencePacks as ExperiencePack[];
  const [query, setQuery] = useState("");
  const [selectedPackId, setSelectedPackId] = useState(packs[0]?.id || "");

  const filteredPacks = useMemo(() => {
    const keyword = compactText(query).toLowerCase();
    if (!keyword) return packs;
    return packs.filter((pack) =>
      [pack.name, pack.summary, pack.version, pack.purpose, ...(pack.resources || [])].join(" ").toLowerCase().includes(keyword),
    );
  }, [packs, query]);

  const selectedPack = filteredPacks.find((pack) => pack.id === selectedPackId) || filteredPacks[0] || null;

  useEffect(() => {
    if (!filteredPacks.length) {
      setSelectedPackId("");
      return;
    }
    if (!filteredPacks.some((pack) => pack.id === selectedPackId)) {
      setSelectedPackId(filteredPacks[0].id);
    }
  }, [filteredPacks, selectedPackId]);

  return (
    <SettingsPageFrame
      title="经验包库"
      description="把运行成功经验、playbook 与主机画像绑定成可复用的运维资产。"
      actions={
        <Button>
          <Plus />
          新建经验包
        </Button>
      }
    >
      <Card className="rounded-lg bg-white">
        <CardContent className="flex flex-col gap-3 pt-0 lg:flex-row lg:items-center">
          <label className="relative min-w-0 flex-1">
            <Search className="pointer-events-none absolute left-2.5 top-2 h-4 w-4 text-slate-400" />
            <Input className="pl-8" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索经验包、场景、版本、来源" />
          </label>
          <div className="flex flex-wrap gap-2">
            <Badge variant="secondary">场景: nginx</Badge>
            <Badge variant="outline">风险: low</Badge>
            <Badge variant="outline">来源: verified</Badge>
            <Badge variant="outline">适用 OS: Linux</Badge>
          </div>
        </CardContent>
      </Card>

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_420px]">
        <Card className="rounded-lg bg-white">
          <CardHeader>
            <CardTitle>经验包列表</CardTitle>
            <CardDescription>默认按最近使用与成功率排序</CardDescription>
          </CardHeader>
          <CardContent className="grid gap-2">
            {filteredPacks.length ? (
              filteredPacks.map((pack) => (
                <button key={pack.id} type="button" className="text-left" onClick={() => setSelectedPackId(pack.id)}>
                  <Card size="sm" className={selectedPack?.id === pack.id ? "rounded-lg border-slate-400 bg-slate-50" : "rounded-lg bg-white"}>
                    <CardContent className="flex flex-col gap-3 pt-0 md:flex-row md:items-center">
                      <span className="h-9 w-1 rounded-full bg-slate-300 data-[tone=success]:bg-emerald-500 data-[tone=warning]:bg-amber-500" data-tone={toTone(pack.confidenceTone)} />
                      <div className="min-w-0 flex-1">
                        <div className="font-medium text-slate-900">{pack.name}</div>
                        <div className="text-xs leading-5 text-slate-500">{pack.summary}</div>
                        <div className="mt-2 flex flex-wrap gap-1.5">
                          <ToneBadge>{pack.version}</ToneBadge>
                          <ToneBadge tone={toTone(pack.confidenceTone)}>{pack.confidence}</ToneBadge>
                        </div>
                      </div>
                      <div className="text-xs text-slate-500">{pack.bindings}</div>
                    </CardContent>
                  </Card>
                </button>
              ))
            ) : (
              <EmptyState title="没有命中的经验包" description="试试更宽松的关键词。" />
            )}
          </CardContent>
        </Card>

        {selectedPack ? (
          <Card className="rounded-lg bg-white">
            <CardHeader>
              <CardTitle>包详情</CardTitle>
              <CardDescription>
                {selectedPack.name} · {selectedPack.version}
              </CardDescription>
            </CardHeader>
            <CardContent className="grid gap-4">
              <div className="flex flex-wrap gap-2">
                <ToneBadge tone={toTone(selectedPack.statusTone)}>{selectedPack.status}</ToneBadge>
                <ToneBadge>{selectedPack.risk}</ToneBadge>
                <ToneBadge>{selectedPack.platform}</ToneBadge>
              </div>
              <section>
                <div className="text-xs font-medium uppercase tracking-normal text-slate-500">用途</div>
                <p className="mt-1 text-sm leading-6 text-slate-700">{selectedPack.purpose}</p>
              </section>
              <section className="rounded-lg border bg-slate-50 p-3">
                <div className="text-xs font-medium uppercase tracking-normal text-slate-500">版本演进</div>
                <div className="mt-3 flex flex-wrap items-center gap-2">
                  {(selectedPack.versionTrail || []).map((version) => (
                    <ToneBadge key={version.label} tone={toTone(version.state)}>
                      {version.label}
                    </ToneBadge>
                  ))}
                </div>
                <p className="mt-2 text-sm text-slate-600">{selectedPack.versionNote}</p>
              </section>
              <section>
                <div className="text-xs font-medium uppercase tracking-normal text-slate-500">关联资源</div>
                <ul className="mt-2 grid gap-1 font-mono text-xs text-slate-600">
                  {(selectedPack.resources || []).map((resource) => (
                    <li key={resource}>{resource}</li>
                  ))}
                </ul>
              </section>
              <div className="flex flex-wrap gap-2">
                <Button>加载到主 Agent</Button>
                <Button variant="outline">附加到主机组</Button>
                <Button variant="outline">创建新版本</Button>
              </div>
            </CardContent>
          </Card>
        ) : null}
      </div>
    </SettingsPageFrame>
  );
}
