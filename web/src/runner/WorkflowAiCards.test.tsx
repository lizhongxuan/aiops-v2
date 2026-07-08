import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { WorkflowAiStepGenerationCard, WorkflowAiToolTimeline, WorkflowContextCard, WorkflowEditPlanCard, WorkflowManualCandidateCard, WorkflowPatchPreviewCard, WorkflowPatchResultCard } from "./WorkflowAiCards";

describe("WorkflowAiCards", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
  });

  it("renders context, patch, result and tool timeline as structured cards", async () => {
    await act(async () => {
      root.render(
        <>
          <WorkflowContextCard context={{ workflowId: "workflow", workflowName: "Redis", revision: "rev", saveState: "saved", selectedNodeId: "collect", validation: { valid: true } }} />
          <WorkflowPatchPreviewCard patch={{ id: "patch", summary: "Rename", operations: [{ op: "update_node" }] }} effectStatus="changed" />
          <WorkflowPatchResultCard result={{ patchId: "patch", effect: { status: "changed", affectedNodes: ["collect"] }, describe: { summary: "done" } }} />
          <WorkflowAiToolTimeline entries={[{ id: "tool", toolName: "workflow.describe", status: "completed", durationMs: 8, traceId: "trace", inputSummary: "读取画布", outputSummary: "已识别 2 个节点" }]} />
        </>,
      );
    });

    expect(container.querySelector('[data-testid="workflow-ai-context-card"]')?.textContent).toContain("Redis");
    expect(container.querySelector('[data-testid="workflow-ai-patch-card"]')?.textContent).toContain("Rename");
    expect(container.querySelector('[data-testid="workflow-ai-result-card"]')?.textContent).toContain("collect");
    const toolTimeline = container.querySelector('[data-testid="workflow-ai-tool-timeline"]');
    expect(toolTimeline?.textContent).toContain("执行过程");
    expect(toolTimeline?.textContent).toContain("读取画布");
    expect(toolTimeline?.textContent).toContain("已完成");
    expect(toolTimeline?.textContent).toContain("已识别 2 个节点");
    expect(toolTimeline?.textContent).not.toContain("completed8mstrace");
  });

  it("renders manual candidate details for review without marking verified", async () => {
    await act(async () => {
      root.render(
        <WorkflowManualCandidateCard
          candidate={{
            candidateId: "candidate",
            manualId: "manual",
            title: "Redis memory manual",
            reviewStatus: "pending",
            operationType: "redis.remediate",
            riskLevel: "medium",
            workflowId: "workflow-redis",
            workflowDigest: "sha256:test",
            requiredEvidence: ["memory_usage"],
            cannotConditions: ["cluster failover in progress"],
            preflightSummary: "check role",
            verifySummary: "memory recovered",
            rollbackSummary: "restore config",
            staleBinding: true,
          }}
        />,
      );
    });

    const text = container.querySelector('[data-testid="workflow-ai-manual-card"]')?.textContent || "";
    expect(text).toContain("pending");
    expect(text).toContain("redis.remediate");
    expect(text).toContain("sha256:test");
    expect(text).toContain("stale");
    expect(text).not.toContain("verified");
  });

  it("renders plan review as a conversational prompt without action buttons", async () => {
    await act(async () => {
      root.render(
        <WorkflowEditPlanCard
          plan={{
            id: "plan",
            message: "生成 pgBackRest 备份工作流",
            items: [
              { id: "step-1", title: "识别备份对象", description: "确认 PostgreSQL 实例和 pgBackRest stanza。" },
              { id: "step-2", title: "环境预检", description: "检查命令、磁盘和连接。" },
            ],
          }}
        />,
      );
    });
    const text = container.querySelector('[data-testid="workflow-ai-plan-card"]')?.textContent || "";
    expect(text).toContain("修改计划");
    expect(text).toContain("回复「确认」开始");
    expect(text).toContain("说明要调整的步骤");
    expect(text).not.toContain("修改第 2 步");
    expect(container.querySelector("button")).toBeNull();
  });

  it("renders step generation details without dumping full scripts", async () => {
    await act(async () => {
      root.render(
        <WorkflowAiStepGenerationCard
          step={{
            index: 2,
            total: 7,
            title: "环境预检",
            goal: "确认 pgBackRest 和 PostgreSQL 连接可用。",
            environment: ["runner target", "read-only shell"],
            scriptSummary: "检查 pgbackrest --version、pgbackrest info、df -h。",
            inputVariables: [{ name: "pg_host", type: "string", required: true }],
            outputVariables: [{ name: "preflight_result", type: "object" }],
            validationSummary: "静态校验通过",
          }}
          status="completed"
        />,
      );
    });
    const text = container.textContent || "";
    expect(text).toContain("环境预检");
    expect(text).toContain("pg_host:string 必填");
    expect(text).toContain("preflight_result:object");
    expect(text).toContain("静态校验通过");
  });
});
