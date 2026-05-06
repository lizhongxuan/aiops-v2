import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import RunnerVariableSelector from "./RunnerVariableSelector.vue";

describe("RunnerVariableSelector", () => {
  it("filters variables and flags type mismatches before selection", async () => {
    const wrapper = mount(RunnerVariableSelector, {
      props: {
        expectedType: "number",
        variables: [
          { scope: "input", name: "backup_id", expression: "input.backup_id", type: "string" },
          { scope: "node", name: "duration_ms", expression: "node.verify.duration_ms", type: "number", sourceNodeId: "verify" },
        ],
      },
    });

    await wrapper.get('[data-testid="runner-variable-selector-search"]').setValue("backup");

    expect(wrapper.text()).toContain("input.backup_id");
    expect(wrapper.text()).toContain("类型可能不匹配");
    expect(wrapper.find('[data-testid="runner-variable-option-node.verify.duration_ms"]').exists()).toBe(false);

    await wrapper.get('[data-testid="runner-variable-option-input.backup_id"]').trigger("click");
    expect(wrapper.emitted("select")?.[0]?.[0]).toMatchObject({
      expression: "input.backup_id",
      type: "string",
    });
  });
});
