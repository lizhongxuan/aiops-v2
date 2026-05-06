import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import MixedVariableTextInput from "./MixedVariableTextInput.vue";

describe("MixedVariableTextInput", () => {
  it("highlights embedded variable references while editing text", async () => {
    const wrapper = mount(MixedVariableTextInput, {
      props: {
        modelValue: "restore ${workflow_input.backup_id} on ${nodes.pre-check.outputs.disk_free}",
      },
    });

    expect(wrapper.get('[data-testid="mixed-variable-preview"]').html()).toContain("workflow_input.backup_id");
    expect(wrapper.get('[data-testid="mixed-variable-preview"]').findAll(".mixed-variable-token")).toHaveLength(2);

    await wrapper.get('[data-testid="mixed-variable-input"]').setValue("run ${workflow_input.backup_id}");

    expect(wrapper.emitted("update:modelValue")?.[0]).toEqual(["run ${workflow_input.backup_id}"]);
  });
});
