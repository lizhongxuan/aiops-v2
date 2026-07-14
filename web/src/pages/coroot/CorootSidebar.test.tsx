import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { CorootSidebar } from "@/pages/coroot/CorootSidebar";

describe("CorootSidebar", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
    sessionStorage.clear();
    vi.restoreAllMocks();
  });

  it("renders aiops return button instead of coroot logo", async () => {
    await renderSidebar();

    expect(buttonNamed(/返回 AIOps/)).toBeTruthy();
    expect(container.querySelector('img[alt="Coroot"]')).toBeNull();
  });

  it("links coroot views through outer aiops routes", async () => {
    await renderSidebar();

    expect(linkNamed(/Applications/)?.getAttribute("href")).toBe("/coroot/p/5hxbfx6p/applications");
    expect(linkNamed(/Service Map/)?.getAttribute("href")).toBe("/coroot/p/5hxbfx6p/map");
    expect(linkNamed(/Logs/)?.getAttribute("href")).toBe("/coroot/p/5hxbfx6p/logs");
  });

  it("omits costs and help entries from the embedded sidebar", async () => {
    await renderSidebar();

    expect(linkNamed(/^Costs/)).toBeUndefined();
    expect(container.textContent).not.toContain("成本");
    expect(linkNamed(/^Help$/)).toBeUndefined();
    expect(container.querySelector('a[href="https://docs.coroot.com/"]')).toBeNull();
  });

  it("uses the saved non-coroot return target", async () => {
    sessionStorage.setItem("aiops.coroot.returnTo", "/incidents?status=open");
    await renderSidebar();

    await act(async () => {
      buttonNamed(/返回 AIOps/)?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    expect(container.textContent).toContain("returned");
  });

  async function renderSidebar() {
    await act(async () => {
      root.render(
        <MemoryRouter initialEntries={["/coroot/p/5hxbfx6p/applications"]}>
          <Routes>
            <Route path="/coroot/p/:projectId/:view" element={<CorootSidebar collapsed={false} />} />
            <Route path="/incidents" element={<div>returned</div>} />
          </Routes>
        </MemoryRouter>,
      );
    });
  }

  function linkNamed(name: RegExp) {
    return Array.from(container.querySelectorAll("a")).find((link) => name.test(link.textContent || ""));
  }

  function buttonNamed(name: RegExp) {
    return Array.from(container.querySelectorAll("button")).find((button) => name.test(button.textContent || ""));
  }
});
