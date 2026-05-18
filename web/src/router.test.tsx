import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { AppRouter } from "@/router";

const routedPaths = [
  "/",
  "/protocol",
  "/incidents",
  "/incidents/incident-1",
  "/erp",
  "/opsgraph",
  "/runbooks",
  "/runbooks/runbook-1",
  "/runner",
  "/runner/payment-health",
  "/postmortems/postmortem-1",
  "/terminal/host-1",
  "/settings",
  "/settings/llm",
  "/settings/hosts",
  "/settings/ops-manuals",
  "/settings/experience-packs",
  "/settings/agent",
  "/settings/skills",
  "/settings/mcp",
  "/mcp",
  "/approval-management",
  "/capability-center",
  "/agent-ui",
  "/ui-cards",
  "/script-configs",
  "/coroot",
  "/lab",
  "/generator",
  "/debug/prompts",
];

describe("AppRouter", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
    globalThis.ResizeObserver = class ResizeObserver {
      observe() {}
      unobserve() {}
      disconnect() {}
    };
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => {
      root.unmount();
    });
    container.remove();
    localStorage.clear();
  });

  it.each(routedPaths)("renders React shell for %s", async (path) => {
    await act(async () => {
      root.render(
        <MemoryRouter initialEntries={[path]}>
          <AppRouter />
        </MemoryRouter>,
      );
    });

    expect(container.textContent).toContain("V2");
    expect(container.textContent).toContain("AIOPS");
    expect(container.textContent?.trim()).not.toBe("");
  });

  it("redirects legacy hosts route to settings hosts", async () => {
    await act(async () => {
      root.render(
        <MemoryRouter initialEntries={["/hosts"]}>
          <AppRouter />
        </MemoryRouter>,
      );
    });

    expect(container.textContent).toContain("主机与租约");
    expect(container.textContent).toContain("主机画像");
  });

  it("redirects legacy experience packs route to settings ops manuals", async () => {
    await act(async () => {
      root.render(
        <MemoryRouter initialEntries={["/experience-packs"]}>
          <AppRouter />
        </MemoryRouter>,
      );
    });

    expect(container.textContent).toContain("运维手册");
    expect(container.textContent).toContain("旧入口已迁移到运维手册");
  });

  it("can collapse the desktop navigation rail to an icon-only column", async () => {
    await act(async () => {
      root.render(
        <MemoryRouter initialEntries={["/debug/prompts"]}>
          <AppRouter />
        </MemoryRouter>,
      );
    });

    const sidebar = container.querySelector('[data-testid="app-shell-sidebar"]');
    const collapseButton = container.querySelector('[aria-label="收起侧边栏"]') as HTMLButtonElement | null;
    expect(sidebar?.getAttribute("data-collapsed")).toBe("false");
    expect(collapseButton).toBeTruthy();

    await act(async () => {
      collapseButton?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    expect(sidebar?.getAttribute("data-collapsed")).toBe("true");
    expect(sidebar?.className).toContain("w-20");
    expect(container.querySelector('[aria-label="展开侧边栏"]')).toBeTruthy();
    expect(container.querySelector('a[title="Prompt Trace"]')).toBeTruthy();
  });
});
