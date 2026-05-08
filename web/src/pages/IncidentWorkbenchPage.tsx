import { Check, RefreshCw, X } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { ComplexPageFrame, EmptyPanel, KeyValueList, MetricStrip, RiskBadge } from "@/pages/complexPageComponents";
import {
  asArray,
  compactText,
  getIncident,
  getOpsGraphBusinessImpact,
  getOpsGraphNeighborhood,
  listRunbookInstances,
  matchRunbooks,
  submitApprovalDecision,
  type ApprovalAuditRecord,
  type IncidentRecord,
} from "@/pages/complexPagesApi";
import { LoadingState, StatusAlert } from "@/pages/settingsComponents";

export function IncidentWorkbenchPage() {
  const { incidentId = "" } = useParams();
  const [incident, setIncident] = useState<IncidentRecord | null>(null);
  const [neighbors, setNeighbors] = useState<Record<string, unknown>[]>([]);
  const [impact, setImpact] = useState<{ capabilities?: Record<string, unknown>[]; tenants?: Record<string, unknown>[] }>({});
  const [runbookMatches, setRunbookMatches] = useState<Record<string, unknown>[]>([]);
  const [runbookInstances, setRunbookInstances] = useState<Record<string, unknown>[]>([]);
  const [loading, setLoading] = useState(true);
  const [busyApproval, setBusyApproval] = useState("");
  const [message, setMessage] = useState<{ type: "success" | "error" | "info"; text: string } | null>(null);

  const title = incident?.title || incident?.name || `事故 ${incidentId}`;
  const entityId = compactText(incident?.entityId || incident?.id || incidentId);
  const hypotheses = asArray(incident?.hypotheses);
  const evidence = asArray(incident?.evidence);
  const postmortem = (incident?.postmortem || {}) as Record<string, unknown>;
  const pendingApprovals = useMemo(() => asArray<ApprovalAuditRecord>(incident?.pendingApprovals), [incident]);

  async function load() {
    if (!incidentId) return;
    setLoading(true);
    try {
      const nextIncident = await getIncident(incidentId);
      setIncident(nextIncident);
      const nextEntity = compactText(nextIncident.entityId || nextIncident.id || incidentId);
      const [neighborhood, businessImpact, matches, instances] = await Promise.allSettled([
        nextEntity ? getOpsGraphNeighborhood(nextEntity, { incidentId }) : Promise.resolve({}),
        nextEntity ? getOpsGraphBusinessImpact(nextEntity) : Promise.resolve({}),
        matchRunbooks({ incidentId, title: nextIncident.title || nextIncident.name, capability: nextIncident.businessCapability || nextIncident.capability }),
        listRunbookInstances({ incidentId }),
      ]);
      setNeighbors(neighborhood.status === "fulfilled" ? (neighborhood.value.neighbors || neighborhood.value.items || []) : []);
      setImpact(businessImpact.status === "fulfilled" ? businessImpact.value : {});
      setRunbookMatches(matches.status === "fulfilled" ? (matches.value.items || matches.value.matches || []) : []);
      setRunbookInstances(instances.status === "fulfilled" ? (instances.value.items || instances.value.instances || []) : []);
      setMessage(null);
    } catch (error) {
      setMessage({ type: "error", text: error instanceof Error ? error.message : "加载事故上下文失败" });
    } finally {
      setLoading(false);
    }
  }

  async function decide(approvalId: string, decision: "approved" | "rejected") {
    setBusyApproval(approvalId);
    try {
      await submitApprovalDecision(approvalId, decision);
      setMessage({ type: "success", text: `审批已${decision === "approved" ? "批准" : "拒绝"}` });
      await load();
    } catch (error) {
      setMessage({ type: "error", text: error instanceof Error ? error.message : "审批操作失败" });
    } finally {
      setBusyApproval("");
    }
  }

  useEffect(() => {
    void load();
  }, [incidentId]);

  return (
    <ComplexPageFrame
      kicker="Incident Detail"
      title={title}
      description={incident?.summary || "事故详情和上下文面板。"}
      actions={<><Button variant="outline" asChild><Link to="/incidents">返回事故列表</Link></Button><Button variant="outline" onClick={() => void load()}><RefreshCw />刷新</Button></>}
    >
      {message ? <StatusAlert type={message.type} title={message.type === "error" ? "操作失败" : "操作完成"} message={message.text} /> : null}
      {loading ? <LoadingState label="加载事故上下文" /> : (
        <>
          <MetricStrip items={[
            { label: "Status", value: <RiskBadge value={incident?.status} /> },
            { label: "Severity", value: <RiskBadge value={incident?.severity || incident?.sev} /> },
            { label: "Environment", value: incident?.environment || incident?.env || "待定" },
            { label: "Capability", value: incident?.businessCapability || incident?.capability || "待定" },
          ]} />
          <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_380px]">
            <div className="grid gap-4">
              <Card className="rounded-lg bg-white"><CardHeader><CardTitle>Hypothesis 排名</CardTitle></CardHeader><CardContent>{hypotheses.length ? <Timeline items={hypotheses} /> : <EmptyPanel title="暂无 hypothesis" description="等待 Agent 或 Coroot 写入。" />}</CardContent></Card>
              <Card className="rounded-lg bg-white"><CardHeader><CardTitle>证据时间线</CardTitle></CardHeader><CardContent>{evidence.length ? <Timeline items={evidence} /> : <EmptyPanel title="暂无证据" description="暂无 Coroot 或人工证据。" />}</CardContent></Card>
              <Card className="rounded-lg bg-white"><CardHeader><CardTitle>复盘草稿</CardTitle></CardHeader><CardContent><p className="text-sm leading-6 text-slate-700">{compactText(postmortem.summary || postmortem.rootCause) || "待补充"}</p></CardContent></Card>
              <Card className="rounded-lg bg-white"><CardHeader><CardTitle>待审批动作</CardTitle><CardDescription>使用现有 `/api/v1/approvals/:id/decision`，不绕过审批路径。</CardDescription></CardHeader><CardContent>{pendingApprovals.length ? <div className="grid gap-2">{pendingApprovals.map((approval) => <div key={approval.id} data-testid="incident-sidebar-approval" className="rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm"><div className="font-medium">{approval.command || approval.toolName || approval.reason || approval.id}</div><div className="mt-2 flex gap-2"><Button variant="outline" disabled={busyApproval === approval.id} onClick={() => void decide(approval.id, "rejected")}><X />拒绝</Button><Button disabled={busyApproval === approval.id} onClick={() => void decide(approval.id, "approved")}><Check />批准</Button></div></div>)}</div> : <EmptyPanel title="暂无待审批动作" description="当前事故没有阻塞审批。" />}</CardContent></Card>
            </div>
            <details className="rounded-lg border bg-white p-4" data-testid="incident-context-drawer" open>
              <summary className="cursor-pointer font-medium">上下文面板</summary>
              <aside className="mt-4 grid gap-4">
                <Card className="rounded-lg bg-slate-50"><CardHeader><CardTitle>ERP 图谱邻域</CardTitle></CardHeader><CardContent>{neighbors.length ? <Timeline items={neighbors} /> : <p className="text-sm text-slate-500">暂无邻域</p>}</CardContent></Card>
                <Card className="rounded-lg bg-slate-50"><CardHeader><CardTitle>业务影响</CardTitle></CardHeader><CardContent><KeyValueList items={[{ label: "Capabilities", value: asArray(impact.capabilities).length }, { label: "Tenants", value: asArray(impact.tenants).length }, { label: "Entity", value: entityId }]} /></CardContent></Card>
                <Card className="rounded-lg bg-slate-50"><CardHeader><CardTitle>Runbook 匹配</CardTitle></CardHeader><CardContent><Timeline items={runbookMatches} empty="暂无匹配 Runbook" /></CardContent></Card>
                <Card className="rounded-lg bg-slate-50"><CardHeader><CardTitle>执行实例</CardTitle></CardHeader><CardContent><Timeline items={runbookInstances} empty="暂无执行实例" /></CardContent></Card>
              </aside>
            </details>
          </div>
        </>
      )}
    </ComplexPageFrame>
  );
}

function Timeline({ items, empty = "暂无记录" }: { items: Record<string, unknown>[]; empty?: string }) {
  if (!items.length) return <p className="text-sm text-slate-500">{empty}</p>;
  return <ul className="grid gap-2 text-sm">{items.map((item, index) => <li key={compactText(item.id || item.title || item.name) || index} className="rounded-lg border bg-white p-3"><div className="font-medium">{compactText(item.title || item.name || item.id) || `记录 ${index + 1}`}</div><div className="mt-1 text-xs leading-5 text-slate-500">{compactText(item.summary || item.description || item.detail || item.status || item.impact) || "-"}</div></li>)}</ul>;
}
