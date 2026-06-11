import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { HostChildAgentTranscript } from "@/api/hostOps";
import type { AiopsTransportChildAgent } from "@/transport/aiopsTransportTypes";

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
    expect(document.body.textContent).toContain("任务");
    expect(document.body.textContent).toContain("对话");
    expect(document.body.textContent).toContain("命令");
    expect(document.body.textContent).toContain("审核");
    expect(document.body.textContent).toContain("回执");
    expect(document.body.querySelector('[data-testid="host-subagent-tab-conversation"]')?.getAttribute("aria-selected")).toBe(
      "true",
    );
    expect(document.body.textContent).toContain("Manager 输入");
    expect(document.body.textContent).toContain("用户追问");
    expect(document.body.textContent).toContain("Assistant 返回");
    expect(document.body.textContent).not.toContain("systemctl is-active example.service");

    const commandTab = document.body.querySelector('[data-testid="host-subagent-tab-commands"]') as HTMLButtonElement;
    await act(async () => {
      commandTab.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    expect(commandTab.getAttribute("aria-selected")).toBe("true");
    expect(document.body.textContent).toContain("工具调用");
    expect(document.body.textContent).toContain("工具结果");
    expect(document.body.textContent).toContain("systemctl is-active example.service");
    expect(document.body.textContent).toContain("active");
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
