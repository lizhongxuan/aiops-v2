import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import RunnerReadinessModal from "./RunnerReadinessModal.vue";

describe("RunnerReadinessModal", () => {
  it("shows blockers and emits close", async () => {
    const wrapper = mount(RunnerReadinessModal, {
      props: {
        show: true,
        result: {
          ready: false,
          blockers: [{ code: "required_arg_missing", message: "缺少 script", nodeId: "n1" }],
          warnings: [],
          infos: [],
        },
      },
    });

    expect(wrapper.get('[data-testid="runner-readiness-modal"]').text()).toContain("校验未通过");
    expect(wrapper.text()).toContain("缺少 script");

    await wrapper.get('[data-testid="runner-readiness-close"]').trigger("click");
    expect(wrapper.emitted("close")).toHaveLength(1);
  });

  it("explains local validation when the server is unavailable", () => {
    const wrapper = mount(RunnerReadinessModal, {
      props: {
        show: true,
        serverReason: "Runner API upstream 尚未配置",
        result: {
          ready: true,
          blockers: [],
          warnings: [],
          infos: [{ code: "failure_defaults_to_exit_path", message: "失败未设置时默认退出。" }],
        },
      },
    });

    expect(wrapper.text()).toContain("本地校验通过");
    expect(wrapper.text()).toContain("Runner API upstream 尚未配置");
    expect(wrapper.text()).toContain("失败未设置时默认退出");
  });
});
