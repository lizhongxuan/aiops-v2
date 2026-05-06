import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import ConditionNodePanel from "./ConditionNodePanel.vue";

describe("ConditionNodePanel", () => {
  it("edits IF and ELIF conditions as structured node data", async () => {
    const wrapper = mount(ConditionNodePanel, {
      props: {
        node: {
          id: "gate",
          type: "condition",
          step: { name: "gate", action: "condition.evaluate", args: {} },
        },
      },
    });

    await wrapper.get('[data-testid="condition-if-expression"]').setValue("vars.disk_free == true");
    await wrapper.get('[data-testid="condition-add-elif"]').trigger("click");
    await wrapper.get('[data-testid="condition-elif-expression-0"]').setValue("vars.force == true");

    const emitted = wrapper.emitted("update:node")?.at(-1)?.[0];
    expect(emitted.condition).toEqual({
      if: "vars.disk_free == true",
      elif: [{ expression: "vars.force == true" }],
      else: true,
    });
    expect(emitted.step.args.expression).toBe("vars.disk_free == true");
  });
});
