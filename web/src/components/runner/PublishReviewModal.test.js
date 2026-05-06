import { flushPromises, mount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { publishRunnerStudioWorkflow } from "../../api/runnerStudioClient";
import PublishReviewModal from "./PublishReviewModal.vue";

vi.mock("../../api/runnerStudioClient", () => ({
  publishRunnerStudioWorkflow: vi.fn(),
}));

const workflow = {
  name: "pg-restore",
  status: "dry_run_passed",
  validated_graph_hash: "graph-hash-1",
  dry_run_graph_hash: "graph-hash-1",
};

const diffSummary = {
  semantic_changes: [{ title: "restore action", detail: "shell.run" }],
  layout_changes: [{ title: "restore position", detail: "x=320 y=120" }],
};

const riskSummary = {
  level: "medium",
  items: ["shell.run touches database primary"],
};

const validationResult = {
  valid: true,
  warnings: ["dry-run warning acknowledged"],
};

describe("PublishReviewModal", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    publishRunnerStudioWorkflow.mockResolvedValue({
      name: "pg-restore",
      status: "published",
      published_graph_hash: "graph-hash-1",
    });
  });

  it("shows diff, risk summary, validation result, and publish note", async () => {
    const wrapper = mount(PublishReviewModal, {
      props: { show: true, workflow, diffSummary, riskSummary, validationResult },
    });

    expect(wrapper.text()).toContain("restore action");
    expect(wrapper.text()).toContain("shell.run");
    expect(wrapper.text()).toContain("restore position");
    expect(wrapper.text()).toContain("medium");
    expect(wrapper.text()).toContain("shell.run touches database primary");
    expect(wrapper.text()).toContain("dry-run warning acknowledged");

    await wrapper.get('[data-testid="publish-note"]').setValue("change window approved");
    expect(wrapper.get('[data-testid="publish-note"]').element.value).toBe("change window approved");
  });

  it("disables publish when validated_graph_hash is missing", () => {
    const wrapper = mount(PublishReviewModal, {
      props: {
        show: true,
        workflow: { ...workflow, validated_graph_hash: "" },
        diffSummary,
        riskSummary,
        validationResult,
      },
    });

    expect(wrapper.get('[data-testid="publish-confirm"]').attributes("disabled")).toBeDefined();
    expect(wrapper.text()).toContain("缺少当前 validated_graph_hash");
  });

  it("disables publish when dry-run has not passed for the current graph", async () => {
    const wrapper = mount(PublishReviewModal, {
      props: {
        show: true,
        workflow: { ...workflow, dry_run_graph_hash: "" },
        diffSummary,
        riskSummary,
        validationResult,
      },
    });

    await wrapper.get('[data-testid="publish-note"]').setValue("change window approved");

    expect(wrapper.get('[data-testid="publish-confirm"]').attributes("disabled")).toBeDefined();
    expect(wrapper.text()).toContain("Dry Run 未通过或已过期");
  });

  it("blocks publish when validation errors are present", async () => {
    const wrapper = mount(PublishReviewModal, {
      props: {
        show: true,
        workflow,
        diffSummary,
        riskSummary,
        validationResult: { valid: false, errors: ["missing edge"], warnings: [] },
      },
    });

    await wrapper.get('[data-testid="publish-note"]').setValue("change window approved");

    expect(wrapper.get('[data-testid="publish-confirm"]').attributes("disabled")).toBeDefined();
    expect(wrapper.text()).toContain("校验未通过");
  });

  it("requires explicit risk and warning acknowledgement before publishing guarded changes", async () => {
    const wrapper = mount(PublishReviewModal, {
      props: {
        show: true,
        workflow,
        diffSummary,
        riskSummary: { level: "high", items: ["shell.run touches production"] },
        validationResult,
      },
    });

    await wrapper.get('[data-testid="publish-note"]').setValue("change window approved");
    expect(wrapper.get('[data-testid="publish-confirm"]').attributes("disabled")).toBeDefined();
    expect(wrapper.text()).toContain("高风险发布必须确认");

    await wrapper.get('[data-testid="publish-risk-acknowledged"]').setValue(true);
    expect(wrapper.get('[data-testid="publish-confirm"]').attributes("disabled")).toBeDefined();
    expect(wrapper.text()).toContain("校验警告必须确认");

    await wrapper.get('[data-testid="publish-warning-acknowledged"]').setValue(true);
    expect(wrapper.get('[data-testid="publish-confirm"]').attributes("disabled")).toBeUndefined();
  });

  it("requires human confirmation before publishing AI drafts", async () => {
    const wrapper = mount(PublishReviewModal, {
      props: {
        show: true,
        workflow: { ...workflow, ai_generated_draft: true },
        diffSummary,
        riskSummary,
        validationResult: { ...validationResult, warnings: [] },
      },
    });

    expect(wrapper.get('[data-testid="publish-confirm"]').attributes("disabled")).toBeDefined();
    await wrapper.get('[data-testid="ai-draft-confirmed"]').setValue(true);
    expect(wrapper.get('[data-testid="publish-confirm"]').attributes("disabled")).toBeDefined();
    await wrapper.get('[data-testid="publish-note"]').setValue("AI draft reviewed");
    expect(wrapper.get('[data-testid="publish-confirm"]').attributes("disabled")).toBeUndefined();
  });

  it("requires a publish note before enabling publish", async () => {
    const wrapper = mount(PublishReviewModal, {
      props: { show: true, workflow, diffSummary, riskSummary, validationResult: { ...validationResult, warnings: [] } },
    });

    expect(wrapper.get('[data-testid="publish-confirm"]').attributes("disabled")).toBeDefined();
    expect(wrapper.text()).toContain("发布说明不能为空");
    await wrapper.get('[data-testid="publish-note"]').setValue("change window approved");
    expect(wrapper.get('[data-testid="publish-confirm"]').attributes("disabled")).toBeUndefined();
  });

  it("publishes with review evidence and emits the published workflow status", async () => {
    const wrapper = mount(PublishReviewModal, {
      props: { show: true, workflow, diffSummary, riskSummary, validationResult },
    });

    await wrapper.get('[data-testid="publish-note"]').setValue("change window approved");
    await wrapper.get('[data-testid="publish-warning-acknowledged"]').setValue(true);
    await wrapper.get('[data-testid="publish-confirm"]').trigger("click");
    await flushPromises();

    expect(publishRunnerStudioWorkflow).toHaveBeenCalledWith("pg-restore", {
      save_note: "change window approved",
      validated_graph_hash: "graph-hash-1",
      dry_run_graph_hash: "graph-hash-1",
      diff: diffSummary,
      risk_summary: riskSummary,
      validation_result: validationResult,
      ai_draft_confirmed: false,
      risk_acknowledged: false,
      warning_acknowledged: true,
    });
    expect(wrapper.emitted("published")?.[0]?.[0]).toMatchObject({ status: "published" });
  });
});
