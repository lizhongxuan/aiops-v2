import { Search } from "lucide-react";
import { useEffect, useState } from "react";
import { Link } from "react-router-dom";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { buildCaseViewModel, type CaseViewModel } from "@/components/cases/caseViewModels";
import { ComplexPageFrame, EmptyPanel, RiskBadge } from "@/pages/complexPageComponents";
import { compactText, listIncidents, type IncidentRecord } from "@/pages/complexPagesApi";
import { Field, LoadingState, SelectField, StatusAlert } from "@/pages/settingsComponents";

type CaseFilters = {
  status: string;
  source: string;
  environment: string;
  hostId: string;
  waitingConfirmation: string;
  lockConflict: string;
};

const defaultFilters: CaseFilters = {
  status: "active",
  source: "",
  environment: "",
  hostId: "",
  waitingConfirmation: "",
  lockConflict: "",
};

const statusOptions = [
  { label: "全部状态", value: "" },
  { label: "处理中", value: "active" },
  { label: "等待确认", value: "waiting_confirmation" },
  { label: "执行中", value: "running_workflow" },
  { label: "验证中", value: "verifying" },
  { label: "已恢复", value: "recovered" },
  { label: "失败", value: "failed" },
];

const sourceOptions = [
  { label: "全部来源", value: "" },
  { label: "Debug Mode", value: "debug_mode" },
  { label: "Coroot", value: "coroot" },
  { label: "人工创建", value: "manual" },
  { label: "告警接入", value: "alert" },
];

const environmentOptions = [
  { label: "全部环境", value: "" },
  { label: "生产", value: "prod" },
  { label: "预发", value: "staging" },
  { label: "测试", value: "test" },
  { label: "开发", value: "dev" },
];

const booleanOptions = [
  { label: "全部", value: "" },
  { label: "是", value: "true" },
  { label: "否", value: "false" },
];

export function IncidentListPage() {
  const [incidents, setIncidents] = useState<IncidentRecord[]>([]);
  const [filters, setFilters] = useState<CaseFilters>(defaultFilters);
  const [loading, setLoading] = useState(true);
  const [message, setMessage] = useState<{ type: "success" | "error" | "info"; text: string } | null>(null);

  async function load(nextFilters = filters) {
    setLoading(true);
    try {
      const payload = await listIncidents(buildIncidentListParams(nextFilters));
      setIncidents(payload.items || payload.incidents || []);
      setMessage(null);
    } catch (error) {
      setMessage({ type: "error", text: error instanceof Error ? error.message : "加载事故列表失败" });
    } finally {
      setLoading(false);
    }
  }

  function updateFilter(key: keyof CaseFilters, value: string) {
    setFilters((previous) => ({ ...previous, [key]: value }));
  }

  function resetFilters() {
    const nextFilters = { ...defaultFilters };
    setFilters(nextFilters);
    void load(nextFilters);
  }

  useEffect(() => {
    void load();
  }, []);

  return (
    <ComplexPageFrame
      kicker="Case 工作台"
      title="Case 工作台"
      description="展示 Debug Mode、Coroot 或人工创建的活跃 Case，详情页承载证据、执行、验证和经验闭环。"
    >
      {message ? <StatusAlert type={message.type} title="操作失败" message={message.text} /> : null}
      <Card className="rounded-lg bg-white" data-testid="case-list-card">
        <CardHeader><CardTitle>Case 列表</CardTitle><CardDescription>慢请求、PG 修复和人工排查都进入同一个 Case 详情。</CardDescription></CardHeader>
        <CardContent className="grid gap-4">
          <section data-testid="case-list-filters" className="grid gap-3 rounded-lg border bg-slate-50 p-3">
            <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-6">
              <Field label="状态">
                <SelectField aria-label="状态筛选" value={filters.status} onChange={(value) => updateFilter("status", value)} options={statusOptions} />
              </Field>
              <Field label="来源">
                <SelectField aria-label="来源筛选" value={filters.source} onChange={(value) => updateFilter("source", value)} options={sourceOptions} />
              </Field>
              <Field label="环境">
                <SelectField aria-label="环境筛选" value={filters.environment} onChange={(value) => updateFilter("environment", value)} options={environmentOptions} />
              </Field>
              <Field label="主机">
                <Input aria-label="主机筛选" value={filters.hostId} onChange={(event) => updateFilter("hostId", event.target.value)} placeholder="主机 ID" />
              </Field>
              <Field label="待确认">
                <SelectField aria-label="待确认筛选" value={filters.waitingConfirmation} onChange={(value) => updateFilter("waitingConfirmation", value)} options={booleanOptions} />
              </Field>
              <Field label="锁冲突">
                <SelectField aria-label="锁冲突筛选" value={filters.lockConflict} onChange={(value) => updateFilter("lockConflict", value)} options={booleanOptions} />
              </Field>
            </div>
            <div className="flex flex-wrap justify-end gap-2">
              <Button variant="outline" onClick={resetFilters}>重置</Button>
              <Button onClick={() => void load()}><Search />应用筛选</Button>
            </div>
          </section>
          {loading ? <LoadingState label="加载事故列表" /> : incidents.length ? (
            <div className="overflow-x-auto">
              <table className="w-full min-w-[760px] text-left text-sm">
                <thead className="border-b text-xs uppercase tracking-normal text-slate-500"><tr><th className="py-2 pr-3">Case</th><th className="py-2 pr-3">风险</th><th className="py-2 pr-3">状态</th><th className="py-2 pr-3">来源</th><th className="py-2 pr-3">环境</th><th className="py-2 pr-3">主机</th><th className="py-2 pr-3">阻塞项</th><th className="py-2 pr-3">更新时间</th></tr></thead>
                <tbody className="divide-y">
                  {incidents.map((incident) => {
                    const caseView = buildCaseViewModel(incident);
                    return (
                      <tr key={caseView.id}>
                        <td className="py-3 pr-3"><Link className="font-medium text-emerald-800 hover:underline" to={`/incidents/${encodeURIComponent(caseView.id)}`}>{caseView.title || incident.name || caseView.id}</Link><div className="mt-1 text-xs text-slate-500">{caseView.businessCapability || "-"}</div></td>
                        <td className="py-3 pr-3"><RiskBadge value={incident.severity || incident.sev || caseView.severityLabel} /></td>
                        <td className="py-3 pr-3"><RiskBadge value={caseView.statusLabel || incident.status} /></td>
                        <td className="py-3 pr-3">{sourceLabel(caseView.source)}</td>
                        <td className="py-3 pr-3">{caseView.environment || "-"}</td>
                        <td className="py-3 pr-3">{hostSummary(caseView)}</td>
                        <td className="py-3 pr-3">{blockingSummary(caseView)}</td>
                        <td className="py-3 pr-3">{compactText(caseView.updatedAt || caseView.createdAt || incident.updatedAt || incident.createdAt) || "-"}</td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          ) : <EmptyPanel title="暂无 Case" description="当 Debug Mode、Coroot webhook、AI 对话中发起修复或人工创建 Case 后，会出现在这里。" />}
        </CardContent>
      </Card>
    </ComplexPageFrame>
  );
}

function buildIncidentListParams(filters: CaseFilters) {
  const params: Record<string, string> = {};
  if (filters.status) params.status = filters.status;
  if (filters.source) params.source = filters.source;
  if (filters.environment) params.environment = filters.environment;
  if (filters.hostId.trim()) params.host_id = filters.hostId.trim();
  if (filters.waitingConfirmation) params.waiting_confirmation = filters.waitingConfirmation;
  if (filters.lockConflict) params.lock_conflict = filters.lockConflict;
  return params;
}

function sourceLabel(source: string) {
  switch (source) {
    case "debug_mode":
      return "Debug Mode";
    case "coroot":
      return "Coroot";
    case "manual":
      return "人工创建";
    case "alert":
      return "告警接入";
    default:
      return source || "-";
  }
}

function hostSummary(caseView: CaseViewModel) {
  const hosts = Array.from(
    new Set([
      ...caseView.hostProfiles.map((profile) => profile.hostId || profile.displayName).filter(Boolean),
      ...caseView.hostLeases.map((lease) => lease.hostId).filter(Boolean),
    ]),
  );
  return hosts.length ? hosts.join(", ") : "-";
}

function blockingSummary(caseView: CaseViewModel) {
  const blockers = caseView.blockingItems
    .filter((item) => item.key === "waiting_confirmation" || item.key === "host_lease_blocked")
    .map((item) => item.label);
  return blockers.length ? blockers.join(" · ") : "-";
}
