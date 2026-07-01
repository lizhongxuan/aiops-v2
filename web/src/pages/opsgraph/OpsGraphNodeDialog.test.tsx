import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { OpsGraphNodeDialog } from "./OpsGraphNodeDialog";

describe("OpsGraphNodeDialog", () => {
  let container: HTMLDivElement;
  let root: ReturnType<typeof createRoot>;

  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
    vi.restoreAllMocks();
  });

  it("edits deployment and ops properties", async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    await act(async () => {
      root.render(
        <OpsGraphNodeDialog
          graph={{
            id: "graph.default",
            name: "服务拓扑",
            nodes: [{ id: "service.order-api", type: "service", name: "order-api", properties: { ports: "8080/http" } }],
            edges: [],
          }}
          node={{ id: "service.order-api", type: "service", name: "order-api", properties: { ports: "8080/http" } }}
          open
          onOpenChange={() => {}}
          onSave={onSave}
        />,
      );
    });

    const owner = document.body.querySelector('input[name="owner"]') as HTMLInputElement;
    const k8sCluster = document.body.querySelector('input[name="k8sCluster"]') as HTMLInputElement;
    await act(async () => {
      setInputValue(owner, "platform-sre");
      setInputValue(k8sCluster, "prod-k8s");
      document.body.querySelector('button[type="submit"]')?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({
      id: "service.order-api",
      properties: expect.objectContaining({ owner: "platform-sre", k8sCluster: "prod-k8s" }),
    }));
  });

  it("allows the host property to be typed manually or selected from host inventory", async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    await act(async () => {
      root.render(
        <OpsGraphNodeDialog
          graph={{
            id: "graph.default",
            name: "服务拓扑",
            nodes: [{ id: "service.order-api", type: "service", name: "order-api", properties: {} }],
            edges: [],
          }}
          hostOptions={[
            { id: "host-a", label: "prod-web-01", value: "10.0.0.11" },
            { id: "host-b", label: "test-120-77-239-90", value: "120.77.239.90" },
          ]}
          node={{ id: "service.order-api", type: "service", name: "order-api", properties: {} }}
          open
          onOpenChange={() => {}}
          onSave={onSave}
        />,
      );
    });

    const form = document.body.querySelector("form");
    const footer = document.body.querySelector('[data-slot="dialog-footer"]');
    const host = document.body.querySelector('input[name="host"]') as HTMLInputElement;
    const hostSelect = document.body.querySelector('select[aria-label="从主机列表选择"]') as HTMLSelectElement;
    expect(form?.className).toContain("overflow-x-hidden");
    expect(footer?.className).toContain("mx-0");
    expect(host).toBeTruthy();
    expect(hostSelect).toBeTruthy();
    expect(host.getAttribute("list")).toBeNull();
    expect(document.body.querySelector("datalist")).toBeNull();

    await act(async () => {
      setInputValue(host, "manual-host-01");
    });
    expect(host.value).toBe("manual-host-01");

    await act(async () => {
      setSelectValue(hostSelect, "120.77.239.90");
      document.body.querySelector('button[type="submit"]')?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({
      properties: expect.objectContaining({ host: "120.77.239.90" }),
    }));
  });

  it("keeps long node forms scrollable inside the dialog", async () => {
    await act(async () => {
      root.render(
        <OpsGraphNodeDialog
          graph={{
            id: "graph.default",
            name: "服务拓扑",
            nodes: [{ id: "middleware.redis", type: "middleware", subtype: "redis", name: "redis", properties: {} }],
            edges: [],
          }}
          node={{ id: "middleware.redis", type: "middleware", subtype: "redis", name: "redis", properties: {} }}
          open
          onOpenChange={() => {}}
          onSave={vi.fn()}
        />,
      );
    });

    const content = document.body.querySelector('[data-slot="dialog-content"]');
    const form = document.body.querySelector("form");
    expect(content?.className).toContain("h-[min(88vh,820px)]");
    expect(content?.className).toContain("flex-col");
    expect(form?.className).toContain("flex-1");
    expect(form?.className).toContain("overflow-y-auto");
  });
});

function setInputValue(input: HTMLInputElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, "value")?.set;
  setter?.call(input, value);
  input.dispatchEvent(new Event("input", { bubbles: true }));
}

function setSelectValue(select: HTMLSelectElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(window.HTMLSelectElement.prototype, "value")?.set;
  setter?.call(select, value);
  select.dispatchEvent(new Event("change", { bubbles: true }));
}
