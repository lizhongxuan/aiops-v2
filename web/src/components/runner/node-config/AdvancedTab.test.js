import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import AdvancedTab from "./AdvancedTab.vue";

describe("AdvancedTab", () => {
  it("edits common execution controls without YAML", async () => {
    const wrapper = mount(AdvancedTab, {
      props: {
        node: {
          id: "restore",
          type: "action",
          step: { name: "restore", action: "shell.run", retries: 1, timeout: "30s", args: {} },
        },
      },
    });

    await wrapper.get('[data-testid="advanced-continue-on-error"]').setValue(true);
    await wrapper.get('[data-testid="advanced-rollback"]').setValue("rollback-restore");
    await wrapper.get('[data-testid="advanced-secrets"]').setValue("PGPASSWORD, SSH_KEY");

    const emitted = wrapper.emitted("update:node")?.at(-1)?.[0];
    expect(emitted.step.continue_on_error).toBe(true);
    expect(emitted.step.args.rollback).toBe("rollback-restore");
    expect(emitted.step.args.secrets).toEqual(["PGPASSWORD", "SSH_KEY"]);
  });
});
