import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import BasicTab from "./BasicTab.vue";

const actions = [{ action: "cmd.run" }, { action: "shell.run" }];

describe("BasicTab", () => {
  it("edits action node name, action, targets, retries, and timeout", async () => {
    const node = {
      id: "restore",
      type: "action",
      step: {
        name: "restore",
        action: "cmd.run",
        targets: ["pg-01"],
        retries: 1,
        timeout: "30s",
      },
    };
    const wrapper = mount(BasicTab, {
      props: {
        node,
        actions,
      },
    });

    await wrapper.get('[data-testid="basic-name"]').setValue("restore-primary");
    await wrapper.get('[data-testid="basic-action"]').setValue("shell.run");
    await wrapper.get('[data-testid="action-target-add"]').trigger("click");
    await wrapper.get('[data-testid="action-target-value-1"]').setValue("pg-02");
    await wrapper.get('[data-testid="basic-retries"]').setValue("3");
    await wrapper.get('[data-testid="basic-timeout"]').setValue("15m");

    const emitted = wrapper.emitted("update:node")?.at(-1)?.[0];
    expect(emitted).toMatchObject({
      id: "restore",
      step: {
        name: "restore-primary",
        action: "shell.run",
        targets: ["pg-01", "pg-02"],
        retries: 3,
        timeout: "15m",
      },
    });
    expect(node.step.name).toBe("restore");
  });
});
