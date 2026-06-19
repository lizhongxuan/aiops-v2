import { useEffect, useMemo, useState } from "react";
import {
  AlertCircleIcon,
  BotIcon,
  CheckIcon,
  CircleDashedIcon,
  TerminalIcon,
  UserIcon,
  WrenchIcon,
  XIcon,
  type LucideIcon,
} from "lucide-react";

import {
  getChildAgentTranscript,
  submitHostOpsApprovalDecision,
  type HostChildAgentTranscript,
  type HostOpsTranscriptItem,
} from "@/api/hostOps";
import { Button } from "@/components/ui/button";
import { Sheet, SheetClose, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import type {
  AiopsHostAgentEvidenceTrace,
  AiopsHostAgentPromptSection,
  AiopsHostAgentRuntimeProfile,
  AiopsHostAgentToolTrace,
  AiopsHostAgentTraceEntry,
  AiopsTransportChildAgent,
  AiopsTransportState,
} from "@/transport/aiopsTransportTypes";

import { HostSubagentTabs, type HostSubagentTabId } from "./HostSubagentTabs";

type LoadState =
  | { status: "idle"; transcript: null; error: "" }
  | { status: "loading"; transcript: null; error: "" }
  | { status: "loaded"; transcript: HostChildAgentTranscript; error: "" }
  | { status: "error"; transcript: null; error: string };

type HostSubagentDrawerProps = {
  open: boolean;
  childAgentId?: string;
  childAgent?: AiopsTransportChildAgent;
  state?: DrawerTransportState;
  onOpenChange?: (open: boolean) => void;
  loadTranscript?: (childAgentId: string) => Promise<HostChildAgentTranscript>;
  submitApprovalDecision?: (approvalId: string, decision: string) => Promise<unknown>;
};

type DrawerTransportState = AiopsTransportState & {
  childAgentTranscripts?: Record<string, Array<Partial<HostOpsTranscriptItem>>>;
};

export function HostSubagentDrawer({
  open,
  childAgentId: childAgentIdProp,
  childAgent,
  state,
  onOpenChange = () => undefined,
  loadTranscript = getChildAgentTranscript,
  submitApprovalDecision = submitHostOpsApprovalDecision,
}: HostSubagentDrawerProps) {
  const selectedChildAgent = childAgent || (childAgentIdProp ? state?.childAgents?.[childAgentIdProp] : undefined);
  const childAgentId = selectedChildAgent?.id ?? childAgentIdProp ?? "";
  const stateTranscript = useMemo(() => transcriptFromState(state, childAgentId), [childAgentId, state]);
  const [loadState, setLoadState] = useState<LoadState>({ status: "idle", transcript: null, error: "" });
  const [activeTab, setActiveTab] = useState<HostSubagentTabId>(() => defaultTabForChildAgent(selectedChildAgent));

  useEffect(() => {
    setActiveTab(defaultTabForChildAgent(selectedChildAgent));
  }, [childAgentId, selectedChildAgent?.status]);

  useEffect(() => {
    if (!open || !childAgentId) {
      setLoadState({ status: "idle", transcript: null, error: "" });
      return;
    }
    if (stateTranscript) {
      setLoadState({ status: "loaded", transcript: stateTranscript, error: "" });
      return;
    }

    let cancelled = false;
    setLoadState({ status: "loading", transcript: null, error: "" });

    loadTranscript(childAgentId)
      .then((transcript) => {
        if (!cancelled) {
          setLoadState({ status: "loaded", transcript, error: "" });
        }
      })
      .catch((error: unknown) => {
        if (!cancelled) {
          setLoadState({
            status: "error",
            transcript: null,
            error: error instanceof Error ? error.message : String(error),
          });
        }
      });

    return () => {
      cancelled = true;
    };
  }, [childAgentId, loadTranscript, open, stateTranscript]);

  const subtitle = useMemo(() => formatDrawerSubtitle(selectedChildAgent), [selectedChildAgent]);

  return (
    <Sheet open={open && Boolean(selectedChildAgent)} onOpenChange={onOpenChange}>
        <SheetContent
          showCloseButton={false}
          overlayClassName="left-0 lg:left-[var(--aiops-shell-sidebar-width,18rem)]"
          className="!top-[var(--aiops-shell-header-height,3.5rem)] !bottom-0 !h-[calc(100dvh-var(--aiops-shell-header-height,3.5rem))] !w-[min(640px,calc(100vw-24px))] !max-w-none gap-0 overflow-hidden sm:!max-w-[640px] lg:!w-[min(640px,calc(100vw-var(--aiops-shell-sidebar-width,18rem)-24px))]"
        >
        <SheetHeader className="border-b border-zinc-200 px-4 py-3">
          <div className="flex min-w-0 items-start gap-3">
            <div className="flex size-8 shrink-0 items-center justify-center rounded-md border border-zinc-200 bg-zinc-50 text-zinc-700">
              <BotIcon className="size-4" aria-hidden="true" />
            </div>
            <div className="min-w-0 flex-1">
              <SheetTitle className="truncate">主机 Agent 详情</SheetTitle>
              <SheetDescription className="truncate">{subtitle}</SheetDescription>
            </div>
            <SheetClose asChild>
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                aria-label="关闭子 agent 对话"
                data-testid="host-subagent-drawer-close"
              >
                <XIcon className="size-4" aria-hidden="true" />
              </Button>
            </SheetClose>
          </div>
        </SheetHeader>

        <HostSubagentTabs activeTab={activeTab} onTabChange={setActiveTab} />

        <div className="min-h-0 flex-1 overflow-y-auto px-4 py-3" data-testid="host-subagent-drawer">
          {activeTab === "task" ? (
            <ChildAgentTaskPanel childAgent={selectedChildAgent} />
          ) : activeTab === "prompt" ? (
            <PromptTracePanel
              trace={mergeTrace(selectedChildAgent, loadState)}
              promptItems={selectPromptTraceItems(loadState)}
            />
          ) : activeTab === "tools" ? (
            <ToolsTracePanel
              trace={mergeTrace(selectedChildAgent, loadState)}
              transcriptItems={selectTranscriptItems(loadState, activeTab)}
              allTranscriptItems={allTranscriptItems(loadState)}
              childAgent={selectedChildAgent}
            />
          ) : activeTab === "mcp-skills" ? (
            <McpSkillsTracePanel trace={mergeTrace(selectedChildAgent, loadState)} />
          ) : activeTab === "evidence" ? (
            <EvidenceTracePanel
              trace={mergeTrace(selectedChildAgent, loadState)}
              transcriptItems={allTranscriptItems(loadState)}
              childAgent={selectedChildAgent}
            />
          ) : activeTab === "receipts" ? (
            <ReceiptsPanel
              trace={mergeTrace(selectedChildAgent, loadState)}
              loadState={loadState}
              items={selectTranscriptItems(loadState, activeTab)}
              childAgentError={selectedChildAgent?.error}
              childAgent={selectedChildAgent}
              submitApprovalDecision={submitApprovalDecision}
            />
          ) : (
            <TranscriptBody
              loadState={loadState}
              items={selectTranscriptItems(loadState, activeTab)}
              emptyLabel={emptyLabelForTab(activeTab)}
              childAgent={selectedChildAgent}
              childAgentError={activeTab === "receipts" ? selectedChildAgent?.error : undefined}
              submitApprovalDecision={submitApprovalDecision}
            />
          )}
        </div>
      </SheetContent>
    </Sheet>
  );
}

type HostAgentTraceView = {
  runtimeProfile?: AiopsHostAgentRuntimeProfile;
  contextDecisions?: AiopsHostAgentTraceEntry[];
  promptSections?: AiopsHostAgentPromptSection[];
  toolSurfaceSnapshot?: AiopsHostAgentToolTrace[];
  mcpInstructionDeltas?: AiopsHostAgentTraceEntry[];
  skillActivationTrace?: AiopsHostAgentTraceEntry[];
  approvalTrace?: AiopsHostAgentTraceEntry[];
  evidenceTrace?: AiopsHostAgentEvidenceTrace[];
  reportTimeline?: AiopsHostAgentTraceEntry[];
  agentMessages?: AiopsHostAgentTraceEntry[];
};

function ChildAgentTaskPanel({
  childAgent,
}: {
  childAgent?: AiopsTransportChildAgent;
}) {
  if (!childAgent) {
    return null;
  }
  const hostName = childAgent.hostDisplayName || childAgent.hostAddress || childAgent.hostId || "未知主机";
  const hostAddress = childAgent.hostAddress || childAgent.hostId || "";

  return (
    <div className="grid gap-2 text-xs text-zinc-600">
      <section className="min-w-0 overflow-hidden rounded-md border border-zinc-200 bg-zinc-50 px-3 py-3">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <span className="size-1.5 shrink-0 rounded-full bg-emerald-500" aria-hidden="true" />
          <span className="min-w-0 flex-1 break-words text-sm font-medium text-zinc-900">{hostName}</span>
          <span className="shrink-0 rounded bg-zinc-100 px-1.5 py-0.5 text-zinc-600">{formatStatus(childAgent.status)}</span>
        </div>
        <dl className="mt-3 grid min-w-0 gap-2">
          {hostAddress ? <OverviewRow label="主机" value={`@${stripAtPrefix(hostAddress)}`} /> : null}
          {childAgent.task ? <OverviewRow label="当前任务" value={childAgent.task} /> : null}
          {childAgent.subtaskStatus ? <OverviewRow label="子任务状态" value={childAgent.subtaskStatus} /> : null}
          {childAgent.queueReason ? <OverviewRow label="排队原因" value={childAgent.queueReason} /> : null}
          {childAgent.source ? <OverviewRow label="来源" value={childAgent.source} /> : null}
          {childAgent.lastInputPreview ? <OverviewRow label="最近输入" value={childAgent.lastInputPreview} /> : null}
          {childAgent.lastOutputPreview ? <OverviewRow label="最近输出" value={childAgent.lastOutputPreview} /> : null}
          {childAgent.error ? <OverviewRow label="错误" value={childAgent.error} tone="danger" /> : null}
        </dl>
      </section>
    </div>
  );
}

function OverviewRow({ label, value, tone = "default" }: { label: string; value: string; tone?: "default" | "danger" }) {
  return (
    <div className="grid min-w-0 grid-cols-[4.5rem_minmax(0,1fr)] gap-2">
      <dt className="shrink-0 text-zinc-400">{label}</dt>
      <dd className={tone === "danger" ? "min-w-0 whitespace-pre-wrap break-words text-red-600" : "min-w-0 whitespace-pre-wrap break-words text-zinc-700"}>
        {value}
      </dd>
    </div>
  );
}

function PromptTracePanel({
  trace,
  promptItems,
}: {
  trace: HostAgentTraceView;
  promptItems: HostOpsTranscriptItem[];
}) {
  const sections = trace.promptSections || [];
  const contextDecisions = trace.contextDecisions || [];
  const hasTrace = promptItems.length > 0 || sections.length > 0 || contextDecisions.length > 0;
  return (
    <div className="grid gap-2">
      <RuntimeProfileView profile={trace.runtimeProfile} />
      {promptItems.length ? (
        <PromptTraceFileList items={promptItems} />
      ) : null}
      {sections.length ? (
        <TraceList
          emptyLabel=""
          entries={sections}
          renderTitle={(entry) => promptCategoryLabel(entry.category) || entry.title || entry.sectionId || entry.id || "Prompt section"}
          fields={["sectionId", "retentionRank", "retentionClass", "compactAction", "compactSchema", "sourceRef", "redaction", "hash", "ref"]}
        />
      ) : null}
      {contextDecisions.length ? (
        <TraceList
          title="Context decisions"
          emptyLabel=""
          entries={contextDecisions}
          fields={["sectionId", "decision", "retentionRank", "compactAction", "sourceRef", "redaction", "hash", "ref"]}
        />
      ) : null}
      {!hasTrace ? (
        <div className="rounded-md border border-dashed border-zinc-300 px-3 py-6 text-center text-sm text-zinc-500">
          暂无 Prompt trace
        </div>
      ) : null}
    </div>
  );
}

function PromptTraceFileList({ items }: { items: HostOpsTranscriptItem[] }) {
  return (
    <section className="grid gap-2" data-testid="host-subagent-prompt-files">
      <div className="text-xs font-medium text-zinc-700">Prompt MD 文件</div>
      {items.map((item) => {
        const traceFile = promptTraceFileForItem(item);
        const visibleTools = promptVisibleToolsForItem(item);
        const round = promptModelCallTitle(item);
        return (
          <article key={item.id} className="rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm">
            <div className="flex min-w-0 flex-wrap items-center gap-2">
              <BotIcon className="size-3.5 shrink-0 text-zinc-500" aria-hidden="true" />
              <span className="font-medium text-zinc-900">{round || "调用 LLM"}</span>
              {item.status ? <span className="ml-auto shrink-0 text-xs text-zinc-500">{formatStatus(item.status)}</span> : null}
            </div>
            {visibleTools.length ? (
              <div className="mt-1 break-words text-xs text-zinc-600">可用工具：{visibleTools.join(", ")}</div>
            ) : null}
            {traceFile ? <PromptMdLink itemId={item.id} traceFile={traceFile} /> : null}
          </article>
        );
      })}
    </section>
  );
}

function ToolsTracePanel({
  trace,
  transcriptItems,
  allTranscriptItems,
  childAgent,
}: {
  trace: HostAgentTraceView;
  transcriptItems: HostOpsTranscriptItem[];
  allTranscriptItems: HostOpsTranscriptItem[];
  childAgent?: AiopsTransportChildAgent;
}) {
  const toolEntries = trace.toolSurfaceSnapshot || [];
  const derivedEntries = toolEntries.length || transcriptItems.length ? [] : deriveToolTraceFromTranscript(allTranscriptItems);
  const displayEntries = toolEntries.length ? toolEntries : derivedEntries;
  return (
    <div className="grid gap-2">
      <TraceList
        emptyLabel={transcriptItems.length || displayEntries.length ? "" : "暂无工具 trace"}
        entries={displayEntries}
        renderTitle={(entry) => entry.name || entry.toolName || entry.id || "Tool"}
        fields={["source", "status", "summary", "sourceRef", "redaction", "hash", "ref"]}
      />
      {transcriptItems.length ? (
        <TranscriptBody
          loadState={{ status: "loaded", transcript: { childAgentId: "", items: transcriptItems }, error: "" }}
          items={transcriptItems}
          emptyLabel="暂无工具记录"
          childAgent={childAgent}
          submitApprovalDecision={async () => undefined}
        />
      ) : null}
    </div>
  );
}

function McpSkillsTracePanel({ trace }: { trace: HostAgentTraceView }) {
  return (
    <div className="grid gap-2">
      <TraceList
        title="MCP instruction delta"
        emptyLabel="暂无 MCP instruction delta"
        entries={trace.mcpInstructionDeltas || []}
        fields={["server", "status", "summary", "sourceRef", "redaction", "hash", "ref"]}
      />
      <TraceList
        title="Skill activation"
        emptyLabel="暂无 skill activation"
        entries={trace.skillActivationTrace || []}
        fields={["skill", "status", "summary", "sourceRef", "redaction", "hash", "ref"]}
      />
    </div>
  );
}

function EvidenceTracePanel({
  trace,
  transcriptItems,
  childAgent,
}: {
  trace: HostAgentTraceView;
  transcriptItems: HostOpsTranscriptItem[];
  childAgent?: AiopsTransportChildAgent;
}) {
  const entries = trace.evidenceTrace?.length ? trace.evidenceTrace : deriveEvidenceTraceFromTranscript(transcriptItems, childAgent);
  return (
    <TraceList
      emptyLabel="暂无证据 trace"
      entries={entries}
      fields={["title", "source", "artifactRef", "evidenceRef", "hash", "sourceRef", "redaction", "ref"]}
    />
  );
}

function ReceiptsPanel({
  trace,
  loadState,
  items,
  childAgentError,
  childAgent,
  submitApprovalDecision,
}: {
  trace: HostAgentTraceView;
  loadState: LoadState;
  items: HostOpsTranscriptItem[];
  childAgentError?: string;
  childAgent?: AiopsTransportChildAgent;
  submitApprovalDecision: (approvalId: string, decision: string) => Promise<unknown>;
}) {
  const reportTimeline = trace.reportTimeline || [];
  if (reportTimeline.length) {
    return (
      <div className="grid gap-2">
        <TraceList
          title="Report timeline"
          emptyLabel=""
          entries={reportTimeline}
          fields={["event", "status", "source", "sourceRef", "redaction", "hash", "ref"]}
        />
        <TranscriptBody
          loadState={loadState}
          items={items}
          emptyLabel="暂无回执或错误"
          childAgent={childAgent}
          childAgentError={childAgentError}
          submitApprovalDecision={submitApprovalDecision}
        />
      </div>
    );
  }
  return (
    <TranscriptBody
      loadState={loadState}
      items={items}
      emptyLabel="暂无回执或错误"
      childAgent={childAgent}
      childAgentError={childAgentError}
      submitApprovalDecision={submitApprovalDecision}
    />
  );
}

function RuntimeProfileView({ profile }: { profile?: AiopsHostAgentRuntimeProfile }) {
  if (!profile) {
    return null;
  }
  return (
    <div className="rounded-md border border-zinc-200 bg-zinc-50 px-3 py-2 text-xs text-zinc-600">
      <div className="font-medium text-zinc-800">Runtime profile</div>
      {profile.id ? <div className="mt-1">id：{profile.id}</div> : null}
      {Array.isArray(profile.capabilities) && profile.capabilities.length ? (
        <div className="mt-1 break-words">capabilities：{profile.capabilities.join(", ")}</div>
      ) : null}
    </div>
  );
}

function TraceList({
  title,
  entries,
  emptyLabel,
  fields,
  renderTitle,
}: {
  title?: string;
  entries: AiopsHostAgentTraceEntry[];
  emptyLabel: string;
  fields: string[];
  renderTitle?: (entry: AiopsHostAgentTraceEntry) => string;
}) {
  if (!entries.length) {
    return emptyLabel ? (
      <div className="rounded-md border border-dashed border-zinc-300 px-3 py-6 text-center text-sm text-zinc-500">
        {emptyLabel}
      </div>
    ) : null;
  }
  return (
    <section className="grid gap-2">
      {title ? <div className="text-xs font-medium text-zinc-700">{title}</div> : null}
      {entries.map((entry, index) => (
        <article key={String(entry.id || index)} className="rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm">
          <div className="font-medium text-zinc-900">{renderTitle?.(entry) || entry.title || entry.event || entry.id || "Trace"}</div>
          <div className="mt-1 grid gap-1 text-xs text-zinc-600">
            {fields.map((field) => {
              const value = traceFieldValue(entry, field);
              return value ? (
                <div key={field} className="break-words">
                  <span className="text-zinc-400">{field}：</span>
                  {value}
                </div>
              ) : null;
            })}
          </div>
        </article>
      ))}
    </section>
  );
}

function TranscriptBody({
  loadState,
  items,
  emptyLabel,
  childAgent,
  childAgentError,
  submitApprovalDecision,
}: {
  loadState: LoadState;
  items: HostOpsTranscriptItem[];
  emptyLabel: string;
  childAgent?: AiopsTransportChildAgent;
  childAgentError?: string;
  submitApprovalDecision: (approvalId: string, decision: string) => Promise<unknown>;
}) {
  if (loadState.status === "loading") {
    return (
      <div className="flex items-center gap-2 rounded-md border border-zinc-200 px-3 py-3 text-sm text-zinc-600">
        <CircleDashedIcon className="size-4 animate-spin" aria-hidden="true" />
        正在读取子 agent 对话
      </div>
    );
  }

  if (loadState.status === "error") {
    return (
      <div className="rounded-md border border-red-200 bg-red-50 px-3 py-3 text-sm text-red-700">
        <div className="flex items-center gap-2 font-medium">
          <AlertCircleIcon className="size-4" aria-hidden="true" />
          读取 transcript 失败
        </div>
        <div className="mt-1 break-words text-red-600">{loadState.error || "未知错误"}</div>
      </div>
    );
  }

  if (loadState.status !== "loaded") {
    return null;
  }

  if (items.length === 0 && !childAgentError) {
    return (
      <div className="rounded-md border border-dashed border-zinc-300 px-3 py-6 text-center text-sm text-zinc-500">
        {emptyLabel}
      </div>
    );
  }

  return (
    <div className="grid gap-2">
      {childAgentError ? (
        <div className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
          {childAgentError}
        </div>
      ) : null}
      {items.map((item) => (
        <TranscriptItemView key={item.id} item={item} childAgent={childAgent} submitApprovalDecision={submitApprovalDecision} />
      ))}
    </div>
  );
}

function TranscriptItemView({
  item,
  childAgent,
  submitApprovalDecision,
}: {
  item: HostOpsTranscriptItem;
  childAgent?: AiopsTransportChildAgent;
  submitApprovalDecision: (approvalId: string, decision: string) => Promise<unknown>;
}) {
  const meta = itemMeta(item.type, childAgent);
  const Icon = meta.icon;
  const isTool = item.type === "tool_call" || item.type === "tool_result";
  const traceFile = promptTraceFileForItem(item);
  const visibleTools = promptVisibleToolsForItem(item);
  const displayContent = item.type === "llm_request" ? cleanModelCallContent(item.content || "") : item.content || "";
  const [decisionState, setDecisionState] = useState<"idle" | "submitting" | "submitted" | "error">("idle");
  const [decisionError, setDecisionError] = useState("");
  const approvalID = item.type === "approval" ? item.approvalId || stringFromPayload(item.payload, "approvalId") : "";
  const pendingApproval = item.type === "approval" && item.status === "pending" && approvalID !== "";

  const decide = async (decision: "accept" | "reject") => {
    if (!approvalID || decisionState === "submitting") {
      return;
    }
    setDecisionState("submitting");
    setDecisionError("");
    try {
      await submitApprovalDecision(approvalID, decision);
      setDecisionState("submitted");
    } catch (error) {
      setDecisionState("error");
      setDecisionError(error instanceof Error ? error.message : String(error));
    }
  };

  return (
    <article
      className="rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-800"
      data-testid={`host-subagent-transcript-item-${item.id}`}
    >
      <div className="mb-1 flex min-w-0 items-center gap-2 text-xs text-zinc-500">
        <Icon className="size-3.5 shrink-0" aria-hidden="true" />
        <span className="shrink-0 font-medium text-zinc-700">{meta.label}</span>
        {item.toolName ? <span className="min-w-0 truncate">· {item.toolName}</span> : null}
        {item.status ? <span className="ml-auto shrink-0">{formatStatus(item.status)}</span> : null}
      </div>
      {displayContent ? (
        <div
          className={
            isTool
              ? "whitespace-pre-wrap break-words rounded bg-zinc-50 px-2 py-1 font-mono text-xs text-zinc-800"
              : "whitespace-pre-wrap break-words leading-6"
          }
        >
          {displayContent}
        </div>
      ) : (
        <div className="text-xs text-zinc-400">无内容</div>
      )}
      {item.type === "llm_request" && visibleTools.length ? (
        <div className="mt-1 break-words text-xs text-zinc-600">可用工具：{visibleTools.join(", ")}</div>
      ) : null}
      {item.type === "llm_request" && traceFile ? <PromptMdLink itemId={item.id} traceFile={traceFile} /> : null}
      {item.createdAt ? <div className="mt-1 text-[11px] text-zinc-400">{formatTimestamp(item.createdAt)}</div> : null}
      {pendingApproval ? (
        <div className="mt-2 flex flex-wrap items-center gap-2 border-t border-zinc-100 pt-2">
          <Button
            type="button"
            size="sm"
            disabled={decisionState === "submitting" || decisionState === "submitted"}
            data-testid={`host-subagent-approval-approve-${safeTestId(approvalID)}`}
            onClick={() => void decide("accept")}
          >
            <CheckIcon className="size-3.5" aria-hidden="true" />
            批准执行
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={decisionState === "submitting" || decisionState === "submitted"}
            data-testid={`host-subagent-approval-reject-${safeTestId(approvalID)}`}
            onClick={() => void decide("reject")}
          >
            <XIcon className="size-3.5" aria-hidden="true" />
            拒绝
          </Button>
          {decisionState === "submitting" ? <span className="text-xs text-zinc-500">正在提交审批</span> : null}
          {decisionState === "submitted" ? <span className="text-xs text-emerald-700">审批请求已提交</span> : null}
          {decisionState === "error" ? <span className="text-xs text-red-600">{decisionError || "审批提交失败"}</span> : null}
        </div>
      ) : null}
    </article>
  );
}

function stringFromPayload(payload: Record<string, unknown> | undefined, key: string) {
  const value = payload?.[key];
  return typeof value === "string" ? value : "";
}

function stringArrayFromPayload(payload: Record<string, unknown> | undefined, key: string): string[] {
  const value = payload?.[key];
  if (Array.isArray(value)) {
    return value.map((item) => String(item).trim()).filter(Boolean);
  }
  if (typeof value === "string") {
    return splitVisibleTools(value);
  }
  return [];
}

function promptTraceFileForItem(item: HostOpsTranscriptItem) {
  if (item.type !== "llm_request") {
    return "";
  }
  return stringFromPayload(item.payload, "traceFile") || traceFileFromContent(item.content || "");
}

function promptVisibleToolsForItem(item: HostOpsTranscriptItem) {
  if (item.type !== "llm_request") {
    return [];
  }
  return stringArrayFromPayload(item.payload, "visibleTools").length
    ? stringArrayFromPayload(item.payload, "visibleTools")
    : visibleToolsFromContent(item.content || "");
}

function promptModelCallTitle(item: HostOpsTranscriptItem) {
  return cleanModelCallContent(item.content || "").split("\n").map((line) => line.trim()).find(Boolean) || "";
}

function traceFileFromContent(content: string) {
  const match = content.match(/^\s*trace:\s*(.+)$/im);
  return match?.[1]?.trim() || "";
}

function visibleToolsFromContent(content: string) {
  const match = content.match(/^\s*visibleTools:\s*(.+)$/im);
  return splitVisibleTools(match?.[1] || "");
}

function splitVisibleTools(value: string) {
  return value.split(",").map((item) => item.trim()).filter(Boolean);
}

function cleanModelCallContent(content: string) {
  return content
    .split("\n")
    .filter((line) => !/^\s*(trace|visibleTools):\s*/i.test(line))
    .join("\n")
    .trim();
}

function PromptMdLink({ itemId, traceFile }: { itemId: string; traceFile: string }) {
  const tracePath = normalizeModelInputTracePath(traceFile);
  return (
    <a
      className="mt-2 inline-flex w-fit items-center rounded-md border border-zinc-200 bg-zinc-50 px-2 py-1 text-xs font-medium text-zinc-700 hover:bg-zinc-100"
      data-testid={`host-subagent-prompt-md-link-${safeTestId(itemId)}`}
      href={`/debug/prompts?path=${encodeURIComponent(tracePath)}&view=raw&raw=markdown`}
    >
      查看 Prompt MD
    </a>
  );
}

function normalizeModelInputTracePath(value: string) {
  const text = value.trim();
  const marker = "model-input-traces/";
  const index = text.indexOf(marker);
  if (index >= 0) {
    return text.slice(index + marker.length).replace(/^\/+/, "");
  }
  return text.replace(/^\/+/, "");
}

function safeTestId(value: string) {
  return value.replace(/[^a-zA-Z0-9_-]/g, "-");
}

function itemMeta(type: string, childAgent?: AiopsTransportChildAgent): { label: string; icon: LucideIcon } {
  switch (type) {
    case "manager_message":
      return { label: "管理 Agent → 主机 Agent", icon: UserIcon };
    case "user_followup":
      return { label: "用户追问", icon: UserIcon };
    case "llm_request":
      return { label: "主机 Agent → LLM", icon: BotIcon };
    case "llm_response":
      return { label: "LLM → 主机 Agent", icon: BotIcon };
    case "assistant_message":
      return { label: formatHostAgentReturnLabel(childAgent), icon: BotIcon };
    case "tool_call":
      return { label: "工具调用", icon: TerminalIcon };
    case "tool_result":
      return { label: "工具结果", icon: WrenchIcon };
    case "approval":
      return { label: "审批", icon: AlertCircleIcon };
    case "error":
      return { label: "错误", icon: AlertCircleIcon };
    default:
      return { label: type || "消息", icon: BotIcon };
  }
}

function formatHostAgentReturnLabel(childAgent?: AiopsTransportChildAgent) {
  const hostLabel = childAgent?.hostDisplayName || childAgent?.hostAddress || childAgent?.hostId || "";
  return hostLabel ? `主机 ${stripAtPrefix(hostLabel)} 返回` : "主机 Agent 返回";
}

function formatHostLabel(childAgent?: AiopsTransportChildAgent) {
  if (!childAgent) {
    return "";
  }
  return childAgent.hostAddress || childAgent.hostId || "";
}

function formatDrawerSubtitle(childAgent?: AiopsTransportChildAgent) {
  if (!childAgent) {
    return "未知主机";
  }
  const displayName = childAgent.hostDisplayName || "未知主机";
  const hostLabel = formatHostLabel(childAgent);
  const hostHandle = hostLabel ? `@${stripAtPrefix(hostLabel)}` : "";
  const hostPart = hostHandle && !sameHostHandle(displayName, hostHandle) ? `${displayName} ${hostHandle}` : displayName;
  return childAgent.task ? `${hostPart} · ${childAgent.task}` : hostPart;
}

function sameHostHandle(left: string, right: string) {
  return stripAtPrefix(left).toLowerCase() === stripAtPrefix(right).toLowerCase();
}

function stripAtPrefix(value: string) {
  return value.trim().replace(/^@+/, "");
}

function transcriptFromState(state: DrawerTransportState | undefined, childAgentId: string): HostChildAgentTranscript | null {
  if (!state || !childAgentId) {
    return null;
  }
  const items = state.childAgentTranscripts?.[childAgentId];
  if (!Array.isArray(items)) {
    return null;
  }
  return {
    childAgentId,
    items: items.map(normalizeStateTranscriptItem),
  };
}

function normalizeStateTranscriptItem(item: Partial<HostOpsTranscriptItem>, index: number): HostOpsTranscriptItem {
  return {
    id: String(item.id || `item-${index + 1}`),
    type: item.type || "assistant_message",
    content: item.content,
    toolName: item.toolName,
    approvalId: item.approvalId,
    status: item.status,
    payload: item.payload,
    createdAt: item.createdAt,
  };
}

function formatStatus(status: string) {
  switch (status) {
    case "planned":
      return "已计划";
    case "spawning":
      return "启动中";
    case "running":
      return "运行中";
    case "waiting":
      return "等待中";
    case "approval_required":
      return "待审批";
    case "completed":
      return "已完成";
    case "failed":
      return "失败";
    case "cancelled":
      return "已取消";
    default:
      return status;
  }
}

function formatTimestamp(value: string) {
  return value.replace("T", " ").replace("Z", "");
}

function mergeTrace(childAgent: AiopsTransportChildAgent | undefined, loadState: LoadState): HostAgentTraceView {
  const transcript = loadState.status === "loaded" ? loadState.transcript : undefined;
  return {
    runtimeProfile: childAgent?.runtimeProfile || transcript?.runtimeProfile,
    contextDecisions: childAgent?.contextDecisions || transcript?.contextDecisions,
    promptSections: childAgent?.promptSections || transcript?.promptSections,
    toolSurfaceSnapshot: childAgent?.toolSurfaceSnapshot || transcript?.toolSurfaceSnapshot,
    mcpInstructionDeltas: childAgent?.mcpInstructionDeltas || transcript?.mcpInstructionDeltas,
    skillActivationTrace: childAgent?.skillActivationTrace || transcript?.skillActivationTrace,
    approvalTrace: childAgent?.approvalTrace || transcript?.approvalTrace,
    evidenceTrace: childAgent?.evidenceTrace || transcript?.evidenceTrace,
    reportTimeline: childAgent?.reportTimeline || transcript?.reportTimeline,
    agentMessages: childAgent?.agentMessages || transcript?.agentMessages,
  };
}

function deriveToolTraceFromTranscript(items: HostOpsTranscriptItem[]): AiopsHostAgentToolTrace[] {
  const commands: string[] = [];
  const seen = new Set<string>();
  for (const item of items) {
    for (const command of extractCommandHints(item.content || "")) {
      const key = command.toLowerCase();
      if (!seen.has(key)) {
        seen.add(key);
        commands.push(command);
      }
    }
  }
  return commands.map((command, index) => ({
    id: `derived-tool-${index + 1}`,
    name: "从 Agent 对话推断",
    toolName: command,
    source: "host_agent_tool",
    status: "completed",
    summary: command,
  }));
}

function extractCommandHints(content: string): string[] {
  const commands: string[] = [];
  const pushCommand = (value: string) => {
    const command = value.trim().replace(/[。.,，；;：:]+$/, "");
    if (command.length >= 2 && command.length <= 180 && looksLikeShellCommand(command)) {
      commands.push(command);
    }
  };

  for (const match of content.matchAll(/`([^`\n]+)`/g)) {
    pushCommand(match[1] || "");
  }
  for (const match of content.matchAll(/(?:只读执行|执行)\s*([^，。,；;\n]+?)(?:并|后|$|，|。|,|；|;)/g)) {
    pushCommand(match[1] || "");
  }
  return commands;
}

function looksLikeShellCommand(command: string) {
  return /^[a-zA-Z0-9_./-]+(?:\s+[^\n\r`$<>;&|]+)*$/.test(command);
}

function deriveEvidenceTraceFromTranscript(
  items: HostOpsTranscriptItem[],
  childAgent?: AiopsTransportChildAgent,
): AiopsHostAgentEvidenceTrace[] {
  const refs: AiopsHostAgentEvidenceTrace[] = [];
  const seen = new Set<string>();
  for (const item of items) {
    const content = item.content || "";
    for (const match of content.matchAll(/\bev-[a-zA-Z0-9_-]+\b/g)) {
      const evidenceRef = match[0];
      if (seen.has(evidenceRef)) {
        continue;
      }
      seen.add(evidenceRef);
      refs.push({
        id: `derived-evidence-${refs.length + 1}`,
        title: itemMeta(item.type, childAgent).label,
        source: "host_agent_report",
        evidenceRef,
        ref: evidenceRef,
      });
    }
  }
  return refs;
}

function promptCategoryLabel(category: unknown) {
  switch (category) {
    case "base_runtime":
      return "Base runtime";
    case "host_overlay":
      return "Host overlay";
    case "host_task_context":
      return "Host task context";
    case "skill_context":
      return "Skill context";
    case "mcp_context":
      return "MCP context";
    default:
      return typeof category === "string" ? category : "";
  }
}

function traceFieldValue(entry: AiopsHostAgentTraceEntry, field: string) {
  const value = entry[field];
  if (value === undefined || value === null || value === "") {
    return "";
  }
  if (Array.isArray(value)) {
    return value.map((item) => String(item)).join(", ");
  }
  if (typeof value === "object") {
    return "";
  }
  return String(value);
}

function defaultTabForChildAgent(childAgent?: AiopsTransportChildAgent): HostSubagentTabId {
  if (childAgent?.status === "approval_required") {
    return "approval";
  }
  if (childAgent?.status === "failed") {
    return "receipts";
  }
  return "conversation";
}

function selectTranscriptItems(loadState: LoadState, activeTab: HostSubagentTabId): HostOpsTranscriptItem[] {
  if (loadState.status !== "loaded") {
    return [];
  }

  return loadState.transcript.items.filter((item) => itemBelongsToTab(item, activeTab));
}

function selectPromptTraceItems(loadState: LoadState): HostOpsTranscriptItem[] {
  if (loadState.status !== "loaded") {
    return [];
  }
  return loadState.transcript.items.filter((item) => item.type === "llm_request" && promptTraceFileForItem(item));
}

function allTranscriptItems(loadState: LoadState): HostOpsTranscriptItem[] {
  return loadState.status === "loaded" ? loadState.transcript.items : [];
}

function itemBelongsToTab(item: HostOpsTranscriptItem, activeTab: HostSubagentTabId) {
  switch (activeTab) {
    case "conversation":
      return (
        item.type === "manager_message" ||
        item.type === "user_followup" ||
        item.type === "llm_request" ||
        item.type === "llm_response" ||
        item.type === "assistant_message"
      );
    case "tools":
      return item.type === "tool_call" || item.type === "tool_result";
    case "approval":
      return item.type === "approval";
    case "receipts":
      return item.type === "error" || item.type === "receipt" || item.type === "command_receipt";
    case "task":
    default:
      return false;
  }
}

function emptyLabelForTab(activeTab: HostSubagentTabId) {
  switch (activeTab) {
    case "tools":
      return "暂无工具记录";
    case "approval":
      return "暂无审核记录";
    case "receipts":
      return "暂无回执或错误";
    case "conversation":
    default:
      return "暂无独立对话记录";
  }
}
