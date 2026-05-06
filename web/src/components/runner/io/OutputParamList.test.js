import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import OutputParamList from "./OutputParamList.vue";

describe("OutputParamList", () => {
  it("edits key, type, extract_source, extract_rule, and description", async () => {
    const wrapper = mount(OutputParamList, {
      props: {
        outputs: [
          {
            key: "restore_lsn",
            type: "string",
            description: "old",
            extract_source: { type: "stdout_text", path: "" },
          },
        ],
      },
    });

    await wrapper.get('[data-testid="output-key-restore_lsn"]').setValue("duration_ms");
    await wrapper.get('[data-testid="output-type-duration_ms"]').setValue("number");
    await wrapper.get('[data-testid="output-source-duration_ms"]').setValue("stdout_jsonpath");
    await wrapper.get('[data-testid="jsonpath-duration_ms"]').setValue("$.duration_ms");
    await wrapper.get('[data-testid="output-description-duration_ms"]').setValue("duration");

    const emitted = wrapper.emitted("update:outputs")?.at(-1)?.[0];
    expect(emitted[0]).toMatchObject({
      key: "duration_ms",
      type: "number",
      description: "duration",
      extract_source: { type: "stdout_jsonpath", path: "$.duration_ms" },
    });
  });

  it("validates allowed extract sources, jsonpath rules, duplicates, and writes outputs back", async () => {
    const wrapper = mount(OutputParamList, {
      props: {
        outputs: [
          { key: "restore_lsn", extract_source: { type: "stdout_jsonpath", path: "duration_ms" } },
          { key: "restore_lsn", extract_source: { type: "secret_file", path: "/tmp/secret" } },
        ],
      },
    });

    expect(wrapper.text()).toContain("输出变量 key 重复");
    expect(wrapper.text()).toContain("extract_source 不支持");
    expect(wrapper.text()).toContain("JSONPath 必须以 $ 开头");

    await wrapper.get('[data-testid="output-add"]').trigger("click");
    expect(wrapper.emitted("update:outputs")?.at(-1)?.[0]).toHaveLength(3);

    await wrapper.get('[data-testid="output-delete-restore_lsn-0"]').trigger("click");
    expect(wrapper.emitted("update:outputs")?.at(-1)?.[0]).toHaveLength(2);
  });
});
