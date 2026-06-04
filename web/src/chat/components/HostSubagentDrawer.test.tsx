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

  it("loads and renders an independent child agent transcript", async () => {
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
            content: "检查 PostgreSQL 版本并初始化主库",
            createdAt: "2026-06-04T01:00:00Z",
          },
          {
            id: "item-user",
            type: "user_followup",
            content: "继续验证复制状态",
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
            content: "pg_isready -h 127.0.0.1",
            status: "running",
            createdAt: "2026-06-04T01:03:00Z",
          },
          {
            id: "item-tool-result",
            type: "tool_result",
            toolName: "shell",
            content: "accepting connections",
            status: "completed",
            createdAt: "2026-06-04T01:04:00Z",
          },
        ],
      });
      await transcript.promise;
    });

    expect(document.body.textContent).toContain("Franklin");
    expect(document.body.textContent).toContain("@1.1.1.1");
    expect(document.body.textContent).toContain("Manager 输入");
    expect(document.body.textContent).toContain("用户追问");
    expect(document.body.textContent).toContain("Assistant 返回");
    expect(document.body.textContent).toContain("工具调用");
    expect(document.body.textContent).toContain("工具结果");
    expect(document.body.textContent).toContain("pg_isready -h 127.0.0.1");
    expect(document.body.textContent).toContain("accepting connections");
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
    task: "初始化主库",
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
