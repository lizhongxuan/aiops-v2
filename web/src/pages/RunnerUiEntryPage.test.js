import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import RunnerUiEntryPage from "./RunnerUiEntryPage.vue";

describe("RunnerUiEntryPage", () => {
  it("marks the standalone Runner UI entry as legacy debug only", () => {
    const legacyAddress = ["127.0.0.1", "8090"].join(":");
    const openLegacyText = ["打开 Runner", "UI"].join(" ");
    const startLegacyText = ["启动 Runner", "UI"].join(" ");
    const wrapper = mount(RunnerUiEntryPage, {
      global: {
        stubs: {
          ExternalLinkIcon: true,
          TerminalIcon: true,
          WorkflowIcon: true,
        },
      },
    });

    expect(wrapper.text()).toContain("Legacy Debug UI");
    expect(wrapper.text()).toContain("主应用产品入口已统一到 /runner 的 Runner Studio");
    expect(wrapper.text()).toContain("可迁移代码清单");
    expect(wrapper.text()).not.toContain(legacyAddress);
    expect(wrapper.text()).not.toContain(openLegacyText);
    expect(wrapper.text()).not.toContain(startLegacyText);
    expect(wrapper.find('[data-testid="runner-open-link"]').exists()).toBe(false);
  });
});
