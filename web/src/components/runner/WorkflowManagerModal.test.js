import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import WorkflowManagerModal from "./WorkflowManagerModal.vue";

const workflows = [
  { name: "pg-restore", title: "PG Restore", status: "published" },
  { name: "cache-warmup", title: "Cache Warmup", status: "draft" },
  { name: "erp-health-check", title: "ERP Health Check", status: "validated" },
  { name: "legacy-archived", title: "Legacy Archived", status: "archived", archived: true },
];

describe("WorkflowManagerModal", () => {
  it("keeps full list, search, filters, archive, clone, and versions inside the modal", async () => {
    const wrapper = mount(WorkflowManagerModal, {
      props: {
        show: true,
        workflows,
        selectedWorkflowName: "pg-restore",
        uiState: { recent: [], favorites: [] },
      },
    });

    expect(wrapper.get('[data-testid="workflow-manager-modal"]').text()).toContain("Cache Warmup");
    expect(wrapper.get('[data-testid="workflow-manager-search"]').exists()).toBe(true);
    expect(wrapper.get('[data-testid="workflow-manager-status-filter"]').exists()).toBe(true);

    await wrapper.get('[data-testid="workflow-manager-search"]').setValue("erp");
    expect(wrapper.text()).toContain("ERP Health Check");
    expect(wrapper.text()).not.toContain("PG Restore");

    await wrapper.get('[data-testid="workflow-manager-include-archived"]').setValue(true);
    await wrapper.get('[data-testid="workflow-manager-search"]').setValue("legacy");
    expect(wrapper.text()).toContain("Legacy Archived");

    await wrapper.get('[data-testid="workflow-clone-legacy-archived"]').trigger("click");
    await wrapper.get('[data-testid="workflow-archive-legacy-archived"]').trigger("click");
    await wrapper.get('[data-testid="workflow-versions-legacy-archived"]').trigger("click");

    expect(wrapper.emitted("clone-workflow")?.[0]).toEqual(["legacy-archived"]);
    expect(wrapper.emitted("archive-workflow")?.[0]).toEqual(["legacy-archived"]);
    expect(wrapper.emitted("view-versions")?.[0]).toEqual(["legacy-archived"]);
  });

  it("requires a name for blank workflow creation and keeps utility create modes available", async () => {
    const wrapper = mount(WorkflowManagerModal, {
      props: {
        show: true,
        workflows,
        selectedWorkflowName: "pg-restore",
        uiState: { recent: [], favorites: [] },
      },
    });

    await wrapper.get('[data-testid="workflow-create-blank"]').trigger("click");
    expect(wrapper.get('[data-testid="workflow-create-form"]').text()).toContain("工作流名称");
    await wrapper.get('[data-testid="workflow-create-name"]').setValue("检查主机资源");
    await wrapper.get('[data-testid="workflow-create-submit"]').trigger("click");

    for (const mode of ["yaml", "clone", "ai"]) {
      await wrapper.get(`[data-testid="workflow-create-${mode}"]`).trigger("click");
    }

    expect(wrapper.emitted("create-workflow")).toEqual([
      [
        {
          mode: "blank",
          name: "检查主机资源",
          slug: "host-resource-check",
          title: "检查主机资源",
        },
      ],
      ["yaml"],
      ["clone"],
      ["ai"],
    ]);
  });

  it("requests dirty confirmation before switching workflows", async () => {
    const wrapper = mount(WorkflowManagerModal, {
      props: {
        show: true,
        workflows,
        selectedWorkflowName: "pg-restore",
        uiState: { recent: [], favorites: [] },
        dirty: true,
      },
    });

    await wrapper.get('[data-testid="workflow-select-cache-warmup"]').trigger("click");

    expect(wrapper.emitted("request-dirty-confirm")?.[0]).toEqual(["cache-warmup"]);
    expect(wrapper.emitted("select")).toBeUndefined();
  });
});
