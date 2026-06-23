import { Check, MessageSquare, X } from "lucide-react";
import { useEffect, useMemo, useState, type ReactNode } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";

import { Button } from "@/components/ui/button";
import {
  buildCaseViewModel,
  type CaseTabView,
  type CaseViewModel,
} from "@/components/cases/caseViewModels";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  ComplexPageFrame,
  EmptyPanel,
  KeyValueList,
  MetricStrip,
  RiskBadge,
} from "@/pages/complexPageComponents";
import {
  asArray,
  compactText,
  getIncident,
  getOpsGraphBusinessImpact,
  getOpsGraphNeighborhood,
  startIncidentChat,
  submitApprovalDecision,
  type ApprovalAuditRecord,
  type IncidentRecord,
} from "@/pages/complexPagesApi";
import { LoadingState, StatusAlert } from "@/pages/settingsComponents";

export function IncidentWorkbenchPage() {
  const { incidentId = "" } = useParams();
  const navigate = useNavigate();
  const [incident, setIncident] = useState<IncidentRecord | null>(null);
  const [neighbors, setNeighbors] = useState<Record<string, unknown>[]>([]);
  const [impact, setImpact] = useState<{
    capabilities?: Record<string, unknown>[];
    tenants?: Record<string, unknown>[];
  }>({});
  const [loading, setLoading] = useState(true);
  const [busyApproval, setBusyApproval] = useState("");
  const [busyStartChat, setBusyStartChat] = useState(false);
  const [message, setMessage] = useState<{
    type: "success" | "error" | "info";
    text: string;
  } | null>(null);

  const caseView = useMemo(
    () => (incident ? buildCaseViewModel(incident) : null),
    [incident],
  );
  const title = caseView?.title || `Case ${incidentId}`;
  const entityId = compactText(
    caseView?.service || incident?.entityId || incident?.id || incidentId,
  );
  const hypotheses = asArray(incident?.hypotheses);
  const evidence = timelineRecords(caseView?.evidence);
  const hostProfiles = timelineRecords(caseView?.hostProfiles);
  const hostLeases = timelineRecords(caseView?.hostLeases);
  const workflowRuns = timelineRecords(caseView?.workflowRuns);
  const verifications = timelineRecords(caseView?.verifications);
  const experienceCandidates = timelineRecords(caseView?.experienceCandidates);
  const postmortem = (incident?.postmortem || {}) as Record<string, unknown>;
  const pendingApprovals = useMemo(
    () => asArray<ApprovalAuditRecord>(incident?.pendingApprovals),
    [incident],
  );
  const hostLeaseBlocked = Boolean(
    caseView?.blockingItems.some((item) => item.key === "host_lease_blocked"),
  );
  const canStartCorootChat = caseView?.source === "coroot";

  async function load() {
    if (!incidentId) return;
    setLoading(true);
    try {
      const nextIncident = await getIncident(incidentId);
      setIncident(nextIncident);
      const nextEntity = compactText(
        nextIncident.entityId || nextIncident.id || incidentId,
      );
      const [neighborhood, businessImpact] = await Promise.allSettled([
        nextEntity
          ? getOpsGraphNeighborhood(nextEntity, { incidentId })
          : Promise.resolve({}),
        nextEntity
          ? getOpsGraphBusinessImpact(nextEntity)
          : Promise.resolve({}),
      ]);
      setNeighbors(
        neighborhood.status === "fulfilled"
          ? neighborhood.value.neighbors ||
              neighborhood.value.items ||
              neighborhood.value.neighborhood?.neighbors ||
              []
          : [],
      );
      setImpact(
        businessImpact.status === "fulfilled" ? businessImpact.value : {},
      );
      setMessage(null);
    } catch (error) {
      setMessage({
        type: "error",
        text: error instanceof Error ? error.message : "加载事故上下文失败",
      });
    } finally {
      setLoading(false);
    }
  }

  async function decide(approvalId: string, decision: "approved" | "rejected") {
    setBusyApproval(approvalId);
    try {
      await submitApprovalDecision(approvalId, decision);
      setMessage({
        type: "success",
        text: `审批已${decision === "approved" ? "批准" : "拒绝"}`,
      });
      await load();
    } catch (error) {
      setMessage({
        type: "error",
        text: error instanceof Error ? error.message : "审批操作失败",
      });
    } finally {
      setBusyApproval("");
    }
  }

  async function startChatFromIncident() {
    if (!incidentId) return;
    setBusyStartChat(true);
    try {
      await startIncidentChat(incidentId);
      setMessage({ type: "success", text: "已进入 Chat 排查" });
      navigate("/");
    } catch (error) {
      setMessage({
        type: "error",
        text: error instanceof Error ? error.message : "进入 Chat 排查失败",
      });
    } finally {
      setBusyStartChat(false);
    }
  }

  useEffect(() => {
    void load();
  }, [incidentId]);

  return (
    <ComplexPageFrame
      kicker="Case 工作台"
      title={title}
      description={
        caseView?.summary || "Case 详情、证据、执行、验证和经验闭环。"
      }
      actions={
        <>
          <Button variant="outline" asChild>
            <Link to="/incidents">返回 Case 列表</Link>
          </Button>
          {canStartCorootChat ? (
            <Button
              onClick={() => void startChatFromIncident()}
              disabled={busyStartChat}
            >
              <MessageSquare />
              进入 Chat 排查
            </Button>
          ) : null}
          <Button variant="outline" asChild>
            <Link
              to={
                caseView?.promptTraceHref ||
                `/debug/prompts?case_id=${encodeURIComponent(incidentId)}`
              }
            >
              Prompt Trace
            </Link>
          </Button>
        </>
      }
    >
      {message ? (
        <StatusAlert
          type={message.type}
          title={message.type === "error" ? "操作失败" : "操作完成"}
          message={message.text}
        />
      ) : null}
      {loading ? (
        <LoadingState label="加载事故上下文" />
      ) : (
        <>
          {caseView ? (
            <CaseHeader caseView={caseView} incident={incident} />
          ) : null}
          {caseView ? <CaseStageTabs tabs={caseView.tabs} /> : null}
          <MetricStrip
            items={[
              {
                label: "状态",
                value: (
                  <RiskBadge
                    value={caseView?.statusLabel || incident?.status}
                  />
                ),
              },
              {
                label: "风险",
                value: (
                  <RiskBadge
                    value={
                      caseView?.severityLabel ||
                      incident?.severity ||
                      incident?.sev
                    }
                  />
                ),
              },
              {
                label: "环境",
                value:
                  caseView?.environment ||
                  incident?.environment ||
                  incident?.env ||
                  "待定",
              },
              {
                label: "业务能力",
                value:
                  caseView?.businessCapability ||
                  incident?.businessCapability ||
                  incident?.capability ||
                  "待定",
              },
            ]}
          />
          {caseView?.blockingItems.length ? (
            <Card className="rounded-lg border-amber-200 bg-amber-50">
              <CardHeader>
                <CardTitle>当前阻塞项</CardTitle>
                <CardDescription>
                  用于判断是否可以进入自动化修复。
                </CardDescription>
              </CardHeader>
              <CardContent>
                <Timeline items={timelineRecords(caseView.blockingItems)} />
              </CardContent>
            </Card>
          ) : null}
          <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_380px]">
            <div className="grid gap-4">
              <Card className="rounded-lg bg-white">
                <CardHeader>
                  <CardTitle>假设排序</CardTitle>
                </CardHeader>
                <CardContent>
                  {hypotheses.length ? (
                    <Timeline items={hypotheses} />
                  ) : (
                    <EmptyPanel
                      title="暂无假设"
                      description="等待 Agent 或 Coroot 写入。"
                    />
                  )}
                </CardContent>
              </Card>
              <Card className="rounded-lg bg-white">
                <CardHeader>
                  <CardTitle>证据时间线</CardTitle>
                  <CardDescription>
                    EvidenceRef 会同时用于 AI 对话卡片和 Prompt Trace 追溯。
                  </CardDescription>
                </CardHeader>
                <CardContent>
                  {evidence.length ? (
                    <Timeline items={evidence} />
                  ) : (
                    <EmptyPanel
                      title="暂无证据"
                      description="暂无 Coroot 或人工证据。"
                    />
                  )}
                </CardContent>
              </Card>
              <Card className="rounded-lg bg-white">
                <CardHeader>
                  <CardTitle>主机环境</CardTitle>
                </CardHeader>
                <CardContent>
                  {hostProfiles.length ? (
                    <Timeline items={hostProfiles} empty="暂无 HostProfile" />
                  ) : (
                    <EmptyPanel
                      title="暂无 HostProfile"
                      description="等待主机客户端上报环境信息。"
                    />
                  )}
                </CardContent>
              </Card>
              <Card className="rounded-lg bg-white">
                <CardHeader>
                  <CardTitle>Runner Workflow</CardTitle>
                </CardHeader>
                <CardContent>
                  {workflowRuns.length ? (
                    <Timeline items={workflowRuns} empty="暂无 Workflow 运行" />
                  ) : (
                    <EmptyPanel
                      title="暂无 Workflow"
                      description="还没有进入 Runner Workflow 执行。"
                    />
                  )}
                </CardContent>
              </Card>
              <Card className="rounded-lg bg-white">
                <CardHeader>
                  <CardTitle>验证结果</CardTitle>
                </CardHeader>
                <CardContent>
                  {verifications.length ? (
                    <Timeline items={verifications} empty="暂无验证结果" />
                  ) : (
                    <EmptyPanel
                      title="暂无验证"
                      description="修复前后验证结果会出现在这里。"
                    />
                  )}
                </CardContent>
              </Card>
              <Card className="rounded-lg bg-white">
                <CardHeader>
                  <CardTitle>复盘草稿</CardTitle>
                </CardHeader>
                <CardContent>
                  <p className="text-sm leading-6 text-slate-700">
                    {compactText(postmortem.summary || postmortem.rootCause) ||
                      "待补充"}
                  </p>
                </CardContent>
              </Card>
              <Card className="rounded-lg bg-white">
                <CardHeader>
                  <CardTitle>待审批动作</CardTitle>
                  <CardDescription>
                    使用现有 `/api/v1/approvals/:id/decision`，HostLease
                    阻塞时不能批准执行。
                  </CardDescription>
                </CardHeader>
                <CardContent>
                  {pendingApprovals.length ? (
                    <div className="grid gap-2">
                      {pendingApprovals.map((approval) => (
                        <div
                          key={approval.id}
                          data-testid="incident-sidebar-approval"
                          className="rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm"
                        >
                          <div className="font-medium">
                            {approval.command ||
                              approval.toolName ||
                              approval.reason ||
                              approval.id}
                          </div>
                          {hostLeaseBlocked ? (
                            <p className="mt-2 text-xs text-amber-900">
                              HostLease 阻塞，需先处理锁冲突后才能批准 Workflow
                              执行。
                            </p>
                          ) : null}
                          <div className="mt-2 flex gap-2">
                            <Button
                              variant="outline"
                              disabled={busyApproval === approval.id}
                              onClick={() =>
                                void decide(approval.id, "rejected")
                              }
                            >
                              <X />
                              拒绝
                            </Button>
                            <Button
                              disabled={
                                busyApproval === approval.id || hostLeaseBlocked
                              }
                              title={
                                hostLeaseBlocked
                                  ? "HostLease 阻塞，不能批准执行"
                                  : undefined
                              }
                              onClick={() =>
                                void decide(approval.id, "approved")
                              }
                            >
                              <Check />
                              批准
                            </Button>
                          </div>
                        </div>
                      ))}
                    </div>
                  ) : (
                    <EmptyPanel
                      title="暂无待审批动作"
                      description="当前事故没有阻塞审批。"
                    />
                  )}
                </CardContent>
              </Card>
            </div>
            <details
              className="rounded-lg border bg-white p-4"
              data-testid="incident-context-drawer"
              open
            >
              <summary className="cursor-pointer font-medium">
                上下文面板
              </summary>
              <aside className="mt-4 grid gap-4">
                <Card className="rounded-lg bg-slate-50">
                  <CardHeader>
                    <CardTitle>ERP 图谱邻域</CardTitle>
                  </CardHeader>
                  <CardContent>
                    {neighbors.length ? (
                      <Timeline items={neighbors} />
                    ) : (
                      <p className="text-sm text-slate-500">暂无邻域</p>
                    )}
                  </CardContent>
                </Card>
                <Card className="rounded-lg bg-slate-50">
                  <CardHeader>
                    <CardTitle>业务影响</CardTitle>
                  </CardHeader>
                  <CardContent>
                    <KeyValueList
                      items={[
                        {
                          label: "能力",
                          value: asArray(impact.capabilities).length,
                        },
                        {
                          label: "租户",
                          value: asArray(impact.tenants).length,
                        },
                        { label: "实体", value: entityId },
                      ]}
                    />
                  </CardContent>
                </Card>
                <Card className="rounded-lg bg-slate-50">
                  <CardHeader>
                    <CardTitle>HostLease</CardTitle>
                  </CardHeader>
                  <CardContent>
                    <Timeline items={hostLeases} empty="暂无 HostLease" />
                  </CardContent>
                </Card>
                <Card className="rounded-lg bg-slate-50">
                  <CardHeader>
                    <CardTitle>经验候选</CardTitle>
                  </CardHeader>
                  <CardContent>
                    <Timeline
                      items={experienceCandidates}
                      empty="暂无经验候选"
                    />
                  </CardContent>
                </Card>
              </aside>
            </details>
          </div>
        </>
      )}
    </ComplexPageFrame>
  );
}

function CaseHeader({
  caseView,
  incident,
}: {
  caseView: CaseViewModel;
  incident: IncidentRecord | null;
}) {
  const rawSeverity = incident?.severity || incident?.sev || caseView.severity;
  const hostIds = Array.from(
    new Set([
      ...caseView.hostProfiles
        .map((profile) => profile.hostId || profile.displayName)
        .filter(Boolean),
      ...caseView.hostLeases.map((lease) => lease.hostId).filter(Boolean),
    ]),
  );

  return (
    <Card data-testid="case-header" className="rounded-lg bg-white">
      <CardHeader>
        <CardTitle>Case 总览</CardTitle>
        <CardDescription>{caseView.title}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-3 text-sm md:grid-cols-2 xl:grid-cols-4">
        <HeaderField
          label="状态"
          value={<RiskBadge value={caseView.statusLabel} />}
        />
        <HeaderField
          label="风险"
          value={<RiskBadge value={rawSeverity || caseView.severityLabel} />}
        />
        <HeaderField label="来源" value={caseSourceLabel(caseView.source)} />
        <HeaderField
          label="环境"
          value={
            caseView.environment ||
            incident?.environment ||
            incident?.env ||
            "待定"
          }
        />
        <HeaderField
          label="业务能力"
          value={
            caseView.businessCapability ||
            incident?.businessCapability ||
            incident?.capability ||
            "待定"
          }
        />
        <HeaderField
          label="服务"
          value={caseView.service || incident?.entityId || "-"}
        />
        <HeaderField
          label="主机"
          value={hostIds.length ? hostIds.join(", ") : "-"}
        />
        <HeaderField label="Case ID" value={caseView.id} />
      </CardContent>
    </Card>
  );
}

function HeaderField({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="rounded-lg border border-slate-100 bg-slate-50 p-3">
      <div className="text-xs font-medium text-slate-500">{label}</div>
      <div className="mt-1 break-words font-medium text-slate-900">
        {value || "-"}
      </div>
    </div>
  );
}

function CaseStageTabs({ tabs }: { tabs: CaseTabView[] }) {
  return (
    <Tabs defaultValue="overview" className="rounded-lg border bg-white p-3">
      <TabsList
        data-testid="case-stage-tabs"
        className="h-auto flex-wrap justify-start"
      >
        {tabs.map((tab) => (
          <TabsTrigger key={tab.key} value={tab.key} className="px-3">
            {tab.count ? `${tab.label} ${tab.count}` : tab.label}
          </TabsTrigger>
        ))}
      </TabsList>
    </Tabs>
  );
}

function caseSourceLabel(source: string) {
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

function timelineRecords(items: unknown[] = []) {
  return items as Record<string, unknown>[];
}

function Timeline({
  items,
  empty = "暂无记录",
}: {
  items: Record<string, unknown>[];
  empty?: string;
}) {
  if (!items.length) return <p className="text-sm text-slate-500">{empty}</p>;
  return (
    <ul className="grid gap-2 text-sm">
      {items.map((item, index) => {
        const meta = Array.from(
          new Set(
            [
              item.evidenceRef,
              item.artifactId,
              item.traceId,
              item.hostId,
              item.leaseId,
              item.workflowId,
              item.packId,
              item.label,
            ]
              .map((value) => compactText(value))
              .filter(Boolean),
          ),
        ).join(" · ");
        return (
          <li
            key={
              compactText(
                item.id ||
                  item.title ||
                  item.name ||
                  item.evidenceRef ||
                  item.leaseId ||
                  item.runId,
              ) || index
            }
            className="rounded-lg border bg-white p-3"
          >
            <div className="font-medium">
              {compactText(
                item.title ||
                  item.name ||
                  item.displayName ||
                  item.id ||
                  item.runId,
              ) || `记录 ${index + 1}`}
            </div>
            <div className="mt-1 text-xs leading-5 text-slate-500">
              {compactText(
                item.summary ||
                  item.description ||
                  item.detail ||
                  item.message ||
                  item.status ||
                  item.impact,
              ) || "-"}
            </div>
            {meta ? (
              <div className="mt-1 break-all text-xs text-slate-400">
                {meta}
              </div>
            ) : null}
          </li>
        );
      })}
    </ul>
  );
}
