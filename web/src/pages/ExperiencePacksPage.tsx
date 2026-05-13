import { CheckCircle2, Power, Search, ShieldCheck } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";

import {
  approveExperiencePackCandidate,
  listExperiencePackCandidates,
  listExperiencePackReuseRecords,
  normalizeExperienceCandidate,
  saveExperiencePackAuthorizationScopes,
  setExperiencePackEnabled,
  type AuthorizationScopeView,
  type ExperienceCandidateView,
  type ExperiencePackView,
  type ReuseRecordView,
} from "@/api/experiencePacks";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { experiencePackFixtures } from "@/data/enterpriseAssistantFixtures";
import { EmptyState, SettingsPageFrame, ToneBadge } from "@/pages/settingsComponents";
import { compactText } from "@/pages/settingsCatalogViewModels";

const FALLBACK_CANDIDATES = experiencePackFixtures.map((fixture) =>
  normalizeExperienceCandidate({
    id: fixture.id,
    pack_id: fixture.packId,
    title: fixture.title,
    summary: fixture.summary,
    status: fixture.state,
    source_case_id: fixture.state === "candidate" ? "case-nginx-reload" : "",
    match_reason: `confidence=${fixture.confidence}`,
    experience_pack: {
      id: fixture.packId,
      title: fixture.title,
      summary: fixture.summary,
      version: fixture.version,
      status: fixture.state === "disabled" ? "disabled" : "enabled",
      review_status: fixture.state === "candidate" ? "pending" : "approved",
      enabled: !["candidate", "disabled"].includes(fixture.state),
      retrieval_eval: { score: fixture.confidence, matched_cases: fixture.state === "authorized" ? 5 : 0, verdict: fixture.state === "authorized" ? "pass" : "pending" },
      workflow_binding: { workflow_id: `wf-${fixture.packId}`, workflow_name: `${fixture.title} Workflow`, status: fixture.state === "candidate" ? "draft" : "bound" },
      authorization_scopes: fixture.state === "authorized" ? [{ type: "environment", value: "prod", searchable: true, reason: "已授权生产环境" }] : [],
    },
  }),
);
type ExperiencePackFallbackEnv = {
  DEV?: boolean;
  MODE?: string;
};

export function shouldUseExperiencePackFixtureFallback(env: ExperiencePackFallbackEnv) {
  return Boolean(env.DEV || env.MODE === "test");
}

const ALLOW_FIXTURE_FALLBACK = shouldUseExperiencePackFixtureFallback(import.meta.env);

function searchableTone(pack?: ExperiencePackView | null): "default" | "success" | "warning" | "danger" {
  if (!pack) return "warning";
  if (pack.searchable) return "success";
  if (pack.reviewStatus !== "approved" || !pack.enabled) return "warning";
  return "danger";
}

function statusLabel(value = "") {
  const labels: Record<string, string> = {
    candidate: "候选",
    pending: "待审核",
    approved: "已审核",
    enabled: "已启用",
    disabled: "已停用",
    pass: "通过",
    warn: "需关注",
    failed_rollback: "失败回退",
    failed: "失败",
    success: "成功",
    succeeded: "成功",
  };
  return labels[value] || value || "未知";
}

function scopeLabel(scope: AuthorizationScopeView) {
  return `${scope.type}: ${scope.value}`;
}

function packFromCandidate(candidate: ExperienceCandidateView) {
  return candidate.experiencePack;
}

export function ExperiencePacksPage() {
  const [query, setQuery] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [activeTab, setActiveTab] = useState("candidates");
  const [candidates, setCandidates] = useState<ExperienceCandidateView[]>(ALLOW_FIXTURE_FALLBACK ? FALLBACK_CANDIDATES : []);
  const [selectedCandidateId, setSelectedCandidateId] = useState(ALLOW_FIXTURE_FALLBACK ? FALLBACK_CANDIDATES[0]?.id || "" : "");
  const [reuseRecords, setReuseRecords] = useState<ReuseRecordView[]>([]);

  async function loadCandidates() {
    setLoading(true);
    setError("");
    try {
      const payload = await listExperiencePackCandidates({ limit: 100 });
      const next = payload.items.length ? payload.items : [];
      setCandidates(next);
      setSelectedCandidateId((current) => current && next.some((item) => item.id === current) ? current : next[0]?.id || "");
    } catch (cause) {
      const message = (cause as Error).message || "经验包加载失败。";
      setError(ALLOW_FIXTURE_FALLBACK ? `${message} 已使用本地样例。` : message);
      const next = ALLOW_FIXTURE_FALLBACK ? FALLBACK_CANDIDATES : [];
      setCandidates(next);
      setSelectedCandidateId((current) => current || next[0]?.id || "");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void loadCandidates();
  }, []);

  const filteredCandidates = useMemo(() => {
    const keyword = compactText(query).toLowerCase();
    if (!keyword) return candidates;
    return candidates.filter((candidate) => [candidate.title, candidate.summary, candidate.matchReason, candidate.sourceCaseId, candidate.experiencePack?.title, candidate.experiencePack?.workflowBinding.workflowName].filter(Boolean).join(" ").toLowerCase().includes(keyword));
  }, [candidates, query]);

  const selectedCandidate = filteredCandidates.find((candidate) => candidate.id === selectedCandidateId) || filteredCandidates[0] || null;
  const selectedPack = selectedCandidate?.experiencePack || null;
  const enabledPacks = filteredCandidates.map(packFromCandidate).filter((pack): pack is ExperiencePackView => Boolean(pack && pack.enabled));

  useEffect(() => {
    async function loadReuseRecords() {
      if (!selectedPack?.id) {
        setReuseRecords([]);
        return;
      }
      try {
        const payload = await listExperiencePackReuseRecords(selectedPack.id, { limit: 20 });
        setReuseRecords(payload.items);
      } catch (_cause) {
        setReuseRecords([]);
      }
    }
    void loadReuseRecords();
  }, [selectedPack?.id]);

  async function approveCandidate(candidate: ExperienceCandidateView) {
    const pack = await approveExperiencePackCandidate(candidate.id, { reviewer: "ui", comment: "前端审核启用" });
    setCandidates((current) => current.map((item) => item.id === candidate.id ? { ...item, status: "approved", experiencePack: pack } : item));
  }

  async function togglePack(pack: ExperiencePackView) {
    const next = await setExperiencePackEnabled(pack.id, !pack.enabled);
    setCandidates((current) => current.map((item) => item.experiencePack?.id === pack.id ? { ...item, experiencePack: next } : item));
  }

  async function saveScopes(pack: ExperiencePackView) {
    const next = await saveExperiencePackAuthorizationScopes(pack.id, { scopes: pack.authorizationScopes.length ? pack.authorizationScopes : [{ type: "environment", value: "prod", searchable: true, reason: "默认生产环境授权" }] });
    setCandidates((current) => current.map((item) => item.experiencePack?.id === pack.id ? { ...item, experiencePack: next } : item));
  }

  return (
    <SettingsPageFrame
      title="经验包"
      description="从运维对话中生成可复用经验包，但必须先审核启用，再配置可检索范围；命中后只推荐 Workflow，不自动执行。"
    >
      {error ? <div className="rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm text-amber-900">{error}</div> : null}
      <Card className="rounded-lg bg-white">
        <CardContent className="flex flex-col gap-3 pt-0 lg:flex-row lg:items-center">
          <label className="relative min-w-0 flex-1">
            <Search className="pointer-events-none absolute left-2.5 top-2 h-4 w-4 text-slate-400" />
            <Input className="pl-8" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索经验包、来源 Case、Workflow、适用范围" />
          </label>
          <div className="flex flex-wrap gap-2">
            <Badge variant="secondary">两道门槛</Badge>
            <Badge variant="outline">审核启用</Badge>
            <Badge variant="outline">授权范围</Badge>
            <Badge variant="outline">推荐 Workflow</Badge>
          </div>
        </CardContent>
      </Card>

      <div
        data-testid="experience-pack-workbench-layout"
        className={`grid gap-4 ${selectedCandidate ? "xl:grid-cols-[minmax(0,1fr)_420px]" : "xl:grid-cols-1"}`}
      >
        <Card className="rounded-lg bg-white" data-testid="experience-pack-workbench">
          <CardHeader>
            <CardTitle>经验包工作台</CardTitle>
            <CardDescription>候选、启用状态、授权范围、Eval 和复用效果在同一处闭环。</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="grid gap-4">
              <div className="flex flex-wrap gap-2" role="tablist" aria-label="经验包视图">
                {[
                  ["candidates", "候选"],
                  ["enabled", "已启用"],
                  ["scopes", "授权范围"],
                  ["eval", "Eval"],
                  ["reuse", "复用记录"],
                ].map(([key, label]) => (
                  <button key={key} type="button" role="tab" aria-selected={activeTab === key} className={`rounded-lg border px-3 py-2 text-sm ${activeTab === key ? "bg-slate-900 text-white" : "bg-white"}`} onClick={() => setActiveTab(key)}>
                    {label}
                  </button>
                ))}
              </div>

              {activeTab === "candidates" ? (
                <section className="grid gap-2">
                {filteredCandidates.length ? filteredCandidates.map((candidate) => (
                  <ExperienceCandidateCard
                    key={candidate.id}
                    candidate={candidate}
                    selected={selectedCandidate?.id === candidate.id}
                    onSelect={() => setSelectedCandidateId(candidate.id)}
                    onApprove={() => void approveCandidate(candidate)}
                  />
                )) : <EmptyState title="没有候选经验包" description="从 AI 对话或 Case 详情提炼后，会先进入候选区；审核启用后再配置可检索范围。" />}
                </section>
              ) : null}

              {activeTab === "enabled" ? (
                <section className="grid gap-2">
                  {enabledPacks.length ? enabledPacks.map((pack) => <EnabledPackCard key={pack.id} pack={pack} onToggle={() => void togglePack(pack)} />) : <EmptyState title="没有已启用经验包" description="候选经验包审核通过后才会出现在这里。" />}
                </section>
              ) : null}

              {activeTab === "scopes" ? selectedPack ? <AuthorizationScopesPanel pack={selectedPack} onSave={() => void saveScopes(selectedPack)} /> : <EmptyState title="未选择经验包" description="先在候选或已启用列表中选择一个经验包。" /> : null}

              {activeTab === "eval" ? selectedPack ? <RetrievalEvalPanel pack={selectedPack} /> : <EmptyState title="未选择经验包" description="选择经验包后查看检索 Eval。" /> : null}

              {activeTab === "reuse" ? (
                <ReuseRecordsPanel records={reuseRecords} />
              ) : null}
            </div>
          </CardContent>
        </Card>

        {selectedCandidate ? <ExperiencePackDetail candidate={selectedCandidate} /> : null}
      </div>
    </SettingsPageFrame>
  );
}

function ExperienceCandidateCard({ candidate, selected, onSelect, onApprove }: { candidate: ExperienceCandidateView; selected: boolean; onSelect: () => void; onApprove: () => void }) {
  const pack = candidate.experiencePack;
  return (
    <Card size="sm" className={selected ? "cursor-pointer rounded-lg border-slate-400 bg-slate-50" : "cursor-pointer rounded-lg bg-white"} onClick={onSelect}>
        <CardContent className="grid gap-3 pt-0">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <div className="font-medium text-slate-900">{candidate.title}</div>
              <div className="text-xs leading-5 text-slate-500">{candidate.summary}</div>
            </div>
            <ToneBadge tone={searchableTone(pack)}>{pack?.searchable ? "可检索" : "不可检索"}</ToneBadge>
          </div>
          <div className="flex flex-wrap gap-2 text-xs text-slate-600">
            <ToneBadge>来源 Case {candidate.sourceCaseId || "-"}</ToneBadge>
            <ToneBadge>生成时间 {candidateGeneratedAt(candidate) || "-"}</ToneBadge>
            <ToneBadge>推荐 Workflow {pack?.workflowBinding.workflowName || "待绑定"}</ToneBadge>
            <ToneBadge>{statusLabel(candidate.status)}</ToneBadge>
          </div>
          <p className="text-xs text-slate-500">{pack?.searchableReason || "经验包尚未生成可检索包体"}</p>
          {pack?.reviewStatus !== "approved" ? <Button size="sm" variant="outline" type="button" onClick={(event) => { event.stopPropagation(); onApprove(); }}><CheckCircle2 />审核通过</Button> : null}
        </CardContent>
    </Card>
  );
}

function candidateGeneratedAt(candidate: ExperienceCandidateView) {
  const source = candidate.raw || {};
  const value = source.createdAt || source.created_at || source.generatedAt || source.generated_at;
  return value ? String(value) : "";
}

function EnabledPackCard({ pack, onToggle }: { pack: ExperiencePackView; onToggle: () => void }) {
  const risk = String(pack.raw.risk || pack.raw.risk_level || pack.raw.riskLevel || "未标注风险");
  const recentReuse = String(pack.raw.recent_reuse_result || pack.raw.recentReuseResult || pack.raw.last_reuse_result || "暂无复用记录");
  return (
    <Card size="sm" className="rounded-lg bg-white">
      <CardContent className="flex flex-col gap-3 pt-0 md:flex-row md:items-center">
        <div className="min-w-0 flex-1">
          <div className="font-medium text-slate-900">{pack.title}</div>
          <div className="text-xs leading-5 text-slate-500">{pack.summary}</div>
          <div className="mt-2 flex flex-wrap gap-2">
            <ToneBadge>{pack.version || "未版本化"}</ToneBadge>
            <ToneBadge>{risk}</ToneBadge>
            <ToneBadge tone={searchableTone(pack)}>{pack.searchable ? "可检索" : "不可检索"}</ToneBadge>
            <ToneBadge>{pack.workflowBinding.workflowName}</ToneBadge>
            <ToneBadge>{recentReuse}</ToneBadge>
          </div>
        </div>
        <Button size="sm" variant="outline" type="button" onClick={onToggle}><Power />{pack.enabled ? "停用" : "启用"}</Button>
      </CardContent>
    </Card>
  );
}

function AuthorizationScopesPanel({ pack, onSave }: { pack: ExperiencePackView; onSave: () => void }) {
  return (
    <section className="grid gap-3 rounded-lg border bg-slate-50 p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3 className="font-medium text-slate-950">授权范围</h3>
          <p className="mt-1 text-sm text-slate-600">{pack.searchableReason}</p>
        </div>
        <ToneBadge tone={searchableTone(pack)}>{pack.searchable ? "可检索" : "不可检索"}</ToneBadge>
      </div>
      <div className="grid gap-2">
        {pack.authorizationScopes.length ? pack.authorizationScopes.map((scope) => (
          <div key={scope.id} className="flex flex-wrap items-center justify-between gap-2 rounded-lg border bg-white p-3 text-sm">
            <span>{scopeLabel(scope)}</span>
            <span className="text-slate-500">{scope.searchable ? "允许检索" : "不可检索"} · {scope.reason || "未说明"}</span>
          </div>
        )) : <p className="text-sm text-slate-500">尚未配置用户、团队、环境、资源类型或业务范围，当前不可检索。</p>}
      </div>
      <Button type="button" variant="outline" onClick={onSave}><ShieldCheck />保存授权范围</Button>
    </section>
  );
}

function RetrievalEvalPanel({ pack }: { pack: ExperiencePackView }) {
  return (
    <section className="grid gap-3 rounded-lg border bg-slate-50 p-4">
      <h3 className="font-medium text-slate-950">检索 Eval</h3>
      <dl className="grid gap-2 text-sm text-slate-600 md:grid-cols-2">
        <div><dt className="font-medium text-slate-500">分数</dt><dd>{pack.retrievalEval.score ?? "-"}</dd></div>
        <div><dt className="font-medium text-slate-500">命中 Case</dt><dd>{pack.retrievalEval.matchedCases}</dd></div>
        <div><dt className="font-medium text-slate-500">结论</dt><dd>{statusLabel(pack.retrievalEval.verdict)}</dd></div>
        <div><dt className="font-medium text-slate-500">更新时间</dt><dd>{pack.retrievalEval.lastEvaluatedAt || "-"}</dd></div>
      </dl>
    </section>
  );
}

function ReuseRecordsPanel({ records }: { records: ReuseRecordView[] }) {
  return (
    <section className="grid gap-2">
      {records.length ? records.map((record) => (
        <Card key={record.id} size="sm" className="rounded-lg bg-white">
          <CardContent className="grid gap-2 pt-0 text-sm">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <Link className="font-medium text-slate-900 underline-offset-4 hover:underline" to={`/incidents/${encodeURIComponent(record.caseId)}`}>{record.caseId}</Link>
              <ToneBadge>{statusLabel(record.result)}</ToneBadge>
            </div>
            <p className="text-slate-600">{record.summary}</p>
            <p className="text-xs text-slate-500">{record.reusedBy || "Agent"} · {record.reusedAt || "-"}</p>
          </CardContent>
        </Card>
      )) : <EmptyState title="暂无复用记录" description="经验包被推荐并执行 Workflow 后，会记录命中 Case、执行结果、验证结果和失败回退。" />}
    </section>
  );
}

function ExperiencePackDetail({ candidate }: { candidate: ExperienceCandidateView | null }) {
  const pack = candidate?.experiencePack;
  if (!candidate) {
    return null;
  }
  return (
    <Card className="rounded-lg bg-white">
      <CardHeader>
        <CardTitle>包详情</CardTitle>
        <CardDescription>{candidate.title}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4">
        <div className="flex flex-wrap gap-2">
          <ToneBadge>{statusLabel(candidate.status)}</ToneBadge>
          <ToneBadge tone={searchableTone(pack)}>{pack?.searchable ? "可检索" : "不可检索"}</ToneBadge>
          {pack?.version ? <ToneBadge>{pack.version}</ToneBadge> : null}
        </div>
        <section>
          <div className="text-xs font-medium uppercase tracking-normal text-slate-500">候选摘要</div>
          <p className="mt-1 text-sm leading-6 text-slate-700">{candidate.summary}</p>
        </section>
        <section className="rounded-lg border bg-slate-50 p-3">
          <div className="text-xs font-medium uppercase tracking-normal text-slate-500">推荐 Workflow</div>
          <p className="mt-1 text-sm text-slate-700">{pack?.workflowBinding.workflowName || "待生成 Workflow 草案"}</p>
          <p className="mt-1 text-xs text-slate-500">经验包命中时只推荐该 Workflow，执行前仍需 Case 中的确认和 HostLease 校验。</p>
        </section>
        <section>
          <div className="text-xs font-medium uppercase tracking-normal text-slate-500">检索状态</div>
          <p className="mt-1 text-sm leading-6 text-slate-700">{pack?.searchableReason || "尚未生成可检索状态。"}</p>
        </section>
      </CardContent>
    </Card>
  );
}
