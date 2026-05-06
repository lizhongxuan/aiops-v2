import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import VariableReferencePicker from "./VariableReferencePicker.vue";

const variables = [
  { scope: "workflow_input", name: "backup_id", path: "workflow_input.backup_id" },
  { scope: "node_output", node_id: "pre-check", name: "disk_free", path: "nodes.pre-check.outputs.disk_free" },
];

describe("VariableReferencePicker", () => {
  it("allows selecting only variables from server resolved scope", async () => {
    const wrapper = mount(VariableReferencePicker, {
      props: {
        modelValue: null,
        variables,
      },
    });

    expect(wrapper.text()).toContain("workflow_input.backup_id");
    expect(wrapper.text()).toContain("nodes.pre-check.outputs.disk_free");
    expect(wrapper.text()).not.toContain("local.private");

    await wrapper.get('[data-testid="variable-option-nodes.pre-check.outputs.disk_free"]').trigger("click");

    expect(wrapper.emitted("update:modelValue")?.[0]).toEqual([
      {
        scope: "node_output",
        node_id: "pre-check",
        name: "disk_free",
        path: "nodes.pre-check.outputs.disk_free",
      },
    ]);
  });
});
