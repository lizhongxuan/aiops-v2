import { mount, flushPromises } from "@vue/test-utils";
import { createPinia, setActivePinia } from "pinia";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useAppStore } from "./store";

const mocks = vi.hoisted(() => ({
  router: { push: vi.fn(), replace: vi.fn() },
  route: { name: "chat", path: "/", query: {} },
}));

vi.mock("vue-router", () => ({
  useRouter: () => mocks.router,
  useRoute: () => mocks.route,
}));

vi.mock("lucide-vue-next", () => {
  const stub = (name) => ({ name, template: "<svg />" });
  return {
    ActivityIcon: stub("ActivityIcon"),
    AppWindowIcon: stub("AppWindowIcon"),
    ArrowLeftIcon: stub("ArrowLeftIcon"),
    EraserIcon: stub("EraserIcon"),
    FileSearchIcon: stub("FileSearchIcon"),
    HistoryIcon: stub("HistoryIcon"),
    MessageSquarePlusIcon: stub("MessageSquarePlusIcon"),
    PanelLeftCloseIcon: stub("PanelLeftCloseIcon"),
    PanelLeftOpenIcon: stub("PanelLeftOpenIcon"),
    PanelsTopLeftIcon: stub("PanelsTopLeftIcon"),
    ServerIcon: stub("ServerIcon"),
    SettingsIcon: stub("SettingsIcon"),
    TerminalIcon: stub("TerminalIcon"),
    UserCircleIcon: stub("UserCircleIcon"),
    WorkflowIcon: stub("WorkflowIcon"),
  };
});

import App from "./App.vue";

function mountApp(configureStore = () => {}) {
  const pinia = createPinia();
  setActivePinia(pinia);
  const store = useAppStore();
  store.fetchState = vi.fn(async () => true);
  store.fetchSessions = vi.fn(async () => true);
  store.connectWs = vi.fn();
  configureStore(store);

  const wrapper = mount(App, {
    shallow: true,
    global: {
      plugins: [pinia],
      stubs: {
        HostModal: true,
        LoginModal: true,
        McpBundleHost: true,
        McpUiCardHost: true,
        SessionHistoryDrawer: true,
        "router-view": true,
        "n-layout-sider": {
          template: "<div><slot /></div>",
          props: ["collapsed", "collapsedWidth", "width", "collapseMode", "showTrigger"],
        },
        "n-menu": {
          name: "NMenu",
          template: "<div />",
          props: ["value", "options", "collapsed", "collapsedWidth", "collapsedIconSize"],
          emits: ["update:value"],
        },
        "n-button": {
          template: "<button @click=\"$emit('click', $event)\"><slot name=\"icon\" /><slot /></button>",
          props: ["quaternary", "tertiary", "circle", "block", "size", "disabled", "type", "title"],
          emits: ["click"],
        },
        "n-badge": { template: "<span><slot /></span>", props: ["dot", "type", "offset"] },
        "n-config-provider": { template: "<div><slot /></div>", props: ["clsPrefix"] },
        "n-dialog-provider": { template: "<div><slot /></div>" },
        "n-message-provider": { template: "<div><slot /></div>" },
        "n-notification-provider": { template: "<div><slot /></div>" },
      },
    },
  });
  return { wrapper, store };
}

describe("App chat route session selection", () => {
  beforeEach(() => {
    Object.assign(mocks.route, { name: "chat", path: "/", query: {} });
    mocks.router.push.mockReset();
    mocks.router.replace.mockReset();
  });

  it("activates a single-host session when the chat route opens from a workspace session", async () => {
    const { store } = mountApp((store) => {
      store.loading = false;
      store.snapshot.kind = "workspace";
      store.snapshot.sessionId = "workspace-1";
      store.snapshot.selectedHostId = "server-local";
      store.snapshot.hosts = [{ id: "server-local", name: "server-local", status: "online" }];
      store.activeSessionId = "workspace-1";
      store.sessionList = [
        { id: "workspace-1", kind: "workspace", title: "workspace", selectedHostId: "server-local" },
        { id: "single-local", kind: "single_host", title: "server-local", selectedHostId: "server-local" },
      ];
      store.createOrActivateSingleHostSessionForHost = vi.fn(async () => true);
    });

    await flushPromises();

    expect(store.createOrActivateSingleHostSessionForHost).toHaveBeenCalledWith(
      "server-local",
      expect.objectContaining({ id: "server-local" }),
    );
  });

  it("does not render a return-to-workspace control in the chat header", async () => {
    const { wrapper } = mountApp((store) => {
      store.loading = false;
      store.snapshot.kind = "single_host";
      store.snapshot.sessionId = "single-local";
      store.snapshot.selectedHostId = "server-local";
      store.activeSessionId = "single-local";
      store.sessionList = [
        { id: "workspace-1", kind: "workspace", title: "workspace", selectedHostId: "server-local" },
        { id: "single-local", kind: "single_host", title: "server-local", selectedHostId: "server-local" },
      ];
      store.workspaceReturnTargets = { "single-local": "workspace-1" };
    });

    await flushPromises();

    expect(wrapper.text()).not.toContain("返回工作台");
  });

  it("exposes the Runner workflow editor entry in the main sidebar", async () => {
    const { wrapper } = mountApp();
    await flushPromises();

    const menu = wrapper.findComponent({ name: "NMenu" });
    expect(menu.props("options").map((item) => item.label)).toContain("Runner 编排");
  });

  it("routes to the Runner workflow editor entry page from the main sidebar", async () => {
    const { wrapper } = mountApp();
    await flushPromises();

    const menu = wrapper.findComponent({ name: "NMenu" });
    await menu.vm.$emit("update:value", "runner-ui");

    expect(mocks.router.push).toHaveBeenCalledWith("/runner");
  });
});
