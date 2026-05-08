import { RefreshCw } from "lucide-react";
import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { ComplexPageFrame, EmptyPanel, RiskBadge } from "@/pages/complexPageComponents";
import { asArray, compactText, getRunbook, matchRunbooks, type RunbookRecord } from "@/pages/complexPagesApi";
import { LoadingState, StatusAlert } from "@/pages/settingsComponents";

export function RunbookDetailPage() {
  const { runbookId = "" } = useParams();
  const [runbook, setRunbook] = useState<RunbookRecord | null>(null);
  const [matches, setMatches] = useState<Record<string, unknown>[]>([]);
  const [selectedProposal, setSelectedProposal] = useState<Record<string, unknown> | null>(null);
  const [loading, setLoading] = useState(true);
  const [message, setMessage] = useState<{ type: "success" | "error" | "info"; text: string } | null>(null);

  async function load() {
    if (!runbookId) return;
    setLoading(true);
    try {
      setRunbook(await getRunbook(runbookId));
      setMessage(null);
    } catch (error) {
      setMessage({ type: "error", text: error instanceof Error ? error.message : "加载 Runbook 失败" });
    } finally {
      setLoading(false);
    }
  }

  async function runMatchTest() {
    const payload = await matchRunbooks({ runbookId, mode: "test" });
    setMatches(payload.items || payload.matches || []);
  }

  useEffect(() => { void load(); }, [runbookId]);

  const steps = asArray(runbook?.steps);
  const verifications = asArray(runbook?.verifications);
  const proposals = asArray(runbook?.proposals);

  return (
    <ComplexPageFrame kicker="Runbook Detail" title={runbook?.title || runbook?.name || "Runbook 详情"} actions={<><Button variant="outline" asChild><Link to="/runbooks">返回目录</Link></Button><Button variant="outline" onClick={() => void load()}><RefreshCw />刷新</Button><Button onClick={() => void runMatchTest()} data-testid="runbook-match-test">匹配测试</Button></>}>
      {message ? <StatusAlert type={message.type} title="操作失败" message={message.text} /> : null}
      {loading ? <LoadingState label="加载 Runbook 详情" /> : (
        <>
          <div className="flex flex-wrap gap-2"><RiskBadge value={runbook?.risk} /><RiskBadge value={runbookId} /><RiskBadge value="Plan-only" /></div>
          <section className="grid gap-4 xl:grid-cols-2">
            <Panel title="步骤" items={steps} empty="暂无步骤" />
            <Panel title="验证项" items={verifications} empty="暂无验证项" />
            <Panel title="匹配测试" items={matches} empty="暂无匹配结果" />
            <Card className="rounded-lg bg-white"><CardHeader><CardTitle>动作提案</CardTitle></CardHeader><CardContent>{proposals.length ? <div className="grid gap-2">{proposals.map((item, index) => <button key={compactText(item.id || item.title) || index} type="button" className="rounded-lg border bg-white p-3 text-left text-sm hover:bg-slate-50" onClick={() => setSelectedProposal(item)}><div className="font-medium">{compactText(item.title || item.name || item.id) || `提案 ${index + 1}`}</div><div className="mt-1 text-xs text-slate-500">{compactText(item.command || item.summary || item.risk) || "点击查看详情"}</div></button>)}</div> : <EmptyPanel title="暂无动作提案" description="动作提案由事故上下文触发。" />}</CardContent></Card>
          </section>
        </>
      )}
      <Sheet open={Boolean(selectedProposal)} onOpenChange={(open) => !open && setSelectedProposal(null)}>
        <SheetContent><SheetHeader><SheetTitle>{compactText(selectedProposal?.title || selectedProposal?.name || "动作提案")}</SheetTitle><SheetDescription>shadcn sheet 展示 action proposal。</SheetDescription></SheetHeader><pre className="mx-4 overflow-auto rounded-lg bg-slate-950 p-3 text-xs text-slate-50">{JSON.stringify(selectedProposal, null, 2)}</pre></SheetContent>
      </Sheet>
    </ComplexPageFrame>
  );
}

function Panel({ title, items, empty }: { title: string; items: Record<string, unknown>[]; empty: string }) {
  return <Card className="rounded-lg bg-white"><CardHeader><CardTitle>{title}</CardTitle></CardHeader><CardContent>{items.length ? <ul className="grid gap-2 text-sm">{items.map((item, index) => <li key={compactText(item.id || item.title || item.name) || index} className="rounded-lg border bg-white p-3"><div className="font-medium">{compactText(item.title || item.name || item.id) || `条目 ${index + 1}`}</div><div className="mt-1 text-xs text-slate-500">{compactText(item.summary || item.description || item.command || item.status) || "-"}</div></li>)}</ul> : <EmptyPanel title={empty} description="暂无数据。" />}</CardContent></Card>;
}
