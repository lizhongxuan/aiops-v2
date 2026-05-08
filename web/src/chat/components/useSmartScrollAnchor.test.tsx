import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { isNearTranscriptBottom, useSmartScrollAnchor } from "./useSmartScrollAnchor";

describe("useSmartScrollAnchor", () => {
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

  it("disables auto follow when user scrolls away from bottom", () => {
    const el = document.createElement("div");
    Object.defineProperty(el, "scrollHeight", { value: 1000, configurable: true });
    Object.defineProperty(el, "clientHeight", { value: 400, configurable: true });
    el.scrollTop = 500;

    expect(isNearTranscriptBottom(el, 24)).toBe(false);
  });

  it("treats within threshold as near bottom", () => {
    const el = document.createElement("div");
    Object.defineProperty(el, "scrollHeight", { value: 1000, configurable: true });
    Object.defineProperty(el, "clientHeight", { value: 400, configurable: true });
    el.scrollTop = 580;

    expect(isNearTranscriptBottom(el, 24)).toBe(true);
  });

  it("shows the scroll-to-bottom affordance after upward scroll and restores follow on click", async () => {
    await act(async () => {
      root.render(<ScrollHarness depKey="a" />);
    });
    const viewport = container.querySelector<HTMLDivElement>("[data-testid='scroll-viewport']");
    const button = container.querySelector<HTMLButtonElement>("button");
    if (!viewport || !button) {
      throw new Error("missing scroll harness elements");
    }
    Object.defineProperty(viewport, "scrollHeight", { value: 1000, configurable: true });
    Object.defineProperty(viewport, "clientHeight", { value: 400, configurable: true });

    act(() => {
      viewport.scrollTop = 500;
      viewport.dispatchEvent(new Event("scroll", { bubbles: true }));
    });

    expect(button.textContent).toBe("visible");

    act(() => {
      button.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    expect(viewport.scrollTop).toBe(1000);
    expect(button.textContent).toBe("hidden");
  });
});

function ScrollHarness({ depKey }: { depKey: string }) {
  const anchor = useSmartScrollAnchor([depKey]);
  return (
    <div>
      <div
        data-testid="scroll-viewport"
        ref={anchor.scrollRef}
        onScroll={anchor.handleScroll}
        onWheel={anchor.handleWheel}
      />
      <button type="button" onClick={anchor.scrollToBottom}>
        {anchor.showScrollToBottom ? "visible" : "hidden"}
      </button>
    </div>
  );
}
