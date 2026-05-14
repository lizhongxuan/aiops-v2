import { CheckCircle2, ChevronLeft, ChevronRight, Power, Search, ShieldCheck, X } from "lucide-react";
import { useEffect, useMemo, useState } from "react";

import {
  approveExperiencePackCandidate,
  confirmRunnerCandidate,
  listExperiencePackCandidates,
  listExperiencePackReuseRecords,
  normalizeExperienceCandidate,
  saveExperiencePackAuthorizationScopes,
  setExperiencePackEnabled,
  type ExperienceCandidateView,
  type ExperiencePackView,
  type ReuseRecordView,
} from "@/api/experiencePacks";
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
const EXPERIENCE_PAGE_SIZE = 6;

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

function packFromCandidate(candidate: ExperienceCandidateView) {
  return candidate.experiencePack;
}

function shortSummary(value = "", maxLength = 96) {
  const text = compactText(value);
  return text.length > maxLength ? `${text.slice(0, maxLength)}...` : text;
}

export function ExperiencePacksPage() {
  const [query, setQuery] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [activeTab, setActiveTab] = useState("library");
  const [detailOpen, setDetailOpen] = useState(false);
  const [candidates, setCandidates] = useState<ExperienceCandidateView[]>(ALLOW_FIXTURE_FALLBACK ? FALLBACK_CANDIDATES : []);
  const [selectedCandidateId, setSelectedCandidateId] = useState(ALLOW_FIXTURE_FALLBACK ? FALLBACK_CANDIDATES[0]?.id || "" : "");
  const [libraryPage, setLibraryPage] = useState(1);
  const [reviewPage, setReviewPage] = useState(1);
  const [reuseRecords, setReuseRecords] = useState<ReuseRecordView[]>([]);
  const [runnerDraftLinks, setRunnerDraftLinks] = useState<Record<string, string>>({});
  const [runnerDraftStatus, setRunnerDraftStatus] = useState<Record<string, string>>({});

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
  const reviewCandidates = filteredCandidates.filter((candidate) => {
    const pack = candidate.experiencePack;
    return candidate.status === "candidate" || pack?.reviewStatus !== "approved" || pack?.validationGate.status === "blocked";
  });
  const libraryTotalPages = Math.max(1, Math.ceil(filteredCandidates.length / EXPERIENCE_PAGE_SIZE));
  const reviewTotalPages = Math.max(1, Math.ceil(reviewCandidates.length / EXPERIENCE_PAGE_SIZE));
  const currentLibraryPage = Math.min(libraryPage, libraryTotalPages);
  const currentReviewPage = Math.min(reviewPage, reviewTotalPages);
  const pagedLibraryCandidates = filteredCandidates.slice((currentLibraryPage - 1) * EXPERIENCE_PAGE_SIZE, currentLibraryPage * EXPERIENCE_PAGE_SIZE);
  const pagedReviewCandidates = reviewCandidates.slice((currentReviewPage - 1) * EXPERIENCE_PAGE_SIZE, currentReviewPage * EXPERIENCE_PAGE_SIZE);

  useEffect(() => {
    setLibraryPage(1);
    setReviewPage(1);
  }, [query, candidates.length]);

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

  async function sendToRunnerStudio(candidate: ExperienceCandidateView) {
    const key = candidate.id;
    setRunnerDraftStatus((current) => ({ ...current, [key]: "正在生成 Runner Studio 草稿..." }));
    try {
      const result = await confirmRunnerCandidate({
        candidateId: candidate.id,
        packId: candidate.packId,
        title: candidate.title,
        summary: candidate.summary,
      });
      setRunnerDraftLinks((current) => ({ ...current, [key]: result.studioDraftLink || `/runner/${encodeURIComponent(result.workflowId || result.id)}` }));
      setRunnerDraftStatus((current) => ({ ...current, [key]: "已创建本地草稿，进入 Runner Studio 后仍需人工审核、Dry Run 和发布。" }));
    } catch (cause) {
      setRunnerDraftStatus((current) => ({ ...current, [key]: (cause as Error).message || "Runner 草稿生成失败" }));
    }
  }

  function openCandidateDetail(candidate: ExperienceCandidateView) {
    setSelectedCandidateId(candidate.id);
    setDetailOpen(true);
  }

  function selectView(key: string) {
    setActiveTab(key);
    setDetailOpen(false);
  }

  const headerActions = (
    <div className="flex w-[min(100%,56rem)] flex-col gap-2 sm:flex-row sm:items-center">
      <label className="relative min-w-0 flex-1">
        <Search className="pointer-events-none absolute left-2.5 top-2 h-4 w-4 text-slate-400" />
        <Input className="h-8 pl-8" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索经验包、来源 Case、Workflow、适用范围" />
      </label>
      <div className="flex shrink-0 flex-wrap gap-2" role="tablist" aria-label="经验包视图">
        {[
          ["library", "经验库"],
          ["review", "待审核经验"],
        ].map(([key, label]) => {
          const selected = activeTab === key && !detailOpen;
          return (
            <button key={key} type="button" role="tab" aria-selected={selected} className={`rounded-lg border px-3 py-1.5 text-sm ${selected ? "bg-slate-900 text-white" : "bg-white"}`} onClick={() => selectView(key)}>
              {label}
            </button>
          );
        })}
      </div>
    </div>
  );

  return (
    <SettingsPageFrame title="经验包" description="" actions={headerActions}>
      {error ? <div className="rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm text-amber-900">{error}</div> : null}

      <div
        data-testid="experience-pack-workbench-layout"
        className="grid min-h-0 flex-1 gap-4 xl:grid-cols-1"
      >
        <Card className="flex min-h-0 flex-col rounded-lg bg-white" data-testid="experience-pack-workbench">
          <CardContent className="flex min-h-0 flex-1 flex-col">
            {activeTab === "library" ? (
              <section className="flex min-h-0 flex-1 flex-col gap-2">
                {filteredCandidates.length ? pagedLibraryCandidates.map((candidate) => (
                  <ExperienceCandidateCard
                    key={candidate.id}
                    candidate={candidate}
                    selected={selectedCandidate?.id === candidate.id}
                    onSelect={() => openCandidateDetail(candidate)}
                    onApprove={() => void approveCandidate(candidate)}
                  />
                )) : <EmptyState title="没有经验包" description="从 AI 对话或 Case 详情提炼后，会先进入待审核经验；审核启用后再进入经验库。" />}
                <PaginationControls
                  page={currentLibraryPage}
                  totalPages={libraryTotalPages}
                  onPrevious={() => setLibraryPage((page) => Math.max(1, page - 1))}
                  onNext={() => setLibraryPage((page) => Math.min(libraryTotalPages, page + 1))}
                />
              </section>
            ) : null}

            {activeTab === "review" ? (
              <section className="flex min-h-0 flex-1 flex-col gap-2">
                {reviewCandidates.length ? pagedReviewCandidates.map((candidate) => (
                    <ReviewQueueCard
                      key={candidate.id}
                      candidate={candidate}
                      selected={selectedCandidate?.id === candidate.id}
                      onSelect={() => openCandidateDetail(candidate)}
                      onApprove={() => void approveCandidate(candidate)}
                      onConfigureScope={() => candidate.experiencePack ? void saveScopes(candidate.experiencePack) : undefined}
                      onSendToRunnerStudio={() => void sendToRunnerStudio(candidate)}
                      runnerDraftLink={runnerDraftLinks[candidate.id]}
                      runnerDraftStatus={runnerDraftStatus[candidate.id]}
                    />
                  )) : <EmptyState title="没有待审核经验" description="候选经验包通过文件完整性、GEP schema、asset_id 和 validation gate 后会进入经验库。" />}
                <PaginationControls
                  page={currentReviewPage}
                  totalPages={reviewTotalPages}
                  onPrevious={() => setReviewPage((page) => Math.max(1, page - 1))}
                  onNext={() => setReviewPage((page) => Math.min(reviewTotalPages, page + 1))}
                />
              </section>
            ) : null}
          </CardContent>
        </Card>
      </div>
      {detailOpen && selectedCandidate ? (
        <ExperiencePackDetailModal
          candidate={selectedCandidate}
          reuseRecords={reuseRecords}
          onClose={() => setDetailOpen(false)}
          onToggle={selectedPack ? () => void togglePack(selectedPack) : undefined}
          onConfigureScope={selectedPack ? () => void saveScopes(selectedPack) : undefined}
        />
      ) : null}
    </SettingsPageFrame>
  );
}

function ExperienceCandidateCard({ candidate, selected, onSelect, onApprove }: { candidate: ExperienceCandidateView; selected: boolean; onSelect: () => void; onApprove: () => void }) {
  const pack = candidate.experiencePack;
  return (
    <Card size="sm" className={selected ? "cursor-pointer rounded-lg border-slate-400 bg-slate-50" : "cursor-pointer rounded-lg bg-white"} onClick={onSelect}>
      <CardContent className="grid gap-2">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0">
            <div className="font-medium text-slate-900">{candidate.title}</div>
            <div className="mt-1 text-xs leading-5 text-slate-500">{shortSummary(pack?.skill.summary || candidate.summary)}</div>
          </div>
          <div className="flex shrink-0 flex-wrap gap-2">
            <ToneBadge tone={searchableTone(pack)}>{pack?.searchable ? "可检索" : "不可检索"}</ToneBadge>
            <ToneBadge>{statusLabel(candidate.status)}</ToneBadge>
          </div>
        </div>
        <div className="flex flex-wrap gap-2 text-xs text-slate-500">
          <span>{pack?.category || "repair"}</span>
          <span>·</span>
          <span>{pack?.usageShape || "diagnostic"}</span>
          {candidate.sourceCaseId ? (
            <>
              <span>·</span>
              <span>{candidate.sourceCaseId}</span>
            </>
          ) : null}
        </div>
        {pack?.reviewStatus !== "approved" ? <Button size="sm" variant="outline" type="button" onClick={(event) => { event.stopPropagation(); onApprove(); }}><CheckCircle2 />审核通过</Button> : null}
      </CardContent>
    </Card>
  );
}

function PaginationControls({
  page,
  totalPages,
  onPrevious,
  onNext,
}: {
  page: number;
  totalPages: number;
  onPrevious: () => void;
  onNext: () => void;
}) {
  return (
    <div data-testid="experience-pack-pagination" className="mt-auto flex items-center justify-end gap-2 pt-3 text-xs text-slate-500">
      <Button size="sm" variant="outline" type="button" disabled={page <= 1} onClick={onPrevious}>
        <ChevronLeft />上一页
      </Button>
      <span>第 {page} / {totalPages} 页</span>
      <Button size="sm" variant="outline" type="button" disabled={page >= totalPages} onClick={onNext}>
        下一页<ChevronRight />
      </Button>
    </div>
  );
}

function ReviewQueueCard({
  candidate,
  selected,
  onSelect,
  onApprove,
  onConfigureScope,
  onSendToRunnerStudio,
  runnerDraftLink = "",
  runnerDraftStatus = "",
}: {
  candidate: ExperienceCandidateView;
  selected: boolean;
  onSelect: () => void;
  onApprove: () => void;
  onConfigureScope: () => void;
  onSendToRunnerStudio: () => void;
  runnerDraftLink?: string;
  runnerDraftStatus?: string;
}) {
  const pack = candidate.experiencePack;
  const gatePassed = Boolean(pack?.validationGate.passed);
  return (
    <Card size="sm" className={selected ? "cursor-pointer rounded-lg border-slate-400 bg-slate-50" : "cursor-pointer rounded-lg bg-white"} onClick={onSelect}>
      <CardContent className="grid gap-3 pt-0">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <div className="font-medium text-slate-900">{candidate.title}</div>
            <div className="text-xs leading-5 text-slate-500">{pack?.skill.summary || candidate.summary}</div>
          </div>
          <ToneBadge tone={gatePassed ? "success" : "danger"}>validation gate {pack?.validationGate.status || "unknown"}</ToneBadge>
        </div>
        <dl className="grid gap-2 text-xs text-slate-600 md:grid-cols-2">
          <div><dt className="font-medium text-slate-500">Skill.md 摘要</dt><dd>{pack?.skill.summary || candidate.summary}</dd></div>
          <div><dt className="font-medium text-slate-500">必要文件完整性</dt><dd>{pack ? "Skill.md / Gene / Capsule 已登记" : "待检查"}</dd></div>
          <div><dt className="font-medium text-slate-500">GEP schema</dt><dd>{pack ? "已校验" : "待校验"}</dd></div>
          <div><dt className="font-medium text-slate-500">asset_id</dt><dd>{pack?.advancedRefs.geneAssetId || "待校验"}</dd></div>
          <div><dt className="font-medium text-slate-500">Capsule 来源</dt><dd>{candidate.sourceCaseId || "待补充"}</dd></div>
          <div><dt className="font-medium text-slate-500">Runner Binding 可用性</dt><dd>{runnerStatus(pack)}</dd></div>
          <div><dt className="font-medium text-slate-500">AVOID cue 候选</dt><dd>{String(pack?.raw.avoid_cue_candidate || pack?.raw.avoidCueCandidate || "无阻断")}</dd></div>
        </dl>
        <div className="flex flex-wrap gap-2">
          <Button size="sm" variant="outline" type="button" disabled={!gatePassed} onClick={(event) => { event.stopPropagation(); onApprove(); }}><CheckCircle2 />approve</Button>
          <Button size="sm" variant="outline" type="button" onClick={(event) => event.stopPropagation()}>request changes</Button>
          <Button size="sm" variant="outline" type="button" onClick={(event) => event.stopPropagation()}>reject</Button>
          <Button size="sm" variant="outline" type="button" onClick={(event) => { event.stopPropagation(); onConfigureScope(); }}><ShieldCheck />configure scope</Button>
          <Button size="sm" variant="outline" type="button" onClick={(event) => { event.stopPropagation(); onSendToRunnerStudio(); }}>发送到 Runner Studio</Button>
        </div>
        {runnerDraftStatus ? <div className="text-xs text-slate-500">{runnerDraftStatus}</div> : null}
        {runnerDraftLink ? (
          <a className="text-xs font-medium text-slate-900 underline-offset-4 hover:underline" href={runnerDraftLink} onClick={(event) => event.stopPropagation()}>
            打开 Runner Studio
          </a>
        ) : null}
      </CardContent>
    </Card>
  );
}

function ExperiencePackDetailModal({
  candidate,
  reuseRecords,
  onClose,
  onToggle,
  onConfigureScope,
}: {
  candidate: ExperienceCandidateView;
  reuseRecords: ReuseRecordView[];
  onClose: () => void;
  onToggle?: () => void;
  onConfigureScope?: () => void;
}) {
  useEffect(() => {
    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") onClose();
    }
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [onClose]);

  return (
    <div className="fixed inset-0 z-[90] flex items-start justify-center overflow-y-auto bg-slate-950/40 p-4 sm:p-8" onClick={onClose}>
      <section
        role="dialog"
        aria-modal="true"
        aria-label="经验详情"
        className="relative w-full max-w-3xl"
        onClick={(event) => event.stopPropagation()}
      >
        <Button className="absolute right-3 top-3 z-10 bg-white" size="icon" variant="outline" type="button" aria-label="关闭经验详情" onClick={onClose}>
          <X />
        </Button>
        <ExperiencePackDetail
          candidate={candidate}
          reuseRecords={reuseRecords}
          onToggle={onToggle}
          onConfigureScope={onConfigureScope}
        />
      </section>
    </div>
  );
}

function ExperiencePackDetail({
  candidate,
  reuseRecords = [],
  onToggle,
  onConfigureScope,
  compact = false,
}: {
  candidate: ExperienceCandidateView | null;
  reuseRecords?: ReuseRecordView[];
  onToggle?: () => void;
  onConfigureScope?: () => void;
  compact?: boolean;
}) {
  const pack = candidate?.experiencePack;
  if (!candidate) {
    return null;
  }
  return (
    <Card className="rounded-lg bg-white">
      <CardHeader>
        <CardTitle>经验详情</CardTitle>
        <CardDescription>{candidate.title}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4">
        <div className="flex flex-wrap gap-2">
          <ToneBadge>{statusLabel(candidate.status)}</ToneBadge>
          <ToneBadge tone={searchableTone(pack)}>{pack?.searchable ? "可检索" : "不可检索"}</ToneBadge>
          {pack?.version ? <ToneBadge>{pack.version}</ToneBadge> : null}
          <ToneBadge>{pack?.category || "repair"}</ToneBadge>
          <ToneBadge>{pack?.usageShape || "diagnostic"}</ToneBadge>
        </div>
        <section className="rounded-lg border bg-slate-50 p-3">
          <div className="text-xs font-medium uppercase tracking-normal text-slate-500">Skill / Runner / GEP</div>
          <div className="mt-2 grid gap-2 text-sm leading-6 text-slate-700">
            <p>Runner 负责怎么执行；Skill 负责为什么这么做、什么时候适用、环境不同时怎么调整、怎么验证和回滚。</p>
            <p>GEP 负责进化治理：记录来源故障、Gene、环境指纹、成功失败、验证方式、版本血缘和不可变审计。</p>
          </div>
        </section>
        <section className="rounded-lg border bg-slate-50 p-3">
          <div className="text-xs font-medium uppercase tracking-normal text-slate-500">检索与适配</div>
          <p className="mt-2 text-sm leading-6 text-slate-700">经验包检索使用 PostgreSQL + pgvector 承载结构化过滤、BM25/关键词、向量语义、环境指纹和 GEP Gene signals_match；环境差异时先生成适配计划和 Runner 变体，用户审核后再执行。</p>
        </section>
        {pack ? (
          <section className="rounded-lg border bg-slate-50 p-3">
            <div className="text-xs font-medium uppercase tracking-normal text-slate-500">检索授权范围</div>
            <p className="mt-1 text-sm leading-6 text-slate-700">{pack.searchableReason}</p>
            <p className="mt-1 text-xs text-slate-500">
              {pack.authorizationScopes.length
                ? pack.authorizationScopes.map((scope) => `${scope.type}:${scope.value}`).join("、")
                : "未配置 scope"}
            </p>
            {onConfigureScope ? (
              <Button className="mt-3" size="sm" variant="outline" type="button" onClick={onConfigureScope}>
                <ShieldCheck />配置可检索范围
              </Button>
            ) : null}
          </section>
        ) : null}
        <section>
          <div className="text-xs font-medium uppercase tracking-normal text-slate-500">Skill.md</div>
          <p className="mt-1 text-sm leading-6 text-slate-700">{pack?.skill.name || candidate.title}：{pack?.skill.summary || candidate.summary}</p>
        </section>
        <section className="rounded-lg border bg-slate-50 p-3">
          <div className="text-xs font-medium uppercase tracking-normal text-slate-500">required files</div>
          <p className="mt-1 text-sm text-slate-700">skills/SKILL.md、GEP Gene、Capsule、Validation、Rollback</p>
        </section>
        <section>
          <div className="text-xs font-medium uppercase tracking-normal text-slate-500">历史效果</div>
          <p className="mt-1 text-sm leading-6 text-slate-700">成功 {pack?.history.successCount ?? 0}，失败 {pack?.history.failureCount ?? 0}，最近结果 {statusLabel(pack?.history.recentResult || "unknown")}。</p>
          {reuseRecords.length ? <p className="mt-1 text-xs text-slate-500">最近复用：{reuseRecords[0].caseId} / {statusLabel(reuseRecords[0].result)}</p> : null}
        </section>
        <section className="rounded-lg border bg-slate-50 p-3">
          <div className="text-xs font-medium uppercase tracking-normal text-slate-500">可选 Runner 工作流</div>
          <p className="mt-1 text-sm text-slate-700">{pack?.workflowBinding.workflowName || pack?.runnerBindings[0]?.workflowName || "待生成 Workflow 草案"} · {runnerStatus(pack)}</p>
          <p className="mt-1 text-xs text-slate-500">经验包命中时只推荐 Runner Workflow，执行前仍需 Case 确认、Dry Run 和 HostLease 校验。</p>
          {onToggle ? <Button className="mt-3" size="sm" variant="outline" type="button" onClick={onToggle}><Power />{pack?.enabled ? "pause" : "enable"}</Button> : null}
        </section>
        {!compact ? (
          <section className="grid gap-2 rounded-lg border bg-slate-50 p-3 text-sm">
            <div className="text-xs font-medium uppercase tracking-normal text-slate-500">高级区</div>
            <div>Gene asset_id：{pack?.advancedRefs.geneAssetId || "未返回"}</div>
            <div>Capsule 列表：{pack?.advancedRefs.capsuleAssetIds.join("、") || "未返回"}</div>
            <div>EvolutionEvent 列表：{String(pack?.raw.evolution_events_count || pack?.raw.evolutionEventCount || "摘要未返回")}</div>
            <div>MemoryGraphEvent 摘要：{String(pack?.raw.memory_events_summary || pack?.raw.memoryEventsSummary || "摘要未返回")}</div>
            <div>AVOID cues：{String(pack?.raw.avoid_cues_summary || pack?.raw.avoidCuesSummary || "无")}</div>
            <div>OS/environment variants：{String(pack?.raw.os_variants || pack?.raw.osVariants || pack?.runnerBindings.map((binding) => binding.osVariant).filter(Boolean).join("、") || "default")}</div>
            <div>Runner Bindings：{pack?.runnerBindings.map((binding) => `${binding.workflowName} / ${binding.status}`).join("、") || "未绑定"}</div>
          </section>
        ) : null}
      </CardContent>
    </Card>
  );
}

function runnerStatus(pack?: ExperiencePackView | null) {
  if (!pack) return "unbound";
  const binding = pack.runnerBindings[0] || pack.workflowBinding;
  return `${binding.workflowName || binding.workflowId} / ${binding.status}`;
}
