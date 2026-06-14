import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

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

  it("renders selected host mention chips", async () => {
    await act(async () => {
      root.render(
        <HostMentionComposer
          value="@1.1.1.1 检查pg"
          mentions={[
            {
              tokenId: "hm-1",
              raw: "@1.1.1.1",
              value: "1.1.1.1",
              start: 0,
              end: 8,
              source: "ip_literal",
            },
          ]}
          onChange={() => {}}
        />,
      );
    });

    expect(container.textContent).toContain("@1.1.1.1");
  });
});
