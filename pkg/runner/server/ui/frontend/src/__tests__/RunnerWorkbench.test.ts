// @vitest-environment jsdom
import { mount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { defineComponent } from "vue";
import RunnerWorkbench from "../components/RunnerWorkbench.vue";
import type { WorkflowGraph } from "../types/workflow";

const graph: WorkflowGraph = {
  version: "v1",
  workflow: { version: "v1.2.3", name: "pg-restore" },
  nodes: [],
  edges: [],
};

const store = {
  state: {
    graph,
    baselineGraph: graph,
    actions: [],
    workflowOptions: [{ name: "pg-restore", version: "v1.2.3", status: "draft" }],
    workflowVersions: [],
    selectedNodeId: null,
    loading: false,
    saving: false,
    publishing: false,
    loadingVersions: false,
    rollingBack: false,
    creatingWorkflow: false,
    switchingWorkflow: false,
    exportingBundle: false,
    importingBundle: false,
    validating: false,
    dryRunning: false,
    submitting: false,
    canceling: false,
    resolvingApprovalNodeId: null,
    resolvingApprovalAction: null,
    replaying: false,
    previewCompiling: false,
    offline: false,
    dirty: false,
    eventConnected: true,
    error: null,
    saveNote: "",
    riskAcknowledged: false,
    warningAcknowledged: false,
    semanticChangeAcknowledged: false,
    workflowStatus: "draft",
    publishedAt: "",
    validation: { valid: true, errors: [], warnings: [] },
    dryRun: null,
    yamlPreview: "",
    run: {
      runId: "run-20260503",
      status: "running",
      nodeStatus: {},
      edgeStatus: {},
      events: [],
      stdout: "",
      stderr: "",
      exports: {},
      hostResults: [],
    },
    historyPast: [],
    historyFuture: [],
    clipboardNode: null,
  },
  graphWithRunState: graph,
  selectedNode: null,
  executionSemanticsChanged: false,
  waitingApprovalNodes: [],
  load: vi.fn(),
  selectNode: vi.fn(),
  addActionNodeFromCatalog: vi.fn(),
  addControlNode: vi.fn(),
  updateNode: vi.fn(),
  connectNodes: vi.fn(),
  deleteSelectedNode: vi.fn(),
  copySelectedNode: vi.fn(),
  pasteNode: vi.fn(),
  undo: vi.fn(),
  redo: vi.fn(),
  autoLayout: vi.fn(),
  updateWorkflow: vi.fn(),
  saveDraft: vi.fn(),
  publishWorkflow: vi.fn(),
  validateGraph: vi.fn(),
  dryRunGraph: vi.fn(),
  submitRun: vi.fn(),
  cancelRun: vi.fn(),
  exportWorkflowBundle: vi.fn(),
  importWorkflowBundle: vi.fn(),
  loadWorkflowVersions: vi.fn(),
  rollbackWorkflowVersion: vi.fn(),
  createWorkflowFromGraph: vi.fn(),
  switchWorkflow: vi.fn(),
  cloneCurrentWorkflow: vi.fn(),
  replayRunHistory: vi.fn(),
  approveNode: vi.fn(),
  rejectNode: vi.fn(),
  replaceGraph: vi.fn(),
  compilePreview: vi.fn(),
  importGraphYAML: vi.fn(),
};

vi.mock("../stores/graphStore", () => ({
  useGraphStore: () => store,
}));

vi.mock("lucide-vue-next", () => {
  const Icon = defineComponent({ name: "IconStub", template: "<span />" });
  return {
    CheckCircle2: Icon,
    Copy: Icon,
    Download: Icon,
    FileCode2: Icon,
    FileUp: Icon,
    History: Icon,
    Play: Icon,
    Plus: Icon,
    RefreshCw: Icon,
    RotateCcw: Icon,
    Save: Icon,
    Send: Icon,
    Square: Icon,
    UploadCloud: Icon,
  };
});

describe("RunnerWorkbench", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    store.state.graph = graph;
    store.state.baselineGraph = graph;
    store.state.workflowOptions = [{ name: "pg-restore", version: "v1.2.3", status: "draft" }];
    store.state.dirty = false;
    store.state.creatingWorkflow = false;
    store.state.switchingWorkflow = false;
    store.state.error = null;
  });

  it("renders workflow identity, lifecycle state, and primary production actions in the topbar", () => {
    const wrapper = mount(RunnerWorkbench, {
      global: {
        stubs: {
          ActionCatalog: true,
          WorkflowCanvas: true,
          PropertyPanel: true,
          RunDrawer: true,
          YamlPreviewModal: true,
          NButton: { props: ["disabled"], template: '<button :disabled="disabled"><slot name="icon" /><slot /></button>' },
          NCheckbox: { template: "<label><slot /></label>" },
          NInput: { props: ["value"], template: '<input :value="value" />' },
          NModal: { props: ["show"], template: '<div v-if="show !== false"><slot /></div>' },
          NSelect: {
            props: ["value", "options"],
            emits: ["update:value"],
            template:
              '<select class="workflow-select" :value="value" @change="$emit(\'update:value\', $event.target.value)"><option v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</option></select>',
          },
          NTag: { template: "<span><slot /></span>" },
          NewWorkflowModal: {
            props: ["show"],
            emits: ["update:show", "create"],
            template:
              '<div v-if="show" class="new-workflow-modal"><button class="submit-new-workflow" @click="$emit(\'create\', { graph: { version: \'v1\', workflow: { version: \'v0.1\', name: \'created\' }, nodes: [], edges: [] }, labels: { source: \'visual-ui\' }, saveNote: \'initial\' })">submit new</button></div>',
          },
        },
      },
    });

    const text = wrapper.find(".topbar").text();
    expect(text).toContain("pg-restore");
    expect(text).toContain("v1.2.3");
    expect(text).toContain("draft");
    expect(text).toContain("saved");
    expect(text).toContain("running");
    expect(text).toContain("Save draft");
    expect(text).toContain("Publish");
    expect(text).toContain("Validate");
    expect(text).toContain("YAML");
    expect(text).toContain("Dry run");
    expect(text).toContain("Run");
  });

  it("exposes workflow selection and graph-native create actions", async () => {
    store.state.workflowOptions = [
      { name: "pg-restore", version: "v1.2.3", status: "draft" },
      { name: "other-flow", version: "v0.1", status: "published" },
    ];
    const wrapper = mount(RunnerWorkbench, {
      global: {
        stubs: {
          ActionCatalog: true,
          WorkflowCanvas: true,
          PropertyPanel: true,
          RunDrawer: true,
          YamlPreviewModal: true,
          NButton: { props: ["disabled"], template: '<button :disabled="disabled" @click="$emit(\'click\')"><slot name="icon" /><slot /></button>' },
          NCheckbox: { template: "<label><slot /></label>" },
          NInput: { props: ["value"], template: '<input :value="value" />' },
          NModal: { props: ["show"], template: '<div v-if="show !== false"><slot /></div>' },
          NSelect: {
            props: ["value", "options"],
            emits: ["update:value"],
            template:
              '<select class="workflow-select" :value="value" @change="$emit(\'update:value\', $event.target.value)"><option v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</option></select>',
          },
          NTag: { template: "<span><slot /></span>" },
          NewWorkflowModal: {
            props: ["show"],
            emits: ["update:show", "create"],
            template:
              '<div v-if="show" class="new-workflow-modal"><button class="submit-new-workflow" @click="$emit(\'create\', { graph: { version: \'v1\', workflow: { version: \'v0.1\', name: \'created\' }, nodes: [], edges: [] }, labels: { source: \'visual-ui\' }, saveNote: \'initial\' })">submit new</button></div>',
          },
        },
      },
    });

    expect(wrapper.find(".workflow-select").exists()).toBe(true);
    expect(wrapper.find(".topbar").text()).toContain("New");
    expect(wrapper.find(".topbar").text()).toContain("Clone");
    expect(wrapper.find(".topbar").text()).toContain("Import Bundle");

    await wrapper.findAll("button").find((button) => button.text().includes("New"))?.trigger("click");
    expect(wrapper.find(".new-workflow-modal").exists()).toBe(true);
    await wrapper.find(".submit-new-workflow").trigger("click");

    expect(store.createWorkflowFromGraph).toHaveBeenCalledWith(
      expect.objectContaining({ workflow: expect.objectContaining({ name: "created" }) }),
      { labels: { source: "visual-ui" }, saveNote: "initial" },
    );
  });

  it("asks for confirmation before switching away from unsaved workflow changes", async () => {
    store.state.dirty = true;
    store.state.workflowOptions = [
      { name: "pg-restore", version: "v1.2.3", status: "draft" },
      { name: "other-flow", version: "v0.1", status: "published" },
    ];
    const wrapper = mount(RunnerWorkbench, {
      global: {
        stubs: {
          ActionCatalog: true,
          WorkflowCanvas: true,
          PropertyPanel: true,
          RunDrawer: true,
          YamlPreviewModal: true,
          NewWorkflowModal: true,
          NButton: { props: ["disabled"], template: '<button :disabled="disabled" @click="$emit(\'click\')"><slot name="icon" /><slot /></button>' },
          NCheckbox: { template: "<label><slot /></label>" },
          NInput: { props: ["value"], template: '<input :value="value" />' },
          NModal: { props: ["show"], template: '<div v-if="show !== false"><slot /></div>' },
          NSelect: {
            props: ["value", "options"],
            emits: ["update:value"],
            template:
              '<select class="workflow-select" :value="value" @change="$emit(\'update:value\', $event.target.value)"><option v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</option></select>',
          },
          NTag: { template: "<span><slot /></span>" },
        },
      },
    });

    await (wrapper.vm as unknown as { requestWorkflowSwitch: (name: string) => void }).requestWorkflowSwitch("other-flow");
    await wrapper.vm.$nextTick();

    expect(store.switchWorkflow).not.toHaveBeenCalled();

    await (wrapper.vm as unknown as { confirmWorkflowSwitch: () => Promise<void> }).confirmWorkflowSwitch();

    expect(store.switchWorkflow).toHaveBeenCalledWith("other-flow", { force: true });
    store.state.dirty = false;
  });
});
