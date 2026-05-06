import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import NotifyNodePanel from "./NotifyNodePanel.vue";

describe("NotifyNodePanel", () => {
  it("edits channel, template, recipients and failure policy", async () => {
    const wrapper = mount(NotifyNodePanel, {
      props: {
        node: {
          id: "notify",
          type: "action",
          step: { name: "notify", action: "notify.send", args: {} },
        },
      },
    });

    await wrapper.get('[data-testid="notify-channel"]').setValue("slack");
    await wrapper.get('[data-testid="notify-recipients"]').setValue("sre, dba");
    await wrapper.get('[data-testid="notify-template"]').setValue("恢复失败: {{ node.restore.stderr }}");
    await wrapper.get('[data-testid="notify-on-failure"]').setValue("continue");

    const emitted = wrapper.emitted("update:node")?.at(-1)?.[0];
    expect(emitted.step.args).toMatchObject({
      channel: "slack",
      recipients: ["sre", "dba"],
      template: "恢复失败: {{ node.restore.stderr }}",
      on_failure: "continue",
    });
  });
});
