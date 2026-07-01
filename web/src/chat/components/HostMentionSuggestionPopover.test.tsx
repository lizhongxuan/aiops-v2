import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { HostMentionSuggestion } from "../hostMentionSearch";
import type { MentionCategorySuggestion } from "../mentionCatalog";
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
    expect(container.textContent).toContain("主机");
    expect(container.textContent).toContain("pg-primary");
    expect(container.textContent).toContain("120.77.239.90");
    expect(container.textContent).toContain("online");
    expect(container.textContent).not.toContain("@pg-primary");
    expect(container.querySelectorAll('[role="option"]')[1]?.getAttribute("aria-selected")).toBe("true");

    await act(async () => {
      container.querySelectorAll('[data-testid="host-mention-suggestion-item"]')[0]?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    expect(onSelect).toHaveBeenCalledWith(sampleSuggestions()[0]);
  });

  it("renders first-level mention categories as module choices", async () => {
    await act(async () => {
      root.render(
        <HostMentionSuggestionPopover
          id="host-mention-suggestions"
          suggestions={sampleCategorySuggestions()}
          highlightedIndex={0}
          onHighlight={vi.fn()}
          onSelect={vi.fn()}
        />,
      );
    });

    expect(container.querySelector('[data-testid="host-mention-suggestion-level"]')?.textContent).toContain("选择类型");
    expect(container.textContent).toContain("主机");
    expect(container.textContent).toContain("选择具体主机");
    expect(container.textContent).toContain("监控");
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
              kind: "host",
              path: "host://server-local",
              payload: {
                hostId: "server-local",
                address: "server-local",
                displayName: "local",
                status: "online",
              },
            },
          ]}
          highlightedIndex={0}
          onHighlight={vi.fn()}
          onSelect={vi.fn()}
        />,
      );
    });

    expect(container.textContent).toContain("127.0.0.1");
    expect(container.textContent).toContain("local");
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
    expect(container.textContent).toContain("没有匹配项");
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
      kind: "host",
      path: "host://host-a",
      payload: {
        hostId: "host-a",
        address: "120.77.239.90",
        displayName: "pg-primary",
        status: "online",
      },
    },
    {
      key: "host-b",
      mention: "@10.0.0.8",
      label: "pg-standby",
      description: "10.0.0.8 · online",
      address: "10.0.0.8",
      status: "online",
      score: 90,
      kind: "host",
      path: "host://host-b",
      payload: {
        hostId: "host-b",
        address: "10.0.0.8",
        displayName: "pg-standby",
        status: "online",
      },
    },
  ];
}

function sampleCategorySuggestions(): MentionCategorySuggestion[] {
  return [
    {
      key: "category-host",
      kind: "category",
      category: "host",
      label: "主机",
      description: "选择具体主机",
      prefix: "@host-",
      score: 1000,
    },
    {
      key: "category-monitor",
      kind: "category",
      category: "monitor",
      label: "监控",
      description: "Coroot RCA",
      prefix: "@monitor-",
      score: 900,
    },
  ];
}
