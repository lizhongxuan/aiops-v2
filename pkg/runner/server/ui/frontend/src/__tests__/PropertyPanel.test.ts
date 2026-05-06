// @vitest-environment jsdom
import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import PropertyPanel from "../components/PropertyPanel.vue";
import type { GraphDiffSummary } from "../utils/graphDiff";
import type { ActionSpec, WorkflowDefinition, WorkflowNode, WorkflowSummary } from "../types/workflow";

const actions: ActionSpec[] = [
  {
    action: "cmd.run",
    title: "Command",
    category: "command",
    node_type: "action",
    required_args: ["cmd"],
    args_schema: {
      type: "object",
      required: ["cmd"],
      properties: {
        cmd: { type: "string", title: "Command" },
        env: { type: "object", title: "Environment" },
      },
    },
  },
];

const workflow: WorkflowDefinition = {
  version: "v0.1",
  name: "parent-flow",
  inventory: {
    groups: {
      app: { hosts: ["app-01"], vars: { capabilities: ["cmd.run"] } },
    },
  },
};

const workflows: WorkflowSummary[] = [
  { name: "restore-verify", version: "v3" },
  { name: "parent-flow", version: "v1" },
];

describe("PropertyPanel", () => {
  it("renders action schema fields with inventory-aware target controls", () => {
    const node: WorkflowNode = {
      id: "probe",
      type: "action",
      position: { x: 0, y: 0 },
      step: { name: "probe", action: "cmd.run", targets: ["app"], args: { cmd: "uptime" } },
    };

    const wrapper = mountPanel(node);

    const labels = wrapper.findAll(".form-item").map((item) => item.attributes("data-label"));
    expect(labels).toContain("Targets");
    expect(labels).toContain("Command");
    expect(labels).toContain("Environment");
    expect(wrapper.text()).toContain("Action arguments");
  });

  it("emits one graph node patch for subflow workflow selection and input vars", async () => {
    const node: WorkflowNode = {
      id: "child",
      type: "subflow",
      position: { x: 0, y: 0 },
      step: { name: "child", action: "workflow.run", args: {} },
    };

    const wrapper = mountPanel(node);
    const select = wrapper.findComponent({ name: "NSelect" });
    await select.vm.$emit("update:value", "restore-verify");

    const editor = wrapper.findComponent({ name: "CodeEditor" });
    await editor.vm.$emit("update:modelValue", '{ "backup_id": "${vars.backup_id}" }');

    const events = wrapper.emitted("update-node") || [];
    expect(events[0]).toEqual([
      "child",
      expect.objectContaining({
        subflow: { workflow_name: "restore-verify" },
        step: expect.objectContaining({
          action: "workflow.run",
          args: { workflow: "restore-verify" },
        }),
      }),
    ]);
    expect(events[1]).toEqual([
      "child",
      expect.objectContaining({
        subflow: { vars: { backup_id: "${vars.backup_id}" } },
        step: expect.objectContaining({
          action: "workflow.run",
          args: { vars: { backup_id: "${vars.backup_id}" } },
        }),
      }),
    ]);
  });

  it("renders runtime state and YAML diff review inside the property panel", () => {
    const node: WorkflowNode = {
      id: "probe",
      type: "action",
      position: { x: 0, y: 0 },
      step: { name: "probe", action: "cmd.run", args: { cmd: "uptime" } },
      state: {
        status: "failed",
        message: "disk_free check failed",
        started_at: "2026-05-03T00:00:00Z",
        finished_at: "2026-05-03T00:00:05Z",
        hosts: {
          "pg-primary": { status: "failed", stderr: "disk full" },
        },
      },
    };
    const diffSummary: GraphDiffSummary = {
      changed: true,
      sections: [
        { kind: "execution", title: "Execution semantics", changed: true, paths: ["nodes.probe.step.args.cmd"] },
        { kind: "layout", title: "Layout", changed: false, paths: [] },
        { kind: "metadata", title: "Metadata", changed: false, paths: [] },
      ],
    };

    const wrapper = mountPanel(node, { diffSummary });

    expect(wrapper.text()).toContain("Run state");
    expect(wrapper.text()).toContain("disk_free check failed");
    expect(wrapper.text()).toContain("pg-primary");
    expect(wrapper.text()).toContain("YAML diff");
    expect(wrapper.text()).toContain("Execution semantics");
    expect(wrapper.text()).toContain("nodes.probe.step.args.cmd");
  });
});

function mountPanel(node: WorkflowNode, extraProps: Partial<InstanceType<typeof PropertyPanel>["$props"]> = {}) {
  return mount(PropertyPanel, {
    props: {
      node,
      actions,
      workflow,
      workflows,
      ...extraProps,
    },
    global: {
      stubs: {
        Alert: { template: "<div><slot /></div>" },
        Descriptions: { template: "<div><slot /></div>" },
        DescriptionsItem: { props: ["label"], template: '<div class="description-item"><slot /></div>' },
        DynamicTags: { props: ["value"], emits: ["update:value"], template: '<div class="dynamic-tags"></div>' },
        Empty: { template: "<div />" },
        Form: { template: "<form><slot /></form>" },
        FormItem: { props: ["label", "validationStatus", "feedback"], template: '<div class="form-item" :data-label="label"><slot /></div>' },
        Input: { props: ["value"], emits: ["update:value"], template: '<input :value="value" @input="$emit(\'update:value\', $event.target.value)" />' },
        InputNumber: { props: ["value"], emits: ["update:value"], template: '<input :value="value" type="number" />' },
        Select: { name: "NSelect", props: ["value", "options"], emits: ["update:value"], template: '<select class="select"></select>' },
        Switch: { props: ["value"], emits: ["update:value"], template: '<button type="button"></button>' },
        TabPane: { props: ["tab"], template: '<section><span>{{ tab }}</span><slot /></section>' },
        Tabs: { template: "<div><slot /></div>" },
        Tag: { template: "<span><slot /></span>" },
        NAlert: { template: "<div><slot /></div>" },
        NDescriptions: { template: "<div><slot /></div>" },
        NDescriptionsItem: { props: ["label"], template: '<div class="description-item"><slot /></div>' },
        NDynamicTags: { props: ["value"], emits: ["update:value"], template: '<div class="dynamic-tags"></div>' },
        NEmpty: { template: "<div />" },
        NForm: { template: "<form><slot /></form>" },
        NFormItem: { props: ["label", "validationStatus", "feedback"], template: '<div class="form-item" :data-label="label"><slot /></div>' },
        NInput: { props: ["value"], emits: ["update:value"], template: '<input :value="value" @input="$emit(\'update:value\', $event.target.value)" />' },
        NInputNumber: { props: ["value"], emits: ["update:value"], template: '<input :value="value" type="number" />' },
        NSelect: { name: "NSelect", props: ["value", "options"], emits: ["update:value"], template: '<select class="select"></select>' },
        NSwitch: { props: ["value"], emits: ["update:value"], template: '<button type="button"></button>' },
        NTabPane: { props: ["tab"], template: '<section><span>{{ tab }}</span><slot /></section>' },
        NTabs: { template: "<div><slot /></div>" },
        NTag: { template: "<span><slot /></span>" },
        CodeEditor: { name: "CodeEditor", props: ["modelValue"], emits: ["update:modelValue"], template: "<pre>{{ modelValue }}</pre>" },
      },
    },
  });
}
