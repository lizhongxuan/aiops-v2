import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import VariableInspectDrawer from "./VariableInspectDrawer.vue";

const state = {
  variables: {
    inputs: [{ nodeId: "restore", key: "backup_id", value: "b-1" }],
    outputs: [{ nodeId: "restore", key: "restore_lsn", value: "0/16B6C50" }],
    exports: [{ key: "promoted", value: false }],
    nodeResults: [{ nodeId: "restore", result: { exit_code: 0, duration_ms: 42000 } }],
  },
};

describe("VariableInspectDrawer", () => {
  it("shows input variables, output variables, runtime exports, and recent node results", () => {
    const wrapper = mount(VariableInspectDrawer, {
      props: { state, selectedNodeId: "restore" },
    });

    expect(wrapper.text()).toContain("输入变量");
    expect(wrapper.text()).toContain("backup_id");
    expect(wrapper.text()).toContain("b-1");
    expect(wrapper.text()).toContain("输出变量");
    expect(wrapper.text()).toContain("restore_lsn");
    expect(wrapper.text()).toContain("0/16B6C50");
    expect(wrapper.text()).toContain("运行态导出变量");
    expect(wrapper.text()).toContain("promoted");
    expect(wrapper.text()).toContain("false");
    expect(wrapper.text()).toContain("最近节点结果");
    expect(wrapper.text()).toContain("exit_code");
  });
});
