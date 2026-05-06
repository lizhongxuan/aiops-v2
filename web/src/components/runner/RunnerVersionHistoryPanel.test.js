import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import RunnerVersionHistoryPanel from "./RunnerVersionHistoryPanel.vue";

const versions = [
  {
    id: "v-2",
    status: "published",
    reason: "publish",
    save_note: "ready",
    checksum: "sha256:222",
    created_at: "2026-05-05T09:00:00Z",
    yaml: "name: pg-restore\nsteps:\n- name: restore\n",
  },
  {
    id: "v-1",
    status: "draft",
    reason: "create",
    checksum: "sha256:111",
    created_at: "2026-05-05T08:00:00Z",
    yaml: "name: pg-restore\nsteps: []\n",
  },
];

describe("RunnerVersionHistoryPanel", () => {
  it("shows version history, previews YAML before restore, and emits restore/export/import actions", async () => {
    const wrapper = mount(RunnerVersionHistoryPanel, {
      props: {
        show: true,
        workflowName: "pg-restore",
        currentYaml: "name: pg-restore\nsteps:\n- name: current\n",
        versions,
        exportText: "",
      },
    });

    expect(wrapper.get('[data-testid="runner-version-history-panel"]').text()).toContain("pg-restore");
    expect(wrapper.text()).toContain("v-2");
    expect(wrapper.text()).toContain("ready");

    await wrapper.get('[data-testid="runner-version-preview-v-1"]').trigger("click");
    expect(wrapper.get('[data-testid="runner-version-preview"]').text()).toContain("steps: []");
    expect(wrapper.get('[data-testid="runner-version-current-preview"]').text()).toContain("current");

    await wrapper.get('[data-testid="runner-version-rollback-v-1"]').trigger("click");
    await wrapper.get('[data-testid="runner-version-export"]').trigger("click");
    await wrapper.get('[data-testid="runner-version-import-mode"]').setValue("yaml");
    await wrapper.get('[data-testid="runner-version-import-text"]').setValue("name: imported\nsteps: []\n");
    await wrapper.get('[data-testid="runner-version-import-submit"]').trigger("click");

    expect(wrapper.emitted("rollback")?.[0]).toEqual(["v-1"]);
    expect(wrapper.emitted("export-bundle")?.[0]).toEqual(["pg-restore"]);
    expect(wrapper.emitted("import-workflow")?.[0]).toEqual([
      {
        mode: "yaml",
        text: "name: imported\nsteps: []\n",
        overwrite: false,
      },
    ]);
  });
});
