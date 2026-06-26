import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { HostMentionSuggestion } from "../hostMentionSearch";
import { HostMentionSuggestionPopover } from "./HostMentionSuggestionPopover";

describe("HostMentionSuggestionPopover", () => {
  let container: HTMLDivElement;
  let root: Root;

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

  it("renders compact command-menu suggestions", async () => {
    const onSelect = vi.fn();

    await act(async () => {
      root.render(
        <HostMentionSuggestionPopover
          id="host-mention-suggestions"
          suggestions={sampleSuggestions()}
          highlightedIndex={1}
          onHighlight={vi.fn()}
          onSelect={onSelect}
        />,
      );
    });

    const popover = container.querySelector('[data-testid="host-mention-suggestion-popover"]');
    expect(popover).not.toBeNull();
    expect(popover?.getAttribute("role")).toBe("listbox");
    expect(container.querySelectorAll('[data-testid="host-mention-suggestion-item"]')).toHaveLength(2);
    expect(container.textContent).toContain("@pg-primary");
    expect(container.textContent).toContain("120.77.239.90 · online");
    expect(container.querySelectorAll('[role="option"]')[1]?.getAttribute("aria-selected")).toBe("true");

    await act(async () => {
      container.querySelectorAll('[data-testid="host-mention-suggestion-item"]')[0]?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    expect(onSelect).toHaveBeenCalledWith(sampleSuggestions()[0]);
  });

  it("renders the local alias suggestion with its target hint", async () => {
    await act(async () => {
      root.render(
        <HostMentionSuggestionPopover
          id="host-mention-suggestions"
          suggestions={[
            {
              key: "local",
              mention: "@local",
              label: "local",
              description: "本机 server-local",
              hostId: "server-local",
              address: "127.0.0.1",
              status: "local",
              score: 1000,
            },
          ]}
          highlightedIndex={0}
          onHighlight={vi.fn()}
          onSelect={vi.fn()}
        />,
      );
    });

    expect(container.textContent).toContain("@local");
    expect(container.textContent).toContain("本机 server-local");
    expect(container.querySelector('[role="option"]')?.getAttribute("aria-selected")).toBe("true");
  });

  it("renders an empty state without suggestion items", async () => {
    await act(async () => {
      root.render(
        <HostMentionSuggestionPopover
          id="host-mention-suggestions"
          suggestions={[]}
          highlightedIndex={0}
          onHighlight={vi.fn()}
          onSelect={vi.fn()}
        />,
      );
    });

    expect(container.querySelector('[data-testid="host-mention-suggestion-empty"]')).not.toBeNull();
    expect(container.querySelectorAll('[data-testid="host-mention-suggestion-item"]')).toHaveLength(0);
    expect(container.textContent).toContain("没有匹配主机");
  });
});

function sampleSuggestions(): HostMentionSuggestion[] {
  return [
    {
      key: "host-a",
      mention: "@120.77.239.90",
      label: "pg-primary",
      description: "120.77.239.90 · online",
      address: "120.77.239.90",
      status: "online",
      score: 100,
    },
    {
      key: "host-b",
      mention: "@10.0.0.8",
      label: "pg-standby",
      description: "10.0.0.8 · online",
      address: "10.0.0.8",
      status: "online",
      score: 90,
    },
  ];
}
