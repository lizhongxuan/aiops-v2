import { useEffect, useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { SettingsPageFrame, ToneBadge } from "@/pages/settingsComponents";

type UiCard = {
  id: string;
  name: string;
  kind?: string;
  renderer?: string;
  status?: string;
  builtIn?: boolean;
  version?: number;
  summary?: string;
  capabilities?: string[];
  triggerTypes?: string[];
};

type CardsResponse = { items?: UiCard[]; stats?: Record<string, number>; total?: number };

async function requestJson<T>(path: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(new URL(path, window.location.origin).toString(), { credentials: "include", ...init, headers: { "Content-Type": "application/json", ...(init.headers || {}) } });
  const payload = (await response.json().catch(() => ({}))) as T;
  if (!response.ok) throw new Error(`Request failed: ${response.status}`);
  return payload;
}

export function UICardManagementPage() {
  const [cards, setCards] = useState<UiCard[]>([]);
  const [stats, setStats] = useState<Record<string, number>>({});
  const [tab, setTab] = useState("overview");
  const [selectedId, setSelectedId] = useState("");
  const [editDraft, setEditDraft] = useState<UiCard | null>(null);
  const [previewResult, setPreviewResult] = useState<Record<string, unknown> | null>(null);
  const [debugInput, setDebugInput] = useState('{"key":"value"}');

  async function load() {
    const payload = await requestJson<CardsResponse>("/api/v1/ui-cards");
    setCards(payload.items || []);
    setStats(payload.stats || {});
    setSelectedId((current) => current || payload.items?.[0]?.id || "");
  }

  useEffect(() => { void load().catch(() => undefined); }, []);

  const selectedCard = useMemo(() => cards.find((card) => card.id === selectedId) || null, [cards, selectedId]);
  const groups = useMemo(() => Object.entries(cards.reduce<Record<string, number>>((acc, card) => {
    const key = card.kind || "unknown";
    acc[key] = (acc[key] || 0) + 1;
    return acc;
  }, {})), [cards]);

  async function preview(card = selectedCard) {
    if (!card) return;
    setPreviewResult(await requestJson<Record<string, unknown>>(`/api/v1/ui-cards/${encodeURIComponent(card.id)}/preview`, { method: "POST", body: JSON.stringify({ input: JSON.parse(debugInput || "{}") }) }));
  }

  return (
    <SettingsPageFrame title="UI 卡片管理" description="管理卡片定义、渲染器和触发调试。">
      <div className="grid gap-3 md:grid-cols-4">
        {[
          ["总计", stats.total ?? cards.length],
          ["启用", stats.active ?? cards.filter((card) => card.status === "active").length],
          ["草稿", stats.draft ?? cards.filter((card) => card.status === "draft").length],
          ["内置", stats.builtIn ?? cards.filter((card) => card.builtIn).length],
        ].map(([label, value]) => (
          <div key={label} className="uic-stat rounded-lg border bg-white p-4"><span>{label}</span><strong>{value}</strong></div>
        ))}
      </div>

      <div className="flex flex-wrap gap-2">
        {[
          ["overview", "概览"],
          ["list", "卡片列表"],
          ["debugger", "触发调试器"],
        ].map(([key, label]) => (
          <button key={key} className={`ops-tabs-tab rounded-lg border px-3 py-2 text-sm ${tab === key ? "active bg-slate-900 text-white" : "bg-white"}`} type="button" onClick={() => setTab(key)}>{label}</button>
        ))}
      </div>

      {tab === "overview" ? (
        <div className="grid gap-3 md:grid-cols-3">
          {groups.map(([kind, count]) => (
            <Card key={kind} className="ops-card rounded-lg bg-white"><CardHeader><CardTitle>{kind}</CardTitle><CardDescription>{count} cards</CardDescription></CardHeader></Card>
          ))}
        </div>
      ) : null}

      {tab === "list" ? (
        <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_420px]">
          <Card className="rounded-lg bg-white">
            <CardHeader><CardTitle>卡片定义列表</CardTitle></CardHeader>
            <CardContent>
              <div className="ops-data-table-table overflow-auto">
                <table className="data-table w-full border-collapse text-sm">
                  <tbody>
                    {cards.map((card) => (
                      <tr key={card.id}>
                        <td className="border-b p-2 font-medium">{card.name}</td>
                        <td className="border-b p-2"><code>{card.renderer}</code></td>
                        <td className="border-b p-2"><ToneBadge>{card.status}</ToneBadge></td>
                        <td className="border-b p-2"><Button size="sm" variant="outline" onClick={() => setSelectedId(card.id)}>详情</Button></td>
                        <td className="border-b p-2"><Button size="sm" variant="outline" onClick={() => { setSelectedId(card.id); setEditDraft(card); }}>编辑</Button></td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </CardContent>
          </Card>
          <div className="grid gap-3">
            {selectedCard ? <Card className="rounded-lg bg-white"><CardHeader><CardTitle>{selectedCard.name}</CardTitle></CardHeader><CardContent className="ops-descriptions grid gap-2 text-sm"><div>kind: {selectedCard.kind}</div><div>renderer: {selectedCard.renderer}</div><div>{selectedCard.summary}</div><Button onClick={() => void preview(selectedCard)}>预览</Button>{previewResult ? <pre className="preview-output overflow-auto rounded bg-slate-950 p-3 text-xs text-white">{JSON.stringify(previewResult, null, 2)}</pre> : null}</CardContent></Card> : null}
            {editDraft ? <Card className="rounded-lg bg-white"><CardHeader><CardTitle>编辑卡片定义</CardTitle></CardHeader><CardContent className="ops-form grid gap-3"><Input value={editDraft.name} onChange={(event) => setEditDraft({ ...editDraft, name: event.target.value })} /><Button onClick={() => void requestJson(`/api/v1/ui-cards/${encodeURIComponent(editDraft.id)}`, { method: "PUT", body: JSON.stringify(editDraft) })}>保存</Button></CardContent></Card> : null}
          </div>
        </div>
      ) : null}

      {tab === "debugger" ? (
        <Card className="rounded-lg bg-white">
          <CardHeader className="ops-card-header"><CardTitle>触发调试器</CardTitle><CardDescription>选择当前卡片执行 preview。</CardDescription></CardHeader>
          <CardContent className="grid gap-3">
            <select className="rounded-lg border p-2" value={selectedId} onChange={(event) => setSelectedId(event.target.value)}>{cards.map((card) => <option key={card.id} value={card.id}>{card.name}</option>)}</select>
            <textarea className="rounded-lg border p-2 font-mono text-xs" rows={4} value={debugInput} onChange={(event) => setDebugInput(event.target.value)} />
            <Button onClick={() => void preview()}>执行预览</Button>
            {previewResult ? <pre className="preview-output overflow-auto rounded bg-slate-950 p-3 text-xs text-white">{JSON.stringify(previewResult, null, 2)}</pre> : null}
          </CardContent>
        </Card>
      ) : null}
    </SettingsPageFrame>
  );
}
