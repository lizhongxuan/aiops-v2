import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import WorkflowQuickList from "./WorkflowQuickList.vue";

const workflows = [
  { name: "pg-restore", title: "PG Restore", status: "published" },
  { name: "cache-warmup", title: "Cache Warmup", status: "draft" },
  { name: "erp-health-check", title: "ERP Health Check", status: "validated" },
];

describe("WorkflowQuickList", () => {
  it("shows only recent and favorite workflows in the sidebar", () => {
    const wrapper = mount(WorkflowQuickList, {
      props: {
        workflows,
        selectedWorkflowName: "pg-restore",
        uiState: {
          recent: ["pg-restore"],
          favorites: ["erp-health-check"],
        },
      },
    });

    expect(wrapper.text()).toContain("PG Restore");
    expect(wrapper.text()).toContain("ERP Health Check");
    expect(wrapper.text()).not.toContain("Cache Warmup");
  });

  it("emits selection, favorite, manager, and create intents without editing graph data", async () => {
    const wrapper = mount(WorkflowQuickList, {
      props: {
        workflows,
        selectedWorkflowName: "pg-restore",
        uiState: {
          recent: ["pg-restore"],
          favorites: [],
        },
      },
    });

    await wrapper.get('[data-testid="runner-workflow-pg-restore"]').trigger("click");
    await wrapper.get('[data-testid="runner-favorite-pg-restore"]').trigger("click");
    await wrapper.get('[data-testid="runner-open-manager"]').trigger("click");
    await wrapper.get('[data-testid="runner-create-workflow"]').trigger("click");

    expect(wrapper.emitted("select")?.[0]).toEqual(["pg-restore"]);
    expect(wrapper.emitted("toggle-favorite")?.[0]).toEqual(["pg-restore"]);
    expect(wrapper.emitted("open-manager")).toHaveLength(1);
    expect(wrapper.emitted("create")).toHaveLength(1);
    expect(workflows[0]).not.toHaveProperty("favorite");
  });
});
