import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import RunnerSchemaEditor from "./RunnerSchemaEditor.vue";

describe("RunnerSchemaEditor", () => {
  it("edits workflow input schema including default, required, description, and secret", async () => {
    const wrapper = mount(RunnerSchemaEditor, {
      props: {
        mode: "inputs",
        inputs: [
          {
            key: "backup_id",
            type: "string",
            required: true,
            default: "base-backup",
            description: "old",
          },
        ],
      },
    });

    await wrapper.get('[data-testid="input-key-backup_id"]').setValue("db_password");
    await wrapper.get('[data-testid="input-type-db_password"]').setValue("secret");
    await wrapper.get('[data-testid="input-required-db_password"]').setValue(false);
    await wrapper.get('[data-testid="input-description-db_password"]').setValue("Database password");
    await wrapper.get('[data-testid="schema-input-secret-db_password"]').setValue(true);

    const emitted = wrapper.emitted("update:inputs")?.at(-1)?.[0];
    expect(emitted[0]).toMatchObject({
      key: "db_password",
      type: "secret",
      required: false,
      description: "Database password",
      secret: true,
    });
    expect(emitted[0]).not.toHaveProperty("default", "base-backup");
  });

  it("edits node output schema including example and extract source", async () => {
    const wrapper = mount(RunnerSchemaEditor, {
      props: {
        mode: "outputs",
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
    await wrapper.get('[data-testid="schema-output-example-duration_ms"]').setValue("42000");
    await wrapper.get('[data-testid="output-source-duration_ms"]').setValue("stdout_jsonpath");
    await wrapper.get('[data-testid="schema-output-source-path-duration_ms"]').setValue("$.duration_ms");
    await wrapper.get('[data-testid="output-description-duration_ms"]').setValue("Restore duration");

    const emitted = wrapper.emitted("update:outputs")?.at(-1)?.[0];
    expect(emitted[0]).toMatchObject({
      key: "duration_ms",
      type: "number",
      description: "Restore duration",
      extract_source: { type: "stdout_jsonpath", path: "$.duration_ms" },
      ui: { example: "42000" },
    });
  });

  it("adds and validates duplicate schema keys", async () => {
    const wrapper = mount(RunnerSchemaEditor, {
      props: {
        mode: "inputs",
        inputs: [{ key: "backup_id" }, { key: "backup_id" }],
      },
    });

    expect(wrapper.text()).toContain("key 重复");

    await wrapper.get('[data-testid="input-add"]').trigger("click");
    expect(wrapper.emitted("update:inputs")?.at(-1)?.[0]).toHaveLength(3);
  });
});
