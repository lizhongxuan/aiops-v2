import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { AppShellChromeProvider } from "@/app/AppShellChromeContext";
import { createAgentUiArtifactsClient } from "@/api/agentUiArtifactsClient";
import { AppRouter } from "@/router";
import { AgentUICenterPage } from "./AgentUICenterPage";

const feedPayload = {
  total: 2,
  items: [
    {
      id: "artifact-coroot-1",
      type: "coroot_chart",
      title: "Coroot p95",
      status: "ready",
      source: "coroot",
      subjectKind: "service",
      subjectId: "checkout-api",
      promptTraceId: "trace-1",
      metadata: { caseId: "case-1", renderer: "CorootChartArtifact" },
      payload: { chart: "p95" },
      actions: [{ id: "prompt", label: "Prompt Trace", href: "/debug/prompts?trace_id=trace-1" }],
      updatedAt: "2026-05-16T10:00:00Z",
    },
    {
      id: "artifact-workflow-1",
      type: "workflow_result",
      title: "Workflow done",
      status: "success",
      source: "runner",
      subjectKind: "workflow",
      subjectId: "wf-1",
      metadata: {},
      payload: {},
      updatedAt: "2026-05-16T10:01:00Z",
    },
  ],
};

function jsonResponse(payload: unknown) {
  return Promise.resolve(new Response(JSON.stringify(payload), { status: 200, headers: { "Content-Type": "application/json" } }));
}

function waitForEffects() {
  return act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

function setInputValue(element: HTMLInputElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set;
  setter?.call(element, value);
  element.dispatchEvent(new Event("input", { bubbles: true }));
}

describe("agent ui artifacts client", () => {
  it("builds filter query paths", async () => {
    const calls: Array<{ method: string; path: string }> = [];
    const client = createAgentUiArtifactsClient({
      get(path: string) {
        calls.push({ method: "GET", path });
        return Promise.resolve({});
      },
      post(path: string) {
        calls.push({ method: "POST", path });
        return Promise.resolve({});
      },
    });

    await client.fetchAgentUiArtifacts({ source: "coroot", type: "coroot_chart" });

    expect(calls).toEqual([{ method: "GET", path: "/api/v1/agent-ui-artifacts?source=coroot&type=coroot_chart" }]);
  });
});

describe("AgentUICenterPage", () => {
  let container: HTMLDivElement;
  let root: Root;
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
    globalThis.ResizeObserver = class ResizeObserver {
      observe() {}
      unobserve() {}
      disconnect() {}
    };
    fetchMock = vi.fn(() => jsonResponse(feedPayload));
    vi.stubGlobal("fetch", fetchMock);
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
    vi.unstubAllGlobals();
  });

  it("renders feed, filters requests, and opens a detail drawer", async () => {
    await act(async () => {
      root.render(
        <MemoryRouter>
          <AppShellChromeProvider>
            <AgentUICenterPage />
          </AppShellChromeProvider>
        </MemoryRouter>,
      );
    });
    await waitForEffects();

    expect(container.textContent).toContain("Agent UI 产物");
    expect(container.textContent).toContain("Coroot p95");
    const sourceInput = container.querySelector('input[aria-label="source filter"]') as HTMLInputElement;
    await act(async () => {
      setInputValue(sourceInput, "coroot");
    });
    await waitForEffects();

    expect(fetchMock).toHaveBeenCalledWith(expect.stringContaining("/api/v1/agent-ui-artifacts?source=coroot"), expect.any(Object));

    const rowButton = Array.from(container.querySelectorAll("button")).find((button) => button.textContent?.includes("Coroot p95")) as HTMLButtonElement;
    await act(async () => rowButton.click());
    await waitForEffects();

    expect(document.body.textContent).toContain("artifact-coroot-1");
    expect(document.body.textContent).toContain("Normalized JSON");
    expect(document.body.textContent).toContain("Prompt Trace");
    expect(document.body.textContent).toContain("trace-1");
  });

  it("is reachable from /agent-ui route", async () => {
    await act(async () => {
      root.render(
        <MemoryRouter initialEntries={["/agent-ui"]}>
          <AppRouter />
        </MemoryRouter>,
      );
    });
    await waitForEffects();

    expect(container.textContent).toContain("Agent UI");
  });
});
