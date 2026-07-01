import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { HostChildAgentTranscript } from "@/api/hostOps";
import type { AiopsTransportChildAgent, AiopsTransportState } from "@/transport/aiopsTransportTypes";

import { HostSubagentDrawer } from "./HostSubagentDrawer";

type Deferred<T> = {
  promise: Promise<T>;
  resolve: (value: T) => void;
  reject: (error: Error) => void;
};

describe("HostSubagentDrawer", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => {
      root.unmount();
    });
    container.remove();
  });

  it("loads and renders an independent child agent transcript in tabs", async () => {
    const transcript = createDeferred<HostChildAgentTranscript>();
    const loadTranscript = vi.fn(() => transcript.promise);

    await act(async () => {
      root.render(
        <HostSubagentDrawer
          open
          childAgent={sampleChildAgent()}
          loadTranscript={loadTranscript}
          onOpenChange={vi.fn()}
        />,
      );
    });

    expect(loadTranscript).toHaveBeenCalledWith("child-1");
    expect(document.body.querySelector('[data-testid="host-subagent-drawer"]')).not.toBeNull();
    expect(document.body.textContent).toContain("正在读取子 agent 对话");

    await act(async () => {
      transcript.resolve({
        childAgentId: "child-1",
        items: [
          {
            id: "item-manager",
            type: "manager_message",
            content: "检查主机状态并执行准备步骤",
            createdAt: "2026-06-04T01:00:00Z",
          },
          {
            id: "item-user",
            type: "user_followup",
            content: "继续验证主机状态",
            createdAt: "2026-06-04T01:01:00Z",
          },
          {
            id: "item-assistant",
            type: "assistant_message",
            content: "我会先执行只读检查，再返回结果。",
            createdAt: "2026-06-04T01:02:00Z",
          },
          {
            id: "item-tool-call",
            type: "tool_call",
            toolName: "shell",
            content: "systemctl is-active example.service",
            status: "running",
            createdAt: "2026-06-04T01:03:00Z",
          },
          {
            id: "item-tool-result",
            type: "tool_result",
            toolName: "shell",
            content: "active",
            status: "completed",
            createdAt: "2026-06-04T01:04:00Z",
          },
        ],
      });
      await transcript.promise;
    });

    expect(document.body.textContent).toContain("Franklin");
    expect(document.body.textContent).toContain("@1.1.1.1");
    expect(document.body.textContent).toContain("概览");
    expect(document.body.textContent).toContain("Agent 对话");
    expect(document.body.textContent).toContain("Prompt Trace");
    expect(document.body.textContent).toContain("工具");
    expect(document.body.textContent).toContain("审核");
    expect(document.body.textContent).toContain("回执");
    expect(document.body.querySelector('[data-testid="host-subagent-tab-conversation"]')?.getAttribute("aria-selected")).toBe(
      "true",
    );
    expect(document.body.textContent).toContain("管理 Agent → 主机 Agent");
    expect(document.body.textContent).toContain("用户追问");
    expect(document.body.textContent).toContain("主机 Franklin 返回");
    expect(document.body.textContent).not.toContain("Assistant 返回");
    expect(document.body.textContent).not.toContain("systemctl is-active example.service");

    const commandTab = document.body.querySelector('[data-testid="host-subagent-tab-tools"]') as HTMLButtonElement;
    await act(async () => {
      commandTab.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    expect(commandTab.getAttribute("aria-selected")).toBe("true");
    expect(document.body.textContent).toContain("工具调用");
    expect(document.body.textContent).toContain("工具结果");
    expect(document.body.textContent).toContain("systemctl is-active example.service");
    expect(document.body.textContent).toContain("active");
  });

  it("renders a compact overview without trace summary or raw session id overflow", async () => {
    await act(async () => {
      root.render(
        <HostSubagentDrawer
          open
          childAgent={{
            ...sampleChildAgent(),
            sessionId: "host-child:hostops:turn-1781724661650847026:remote-120-77-239-90",
            lastInputPreview: "查看@120.77.239.90主机Docker容器资源占用,只读执行docker stats --no-stream并总结",
            lastOutputPreview:
              "## 任务执行完成报告 **任务状态：** ✅ 已完成 **执行摘要：** 在主机 120.77.239.90 上成功执行 `docker stats --no-stream` 命令",
          }}
          loadTranscript={async () => ({ childAgentId: "child-1", items: [] })}
          onOpenChange={vi.fn()}
        />,
      );
    });
    await flushMicrotasks();

    await clickTab("task");

    expect(document.body.textContent).toContain("概览");
    expect(document.body.textContent).toContain("@1.1.1.1");
    expect(document.body.textContent).toContain("最近输入");
    expect(document.body.textContent).toContain("最近输出");
    expect(document.body.textContent).not.toContain("Trace 摘要");
    expect(document.body.textContent).not.toContain("host-child:hostops");
  });

  it("does not repeat the host handle in the drawer subtitle when display name already includes it", async () => {
    await act(async () => {
      root.render(
        <HostSubagentDrawer
          open
          childAgent={{
            ...sampleChildAgent(),
            hostAddress: "1.1.1.1",
            hostDisplayName: "@1.1.1.1",
            task: "执行主机 A 准备步骤",
          }}
          loadTranscript={async () => ({ childAgentId: "child-1", items: [] })}
          onOpenChange={vi.fn()}
        />,
      );
    });
    await flushMicrotasks();

    const description = document.body.querySelector('[data-slot="sheet-description"]');
    expect(description?.textContent).toBe("@1.1.1.1 · 执行主机 A 准备步骤");
  });

  it("keeps Agent conversation and Prompt Trace separate while linking model prompt markdown", async () => {
    await act(async () => {
      root.render(
        <HostSubagentDrawer
          open
          childAgent={sampleChildAgent()}
          loadTranscript={async () => ({
            childAgentId: "child-1",
            items: [
              {
                id: "manager-1",
                type: "manager_message",
                content: "主控 Agent 分配：检查主机负载和服务状态",
              },
              {
                id: "llm-request-1",
                type: "llm_request",
                content: "第 1 轮调用 LLM\ntrace: /opt/aiops-v2/data/model-input-traces/host-a/iteration-001.md",
                payload: {
                  traceFile: "/opt/aiops-v2/data/model-input-traces/host-a/iteration-001.md",
                  visibleTools: ["exec_command", "grep"],
                },
              },
              {
                id: "llm-response-1",
                type: "llm_response",
                content: "我需要先读取系统指标。",
              },
              {
                id: "assistant-1",
                type: "assistant_message",
                content: "我会先读取系统指标，再决定是否需要执行修复动作。",
              },
              {
                id: "tool-1",
                type: "tool_call",
                toolName: "shell",
                content: "uptime",
              },
            ],
          })}
          onOpenChange={vi.fn()}
        />,
      );
    });
    await flushMicrotasks();

    expect(document.body.textContent).toContain("主机 Agent → LLM");
    expect(document.body.textContent).toContain("第 1 轮调用 LLM");
    expect(document.body.textContent).toContain("查看 Prompt MD");
    expect(document.body.textContent).toContain("可用工具：exec_command, grep");
    expect(document.body.textContent).not.toContain("trace: /opt/aiops-v2/data/model-input-traces/host-a/iteration-001.md");
    const promptLink = document.body.querySelector('[data-testid="host-subagent-prompt-md-link-llm-request-1"]') as HTMLAnchorElement;
    expect(promptLink?.getAttribute("href")).toBe("/debug/prompts?path=host-a%2Fiteration-001.md&view=raw&raw=markdown");
    expect(promptLink?.getAttribute("href")).not.toContain("/opt");

    await clickTab("prompt");

    expect(document.body.textContent).not.toContain("主机 Agent 与 LLM 对话");
    expect(document.body.textContent).not.toContain("管理 Agent → 主机 Agent");
    expect(document.body.textContent).not.toContain("主控 Agent 分配：检查主机负载和服务状态");
    expect(document.body.textContent).not.toContain("我会先读取系统指标，再决定是否需要执行修复动作。");
    expect(document.body.textContent).toContain("Prompt MD 文件");
    expect(document.body.textContent).toContain("查看 Prompt MD");
    expect(document.body.textContent).not.toContain("trace: /opt/aiops-v2/data/model-input-traces/host-a/iteration-001.md");
    expect(document.body.textContent).not.toContain("暂无 Prompt trace");
    expect(document.body.textContent).not.toContain("暂无 context decision");
    expect(document.body.textContent).not.toContain("uptime");
  });

  it("derives tool and evidence trace from transcript when structured trace is absent", async () => {
    await act(async () => {
      root.render(
        <HostSubagentDrawer
          open
          childAgent={sampleChildAgent()}
          loadTranscript={async () => ({
            childAgentId: "child-1",
            items: [
              {
                id: "manager-1",
                type: "manager_message",
                content: "查看@1.1.1.1主机Docker容器资源占用,只读执行docker stats --no-stream并总结",
              },
              {
                id: "assistant-1",
                type: "assistant_message",
                content: "执行 `docker stats --no-stream` 成功。证据引用：ev-92398d5c63ebacf1",
                status: "completed",
              },
            ],
          })}
          onOpenChange={vi.fn()}
        />,
      );
    });
    await flushMicrotasks();

    await clickTab("tools");
    expect(document.body.textContent).not.toContain("暂无工具 trace");
    expect(document.body.textContent).toContain("从 Agent 对话推断");
    expect(document.body.textContent).toContain("docker stats --no-stream");

    await clickTab("evidence");
    expect(document.body.textContent).not.toContain("暂无证据 trace");
    expect(document.body.textContent).toContain("ev-92398d5c63ebacf1");
    expect(document.body.textContent).toContain("主机 Franklin 返回");
  });

  it("keeps the host subagent drawer below the app header and avoids covering the sidebar with the overlay", async () => {
    await act(async () => {
      root.render(
        <HostSubagentDrawer
          open
          childAgent={sampleChildAgent()}
          loadTranscript={async () => ({ childAgentId: "child-1", items: [] })}
          onOpenChange={vi.fn()}
        />,
      );
    });
    await flushMicrotasks();

    const drawer = document.body.querySelector('[data-slot="sheet-content"]');
    const overlay = document.body.querySelector('[data-slot="sheet-overlay"]');
    expect(drawer?.className).toContain("top-[var(--aiops-shell-header-height");
    expect(drawer?.className).toContain("h-[calc(100dvh-var(--aiops-shell-header-height");
    expect(overlay?.className).toContain("lg:left-[var(--aiops-shell-sidebar-width");
  });

  it("opens transcript for the selected host-bound child agent only", async () => {
    await act(async () => {
      root.render(
        <HostSubagentDrawer
          open
          childAgentId="child-host-b"
          state={{
            childAgents: {
              "child-host-a": {
                id: "child-host-a",
                hostId: "host-a",
                hostDisplayName: "主机A",
                status: "running",
              },
              "child-host-b": {
                id: "child-host-b",
                hostId: "host-b",
                hostDisplayName: "主机B",
                status: "running",
              },
            },
            childAgentTranscripts: {
              "child-host-a": [{ id: "a-1", content: "host-a transcript" }],
              "child-host-b": [{ id: "b-1", content: "host-b transcript" }],
            },
          } as AiopsTransportState}
          onOpenChange={vi.fn()}
        />,
      );
    });

    expect(document.body.textContent).toContain("主机B");
    expect(document.body.textContent).toContain("host-b transcript");
    expect(document.body.textContent).not.toContain("host-a transcript");
  });

  it("defaults approval_required agents to approval tab", async () => {
    await act(async () => {
      root.render(
        <HostSubagentDrawer
          open
          childAgent={{ ...sampleChildAgent(), status: "approval_required" }}
          loadTranscript={async () => ({
            childAgentId: "child-1",
            items: [
              {
                id: "approval-1",
                type: "approval",
                content: "等待执行 systemctl restart 的审核",
                status: "pending",
              },
            ],
          })}
          onOpenChange={vi.fn()}
        />,
      );
    });
    await flushMicrotasks();

    expect(document.body.querySelector('[data-testid="host-subagent-tab-approval"]')?.getAttribute("aria-selected")).toBe(
      "true",
    );
    expect(document.body.textContent).toContain("等待执行 systemctl restart 的审核");
  });

  it("submits pending host command approval decisions from approval tab", async () => {
    const submitApprovalDecision = vi.fn(async () => ({ status: "accepted" }));
    await act(async () => {
      root.render(
        <HostSubagentDrawer
          open
          childAgent={{ ...sampleChildAgent(), status: "approval_required" }}
          loadTranscript={async () => ({
            childAgentId: "child-1",
            items: [
              {
                id: "approval-1",
                type: "approval",
                approvalId: "hostcmd-approval-1",
                content: "等待执行非白名单主机命令：touch /tmp/aiops-check",
                status: "pending",
              },
            ],
          })}
          submitApprovalDecision={submitApprovalDecision}
          onOpenChange={vi.fn()}
        />,
      );
    });
    await flushMicrotasks();

    const approve = document.body.querySelector('[data-testid="host-subagent-approval-approve-hostcmd-approval-1"]') as HTMLButtonElement;
    const reject = document.body.querySelector('[data-testid="host-subagent-approval-reject-hostcmd-approval-1"]') as HTMLButtonElement;
    expect(approve).not.toBeNull();
    expect(reject).not.toBeNull();

    await act(async () => {
      approve.dispatchEvent(new MouseEvent("click", { bubbles: true }));
      await Promise.resolve();
    });

    expect(submitApprovalDecision).toHaveBeenCalledWith("hostcmd-approval-1", "accept");
    expect(document.body.textContent).toContain("审批请求已提交");
  });

  it("defaults failed agents to receipt tab with errors", async () => {
    await act(async () => {
      root.render(
        <HostSubagentDrawer
          open
          childAgent={{ ...sampleChildAgent(), status: "failed", error: "命令退出码 1" }}
          loadTranscript={async () => ({
            childAgentId: "child-1",
            items: [
              {
                id: "error-1",
                type: "error",
                content: "主机验证失败",
                status: "failed",
              },
            ],
          })}
          onOpenChange={vi.fn()}
        />,
      );
    });
    await flushMicrotasks();

    expect(document.body.querySelector('[data-testid="host-subagent-tab-receipts"]')?.getAttribute("aria-selected")).toBe(
      "true",
    );
    expect(document.body.textContent).toContain("主机验证失败");
    expect(document.body.textContent).toContain("命令退出码 1");
  });

  it("shows empty, error, and close states", async () => {
    const onOpenChange = vi.fn();

    await act(async () => {
      root.render(
        <HostSubagentDrawer
          open
          childAgent={sampleChildAgent()}
          loadTranscript={async () => ({ childAgentId: "child-1", items: [] })}
          onOpenChange={onOpenChange}
        />,
      );
    });
    await flushMicrotasks();

    expect(document.body.textContent).toContain("暂无独立对话记录");

    await act(async () => {
      root.render(
        <HostSubagentDrawer
          open
          childAgent={sampleChildAgent()}
          loadTranscript={async () => {
            throw new Error("transcript unavailable");
          }}
          onOpenChange={onOpenChange}
        />,
      );
    });
    await flushMicrotasks();

    expect(document.body.textContent).toContain("读取 transcript 失败");
    expect(document.body.textContent).toContain("transcript unavailable");

    const close = document.body.querySelector('[data-testid="host-subagent-drawer-close"]') as HTMLButtonElement;
    await act(async () => {
      close.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("renders prompt context tool mcp skills approval evidence report tabs", async () => {
    await act(async () => {
      root.render(
        <HostSubagentDrawer
          open
          childAgent={sampleTraceChildAgent()}
          loadTranscript={async () => sampleTraceTranscript()}
          onOpenChange={vi.fn()}
        />,
      );
    });
    await flushMicrotasks();

    expect(document.body.textContent).toContain("概览");
    expect(document.body.textContent).toContain("Agent 对话");
    expect(document.body.textContent).toContain("Prompt Trace");
    expect(document.body.textContent).toContain("工具");
    expect(document.body.textContent).toContain("MCP/Skills");
    expect(document.body.textContent).toContain("审核");
    expect(document.body.textContent).toContain("证据");
    expect(document.body.textContent).toContain("回执");
    expect(document.body.textContent).not.toContain("Trace 摘要");
    expect(document.body.textContent).not.toContain("host_agent.binding.v1");

    await clickTab("prompt");
    expect(document.body.textContent).toContain("Base runtime");
    expect(document.body.textContent).toContain("Host overlay");
    expect(document.body.textContent).toContain("Host task context");
    expect(document.body.textContent).toContain("Skill context");
    expect(document.body.textContent).toContain("MCP context");
    expect(document.body.textContent).toContain("host_agent.binding.v1");

    await clickTab("tools");
    expect(document.body.textContent).toContain("host_agent_tool");
    expect(document.body.textContent).toContain("human_terminal");
    expect(document.body.textContent).toContain("HostCommandTool");
    expect(document.body.textContent).toContain("operator-terminal");

    await clickTab("mcp-skills");
    expect(document.body.textContent).toContain("MCP instruction delta");
    expect(document.body.textContent).toContain("generic-docs");
    expect(document.body.textContent).toContain("Skill activation");
    expect(document.body.textContent).toContain("generic-log-review");

    await clickTab("evidence");
    expect(document.body.textContent).toContain("artifact://evidence/service-status");
    expect(document.body.textContent).toContain("hash:service-status");

    await clickTab("receipts");
    expect(document.body.textContent).toContain("report.created");
    expect(document.body.textContent).toContain("report.sent_to_manager");
  });

  it("shows retention rank compact action source ref and redaction state", async () => {
    await act(async () => {
      root.render(
        <HostSubagentDrawer
          open
          childAgent={sampleTraceChildAgent()}
          loadTranscript={async () => sampleTraceTranscript()}
          onOpenChange={vi.fn()}
        />,
      );
    });
    await flushMicrotasks();

    await clickTab("prompt");

    expect(document.body.textContent).toContain("P0");
    expect(document.body.textContent).toContain("keep");
    expect(document.body.textContent).toContain("agent-message:generic-task");
    expect(document.body.textContent).toContain("redacted");
    expect(document.body.textContent).toContain("ref://prompt/base-runtime");
    expect(document.body.textContent).toContain("hash:prompt-base");
    expect(document.body.textContent).not.toContain("raw-sensitive-token");
  });

  it("marks human terminal command source separately from host agent tool call", async () => {
    await act(async () => {
      root.render(
        <HostSubagentDrawer
          open
          childAgent={sampleTraceChildAgent()}
          loadTranscript={async () => sampleTraceTranscript()}
          onOpenChange={vi.fn()}
        />,
      );
    });
    await flushMicrotasks();

    await clickTab("tools");

    expect(document.body.textContent).toContain("host_agent_tool");
    expect(document.body.textContent).toContain("HostCommandTool");
    expect(document.body.textContent).toContain("human_terminal");
    expect(document.body.textContent).toContain("operator-terminal");
  });

  it("shows queued cancelled superseded host subtasks with reason", async () => {
    await act(async () => {
      root.render(
        <HostSubagentDrawer
          open
          childAgent={{
            ...sampleTraceChildAgent(),
            subtaskStatus: "queued",
            queueReason: "waiting for host session capacity",
            source: "manager_plan",
          }}
          loadTranscript={async () => sampleTraceTranscript()}
          onOpenChange={vi.fn()}
        />,
      );
    });
    await flushMicrotasks();

    await clickTab("task");

    expect(document.body.textContent).toContain("queued");
    expect(document.body.textContent).toContain("waiting for host session capacity");
    expect(document.body.textContent).toContain("manager_plan");

    await act(async () => {
      root.render(
        <HostSubagentDrawer
          open
          childAgent={{
            ...sampleTraceChildAgent(),
            status: "cancelled",
            subtaskStatus: "superseded",
            queueReason: "replaced by newer host task",
            source: "user_followup",
          }}
          loadTranscript={async () => sampleTraceTranscript()}
          onOpenChange={vi.fn()}
        />,
      );
    });
    await flushMicrotasks();

    await clickTab("task");

    expect(document.body.textContent).toContain("superseded");
    expect(document.body.textContent).toContain("replaced by newer host task");
    expect(document.body.textContent).toContain("user_followup");
  });
});

function sampleChildAgent(): AiopsTransportChildAgent {
  return {
    id: "child-1",
    missionId: "mission-1",
    sessionId: "session-child-1",
    hostId: "host-1",
    hostAddress: "1.1.1.1",
    hostDisplayName: "Franklin",
    status: "running",
    task: "执行主机准备步骤",
  };
}

function sampleTraceChildAgent(): AiopsTransportChildAgent {
  return {
    id: "child-trace-1",
    missionId: "mission-generic",
    sessionId: "host-child:generic-1",
    hostId: "host-generic-a",
    hostAddress: "host-a.internal",
    hostDisplayName: "Generic host A",
    status: "running",
    task: "Inspect generic service health and resource state",
    runtimeProfile: {
      id: "host-agent-full-runtime",
      capabilities: ["prompt_compiler", "context_governance", "trace", "approval_gate"],
    },
    promptSections: [
      {
        id: "prompt-base",
        title: "Base runtime",
        category: "base_runtime",
        sectionId: "base.runtime.v1",
        retentionRank: "P0",
        compactAction: "keep",
        sourceRef: "ref://prompt/base-runtime",
        redaction: "hash:prompt-base",
      },
      {
        id: "prompt-overlay",
        title: "Host overlay",
        category: "host_overlay",
        sectionId: "host_agent.binding.v1",
        retentionRank: "P0",
        compactAction: "keep",
        sourceRef: "agent-message:generic-task",
        redaction: "redacted",
      },
      {
        id: "prompt-task",
        title: "Host task context",
        category: "host_task_context",
        sectionId: "host_agent.assigned_subtask.v1",
        retentionRank: "P1",
        compactAction: "compact",
        sourceRef: "agent-message:generic-task",
        redaction: "ref://task/context",
      },
      {
        id: "prompt-skill",
        title: "Skill context",
        category: "skill_context",
        sectionId: "skill.generic_log_review.v1",
        retentionRank: "P2",
        compactAction: "summarize",
        sourceRef: "skill://generic-log-review",
        redaction: "hash:skill-context",
      },
      {
        id: "prompt-mcp",
        title: "MCP context",
        category: "mcp_context",
        sectionId: "mcp.generic_docs.instructions.v1",
        retentionRank: "P2",
        compactAction: "delta",
        sourceRef: "mcp://generic-docs",
        redaction: "ref://mcp/context",
      },
    ],
    toolSurfaceSnapshot: [
      {
        id: "tool-host-command",
        name: "HostCommandTool",
        source: "host_agent_tool",
        status: "allowed",
        summary: "Read generic service and process state",
        redaction: "ref://tool/host-command",
      },
      {
        id: "tool-human-terminal",
        name: "operator-terminal",
        source: "human_terminal",
        status: "recorded",
        summary: "Manual terminal observation attached by operator",
        redaction: "hash:human-terminal",
      },
    ],
    mcpInstructionDeltas: [
      {
        id: "mcp-delta-1",
        server: "generic-docs",
        sourceRef: "mcp://generic-docs/instructions-delta",
        redaction: "ref://mcp/delta",
        summary: "MCP instruction delta",
      },
    ],
    skillActivationTrace: [
      {
        id: "skill-activation-1",
        skill: "generic-log-review",
        status: "activated",
        sourceRef: "skill://generic-log-review",
        redaction: "hash:skill-activation",
      },
    ],
    evidenceTrace: [
      {
        id: "evidence-1",
        title: "Generic service status",
        source: "host_agent_tool",
        artifactRef: "artifact://evidence/service-status",
        hash: "hash:service-status",
        redaction: "ref://evidence/service-status",
      },
    ],
    reportTimeline: [
      { id: "report-1", event: "report.created", status: "completed", sourceRef: "ref://report/draft" },
      { id: "report-2", event: "report.sent_to_manager", status: "completed", sourceRef: "ref://report/final" },
    ],
  };
}

function sampleTraceTranscript(): HostChildAgentTranscript {
  return {
    childAgentId: "child-trace-1",
    items: [
      {
        id: "agent-message-1",
        type: "manager_message",
        content: "Inspect generic host resources without exposing sensitive values.",
      },
      {
        id: "approval-trace-1",
        type: "approval",
        approvalId: "approval-generic-read",
        content: "Approval trace: read generic file metadata",
        status: "approved",
        payload: { sourceRef: "ref://approval/generic-read", redaction: "hash:approval" },
      },
    ],
    agentMessages: [
      { id: "agent-message-1", role: "manager", content: "Inspect generic host resources." },
      { id: "agent-message-2", role: "host_agent", content: "Collected redacted evidence references." },
    ],
    approvalTrace: [
      {
        id: "approval-trace-1",
        approvalId: "approval-generic-read",
        status: "approved",
        sourceRef: "ref://approval/generic-read",
        redaction: "hash:approval",
      },
    ],
  };
}

async function clickTab(tab: string) {
  const element = document.body.querySelector(`[data-testid="host-subagent-tab-${tab}"]`) as HTMLButtonElement;
  expect(element).not.toBeNull();
  await act(async () => {
    element.dispatchEvent(new MouseEvent("click", { bubbles: true }));
  });
}

function createDeferred<T>(): Deferred<T> {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((promiseResolve, promiseReject) => {
    resolve = promiseResolve;
    reject = promiseReject;
  });
  return { promise, resolve, reject };
}

async function flushMicrotasks() {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
}
