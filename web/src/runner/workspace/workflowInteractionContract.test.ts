import { describe, expect, it } from "vitest";
import {
  CONTEXT_DOCK_DESKTOP_TABS,
  CONTEXT_DOCK_MOBILE_MODES,
  WORKFLOW_ARIA_CONTRACT,
  WORKFLOW_INTERACTION_COPY,
  WORKFLOW_INTERACTION_SEMANTICS,
  WORKFLOW_INTERACTION_TEST_IDS,
  WORKFLOW_KEYBOARD_COMMANDS,
  deriveWorkflowTruthAnswers,
  isWorkflowLifecycleAction,
  validateWorkflowInteractionAxes,
  type WorkflowInteractionAxes,
} from "./workflowInteractionContract";

const settled: WorkflowInteractionAxes = {
  taskState: "settled",
  candidateState: "none",
  approvalState: "none",
  connectionState: "online",
  validationState: "valid",
  lifecycleState: "draft",
};

describe("workflow interaction acceptance contract", () => {
  it("freezes user-facing vocabulary and responsive navigation", () => {
    expect(Object.values(WORKFLOW_INTERACTION_COPY)).toEqual(
      expect.arrayContaining([
        "正在准备修改",
        "待审阅",
        "应用修改",
        "已保存",
        "待检查",
        "还需配置",
        "已发布",
        "已保存但有问题",
        "需要解决冲突",
      ]),
    );
    expect(CONTEXT_DOCK_DESKTOP_TABS.map((item) => item.label)).toEqual(["AI 协作", "变更", "运行"]);
    expect(CONTEXT_DOCK_DESKTOP_TABS.map((item) => String(item.id))).not.toContain("configuration");
    expect(CONTEXT_DOCK_MOBILE_MODES.map((item) => item.label)).toEqual(["画布", "AI", "变更", "运行"]);
    expect(CONTEXT_DOCK_MOBILE_MODES.map((item) => String(item.id))).not.toContain("configuration");
  });

  it("defines exact typed targets and never classifies utility actions as lifecycle actions", () => {
    expect(isWorkflowLifecycleAction("check")).toBe(true);
    expect(isWorkflowLifecycleAction("dry_run")).toBe(true);
    expect(isWorkflowLifecycleAction("publish")).toBe(true);
    expect(isWorkflowLifecycleAction("publish_changes")).toBe(true);
    expect(isWorkflowLifecycleAction("run_published")).toBe(true);
    expect(isWorkflowLifecycleAction("undo")).toBe(false);
    expect(isWorkflowLifecycleAction("redo")).toBe(false);
  });

  it("keeps connection orthogonal but rejects impossible transactional combinations", () => {
    expect(validateWorkflowInteractionAxes({ ...settled, connectionState: "offline", candidateState: "ready" })).toEqual([]);
    expect(
      validateWorkflowInteractionAxes({
        ...settled,
        taskState: "applying",
        candidateState: "none",
        approvalState: "approved",
      }),
    ).toContain("applying_requires_candidate");
    expect(validateWorkflowInteractionAxes({ ...settled, approvalState: "requested", candidateState: "none" })).toContain(
      "approval_requires_candidate",
    );
  });

  it("answers saved, checked and runnable questions without exposing hashes", () => {
    expect(deriveWorkflowTruthAnswers({ ...settled, candidateState: "ready" })).toEqual({
      savedVersion: "画布包含待应用修改；已保存版本没有改变",
      checked: "已保存版本已检查，可以运行",
      runnable: "先应用或放弃待审阅修改，再运行已保存版本",
    });
    expect(deriveWorkflowTruthAnswers({ ...settled, validationState: "configuration_required" }).checked).toBe(
      "结构已检查，还需配置后才能运行",
    );
  });

  it("freezes cross-surface, candidate editing and Follow AI semantics", () => {
    expect(WORKFLOW_INTERACTION_SEMANTICS.scopeChip).toContain("发送消息时冻结");
    expect(WORKFLOW_INTERACTION_SEMANTICS.followAi).toContain("暂停");
    expect(WORKFLOW_INTERACTION_SEMANTICS.candidateEdit).toContain("previewHash");
    expect(WORKFLOW_INTERACTION_SEMANTICS.conflict).toContain("三方");
  });

  it("defines stable selectors and keyboard/ARIA equivalents for the critical path", () => {
    expect(WORKFLOW_INTERACTION_TEST_IDS.contextDock).toBe("workflow-context-dock");
    expect(WORKFLOW_INTERACTION_TEST_IDS.primaryAction).toBe("workflow-primary-action");
    expect(WORKFLOW_INTERACTION_TEST_IDS.changeItem("change-7")).toBe("workflow-change-item-change-7");

    const commandIds = new Set(WORKFLOW_KEYBOARD_COMMANDS.map((command) => command.id));
    expect(commandIds).toEqual(
      new Set(["search", "add_node", "delete_selection", "undo", "redo", "fit", "open_ai", "open_issues", "apply_changes"]),
    );
    expect(WORKFLOW_KEYBOARD_COMMANDS.every((command) => command.ariaLabel.length > 0)).toBe(true);
    expect(WORKFLOW_ARIA_CONTRACT.progressLiveRegion.politeness).toBe("polite");
    expect(WORKFLOW_ARIA_CONTRACT.blockingErrorLiveRegion.politeness).toBe("assertive");
    expect(WORKFLOW_ARIA_CONTRACT.outline.readsInternalIds).toBe(false);
  });
});
