import { normalizeCase, type CaseView } from "../../api/cases";
import { zhCaseStatusLabel, zhRiskLevelLabel } from "../../lib/zhLabels";

type BlockingTone = "danger" | "warning" | "neutral";

export type CaseBlockingItem = {
  key:
    | "missing_evidence"
    | "waiting_confirmation"
    | "host_lease_blocked"
    | "workflow_failed"
    | "verification_failed";
  label: string;
  tone: BlockingTone;
  message: string;
};

export type CaseTabView = {
  key: "overview" | "evidence" | "host_profiles" | "execution" | "verification" | "experience" | "timeline";
  label: string;
  count: number;
};

export type CaseViewModel = CaseView & {
  statusLabel: string;
  severityLabel: string;
  evidenceRefs: string[];
  blockingItems: CaseBlockingItem[];
  tabs: CaseTabView[];
  promptTraceHref: string;
};

function normalizeStatus(value: string) {
  return String(value || "").trim().toLowerCase();
}

function addBlockingItem(items: CaseBlockingItem[], item: CaseBlockingItem) {
  if (!items.some((existing) => existing.key === item.key)) items.push(item);
}

function isLeaseBlocked(status: string) {
  return ["conflict", "denied", "expired", "unhealthy", "failed"].includes(normalizeStatus(status));
}

function isFailed(status: string) {
  return ["failed", "error", "rejected", "cancelled", "canceled"].includes(normalizeStatus(status));
}

function isWaitingForConfirmation(caseView: CaseView) {
  const status = normalizeStatus(caseView.status);
  return status === "waiting_confirmation" || caseView.pendingActions.some((action) => normalizeStatus(action.status) === "pending");
}

export function getCaseEvidenceRefs(input: unknown): string[] {
  const caseView = normalizeCase(input);
  return Array.from(
    new Set(
      caseView.evidence
        .map((item) => item.evidenceRef || item.id || item.artifactId)
        .map((value) => value.trim())
        .filter(Boolean),
    ),
  );
}

export function buildCaseTabs(input: unknown): CaseTabView[] {
  const caseView = normalizeCase(input);
  return [
    { key: "overview", label: "概览", count: 0 },
    { key: "evidence", label: "证据", count: caseView.evidence.length },
    { key: "host_profiles", label: "主机环境", count: caseView.hostProfiles.length },
    { key: "execution", label: "执行", count: caseView.workflowRuns.length },
    { key: "verification", label: "验证", count: caseView.verifications.length },
    { key: "experience", label: "经验", count: caseView.experienceCandidates.length },
    { key: "timeline", label: "时间线", count: caseView.timeline.length },
  ];
}

export function buildCaseBlockingItems(input: unknown): CaseBlockingItem[] {
  const caseView = normalizeCase(input);
  const items: CaseBlockingItem[] = [];

  if (!caseView.evidence.length) {
    addBlockingItem(items, {
      key: "missing_evidence",
      label: "证据不足",
      tone: "warning",
      message: "Case 还没有可追溯的 EvidenceRef，不能形成完整闭环。",
    });
  }

  if (isWaitingForConfirmation(caseView)) {
    addBlockingItem(items, {
      key: "waiting_confirmation",
      label: "等待确认",
      tone: "warning",
      message: "存在需要用户确认的动作，前端不能直接绕过确认执行。",
    });
  }

  if (caseView.hostLeases.some((lease) => isLeaseBlocked(lease.status))) {
    addBlockingItem(items, {
      key: "host_lease_blocked",
      label: "HostLease 阻塞",
      tone: "danger",
      message: "目标主机租约存在冲突、拒绝或过期状态，需要先处理锁冲突。",
    });
  }

  if (caseView.workflowRuns.some((run) => isFailed(run.status))) {
    addBlockingItem(items, {
      key: "workflow_failed",
      label: "Workflow 失败",
      tone: "danger",
      message: "至少一个 Runner Workflow 执行失败，需要查看失败步骤和回滚结果。",
    });
  }

  if (caseView.verifications.some((verification) => isFailed(verification.status))) {
    addBlockingItem(items, {
      key: "verification_failed",
      label: "验证失败",
      tone: "danger",
      message: "修复后的验证未通过，不能将 Case 标记为恢复。",
    });
  }

  return items;
}

export function buildCaseViewModel(input: unknown): CaseViewModel {
  const caseView = normalizeCase(input);
  return {
    ...caseView,
    statusLabel: zhCaseStatusLabel(caseView.status),
    severityLabel: zhRiskLevelLabel(caseView.severity),
    evidenceRefs: getCaseEvidenceRefs(caseView),
    blockingItems: buildCaseBlockingItems(caseView),
    tabs: buildCaseTabs(caseView),
    promptTraceHref: `/debug/prompts?case_id=${encodeURIComponent(caseView.id)}`,
  };
}
