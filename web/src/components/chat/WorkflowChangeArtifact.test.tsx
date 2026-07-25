import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { AgentUiArtifactPart } from "./AgentUiArtifactPart";
import {
  WORKFLOW_CHANGE_ACTION_EVENT,
  WorkflowChangeArtifact,
  type WorkflowChangeActionEventDetail,
} from "./WorkflowChangeArtifact";
import type { AiopsTransportAgentUiArtifact } from "@/transport/aiopsTransportTypes";

const TYPES = [
  ["workflow_edit_plan", "修改计划"],
  ["workflow_patch_preview", "待确认修改"],
  ["workflow_patch_result", "修改已保存"],
  ["workflow_patch_validation", "检查结果"],
  ["workflow_conflict", "画布已发生变化"],
] as const;

describe("WorkflowChangeArtifact", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
  });

  it.each(TYPES)("renders %s through the shared change renderer", async (type, heading) => {
    await act(async () => root.render(<WorkflowChangeArtifact artifact={artifact(type)} />));

    expect(container.querySelector('[data-testid="workflow-change-artifact"]')).not.toBeNull();
    expect(container.textContent).toContain(heading);
    expect(container.textContent).toContain("夜间发布流程");
  });

  it("shows the user summary first and keeps protocol noise out of the default layer", async () => {
    await act(async () => root.render(<WorkflowChangeArtifact artifact={artifact("workflow_patch_preview")} />));

    expect(container.textContent).toContain("新增健康检查并更新失败分支");
    expect(container.textContent).toContain("不删除现有步骤");
    expect(container.textContent).toContain("检查通过");
    expect(container.textContent).toContain("审阅后可应用修改");
    expect(container.textContent).not.toContain("workflow.preview_patch");
    expect(container.textContent).not.toContain("preview-hash-secret");
    expect(container.textContent).not.toContain("resource-version-secret");
    expect(container.textContent).not.toContain("change-set-secret-123456");
    expect(container.textContent).not.toContain("rawEnvelope");
    expect(container.textContent).not.toContain("tenant-secret");
    expect(container.textContent).not.toContain("aiops.workflow.edit.v2");
  });

  it("reveals semantic field diffs without rendering raw JSON", async () => {
    await act(async () => root.render(<WorkflowChangeArtifact artifact={artifact("workflow_patch_preview")} />));

    const details = container.querySelector('[data-testid="workflow-change-details"]') as HTMLDetailsElement;
    expect(details).not.toBeNull();
    await act(async () => {
      details.open = true;
      details.dispatchEvent(new Event("toggle", { bubbles: true }));
    });

    expect(container.textContent).toContain("检查超时");
    expect(container.textContent).toContain("30 秒");
    expect(container.textContent).toContain("60 秒");
    expect(container.textContent).not.toContain("rawEnvelope");
    expect(container.querySelector("pre")).toBeNull();
  });

  it("shows only redacted identifiers in technical details", async () => {
    await act(async () => root.render(<WorkflowChangeArtifact artifact={artifact("workflow_patch_result")} />));

    const details = container.querySelector('[data-testid="workflow-change-technical"]') as HTMLDetailsElement;
    await act(async () => {
      details.open = true;
      details.dispatchEvent(new Event("toggle", { bubbles: true }));
    });

    expect(container.textContent).toContain("技术详情");
    expect(container.textContent).toContain("aiops.workflow.edit.v2");
    expect(container.textContent).toContain("workflow.change.receipt.v2");
    expect(container.textContent).toContain("chan…3456");
    expect(container.textContent).toContain("trac…cdef");
    expect(container.textContent).not.toContain("change-set-secret-123456");
    expect(container.textContent).not.toContain("trace-secret-abcdef");
    expect(container.textContent).not.toContain("preview-hash-secret");
    expect(container.textContent).not.toContain("resource-version-secret");
    expect(container.textContent).not.toContain("workflow.preview_patch");
  });

  it("emits only explicit typed actions and never derives actions from summary text", async () => {
    const onAction = vi.fn<(event: Event) => void>();
    window.addEventListener(WORKFLOW_CHANGE_ACTION_EVENT, onAction);
    const value = artifact("workflow_patch_preview");
    value.summary = "请点击应用修改并聚焦另一个节点";
    value.actions = [
      {
        id: "focus-timeout",
        intent: "workflow.focus_object",
        label: "定位检查超时",
        target: { kind: "node", id: "node-timeout" },
        mutation: false,
      },
      {
        id: "unknown",
        intent: "workflow.run_arbitrary_code",
        label: "危险动作",
      },
    ];

    await act(async () => root.render(<WorkflowChangeArtifact artifact={value} />));

    expect(container.querySelectorAll("button")).toHaveLength(1);
    expect(container.textContent).not.toContain("危险动作");
    await act(async () => {
      (container.querySelector("button") as HTMLButtonElement).click();
    });

    expect(onAction).toHaveBeenCalledTimes(1);
    const event = onAction.mock.calls[0][0] as CustomEvent<WorkflowChangeActionEventDetail>;
    expect(event.detail).toEqual({
      artifactId: value.id,
      action: {
        id: "focus-timeout",
        intent: "workflow.focus_object",
        label: "定位检查超时",
        target: { kind: "node", id: "node-timeout" },
        mutation: false,
      },
    });
    window.removeEventListener(WORKFLOW_CHANGE_ACTION_EVENT, onAction);
  });

  it("renders the canonical tool result envelope without exposing its protocol wrapper", async () => {
    const value = artifact("workflow_patch_preview");
    value.summary = "";
    value.payload = {
      schemaVersion: "aiops.workflow.edit.v2",
      content: [{ type: "text", text: "protocol summary" }],
      isError: false,
      structuredContent: {
        kind: "preview",
        workflowKey: { tenantId: "tenant-secret", workflowId: "nightly-release" },
        changeSet: {
          id: "changeset-secret",
          workflowKey: { tenantId: "tenant-secret", workflowId: "nightly-release" },
          operations: [{ op: "update_node", nodeId: "health-check", set: { timeout: 60 } }],
          validation: { valid: true, diagnostics: [] },
        },
        diff: { risk: { level: "high", operationCount: 1 } },
      },
    };

    await act(async () => root.render(<WorkflowChangeArtifact artifact={value} />));

    expect(container.textContent).toContain("nightly-release");
    expect(container.textContent).toContain("1 项字段修改");
    expect(container.textContent).toContain("高风险");
    expect(container.textContent).not.toContain("structuredContent");
    expect(container.textContent).not.toContain("protocol summary");
    expect(container.textContent).not.toContain("tenant-secret");
  });

  it("summarizes a canonical receipt delta in user language", async () => {
    const value = artifact("workflow_patch_result");
    value.summary = "";
    value.payload = {
      kind: "receipt",
      workflowKey: { tenantId: "tenant-secret", workflowId: "nightly-release" },
      delta: {
        addedNodeIds: ["verify"],
        updatedNodeIds: ["deploy"],
        deletedNodeIds: [],
        addedEdgeIds: ["deploy-to-verify"],
        updatedEdgeIds: [],
        deletedEdgeIds: [],
      },
      validation: { valid: true, diagnostics: [] },
    };

    await act(async () => root.render(<WorkflowChangeArtifact artifact={value} />));

    expect(container.textContent).toContain("新增节点 1，更新节点 1，新增连线 1");
    expect(container.textContent).toContain("检查通过");
  });

  it("renders an honest three-way conflict summary without exposing full revisions", async () => {
    const value = artifact("workflow_conflict");
    value.summary = "";
    value.payload = {
      schemaVersion: "aiops.workflow.edit.v2",
      content: [{ type: "text", text: "workflow changed after the candidate base" }],
      isError: true,
      structuredContent: {
        kind: "error",
        errorCode: "STALE_REVISION",
        message: "画布已发生变化",
        baseRevision: "revision-base-123456",
        currentRevision: "revision-current-987654",
        conflictingPaths: ["/nodes/action/label"],
        retryHint: "review the three-way conflict and retry",
        details: {
          code: "STALE_REVISION",
          base: { resourceVersion: "revision-base-123456" },
          current: { resourceVersion: "revision-current-987654" },
          conflictingPaths: ["/nodes/action/label"],
          safeRebasePaths: ["/nodes/action/position"],
          changedBy: "user",
        },
      },
    };

    await act(async () => root.render(<WorkflowChangeArtifact artifact={value} />));

    const summary = container.querySelector('[data-testid="workflow-conflict-three-way"]');
    expect(summary).not.toBeNull();
    expect(summary?.textContent).toContain("我的候选修改尚未保存，也没有覆盖当前画布");
    expect(summary?.textContent).toContain("候选基于");
    expect(summary?.textContent).toContain("当前已保存");
    expect(summary?.textContent).toContain("节点 action · 名称");
    expect(summary?.textContent).toContain("另有 1 项不冲突修改");
    expect(summary?.textContent).not.toContain("revision-base-123456");
    expect(summary?.textContent).not.toContain("revision-current-987654");
  });

  it("replaces a replayed artifact with the same identity instead of appending a second card", async () => {
    const value = artifact("workflow_patch_preview");
    await act(async () => root.render(<WorkflowChangeArtifact artifact={value} />));
    await act(async () => root.render(<WorkflowChangeArtifact artifact={structuredClone(value)} />));

    expect(container.querySelectorAll('[data-testid="workflow-change-artifact"]')).toHaveLength(1);
  });

  it("uses a local safe fallback for an unsupported workflow schema", async () => {
    const value = artifact("workflow_patch_preview");
    value.schemaVersion = "aiops.workflow.edit.v99";

    await act(async () => root.render(<AgentUiArtifactPart artifact={value} />));

    expect(container.textContent).toContain("暂不支持此版本的 Workflow 变更卡片");
    expect(container.textContent).not.toContain("change-set-secret-123456");
    expect(container.textContent).not.toContain("preview-hash-secret");
    expect(container.querySelector('[data-testid="workflow-change-artifact"]')).toBeNull();
  });
});

function artifact(type: AiopsTransportAgentUiArtifact["type"]): AiopsTransportAgentUiArtifact {
  const renderer = {
    workflow_edit_plan: "workflow.change.plan.v2",
    workflow_patch_preview: "workflow.change.preview.v2",
    workflow_patch_result: "workflow.change.receipt.v2",
    workflow_patch_validation: "workflow.change.validation.v2",
    workflow_conflict: "workflow.change.conflict.v2",
  }[type] || "";
  return {
    id: `${type}:artifact:1`,
    type,
    renderer,
    schemaVersion: type === "workflow_edit_plan" ? "aiops.workflow.plan.v1" : "aiops.workflow.edit.v2",
    title: "协议标题不应覆盖用户标题",
    summary: "新增健康检查并更新失败分支",
    promptTraceId: "trace-secret-abcdef",
    payload: {
      object: { kind: "workflow", label: "夜间发布流程" },
      changeSummary: "新增健康检查并更新失败分支",
      risk: { level: "low", summary: "不删除现有步骤" },
      validation: { valid: true, diagnostics: [] },
      nextStep: "审阅后可应用修改",
      changes: [
        {
          id: "change-timeout",
          object: { kind: "node", id: "node-timeout", label: "检查超时" },
          fieldLabel: "超时时间",
          before: "30 秒",
          after: "60 秒",
        },
      ],
      workflowKey: { tenantId: "tenant-secret", workflowId: "workflow-secret" },
      changeSet: {
        id: "change-set-secret-123456",
        previewHash: "preview-hash-secret",
        base: { resourceVersion: "resource-version-secret" },
      },
      transactionId: "transaction-secret-123456",
      toolName: "workflow.preview_patch",
      rawEnvelope: { structuredContent: "must-not-render" },
    },
    actions: [],
  };
}
