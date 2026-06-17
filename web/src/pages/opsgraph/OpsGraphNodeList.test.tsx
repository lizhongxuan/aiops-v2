import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { OpsGraphNodeList } from "./OpsGraphNodeList";

describe("OpsGraphNodeList", () => {
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
  });

  it("filters nodes by name and type", async () => {
    await act(async () => {
      root.render(<OpsGraphNodeList nodes={[
        { id: "service.checkout", type: "service", name: "checkout-api" },
        { id: "middleware.redis", type: "middleware", name: "redis-cache" },
        { id: "host.worker", type: "host", name: "worker-01" },
      ]} />);
    });

    const input = container.querySelector('input[aria-label="搜索节点"]') as HTMLInputElement | null;
    expect(input).toBeTruthy();

    await act(async () => {
      setInputValue(input!, "redis");
    });

    expect(container.textContent).toContain("redis-cache");
    expect(container.textContent).not.toContain("checkout-api");
    expect(container.textContent).not.toContain("worker-01");

    await act(async () => {
      setInputValue(input!, "主机");
    });

    expect(container.textContent).toContain("worker-01");
    expect(container.textContent).not.toContain("redis-cache");
  });

  it("uses a flexible scroll area for long node lists", async () => {
    await act(async () => {
      root.render(<OpsGraphNodeList nodes={Array.from({ length: 12 }, (_, index) => ({
        id: `service.${index}`,
        type: "service",
        name: `service-${index}`,
      }))} />);
    });

    expect(container.querySelector('[data-testid="opsgraph-node-list"]')?.className).toContain("min-h-0");
    expect(container.querySelector('[data-testid="opsgraph-node-list-scroll"]')?.className).toContain("overflow-y-auto");
    expect(container.querySelector('[data-testid="opsgraph-node-list-scroll"]')?.className).toContain("min-h-0");
  });
});

function setInputValue(input: HTMLInputElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set;
  setter?.call(input, value);
  input.dispatchEvent(new Event("input", { bubbles: true }));
}
