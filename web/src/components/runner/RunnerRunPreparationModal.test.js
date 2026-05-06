import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import RunnerRunPreparationModal from "./RunnerRunPreparationModal.vue";

describe("RunnerRunPreparationModal", () => {
  it("explains missing configuration before run", () => {
    const wrapper = mount(RunnerRunPreparationModal, {
      props: {
        show: true,
        workflowName: "检查主机资源",
        readiness: {
          ready: false,
          blockers: [{ code: "required_arg_missing", message: "节点 check 缺少必填参数：script" }],
        },
      },
    });

    expect(wrapper.get('[data-testid="runner-run-preparation-modal"]').text()).toContain("运行准备检查");
    expect(wrapper.text()).toContain("暂时不能运行");
    expect(wrapper.text()).toContain("缺少必填参数");
  });

  it("explains server dependency in local draft mode", () => {
    const wrapper = mount(RunnerRunPreparationModal, {
      props: {
        show: true,
        mode: "dry-run",
        readiness: { ready: true, blockers: [] },
        serverReason: "Runner API upstream 尚未配置",
      },
    });

    expect(wrapper.text()).toContain("Dry Run 准备检查");
    expect(wrapper.text()).toContain("Runner API upstream 尚未配置");
    expect(wrapper.text()).toContain("保存草稿可以继续使用本地模式");
  });
});
