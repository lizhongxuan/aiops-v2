import { flushPromises, mount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { validateRunnerStudioWorkflowGraph } from "../../../api/runnerStudioClient";
import { draftRunnerWorkflowWithAI } from "./aiRunnerApi";
import RunnerAiAssistantModal from "./RunnerAiAssistantModal.vue";

vi.mock("./aiRunnerApi", () => ({
  draftRunnerWorkflowWithAI: vi.fn(),
}));

vi.mock("../../../api/runnerStudioClient", () => ({
  validateRunnerStudioWorkflowGraph: vi.fn(),
}));

const graph = {
  version: "v1",
  workflow: { name: "pg-restore" },
  nodes: [],
  edges: [],
};

const patchedGraph = {
  ...graph,
  nodes: [{ id: "restore", type: "action", step: { name: "restore", action: "shell.run" } }],
};

describe("RunnerAiAssistantModal", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    draftRunnerWorkflowWithAI.mockResolvedValue({
      graph_patch: {
        operations: [{ op: "add_node", node_id: "restore" }],
        graph: patchedGraph,
      },
      diff_summary: {
        semantic_changes: [{ title: "新增 restore 节点", detail: "shell.run" }],
        layout_changes: [{ title: "自动布局", detail: "restore x=320 y=120" }],
      },
    });
    validateRunnerStudioWorkflowGraph.mockResolvedValue({ ok: true });
  });

  it("shows AI diff before applying and validates the patched graph after apply", async () => {
    const wrapper = mount(RunnerAiAssistantModal, {
      props: {
        show: true,
        workflow: { name: "pg-restore", status: "draft" },
        graph,
      },
    });

    await wrapper.get('[data-testid="runner-ai-instruction"]').setValue("生成 PostgreSQL 恢复流程");
    await wrapper.get('[data-testid="runner-ai-generate"]').trigger("click");
    await flushPromises();

    expect(draftRunnerWorkflowWithAI).toHaveBeenCalledWith({
      workflow_name: "pg-restore",
      workflow_status: "draft",
      instruction: "生成 PostgreSQL 恢复流程",
      graph,
    });
    expect(wrapper.text()).toContain("新增 restore 节点");
    expect(wrapper.text()).toContain("shell.run");
    expect(wrapper.emitted("apply-patch")).toBeUndefined();

    await wrapper.get('[data-testid="runner-ai-apply"]').trigger("click");
    await flushPromises();

    expect(validateRunnerStudioWorkflowGraph).toHaveBeenCalledWith({ graph: patchedGraph });
    expect(wrapper.emitted("apply-patch")?.[0]?.[0]).toMatchObject({
      graph_patch: { operations: [{ op: "add_node", node_id: "restore" }] },
      graph: patchedGraph,
    });
  });

  it("does not request AI patches for non-draft workflows", async () => {
    const wrapper = mount(RunnerAiAssistantModal, {
      props: {
        show: true,
        workflow: { name: "pg-restore", status: "published" },
        graph,
      },
    });

    await wrapper.get('[data-testid="runner-ai-instruction"]').setValue("修改生产流程");
    await wrapper.get('[data-testid="runner-ai-generate"]').trigger("click");

    expect(draftRunnerWorkflowWithAI).not.toHaveBeenCalled();
    expect(wrapper.text()).toContain("只能在 draft 工作流中使用 AI patch");
  });

  it("shows AI failure explanations without modifying or validating the graph", async () => {
    draftRunnerWorkflowWithAI.mockResolvedValue({
      error_explanation: "缺少目标主机，无法生成安全 patch",
    });
    const wrapper = mount(RunnerAiAssistantModal, {
      props: {
        show: true,
        workflow: { name: "pg-restore", status: "draft" },
        graph,
      },
    });

    await wrapper.get('[data-testid="runner-ai-instruction"]').setValue("生成恢复流程");
    await wrapper.get('[data-testid="runner-ai-generate"]').trigger("click");
    await flushPromises();

    expect(wrapper.text()).toContain("缺少目标主机");
    expect(wrapper.find('[data-testid="runner-ai-apply"]').attributes("disabled")).toBeDefined();
    expect(validateRunnerStudioWorkflowGraph).not.toHaveBeenCalled();
    expect(wrapper.emitted("apply-patch")).toBeUndefined();
  });

  it("does not apply an AI patch when graph validation rejects the candidate graph", async () => {
    validateRunnerStudioWorkflowGraph.mockResolvedValue({
      valid: false,
      errors: [{ message: "AI patch validation failed" }],
    });
    const wrapper = mount(RunnerAiAssistantModal, {
      props: {
        show: true,
        workflow: { name: "pg-restore", status: "draft" },
        graph,
      },
    });

    await wrapper.get('[data-testid="runner-ai-instruction"]').setValue("生成非法 patch");
    await wrapper.get('[data-testid="runner-ai-generate"]').trigger("click");
    await flushPromises();
    await wrapper.get('[data-testid="runner-ai-apply"]').trigger("click");
    await flushPromises();

    expect(validateRunnerStudioWorkflowGraph).toHaveBeenCalledWith({ graph: patchedGraph });
    expect(wrapper.text()).toContain("AI patch validation failed");
    expect(wrapper.emitted("apply-patch")).toBeUndefined();
  });
});
