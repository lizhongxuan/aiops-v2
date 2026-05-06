import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import ApprovalNodePanel from "./ApprovalNodePanel.vue";

describe("ApprovalNodePanel", () => {
  it("edits approvers, timeout, timeout policy and risk reason", async () => {
    const wrapper = mount(ApprovalNodePanel, {
      props: {
        node: {
          id: "approve",
          type: "manual_approval",
          step: { name: "approve", action: "manual.approval", args: {} },
        },
      },
    });

    await wrapper.get('[data-testid="approval-subjects"]').setValue("oncall, dba");
    await wrapper.get('[data-testid="approval-timeout"]').setValue("20m");
    await wrapper.get('[data-testid="approval-on-timeout"]').setValue("reject");
    await wrapper.get('[data-testid="approval-risk-reason"]').setValue("生产恢复前必须审批");

    const emitted = wrapper.emitted("update:node")?.at(-1)?.[0];
    expect(emitted.approval).toEqual({
      subjects: ["oncall", "dba"],
      timeout: "20m",
      on_timeout: "reject",
      risk_reason: "生产恢复前必须审批",
    });
    expect(emitted.step.args.subjects).toEqual(["oncall", "dba"]);
  });
});
