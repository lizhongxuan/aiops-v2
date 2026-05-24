import { act } from "react";
import type { ComponentProps } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { ExternalReferenceViewer } from "./ExternalReferenceViewer";
import type { ExternalReferenceContent } from "@/api/externalReferences";

describe("ExternalReferenceViewer", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
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

  it("loads and shows raw external reference content on demand", async () => {
    let resolveReference: (reference: ExternalReferenceContent) => void = () => {};
    const pendingReference = new Promise<ExternalReferenceContent>((resolve) => {
      resolveReference = resolve;
    });
    await renderViewer({
      loadReference: async () => pendingReference,
    });

    await clickViewButton();

    expect(container.textContent).toContain("正在读取原始证据");
    resolveReference(makeReference({ content: "raw evidence" }));
    await flush();
    await flush();

    expect(container.textContent).toContain("raw evidence");
    expect(container.textContent).toContain("digest verified");
  });

  it("shows an external reference load failure without blanking the tool block", async () => {
    await renderViewer({
      referenceId: "ref-missing",
      summary: "外溢摘要仍然可见",
      loadReference: async () => {
        throw new Error("not found");
      },
    });

    await clickViewButton();
    await flush();

    expect(container.textContent).toContain("外溢摘要仍然可见");
    expect(container.textContent).toContain("原始证据读取失败");
    expect(container.textContent).toContain("not found");
  });

  it("shows digest mismatch without hiding content", async () => {
    await renderViewer({
      loadReference: async () => makeReference({
        content: "tampered",
        digest: "sha256:3bdaa3c452e349e0b3a07cbcf915a971518544204666e169d7166fd618eb96ae",
      }),
    });

    await clickViewButton();
    await flush();
    await flush();

    expect(container.textContent).toContain("digest mismatch");
    expect(container.textContent).toContain("tampered");
  });

  it("falls back for unknown kinds and empty content", async () => {
    await renderViewer({
      loadReference: async () => makeReference({ kind: "unknown", content: "" }),
    });

    await clickViewButton();
    await flush();
    await flush();

    expect(container.textContent).toContain("unknown kind");
    expect(container.textContent).toContain("没有可展示的原始内容");
  });

  async function renderViewer(props: Partial<ComponentProps<typeof ExternalReferenceViewer>> = {}) {
    await act(async () => {
      root.render(<ExternalReferenceViewer referenceId="ref-1" {...props} />);
    });
  }

  async function clickViewButton() {
    const button = Array.from(container.querySelectorAll("button")).find((item) => item.textContent?.includes("查看原始证据"));
    expect(button).toBeTruthy();
    await act(async () => {
      button?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
  }
});

function makeReference(overrides: Partial<ExternalReferenceContent> = {}): ExternalReferenceContent {
  const content = overrides.content ?? "raw evidence";
  return {
    id: "ref-1",
    kind: "blob",
    contentType: "text/plain",
    summary: "原始日志",
    content,
    bytes: content.length,
    digest: "sha256:3bdaa3c452e349e0b3a07cbcf915a971518544204666e169d7166fd618eb96ae",
    title: "ref-1",
    uri: "store://tool-spills/ref-1",
    cardRef: "",
    filePath: "",
    raw: {},
    ...overrides,
  };
}

function flush() {
  return act(() => new Promise((resolve) => window.setTimeout(resolve, 0)));
}
