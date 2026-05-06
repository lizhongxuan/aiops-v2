import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import ActionNodePanel from "./ActionNodePanel.vue";

const actions = [
  {
    action: "shell.run",
    title: "Shell Script",
    inputs_schema: {
      type: "object",
      required: ["script"],
      properties: {
        script: { type: "string", title: "Script", description: "Shell script content." },
        export_vars: { type: "boolean", title: "Export Vars" },
        env: { type: "object", title: "Environment" },
      },
    },
  },
];

const node = {
  id: "restore",
  type: "action",
  step: { name: "restore", action: "shell.run", args: { script: "echo old" } },
};

describe("ActionNodePanel", () => {
  it("renders action catalog schema fields and writes them to step args", async () => {
    const wrapper = mount(ActionNodePanel, {
      props: { node, actions },
    });

    expect(wrapper.get('[data-testid="action-node-panel"]').text()).toContain("Shell Script");
    expect(wrapper.get('[data-testid="basic-name"]').element.value).toBe("restore");

    await wrapper.get('[data-testid="action-schema-field-script"]').setValue("echo restore");
    await wrapper.get('[data-testid="action-schema-field-export_vars"]').setValue(true);
    const emitted = wrapper.emitted("update:node")?.at(-1)?.[0];
    expect(emitted.step.args).toEqual({
      script: "echo restore",
      export_vars: true,
    });
  });

  it("edits action environment variables as rows instead of raw JSON", async () => {
    const wrapper = mount(ActionNodePanel, {
      props: { node, actions },
    });

    await wrapper.get('[data-testid="action-env-add"]').trigger("click");
    await wrapper.get('[data-testid="action-env-key-0"]').setValue("PGDATA");
    await wrapper.get('[data-testid="action-env-value-0"]').setValue("/data/pg");
    await wrapper.get('[data-testid="action-env-add"]').trigger("click");
    await wrapper.get('[data-testid="action-env-key-1"]').setValue("PGPORT");
    await wrapper.get('[data-testid="action-env-value-1"]').setValue("5432");

    let emitted = wrapper.emitted("update:node")?.at(-1)?.[0];
    expect(emitted.step.args.env).toEqual({
      PGDATA: "/data/pg",
      PGPORT: "5432",
    });

    await wrapper.get('[data-testid="action-env-delete-0"]').trigger("click");

    emitted = wrapper.emitted("update:node")?.at(-1)?.[0];
    expect(emitted.step.args.env).toEqual({ PGPORT: "5432" });
  });

  it("allows each action node to define zero or more custom input variables", async () => {
    const wrapper = mount(ActionNodePanel, {
      props: { node, actions },
    });

    expect(wrapper.find('[data-testid="action-input-key-0"]').exists()).toBe(false);

    await wrapper.get('[data-testid="action-input-add"]').trigger("click");
    await wrapper.get('[data-testid="action-input-key-0"]').setValue("cpu_threshold");
    await wrapper.get('[data-testid="action-input-type-0"]').setValue("number");
    await wrapper.get('[data-testid="action-input-required-0"]').setValue(true);

    let emitted = wrapper.emitted("update:node")?.at(-1)?.[0];
    expect(emitted.inputs).toEqual([{ key: "cpu_threshold", type: "number", required: true }]);

    await wrapper.get('[data-testid="action-input-add"]').trigger("click");
    await wrapper.get('[data-testid="action-input-key-1"]').setValue("mount_point");

    emitted = wrapper.emitted("update:node")?.at(-1)?.[0];
    expect(emitted.inputs).toEqual([
      { key: "cpu_threshold", type: "number", required: true },
      { key: "mount_point", type: "string", required: false },
    ]);

    await wrapper.get('[data-testid="action-input-delete-0"]').trigger("click");

    emitted = wrapper.emitted("update:node")?.at(-1)?.[0];
    expect(emitted.inputs).toEqual([{ key: "mount_point", type: "string", required: false }]);
  });

  it("edits action targets as a simple list and allows clearing them", async () => {
    const wrapper = mount(ActionNodePanel, {
      props: {
        node: { ...node, step: { ...node.step, targets: ["local"] } },
        actions,
      },
    });

    expect(wrapper.get('[data-testid="action-target-value-0"]').element.value).toBe("local");

    await wrapper.get('[data-testid="action-target-add"]').trigger("click");
    await wrapper.get('[data-testid="action-target-value-1"]').setValue("pg-primary");

    let emitted = wrapper.emitted("update:node")?.at(-1)?.[0];
    expect(emitted.step.targets).toEqual(["local", "pg-primary"]);

    await wrapper.get('[data-testid="action-target-delete-0"]').trigger("click");
    await wrapper.get('[data-testid="action-target-delete-0"]').trigger("click");

    emitted = wrapper.emitted("update:node")?.at(-1)?.[0];
    expect(emitted.step).not.toHaveProperty("targets");
  });
});
