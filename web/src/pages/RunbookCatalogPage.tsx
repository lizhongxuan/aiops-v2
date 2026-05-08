import { RefreshCw } from "lucide-react";
import { useEffect, useState } from "react";
import { Link } from "react-router-dom";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { ComplexPageFrame, EmptyPanel, RiskBadge } from "@/pages/complexPageComponents";
import { compactText, listRunbooks, type RunbookRecord } from "@/pages/complexPagesApi";
import { LoadingState, StatusAlert } from "@/pages/settingsComponents";

export function RunbookCatalogPage() {
  const [runbooks, setRunbooks] = useState<RunbookRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [message, setMessage] = useState<{ type: "success" | "error" | "info"; text: string } | null>(null);

  async function load() {
    setLoading(true);
    try {
      const payload = await listRunbooks();
      setRunbooks(payload.items || payload.runbooks || []);
      setMessage(null);
    } catch (error) {
      setMessage({ type: "error", text: error instanceof Error ? error.message : "加载 Runbook 失败" });
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => { void load(); }, []);

  return (
    <ComplexPageFrame kicker="Runbook Catalog" title="Runbook" description="管理可计划、可审计、需审批的生产操作方案。" actions={<Button variant="outline" onClick={() => void load()}><RefreshCw />刷新</Button>}>
      {message ? <StatusAlert type={message.type} title="操作失败" message={message.text} /> : null}
      <Card className="rounded-lg bg-white">
        <CardHeader><CardTitle>Runbook 列表</CardTitle><CardDescription>动作提案由事故上下文触发。</CardDescription></CardHeader>
        <CardContent>{loading ? <LoadingState label="加载 Runbook" /> : runbooks.length ? <div className="overflow-x-auto"><table className="w-full min-w-[760px] text-left text-sm"><thead className="border-b text-xs uppercase tracking-normal text-slate-500"><tr><th className="py-2 pr-3">Runbook</th><th className="py-2 pr-3">Scope</th><th className="py-2 pr-3">Risk</th><th className="py-2 pr-3">关联能力</th><th className="py-2 pr-3">最后更新</th><th className="py-2 text-right">操作</th></tr></thead><tbody className="divide-y">{runbooks.map((item) => <tr key={item.id}><td className="py-3 pr-3">{item.title || item.name || item.id}</td><td className="py-3 pr-3">{item.scope || item.environment || "-"}</td><td className="py-3 pr-3"><RiskBadge value={item.risk} /></td><td className="py-3 pr-3">{Array.isArray(item.capabilities) ? item.capabilities.join(", ") : item.capability || "-"}</td><td className="py-3 pr-3">{compactText(item.updatedAt || item.updated_at) || "-"}</td><td className="py-3 text-right"><Button variant="outline" asChild><Link to={`/runbooks/${encodeURIComponent(item.id)}`}>查看</Link></Button></td></tr>)}</tbody></table></div> : <EmptyPanel title="暂无 Runbook" description="这里只展示目录和匹配信息。" />}</CardContent>
      </Card>
    </ComplexPageFrame>
  );
}
