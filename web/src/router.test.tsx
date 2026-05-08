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
  "/settings/experience-packs",
  "/settings/agent",
  "/settings/skills",
  "/settings/mcp",
  "/mcp",
  "/approval-management",
  "/capability-center",
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

    expect(container.textContent).toContain("Hosts");
    expect(container.textContent).toContain("主机清单");
  });

  it("redirects legacy experience packs route to settings experience packs", async () => {
    await act(async () => {
      root.render(
        <MemoryRouter initialEntries={["/experience-packs"]}>
          <AppRouter />
        </MemoryRouter>,
      );
    });

    expect(container.textContent).toContain("Experience Packs");
    expect(container.textContent).toContain("经验包库");
  });
});
