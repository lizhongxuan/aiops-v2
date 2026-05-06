import { flushPromises, mount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";
import YamlDiffModal from "./YamlDiffModal.vue";
import {
  compileRunnerStudioWorkflowGraph,
  parseRunnerStudioWorkflowYaml,
} from "../../api/runnerStudioClient";

vi.mock("../../api/runnerStudioClient", () => ({
  compileRunnerStudioWorkflowGraph: vi.fn(),
  parseRunnerStudioWorkflowYaml: vi.fn(),
}));

const graph = {
  version: "v1",
  workflow: { name: "pg-restore" },
  nodes: [{ id: "restore", type: "action", step: { name: "restore", action: "shell.run" } }],
  edges: [],
};

describe("YamlDiffModal", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    compileRunnerStudioWorkflowGraph.mockResolvedValue({
      yaml: "version: v1\nworkflow:\n  name: pg-restore\n",
      diff: {
        semantic_changes: [{ title: "step restore", detail: "shell.run" }],
        layout_changes: [],
      },
    });
    parseRunnerStudioWorkflowYaml.mockResolvedValue({
      graph: { ...graph, workflow: { name: "pg-restore-edited" } },
      diff: {
        semantic_changes: [{ title: "workflow name", detail: "pg-restore -> pg-restore-edited" }],
        layout_changes: [],
      },
    });
  });

  it("does not mount the YAML modal in the main page until explicitly opened", async () => {
    const wrapper = mount(YamlDiffModal, {
      props: { show: false, graph },
    });

    expect(wrapper.find('[data-testid="yaml-diff-modal"]').exists()).toBe(false);
    expect(compileRunnerStudioWorkflowGraph).not.toHaveBeenCalled();
  });

  it("uses the Runner Studio client for Graph to YAML and YAML to Graph", async () => {
    const wrapper = mount(YamlDiffModal, {
      props: { show: true, graph },
    });
    await flushPromises();

    expect(compileRunnerStudioWorkflowGraph).toHaveBeenCalledWith({ graph });
    expect(wrapper.get('[data-testid="yaml-editor"]').element.value).toContain("pg-restore");

    await wrapper.get('[data-testid="yaml-editor"]').setValue("version: v1\nworkflow:\n  name: pg-restore-edited\n");
    await wrapper.get('[data-testid="yaml-parse-apply"]').trigger("click");
    await flushPromises();

    expect(parseRunnerStudioWorkflowYaml).toHaveBeenCalledWith({
      yaml: "version: v1\nworkflow:\n  name: pg-restore-edited\n",
    });
    expect(wrapper.emitted("apply-graph")?.[0]?.[0]).toMatchObject({
      workflow: { name: "pg-restore-edited" },
    });
  });
});
