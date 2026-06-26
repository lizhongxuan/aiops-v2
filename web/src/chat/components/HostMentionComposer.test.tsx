import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { HostMentionComposer } from "./HostMentionComposer";

describe("HostMentionComposer", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => {
      root.unmount();
    });
    container.remove();
  });

  it("highlights resolved host mentions while keeping plain textarea text", async () => {
    await act(async () => {
      root.render(
        <HostMentionComposer
          value="这是@1.1.1.1主机,检查pg"
          mentions={[
            {
              tokenId: "hm-1",
              raw: "@1.1.1.1",
              value: "1.1.1.1",
              start: 2,
              end: 10,
              source: "ip_literal",
            },
          ]}
          onChange={() => {}}
        />,
      );
    });

    expect(container.querySelector('[data-testid="composer-host-list"]')).toBeNull();
    expect(container.querySelector('[data-testid="host-mention-chip-list"]')).toBeNull();
    expect(container.querySelector('[data-testid="composer-inline-host-overlay"]')).not.toBeNull();
    expect(container.querySelector('[data-testid="composer-inline-host-mention"]')?.textContent).toBe("@1.1.1.1");
    const textarea = container.querySelector("textarea") as HTMLTextAreaElement;
    expect(textarea.value).toBe("这是@1.1.1.1主机,检查pg");
    expect(textarea.className).toContain("text-transparent");
  });

  it("does not render duplicated host label overlays", async () => {
    await act(async () => {
      root.render(
        <HostMentionComposer
          value="这是@pg-a主机,检查pg"
          mentions={[
            {
              tokenId: "hm-pg-a",
              raw: "@pg-a",
              value: "pg-a",
              start: 2,
              end: 7,
              source: "hostname_literal",
              hostId: "accept-host-a",
              displayName: "@pg-a",
              resolved: true,
            },
          ]}
          onChange={() => {}}
        />,
      );
    });

    expect(container.querySelector('[data-testid="composer-inline-host-overlay"]')).not.toBeNull();
    expect(container.querySelector('[data-testid="composer-inline-host-mention"]')?.textContent).toBe("@pg-a");
    const textarea = container.querySelector("textarea") as HTMLTextAreaElement;
    const occurrences = (textarea.value.match(/@pg-a/g) || []).length;
    expect(occurrences).toBe(1);
  });

  it("keeps every host mention occurrence in plain textarea text without a separate selected-host list", async () => {
    await act(async () => {
      root.render(
        <HostMentionComposer
          value="@host-a @host-b @host-a 检查两台主机"
          mentions={[
            {
              tokenId: "hm-a-1",
              raw: "@host-a",
              value: "host-a",
              start: 0,
              end: 7,
              source: "hostname_literal",
              hostId: "host-a",
              displayName: "主机A",
              resolved: true,
            },
            {
              tokenId: "hm-b-1",
              raw: "@host-b",
              value: "host-b",
              start: 8,
              end: 15,
              source: "hostname_literal",
              hostId: "host-b",
              displayName: "主机B",
              resolved: true,
            },
            {
              tokenId: "hm-a-2",
              raw: "@host-a",
              value: "host-a",
              start: 16,
              end: 23,
              source: "hostname_literal",
              hostId: "host-a",
              displayName: "主机A",
              resolved: true,
            },
          ]}
          onChange={() => {}}
        />,
      );
    });

    const list = container.querySelector('[data-testid="composer-host-list"]');
    expect(list).toBeNull();
    expect(container.querySelector('[data-testid="composer-inline-host-overlay"]')).not.toBeNull();
    expect(container.querySelectorAll('[data-testid="composer-inline-host-mention"]')).toHaveLength(3);
    const textarea = container.querySelector("textarea") as HTMLTextAreaElement;
    expect(textarea.value).toBe("@host-a @host-b @host-a 检查两台主机");
  });
});
