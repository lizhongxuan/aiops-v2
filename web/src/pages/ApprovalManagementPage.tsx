import { RefreshCcw, Search } from "lucide-react";
import { useEffect, useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ComplexPageFrame, EmptyPanel, KeyValueList, MetricStrip, RiskBadge } from "@/pages/complexPageComponents";
import {
  compactText,
  fetchApprovalAudits,
  fetchApprovalGrants,
  updateApprovalGrant,
  type ApprovalAuditRecord,
  type ApprovalGrantRecord,
} from "@/pages/complexPagesApi";
import { LoadingState, SelectField, StatusAlert } from "@/pages/settingsComponents";

function decisionLabel(decision?: string) {
  switch (decision) {
    case "approved":
      return "已批准";
    case "rejected":
      return "已拒绝";
    case "auto_accepted":
      return "自动放行";
    case "pending":
      return "待审核";
    default:
      return decision || "未知";
  }
}

function formatTime(value?: string) {
  if (!value) return "-";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString("zh-CN", { hour12: false });
}

export function ApprovalManagementPage() {
  const [activeTab, setActiveTab] = useState("audits");
  const [audits, setAudits] = useState<ApprovalAuditRecord[]>([]);
  const [grants, setGrants] = useState<ApprovalGrantRecord[]>([]);
  const [stats, setStats] = useState<Record<string, unknown>>({});
  const [decision, setDecision] = useState("");
  const [host, setHost] = useState("");
  const [selectedAudit, setSelectedAudit] = useState<ApprovalAuditRecord | null>(null);
  const [loading, setLoading] = useState(true);
  const [message, setMessage] = useState<{ type: "success" | "error" | "info"; text: string } | null>(null);

  const filteredAudits = useMemo(() => {
    return audits.filter((item) => (!decision || item.decision === decision) && (!host || compactText(item.host).includes(host)));
  }, [audits, decision, host]);

  async function loadAudits() {
    setLoading(true);
    try {
      const payload = await fetchApprovalAudits({ page: "1", pageSize: "50", decision, host });
      setAudits(payload.items || payload.audits || []);
      setStats(payload.stats || {});
      setMessage(null);
    } catch (error) {
      setMessage({ type: "error", text: error instanceof Error ? error.message : "加载审批流水失败" });
    } finally {
      setLoading(false);
    }
  }

  async function loadGrants(nextHost = host) {
    setLoading(true);
    try {
      const payload = await fetchApprovalGrants(nextHost);
      setGrants(payload.items || payload.grants || []);
      setMessage(null);
    } catch (error) {
      setMessage({ type: "error", text: error instanceof Error ? error.message : "加载授权记录失败" });
    } finally {
      setLoading(false);
    }
  }

  async function grantAction(grantId: string, action: "revoke" | "disable" | "enable") {
    try {
      await updateApprovalGrant(grantId, action);
      await loadGrants(host);
      setMessage({ type: "success", text: "授权状态已更新" });
    } catch (error) {
      setMessage({ type: "error", text: error instanceof Error ? error.message : "更新授权失败" });
    }
  }

  useEffect(() => {
    void loadAudits();
  }, []);

  return (
    <ComplexPageFrame
      kicker="Approvals"
      title="审批管理"
      description="集中查看审批流水、授权记录，管理命令授权的生命周期。"
      actions={
        <Button variant="outline" onClick={() => (activeTab === "audits" ? void loadAudits() : void loadGrants(host))}>
          <RefreshCcw />
          刷新
        </Button>
      }
    >
      {message ? <StatusAlert type={message.type} title={message.type === "error" ? "操作失败" : "操作完成"} message={message.text} /> : null}
      <MetricStrip
        items={[
          { label: "今日审批", value: String(stats.todayTotal ?? audits.length) },
          { label: "待审核", value: String(stats.pending ?? audits.filter((item) => item.decision === "pending").length), tone: "warn" },
          { label: "自动放行", value: String(stats.autoAccepted ?? audits.filter((item) => item.decision === "auto_accepted").length), tone: "ok" },
          { label: "授权命令", value: String(stats.grantedCommands ?? grants.length) },
        ]}
      />
      <Tabs value={activeTab} onValueChange={(value) => { setActiveTab(value); if (value === "grants") void loadGrants(host); }}>
        <TabsList>
          <TabsTrigger value="audits">审批流水</TabsTrigger>
          <TabsTrigger value="grants">授权记录</TabsTrigger>
        </TabsList>
        <TabsContent value="audits">
          <Card className="rounded-lg bg-white">
            <CardHeader>
              <CardTitle>审批流水</CardTitle>
              <CardDescription>兼容 `/approval-audits` 的 items/audits 返回结构。</CardDescription>
            </CardHeader>
            <CardContent className="grid gap-3">
              <div className="grid gap-2 md:grid-cols-[1fr_180px_180px]">
                <label className="relative">
                  <Search className="pointer-events-none absolute left-2.5 top-2 h-4 w-4 text-slate-400" />
                  <Input className="pl-8" value={host} onChange={(event) => setHost(event.target.value)} placeholder="按主机过滤" />
                </label>
                <SelectField value={decision} onChange={setDecision} options={[{ label: "全部决策", value: "" }, { label: "pending", value: "pending" }, { label: "approved", value: "approved" }, { label: "rejected", value: "rejected" }, { label: "auto_accepted", value: "auto_accepted" }]} />
                <Button variant="outline" onClick={() => void loadAudits()}>应用过滤</Button>
              </div>
              {loading ? <LoadingState label="加载审批流水" /> : filteredAudits.length ? (
                <div className="overflow-x-auto">
                  <table className="w-full min-w-[860px] text-left text-sm">
                    <thead className="border-b text-xs uppercase tracking-normal text-slate-500">
                      <tr><th className="py-2 pr-3">时间</th><th className="py-2 pr-3">主机</th><th className="py-2 pr-3">工具</th><th className="py-2 pr-3">决策</th><th className="py-2 text-right">操作</th></tr>
                    </thead>
                    <tbody className="divide-y">
                      {filteredAudits.map((item) => (
                        <tr key={item.id}>
                          <td className="py-3 pr-3">{formatTime(item.createdAt)}</td>
                          <td className="py-3 pr-3">{item.host || "-"}</td>
                          <td className="py-3 pr-3">{item.toolName || item.command || "-"}</td>
                          <td className="py-3 pr-3"><RiskBadge value={decisionLabel(item.decision)} /></td>
                          <td className="py-3 text-right"><Button variant="outline" onClick={() => setSelectedAudit(item)}>详情</Button></td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              ) : <EmptyPanel title="暂无审批流水" description="调整过滤条件或稍后刷新。" />}
            </CardContent>
          </Card>
        </TabsContent>
        <TabsContent value="grants">
          <Card className="rounded-lg bg-white">
            <CardHeader><CardTitle>授权记录</CardTitle><CardDescription>支持 revoke/disable/enable 生命周期操作。</CardDescription></CardHeader>
            <CardContent className="grid gap-3">
              {loading ? <LoadingState label="加载授权记录" /> : grants.length ? (
                <div className="overflow-x-auto">
                  <table className="w-full min-w-[760px] text-left text-sm">
                    <thead className="border-b text-xs uppercase tracking-normal text-slate-500"><tr><th className="py-2 pr-3">授权 ID</th><th className="py-2 pr-3">主机</th><th className="py-2 pr-3">命令/工具</th><th className="py-2 pr-3">状态</th><th className="py-2 text-right">操作</th></tr></thead>
                    <tbody className="divide-y">
                      {grants.map((grant) => (
                        <tr key={grant.id}>
                          <td className="py-3 pr-3">{grant.id}</td><td className="py-3 pr-3">{grant.hostId || "-"}</td><td className="py-3 pr-3">{grant.command || grant.toolName || "-"}</td><td className="py-3 pr-3"><RiskBadge value={grant.status} /></td>
                          <td className="py-3"><div className="flex justify-end gap-2">{grant.status === "disabled" ? <Button variant="outline" onClick={() => void grantAction(grant.id, "enable")}>启用</Button> : <><Button variant="outline" onClick={() => void grantAction(grant.id, "disable")}>禁用</Button><Button variant="destructive" onClick={() => void grantAction(grant.id, "revoke")}>撤销</Button></>}</div></td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              ) : <EmptyPanel title="暂无授权记录" description="没有符合条件的授权。" />}
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>

      <Sheet open={Boolean(selectedAudit)} onOpenChange={(open) => !open && setSelectedAudit(null)}>
        <SheetContent className="sm:max-w-lg">
          <SheetHeader><SheetTitle>审批详情</SheetTitle><SheetDescription>{selectedAudit?.id}</SheetDescription></SheetHeader>
          <div className="px-4">
            <KeyValueList items={[
              { label: "时间", value: formatTime(selectedAudit?.createdAt) },
              { label: "主机", value: selectedAudit?.host },
              { label: "操作人", value: selectedAudit?.operator },
              { label: "工具", value: selectedAudit?.toolName },
              { label: "决策", value: decisionLabel(selectedAudit?.decision) },
              { label: "原因", value: selectedAudit?.reason },
            ]} />
          </div>
        </SheetContent>
      </Sheet>
    </ComplexPageFrame>
  );
}
