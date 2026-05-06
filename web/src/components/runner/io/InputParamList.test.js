import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import InputParamList from "./InputParamList.vue";

const variables = [
  { scope: "workflow_input", name: "backup_id", path: "workflow_input.backup_id" },
  { scope: "node_output", node_id: "pre-check", name: "disk_free", path: "nodes.pre-check.outputs.disk_free" },
];

describe("InputParamList", () => {
  it("edits key, label, type, value_source, value, required, and description", async () => {
    const wrapper = mount(InputParamList, {
      props: {
        params: [
          {
            key: "script",
            label: "Script",
            type: "string",
            required: true,
            description: "old",
            value_source: { type: "constant", value: "./restore.sh" },
          },
        ],
        variables,
      },
    });

    await wrapper.get('[data-testid="input-key-script"]').setValue("backup_id");
    await wrapper.get('[data-testid="input-label-backup_id"]').setValue("Backup ID");
    await wrapper.get('[data-testid="input-type-backup_id"]').setValue("number");
    await wrapper.get('[data-testid="input-source-backup_id"]').setValue("expression");
    await wrapper.get('[data-testid="input-expression-backup_id"]').setValue("vars.backup_id + 1");
    await wrapper.get('[data-testid="input-required-backup_id"]').setValue(false);
    await wrapper.get('[data-testid="input-description-backup_id"]').setValue("new desc");

    const emitted = wrapper.emitted("update:params")?.at(-1)?.[0];
    expect(emitted[0]).toMatchObject({
      key: "backup_id",
      label: "Backup ID",
      type: "number",
      required: false,
      description: "new desc",
      value_source: { type: "expression", expression: "vars.backup_id + 1" },
    });
  });

  it("sorts, copies, deletes, and shows duplicate key validation", async () => {
    const wrapper = mount(InputParamList, {
      props: {
        params: [
          { key: "script", value_source: { type: "constant", value: "" } },
          { key: "script", value_source: { type: "constant", value: "" } },
        ],
        variables,
      },
    });

    expect(wrapper.text()).toContain("输入参数 key 重复");

    await wrapper.get('[data-testid="input-copy-script-0"]').trigger("click");
    expect(wrapper.emitted("update:params")?.at(-1)?.[0]).toHaveLength(3);

    await wrapper.get('[data-testid="input-move-down-script-0"]').trigger("click");
    expect(wrapper.emitted("update:params")?.at(-1)?.[0][1].key).toBe("script");

    await wrapper.get('[data-testid="input-delete-script-0"]').trigger("click");
    expect(wrapper.emitted("update:params")?.at(-1)?.[0]).toHaveLength(2);
  });
});
