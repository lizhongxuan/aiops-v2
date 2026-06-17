import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { OpsGraphRelationshipDialog } from "./OpsGraphRelationshipDialog";

describe("OpsGraphRelationshipDialog", () => {
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

  it("edits relationship type and connection properties", async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    await act(async () => {
      root.render(
        <OpsGraphRelationshipDialog
          graph={{
            id: "graph.default",
            name: "服务拓扑",
            nodes: [
              { id: "service.order-api", type: "service", name: "order-api" },
              { id: "middleware.pg", type: "middleware", subtype: "postgres", name: "order-postgres" },
              { id: "external.pay", type: "external", name: "pay-provider" },
            ],
            edges: [],
          }}
          relationship={{
            id: "e1",
            from: "service.order-api",
            type: "depends_on",
            to: "middleware.pg",
            properties: { protocol: "postgres", port: "5432" },
          }}
          open
          onOpenChange={() => {}}
          onSave={onSave}
        />,
      );
    });

    await act(async () => {
      setSelectValue(document.body.querySelector('select[name="type"]') as HTMLSelectElement, "calls");
      setInputValue(document.body.querySelector('input[name="protocol"]') as HTMLInputElement, "https");
      setInputValue(document.body.querySelector('input[name="port"]') as HTMLInputElement, "443");
      setInputValue(document.body.querySelector('input[name="path"]') as HTMLInputElement, "/charge");
      setTextareaValue(document.body.querySelector('textarea[name="note"]') as HTMLTextAreaElement, "支付链路");
      document.body.querySelector('button[type="submit"]')?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({
      id: "e1",
      from: "service.order-api",
      type: "calls",
      to: "middleware.pg",
      note: "支付链路",
      properties: { protocol: "https", port: "443", path: "/charge" },
    }));
  });
});

function setInputValue(input: HTMLInputElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, "value")?.set;
  setter?.call(input, value);
  input.dispatchEvent(new Event("input", { bubbles: true }));
}

function setTextareaValue(input: HTMLTextAreaElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(window.HTMLTextAreaElement.prototype, "value")?.set;
  setter?.call(input, value);
  input.dispatchEvent(new Event("input", { bubbles: true }));
}

function setSelectValue(select: HTMLSelectElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(window.HTMLSelectElement.prototype, "value")?.set;
  setter?.call(select, value);
  select.dispatchEvent(new Event("change", { bubbles: true }));
}
