import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { WorkflowAiDrawer } from "./WorkflowAiDrawer";
import "@/components/runner/runnerStudio.css";

describe("WorkflowAiDrawer", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
    vi.stubGlobal("navigator", {
      clipboard: { writeText: vi.fn(() => Promise.resolve()) },
    });
  });

  afterEach(() => {
    act(() => root.unmount());
    vi.unstubAllGlobals();
    container.remove();
  });

  it("defaults closed and opens as a focused chat drawer without context card shortcuts", async () => {
    await act(async () => {
      root.render(<WorkflowAiDrawer open={false} context={{ workflowId: "workflow" }} />);
    });
    expect(container.querySelector('[data-testid="workflow-ai-drawer"]')).toBeNull();

    await act(async () => {
      root.render(<WorkflowAiDrawer open context={{ workflowId: "workflow", workflowName: "Redis", revision: "rev", selectedNodeId: "collect", saveState: "saved", lastModifiedLabel: "修改于 10:30", validation: { valid: true } }} />);
    });
    expect(container.querySelector('[data-testid="workflow-ai-drawer"]')?.textContent).toContain("Workflow AI");
    expect(container.querySelector('[data-testid="workflow-ai-updated-label"]')?.textContent).toContain("修改于 10:30");
    expect(container.querySelector('[data-testid="workflow-ai-context-card"]')).toBeNull();
    expect(container.querySelector('[data-testid="workflow-ai-empty"]')).toBeNull();
  });

  it("prefills the composer from a create-mode initial message", async () => {
    await act(async () => {
      root.render(<WorkflowAiDrawer open context={{ workflowName: "新建 Workflow" }} initialMessage="每天巡检 Redis 内存" />);
    });
    expect((container.querySelector("textarea") as HTMLTextAreaElement).value).toBe("每天巡检 Redis 内存");
  });

  it("renders user requests and agent plans in a chat transcript", async () => {
    const onSubmit = vi.fn();
    await act(async () => {
      root.render(<WorkflowAiDrawer open context={{ workflowId: "workflow", workflowName: "Redis" }} onSubmit={onSubmit} />);
    });

    const textarea = container.querySelector("textarea") as HTMLTextAreaElement;
    await act(async () => {
      const setter = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")?.set;
      setter?.call(textarea, "添加验证步骤");
      textarea.dispatchEvent(new Event("input", { bubbles: true }));
    });
    await act(async () => {
      Array.from(container.querySelectorAll("button")).find((button) => button.textContent?.includes("Send"))?.click();
    });

    await act(async () => {
      root.render(
        <WorkflowAiDrawer
          open
          context={{ workflowId: "workflow", workflowName: "Redis" }}
          plan={{ id: "plan", items: [{ id: "item", title: "添加验证步骤", description: "先生成一个可审查的步骤 patch。" }] }}
          onSubmit={onSubmit}
        />,
      );
    });

    const transcript = container.querySelector('[data-testid="workflow-ai-chat-transcript"]');
    expect(transcript).not.toBeNull();
    expect(transcript?.querySelector('[data-testid="workflow-ai-message-user"]')?.textContent).toContain("添加验证步骤");
    expect(transcript?.querySelector('[data-testid="workflow-ai-message-assistant"]')?.textContent).toContain("修改计划");
    expect((container.querySelector("textarea") as HTMLTextAreaElement).value).toBe("");
  });

  it("shows a planning progress message before the plan arrives", async () => {
    const onSubmit = vi.fn();
    await act(async () => {
      root.render(
        <WorkflowAiDrawer
          open
          context={{ workflowId: "workflow", workflowName: "Redis" }}
          stage="planning"
          onSubmit={onSubmit}
        />,
      );
    });

    const thinkingText = container.querySelector('[data-testid="workflow-ai-thinking-card"]')?.textContent || "";
    expect(thinkingText).toContain("正在思考");
    expect(thinkingText).not.toContain("读取画布");
    expect(thinkingText).not.toContain("读取画布并处理");
    expect(thinkingText).not.toContain("判断需要普通回复");
    expect(thinkingText).not.toContain("不套用完整运维模板");
    const textarea = container.querySelector("textarea") as HTMLTextAreaElement;
    expect(textarea.disabled).toBe(true);
    expect(textarea.placeholder).toContain("正在生成");
    expect((Array.from(container.querySelectorAll("button")).find((button) => button.textContent?.includes("Send")) as HTMLButtonElement).disabled).toBe(true);
    await act(async () => {
      textarea.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true }));
    });
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("shows a normal chat thinking message without mentioning edit plans", async () => {
    await act(async () => {
      root.render(
        <WorkflowAiDrawer
          open
          context={{ workflowId: "workflow", workflowName: "Redis" }}
          stage="chatting"
        />,
      );
    });

    const thinkingText = container.querySelector('[data-testid="workflow-ai-chat-thinking-card"]')?.textContent || "";
    expect(thinkingText).toContain("正在回复");
    expect(thinkingText).toContain("读取当前画布");
    expect(thinkingText).toContain("等待模型回复");
    expect(thinkingText).not.toContain("生成修改计划");
    expect(thinkingText).not.toContain("结合当前工作流上下文");
    expect(container.querySelector('[data-testid="workflow-ai-thinking-card"]')).toBeNull();
    expect(container.textContent).not.toContain("正在生成修改计划");
    expect((container.querySelector("textarea") as HTMLTextAreaElement).placeholder).toContain("正在回复");
  });

  it("keeps previous assistant replies when the next user message is submitted", async () => {
    const onSubmit = vi.fn();
    await act(async () => {
      root.render(
        <WorkflowAiDrawer
          open
          context={{ workflowId: "workflow", workflowName: "Redis" }}
          readonlyAnswerTitle="Workflow AI 回复"
          readonlyAnswer="第一条回复：可以解释当前工作流。"
          onSubmit={onSubmit}
        />,
      );
    });

    const textarea = container.querySelector("textarea") as HTMLTextAreaElement;
    await act(async () => {
      const setter = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")?.set;
      setter?.call(textarea, "你帮我随便添加一个节点");
      textarea.dispatchEvent(new Event("input", { bubbles: true }));
    });
    await act(async () => {
      Array.from(container.querySelectorAll("button")).find((button) => button.textContent?.includes("Send"))?.click();
    });
    await act(async () => {
      root.render(
        <WorkflowAiDrawer
          open
          context={{ workflowId: "workflow", workflowName: "Redis" }}
          stage="planning"
          onSubmit={onSubmit}
        />,
      );
    });
    await act(async () => {
      root.render(
        <WorkflowAiDrawer
          open
          context={{ workflowId: "workflow", workflowName: "Redis" }}
          plan={{
            id: "plan",
            items: [{ id: "item", title: "添加 Redis 健康检查节点", description: "根据当前 Redis 工作流上下文补一个可连接性检查节点。" }],
          }}
          onSubmit={onSubmit}
        />,
      );
    });

    const transcript = container.querySelector('[data-testid="workflow-ai-chat-transcript"]');
    expect(transcript?.textContent).toContain("第一条回复：可以解释当前工作流。");
    expect(transcript?.textContent).toContain("你帮我随便添加一个节点");
    expect(transcript?.textContent).toContain("添加 Redis 健康检查节点");
    expect(transcript?.textContent).not.toContain("本次计划生成过程");
    expect(transcript?.textContent).not.toContain("我先生成计划");
    expect(onSubmit).toHaveBeenCalledWith("你帮我随便添加一个节点");
  });

  it("starts a new conversation from the footer and clears the current transcript", async () => {
    const onNewSession = vi.fn();
    await act(async () => {
      root.render(
        <WorkflowAiDrawer
          open
          context={{ workflowId: "workflow", workflowName: "Redis" }}
          session={{ id: "session-1", workflowId: "workflow", status: "active" }}
          readonlyAnswer="旧会话回复"
          onNewSession={onNewSession}
        />,
      );
    });

    expect(container.querySelector('[data-testid="workflow-ai-chat-transcript"]')?.textContent).toContain("旧会话回复");

    await act(async () => {
      Array.from(container.querySelectorAll("button")).find((button) => button.textContent?.includes("新会话"))?.click();
    });
    await act(async () => {
      root.render(
        <WorkflowAiDrawer
          open
          context={{ workflowId: "workflow", workflowName: "Redis" }}
          session={{ id: "session-2", workflowId: "workflow", status: "active" }}
          onNewSession={onNewSession}
        />,
      );
    });

    const transcript = container.querySelector('[data-testid="workflow-ai-chat-transcript"]');
    expect(onNewSession).toHaveBeenCalledTimes(1);
    expect(transcript?.textContent).not.toContain("旧会话回复");
    expect(transcript?.textContent).toContain("你可以直接问我");
    expect((container.querySelector("textarea") as HTMLTextAreaElement).value).toBe("");
  });

  it("renders normal readonly answers as plain chat text without a fixed heading", async () => {
    await act(async () => {
      root.render(
        <WorkflowAiDrawer
          open
          context={{ workflowId: "workflow", workflowName: "Redis" }}
          readonlyAnswerTitle="Workflow AI 回复"
          readonlyAnswer="当前工作流只有 Start 一个节点，还没有执行步骤。"
        />,
      );
    });

    const answer = container.querySelector('[data-testid="workflow-ai-readonly-answer"]');
    expect(answer?.textContent).toContain("当前工作流只有 Start 一个节点");
    expect(answer?.querySelector("h3")).toBeNull();
    expect(answer?.textContent).not.toContain("Workflow AI 回复");
  });

  it("shows the current step while applying a confirmed plan", async () => {
    await act(async () => {
      root.render(
        <WorkflowAiDrawer
          open
          context={{ workflowId: "workflow", workflowName: "Redis" }}
          stage="applying_plan"
          activeStep={{
            index: 2,
            total: 6,
            title: "对象识别与预检",
            goal: "确认目标 PostgreSQL 实例和 pgBackRest 配置可用。",
            environment: "读取 PGHOST、PGPORT、PGBACKREST_STANZA。",
            scriptSummary: "生成只读预检 Python 脚本。",
          }}
        />,
      );
    });

    const card = container.querySelector('[data-testid="workflow-ai-step-progress-card"]');
    expect(card?.textContent).toContain("正在生成步骤 2/6");
    expect(card?.textContent).toContain("对象识别与预检");
    expect(card?.textContent).toContain("目标");
    expect(card?.textContent).toContain("环境");
    expect(card?.textContent).toContain("脚本");
  });

  it("renders generated step history as chat process messages", async () => {
    await act(async () => {
      root.render(
        <WorkflowAiDrawer
          open
          context={{ workflowId: "workflow", workflowName: "Redis" }}
          stage="applying_plan"
          stepHistory={[
            {
              index: 1,
              total: 2,
              title: "识别对象",
              goal: "读取工作流对象和元数据。",
              environment: "当前 Workflow 图层。",
              scriptSummary: "生成对象识别脚本。",
              status: "completed",
            },
            {
              index: 2,
              total: 2,
              title: "生成预检",
              goal: "检查输入变量。",
              environment: "读取 workflow_context。",
              scriptSummary: "生成预检脚本。",
              status: "running",
            },
          ]}
        />,
      );
    });

    const steps = container.querySelectorAll('[data-testid="workflow-ai-step-history-card"]');
    expect(steps).toHaveLength(2);
    expect(steps[0].textContent).toContain("完成步骤 1/2");
    expect(steps[0].textContent).toContain("识别对象");
    expect(steps[1].textContent).toContain("正在生成步骤 2/2");
    expect(steps[1].textContent).toContain("生成预检");
  });

  it("keeps long plan text inside the drawer and supports drag resizing", async () => {
    await act(async () => {
      root.render(
        <WorkflowAiDrawer
          open
          context={{ workflowId: "workflow", workflowName: "Redis" }}
          plan={{
            id: "plan",
            items: [{
              id: "item",
              title: "识别工作流对象与元数据",
              description: "读取 runner_candidate_candidate_pack_1778718004043520000_redis_dry_run 的元数据、历史运行记录及图层拓扑。",
            }],
          }}
        />,
      );
    });

    const drawer = container.querySelector<HTMLElement>('[data-testid="workflow-ai-drawer"]');
    const planRow = container.querySelector<HTMLElement>(".workflow-ai-plan-row");
    expect(drawer).not.toBeNull();
    expect(planRow).not.toBeNull();
    expect(getComputedStyle(planRow as HTMLElement).whiteSpace).not.toBe("nowrap");
    expect(getComputedStyle(planRow as HTMLElement).overflowWrap).toBe("anywhere");

    const handle = container.querySelector<HTMLElement>('[data-testid="workflow-ai-resize-handle"]');
    expect(handle).not.toBeNull();
    Object.defineProperty(window, "innerWidth", { configurable: true, value: 1200 });
    await act(async () => {
      handle?.dispatchEvent(new PointerEvent("pointerdown", { bubbles: true, clientX: 780, pointerId: 1 }));
      window.dispatchEvent(new PointerEvent("pointermove", { bubbles: true, clientX: 650, pointerId: 1 }));
      window.dispatchEvent(new PointerEvent("pointerup", { bubbles: true, clientX: 650, pointerId: 1 }));
    });
    expect(drawer?.style.width).toBe("550px");
    expect(localStorage.getItem("runner.workflowAi.drawerWidth")).toBe("550");
  });

  it("keeps plan review inside the chat flow and waits for composer reply", async () => {
    const onSubmit = vi.fn();
    await act(async () => {
      root.render(
        <WorkflowAiDrawer
          open
          context={{ workflowId: "workflow", workflowName: "Redis" }}
          plan={{ id: "plan", items: [{ id: "item", title: "Add verify" }] }}
          onSubmit={onSubmit}
        />,
      );
    });

    expect(container.querySelector('[data-testid="workflow-ai-plan-card"]')?.textContent).toContain("回复「确认」开始");
    expect(container.textContent).not.toContain("确认计划并开始修改");

    const textarea = container.querySelector("textarea") as HTMLTextAreaElement;
    await act(async () => {
      const setter = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")?.set;
      setter?.call(textarea, "确认");
      textarea.dispatchEvent(new Event("input", { bubbles: true }));
    });
    await act(async () => {
      Array.from(container.querySelectorAll("button")).find((button) => button.textContent?.includes("Send"))?.click();
    });
    expect(onSubmit).toHaveBeenCalledWith("确认");
    expect(container.textContent).not.toContain("Start");
    expect(container.textContent).not.toContain("Apply");
    expect(container.querySelector('[data-testid="workflow-ai-permission-dialog"]')).toBeNull();
  });

  it("replaces the active plan card when the plan is revised", async () => {
    await act(async () => {
      root.render(
        <WorkflowAiDrawer
          open
          context={{ workflowId: "workflow", workflowName: "Redis" }}
          plan={{ id: "plan-1", items: [{ id: "backup", title: "执行备份", description: "执行 pg_dump。" }] }}
        />,
      );
    });

    await act(async () => {
      root.render(
        <WorkflowAiDrawer
          open
          context={{ workflowId: "workflow", workflowName: "Redis" }}
          plan={{ id: "plan-2", items: [{ id: "disk", title: "检查磁盘空间", description: "先检查备份目录。" }] }}
        />,
      );
    });

    const planCards = container.querySelectorAll('[data-testid="workflow-ai-plan-card"]');
    expect(planCards).toHaveLength(1);
    expect(planCards[0].textContent).toContain("检查磁盘空间");
    expect(planCards[0].textContent).not.toContain("执行备份");
  });

  it("submits with Enter and preserves Shift Enter for multiline input", async () => {
    const onSubmit = vi.fn();
    await act(async () => {
      root.render(<WorkflowAiDrawer open context={{ workflowName: "Redis" }} onSubmit={onSubmit} />);
    });
    const textarea = container.querySelector("textarea") as HTMLTextAreaElement;
    const setter = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")?.set;

    await act(async () => {
      setter?.call(textarea, "确认");
      textarea.dispatchEvent(new Event("input", { bubbles: true }));
      textarea.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true }));
    });
    expect(onSubmit).toHaveBeenCalledWith("确认");

    await act(async () => {
      setter?.call(textarea, "第一行");
      textarea.dispatchEvent(new Event("input", { bubbles: true }));
      textarea.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", shiftKey: true, bubbles: true }));
    });
    expect(onSubmit).toHaveBeenCalledTimes(1);
  });

  it("shows result undo, non-effect, budget pause and opens events from the footer", async () => {
    const onUndo = vi.fn();
    const onContinue = vi.fn();
    const onOpenEvents = vi.fn();
    await act(async () => {
      root.render(
        <WorkflowAiDrawer
          open
          context={{ workflowId: "workflow", workflowName: "Redis" }}
          stage="budget_paused"
          patch={{ id: "patch", summary: "No change", operations: [{ op: "update_node" }] }}
          effectStatus="no_effect"
          result={{ patchId: "patch", effect: { status: "changed", affectedNodes: ["collect"] }, undoCheckpoint: { id: "undo" } }}
          toolLog={[{ id: "tool", toolName: "workflow.apply_patch", status: "completed", traceId: "trace" }]}
          onUndo={onUndo}
          onContinue={onContinue}
          onOpenEvents={onOpenEvents}
        />,
      );
    });

    expect(container.querySelector('[data-testid="workflow-ai-non-effect-card"]')?.textContent).toContain("no_effect");
    expect(container.querySelector('[data-testid="workflow-ai-budget-card"]')?.textContent).toContain("Continue next batch");
    await act(async () => {
      Array.from(container.querySelectorAll("button")).find((button) => button.textContent === "撤销")?.click();
    });
    expect(onUndo).toHaveBeenCalledTimes(1);
    await act(async () => {
      Array.from(container.querySelectorAll("button")).find((button) => button.textContent?.includes("Continue next batch"))?.click();
    });
    expect(onContinue).toHaveBeenCalledTimes(1);
    await act(async () => {
      Array.from(container.querySelectorAll("button")).find((button) => button.textContent?.includes("事件"))?.click();
    });
    expect(onOpenEvents).toHaveBeenCalledTimes(1);
    expect(container.textContent).not.toContain("Copy log");
  });

  it("scrolls the chat transcript to the latest output", async () => {
    const scrollTo = vi.fn();
    Object.defineProperty(HTMLElement.prototype, "scrollTo", { configurable: true, value: scrollTo });
    await act(async () => {
      root.render(
        <WorkflowAiDrawer
          open
          context={{ workflowId: "workflow", workflowName: "Redis" }}
          readonlyAnswer="第一条回复"
        />,
      );
    });
    await act(async () => {
      root.render(
        <WorkflowAiDrawer
          open
          context={{ workflowId: "workflow", workflowName: "Redis" }}
          readonlyAnswer="第二条回复"
        />,
      );
    });
    expect(scrollTo).toHaveBeenCalled();
    expect(scrollTo.mock.calls.at(-1)?.[0]).toMatchObject({ top: expect.any(Number), behavior: "smooth" });
  });
});
