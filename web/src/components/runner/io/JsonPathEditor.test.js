import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import JsonPathEditor from "./JsonPathEditor.vue";

describe("JsonPathEditor", () => {
  it("reports field-level errors for invalid jsonpath and emits valid rules", async () => {
    const wrapper = mount(JsonPathEditor, {
      props: {
        modelValue: "duration_ms",
        testId: "jsonpath-duration_ms",
      },
    });

    expect(wrapper.text()).toContain("JSONPath 必须以 $ 开头");

    await wrapper.get('[data-testid="jsonpath-duration_ms"]').setValue("$.duration_ms");

    expect(wrapper.text()).not.toContain("JSONPath 必须以 $ 开头");
    expect(wrapper.emitted("update:modelValue")?.[0]).toEqual(["$.duration_ms"]);
  });
});
