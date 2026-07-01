import { Check, Pin, RefreshCw, Send, ShieldAlert, X } from "lucide-react";
import { useMemo, useState } from "react";

import { useAssistantTransportState } from "@assistant-ui/react";

import { AiopsComposer } from "@/chat/components/AiopsComposer";
import { AiopsThread } from "@/chat/components/AiopsThread";
import { SessionContextBar } from "@/chat/components/SessionContextBar";
import { MessageMarkdown } from "@/chat/components/MessageMarkdown";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { EmptyPanel, RiskBadge } from "@/pages/complexPageComponents";
import { resolveUiFixtureState } from "@/lib/uiFixtureRuntime";
import { ChatTransportProvider } from "@/transport/ChatTransportProvider";
import { createInitialAiopsTransportState } from "@/transport/aiopsTransportRuntime";
import { getCachedAiopsTransportState } from "@/transport/aiopsTransportStateCache";
import type {
  AiopsProcessBlock,
  AiopsTransportApproval,
  AiopsTransportMcpSurface,
  AiopsTransportState,
} from "@/transport/aiopsTransportTypes";
import { useAiopsTransportCommands } from "@/transport/useAiopsTransportCommands";

export function ProtocolWorkspacePage() {
  const fallbackState = useMemo(
    () => resolveUiFixtureState() || getCachedAiopsTransportState("workspace") || createInitialAiopsTransportState("protocol-workspace"),
    [],
  );
  const [activeThreadId, setActiveThreadId] = useState(fallbackState.threadId || "protocol-workspace");
  const [autoResume, setAutoResume] = useState(shouldAutoResumeProtocolState(fallbackState));
  const [initialState, setInitialState] = useState(fallbackState);

  return (
    <SessionContextBar
      kind="workspace"
      title="协作工作台"
      newSessionLabel="新建工作台"
      description="主 Agent 保持复杂运维会话，并按目标主机、主机组或 K8s 范围编排 host agent。"
      activeThreadId={activeThreadId}
      terminalHref="/terminal/server-local"
      onThreadChange={(nextThreadId, nextInitialState, nextAutoResume) => {
        setActiveThreadId(nextThreadId);
        setAutoResume(Boolean(nextAutoResume));
        setInitialState(nextInitialState || createInitialAiopsTransportState(nextThreadId));
      }}
    >
      <ChatTransportProvider autoResume={autoResume} cacheScope="workspace" key={activeThreadId} initialState={initialState} threadId={activeThreadId}>
        <ProtocolWorkspaceContent />
      </ChatTransportProvider>
    </SessionContextBar>
  );
}

function shouldAutoResumeProtocolState(state: AiopsTransportState) {
  return state.status === "working" || state.status === "blocked" || Object.keys(state.runtimeLiveness?.activeTurns || {}).length > 0;
}

function ProtocolWorkspaceContent() {
  const state = useAssistantTransportState() as AiopsTransportState;
  const commands = useAiopsTransportCommands();
  const turns = state.turnOrder.map((turnId) => state.turns[turnId]).filter(Boolean);
  const currentTurn = state.currentTurnId ? state.turns[state.currentTurnId] : turns[turns.length - 1];
  const process = currentTurn?.process || [];
  const approvals = Object.values(state.pendingApprovals || {});
  const surfaces = Object.values(state.mcpSurfaces || {});
  const artifacts = Object.values(state.artifacts || {});

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="mx-auto flex min-h-0 w-full max-w-6xl flex-1 flex-col gap-4 px-4 py-6 lg:px-8 xl:grid xl:grid-cols-[minmax(0,1fr)_360px]">
        <div className="flex min-h-0 flex-col gap-4">
          <div className="grid gap-3 md:grid-cols-4">
            <StatusMetric label="Runtime" value={<RiskBadge value={state.status} />} />
            <StatusMetric label="Turns" value={turns.length} />
            <StatusMetric label="Pending Approvals" value={approvals.length} />
            <StatusMetric label="MCP Surfaces" value={surfaces.length} />
          </div>

          {state.lastError ? (
            <div className="rounded-2xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">{state.lastError}</div>
          ) : null}

          <Card className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-[2rem] border-slate-200 bg-white/92 shadow-[0_20px_60px_rgba(15,23,42,0.08)]">
            <CardHeader className="border-b border-slate-100">
              <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
                <div>
                  <CardTitle>复杂运维 AI Chat</CardTitle>
                  <CardDescription>主 Agent 会话、host agent 编排和结构化输出都继续走 AssistantTransport。</CardDescription>
                </div>
                <div className="flex flex-wrap gap-2">
                  <Button
                    variant="outline"
                    onClick={() => commands.stop("protocol stop requested")}
                    disabled={state.status !== "working" && state.status !== "blocked"}
                  >
                    <X />
                    Stop
                  </Button>
                  <Button onClick={() => commands.retry(currentTurn?.id)} disabled={!currentTurn}>
                    <RefreshCw />
                    Retry
                  </Button>
                </div>
              </div>
            </CardHeader>
            <CardContent className="flex min-h-0 flex-1 flex-col overflow-hidden p-0">
              <AiopsThread />
              <AiopsComposer variant="chat" />
            </CardContent>
          </Card>
        </div>

        <aside className="grid min-h-0 content-start gap-4 overflow-y-auto pr-1">
          <Card className="rounded-[1.5rem] border-slate-200 bg-white/92">
            <CardHeader>
              <CardTitle>Main Agent Process</CardTitle>
              <CardDescription>当前 turn 的意图、过程块和输出摘要。</CardDescription>
            </CardHeader>
            <CardContent>
              {currentTurn ? (
                <div className="grid gap-3">
                  {currentTurn.user?.text ? <MessageBlock tone="dark" text={currentTurn.user.text} /> : null}
                  {currentTurn.intent?.text ? <MessageBlock tone="info" text={currentTurn.intent.text} /> : null}
                  {process.length ? <ProcessBlocks items={process} /> : null}
                  {currentTurn.final?.text ? <MessageBlock tone="light" text={currentTurn.final.text} /> : null}
                </div>
              ) : (
                <EmptyPanel title="暂无 protocol turn" description="发送消息后，主 Agent 的过程和结论会显示在这里。" />
              )}
            </CardContent>
          </Card>

          <Card className="rounded-[1.5rem] border-slate-200 bg-white/92">
            <CardHeader>
              <CardTitle>Approval Rail</CardTitle>
              <CardDescription>审批仍走 `aiops.approval-decision` transport command。</CardDescription>
            </CardHeader>
            <CardContent>
              {approvals.length ? (
                <div className="grid gap-2">
                  {approvals.map((approval) => (
                    <ApprovalRailItem key={approval.id} approval={approval} />
                  ))}
                </div>
              ) : (
                <EmptyPanel title="没有待审批" description="当前工作台没有 pending approval。" />
              )}
            </CardContent>
          </Card>

          <Card className="rounded-[1.5rem] border-slate-200 bg-white/92">
            <CardHeader>
              <CardTitle>MCP Surfaces</CardTitle>
              <CardDescription>MCP 打开、刷新和 pin 操作仍然走 transport command。</CardDescription>
            </CardHeader>
            <CardContent>
              {surfaces.length ? (
                <div className="grid gap-2">
                  {surfaces.map((surface) => (
                    <McpSurfaceRailItem key={surface.id} surface={surface} />
                  ))}
                </div>
              ) : (
                <EmptyPanel title="暂无 MCP surface" description="Agent 暴露 MCP surface 后会显示在这里。" />
              )}
            </CardContent>
          </Card>

          <Card className="rounded-[1.5rem] border-slate-200 bg-white/92">
            <CardHeader>
              <CardTitle>Artifacts / Evidence</CardTitle>
              <CardDescription>来自 transport state 的 artifacts。</CardDescription>
            </CardHeader>
            <CardContent>
              {artifacts.length ? (
                <ul className="grid gap-2 text-sm">
                  {artifacts.map((artifact) => (
                    <li key={artifact.id} className="rounded-xl border border-slate-200 bg-slate-50 px-3 py-3">
                      <div className="font-medium text-slate-900">{artifact.title || artifact.id}</div>
                      <div className="mt-1 text-xs text-slate-500">{artifact.preview || artifact.kind || "-"}</div>
                    </li>
                  ))}
                </ul>
              ) : (
                <div className="rounded-xl border border-dashed border-slate-200 px-3 py-3 text-sm text-slate-500">
                  主 Agent 或工具产出证据后会显示在这里。
                </div>
              )}
            </CardContent>
          </Card>

          <Card className="rounded-[1.5rem] border-slate-200 bg-white/92">
            <CardHeader>
              <CardTitle>Workspace Contract</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="rounded-xl border border-dashed border-slate-200 p-3 text-sm text-slate-600">
                <Send className="mb-2 h-4 w-4" />
                <span>{"工作台继续只有一条生产链路：TurnItem -> AiopsTransportState -> AssistantTransport -> React。"}</span>
              </div>
            </CardContent>
          </Card>
        </aside>
      </div>
    </div>
  );
}

function StatusMetric({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="rounded-2xl border border-slate-200 bg-white/88 px-4 py-4 shadow-sm">
      <div className="text-xs font-medium uppercase tracking-wide text-slate-500">{label}</div>
      <div className="mt-2 text-lg font-semibold text-slate-900">{value}</div>
    </div>
  );
}

function MessageBlock({ tone, text }: { tone: "dark" | "info" | "light"; text: string }) {
  return (
    <div
      className={[
        "rounded-2xl px-4 py-3 text-sm leading-6",
        tone === "dark" ? "bg-slate-950 text-white" : "",
        tone === "info" ? "border border-blue-200 bg-blue-50 text-blue-950" : "",
        tone === "light" ? "border border-slate-200 bg-white text-slate-700" : "",
      ].join(" ")}
    >
      <MessageMarkdown text={text} />
    </div>
  );
}

function ProcessBlocks({ items }: { items: AiopsProcessBlock[] }) {
  return (
    <div className="grid gap-2">
      {items.map((block) => (
        <div key={block.id} className="rounded-2xl border border-slate-200 bg-slate-50 p-3 text-sm">
          <div className="flex items-center gap-2">
            <RiskBadge value={block.status} />
            <span className="font-medium text-slate-900">{block.displayKind || block.kind}</span>
          </div>
          <p className="mt-2 leading-6 text-slate-600">{block.text}</p>
          {block.command ? (
            <pre className="mt-2 overflow-auto rounded-xl bg-slate-950 p-3 text-xs text-slate-50">{block.command}</pre>
          ) : null}
        </div>
      ))}
    </div>
  );
}

function ApprovalRailItem({ approval }: { approval: AiopsTransportApproval }) {
  const commands = useAiopsTransportCommands();
  return (
    <div className="rounded-2xl border border-amber-200 bg-amber-50 p-3 text-sm" data-testid="protocol-approval-item">
      <div className="flex items-center gap-2">
        <ShieldAlert className="h-4 w-4 text-amber-700" />
        <span className="font-medium">{approval.command || approval.reason || approval.id}</span>
      </div>
      <div className="mt-2 flex gap-2">
        <Button variant="outline" onClick={() => commands.approvalDecision(approval.id, "reject")}>
          <X />
          Reject
        </Button>
        <Button onClick={() => commands.approvalDecision(approval.id, "accept")}>
          <Check />
          Approve
        </Button>
      </div>
    </div>
  );
}

function McpSurfaceRailItem({ surface }: { surface: AiopsTransportMcpSurface }) {
  const commands = useAiopsTransportCommands();
  return (
    <div className="rounded-2xl border border-slate-200 bg-slate-50 p-3 text-sm" data-testid="protocol-mcp-surface">
      <div className="flex items-center justify-between gap-2">
        <span className="font-medium text-slate-900">{surface.title || surface.id}</span>
        <RiskBadge value={surface.status || "unknown"} />
      </div>
      <div className="mt-2 flex flex-wrap gap-2">
        <Button
          variant="outline"
          onClick={() => commands.mcpAction(surface.id, surface.status === "connected" ? "close" : "open")}
        >
          {surface.status === "connected" ? "Close" : "Open"}
        </Button>
        <Button variant="outline" onClick={() => commands.mcpRefresh(surface.id)}>
          <RefreshCw />
          Refresh
        </Button>
        <Button variant="outline" onClick={() => commands.mcpPin(surface.id, !surface.pinned)}>
          <Pin />
          {surface.pinned ? "Unpin" : "Pin"}
        </Button>
      </div>
    </div>
  );
}
