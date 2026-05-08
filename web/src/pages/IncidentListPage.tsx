import { RefreshCw } from "lucide-react";
import { useEffect, useState } from "react";
import { Link } from "react-router-dom";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { ComplexPageFrame, EmptyPanel, RiskBadge } from "@/pages/complexPageComponents";
import { compactText, listIncidents, type IncidentRecord } from "@/pages/complexPagesApi";
import { LoadingState, StatusAlert } from "@/pages/settingsComponents";

export function IncidentListPage() {
  const [incidents, setIncidents] = useState<IncidentRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [message, setMessage] = useState<{ type: "success" | "error" | "info"; text: string } | null>(null);

  async function load() {
    setLoading(true);
    try {
      const payload = await listIncidents({ status: "active" });
      setIncidents(payload.items || payload.incidents || []);
      setMessage(null);
    } catch (error) {
      setMessage({ type: "error", text: error instanceof Error ? error.message : "加载事故列表失败" });
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  return (
    <ComplexPageFrame
      kicker="Incidents"
      title="事故工作台"
      description="展示 Coroot webhook 或人工创建的活跃事故，详情页继续通过 API view-model 获取上下文。"
      actions={<Button variant="outline" onClick={() => void load()}><RefreshCw />刷新</Button>}
    >
      {message ? <StatusAlert type={message.type} title="操作失败" message={message.text} /> : null}
      <Card className="rounded-lg bg-white">
        <CardHeader><CardTitle>事故列表</CardTitle><CardDescription>按活跃状态筛选。</CardDescription></CardHeader>
        <CardContent>
          {loading ? <LoadingState label="加载事故列表" /> : incidents.length ? (
            <div className="overflow-x-auto">
              <table className="w-full min-w-[760px] text-left text-sm">
                <thead className="border-b text-xs uppercase tracking-normal text-slate-500"><tr><th className="py-2 pr-3">事故</th><th className="py-2 pr-3">SEV</th><th className="py-2 pr-3">状态</th><th className="py-2 pr-3">业务能力</th><th className="py-2 pr-3">更新时间</th></tr></thead>
                <tbody className="divide-y">
                  {incidents.map((incident) => (
                    <tr key={incident.id}>
                      <td className="py-3 pr-3"><Link className="font-medium text-emerald-800 hover:underline" to={`/incidents/${encodeURIComponent(incident.id)}`}>{incident.title || incident.name || incident.id}</Link></td>
                      <td className="py-3 pr-3"><RiskBadge value={incident.severity || incident.sev} /></td>
                      <td className="py-3 pr-3"><RiskBadge value={incident.status} /></td>
                      <td className="py-3 pr-3">{incident.businessCapability || incident.capability || "-"}</td>
                      <td className="py-3 pr-3">{compactText(incident.updatedAt || incident.createdAt) || "-"}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : <EmptyPanel title="暂无事故" description="当 Coroot webhook 或人工创建事故后，会出现在这里。" />}
        </CardContent>
      </Card>
    </ComplexPageFrame>
  );
}
