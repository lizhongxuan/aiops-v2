import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import RunnerVariableTokenInput from "./RunnerVariableTokenInput.vue";

const variables = [
  {
    scope: "input",
    name: "backup_id",
    expression: "input.backup_id",
    path: "input.backup_id",
    selector: { scope: "input", name: "backup_id" },
    type: "string",
  },
  {
    scope: "node",
    name: "restore_lsn",
    expression: "node.restore.restore_lsn",
    path: "node.restore.restore_lsn",
    selector: { scope: "node", nodeId: "restore", name: "restore_lsn" },
    sourceNodeId: "restore",
    type: "string",
  },
];

describe("RunnerVariableTokenInput", () => {
  it("inserts selected variables as Runner expression tokens", async () => {
    const wrapper = mount(RunnerVariableTokenInput, {
      props: {
        modelValue: "echo ",
        variables,
        inputTestId: "script-field",
      },
    });

    await wrapper.get('[data-testid="runner-variable-selector-search"]').setValue("restore");
    await wrapper.get('[data-testid="runner-variable-option-node.restore.restore_lsn"]').trigger("click");

    expect(wrapper.emitted("update:modelValue")?.at(-1)?.[0]).toBe("echo ${node.restore.restore_lsn}");
    expect(wrapper.get('[data-testid="runner-variable-token-preview"]').text()).toContain("node.restore.restore_lsn");
  });

  it("marks stale tokens and emits locate events for source node tokens", async () => {
    const wrapper = mount(RunnerVariableTokenInput, {
      props: {
        modelValue: "restore=${node.restore.restore_lsn} missing=${node.restore.deleted}",
        variables,
        inputTestId: "script-field",
      },
    });

    expect(wrapper.get('[data-testid="runner-variable-token-preview"]').text()).toContain("missing");
    expect(wrapper.get('[data-testid="runner-variable-token-warning-node.restore.deleted"]').text()).toContain("已失效");

    await wrapper.get('[data-testid="runner-variable-token-node.restore.restore_lsn"]').trigger("click");

    expect(wrapper.emitted("locate-node")?.[0]).toEqual(["restore"]);
  });
});
